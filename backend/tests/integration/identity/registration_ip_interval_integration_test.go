//go:build integration

package identity_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	identityhttp "github.com/TokenFlux/TokenRouter/internal/identity/httpapi"
	identitypostgres "github.com/TokenFlux/TokenRouter/internal/identity/postgres"
	identitytestkit "github.com/TokenFlux/TokenRouter/internal/identity/testkit"
	postgresinfra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/pkg/locale"
	"github.com/TokenFlux/TokenRouter/internal/settings"
	settingspostgres "github.com/TokenFlux/TokenRouter/internal/settings/postgres"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// registration_ip_interval_integration_test.go exercises auth_service.go,
// auth_state.go, registration_ip_interval.go, and session_handler.go against
// migrated PostgreSQL databases. Separate service objects share durable interval
// rows, while concurrent attempts use separate transactions and connections.
// User-create failures expose reservation rollback, and direct timestamp changes
// test expiry without depending on wall-clock sleeps or modifying production data.

// newIntervalAuth constructs registration with database-backed settings and users.
// The caller owns the migrated database and Ent client; each invocation builds
// independent service objects over those shared durable records.
func newIntervalAuth(client *dbent.Client, db *sql.DB) (*identity.AuthService, *identitytestkit.SettingsReaders) {
	cfg := &config.Config{}
	cfg.JWT.Secret = "registration-interval-fixture"
	cfg.JWT.ExpireHour = 1
	cfg.Default.UserConcurrency = 1
	readers := identitytestkit.Settings(settingspostgres.NewSettingRepository(client), cfg)
	auth := identitytestkit.Auth(client, &identity.AuthDependencies{Users: identitypostgres.NewUserStoreWithSQL(client, db), Settings: readers, Options: identitytestkit.AuthOptions(cfg)})
	return auth, readers
}

// setIntervalPolicy saves a complete enabled signup policy for one isolated case.
// Database persistence makes later service reads observe duration changes without
// reconstructing their settings readers or restarting application fixtures.
func setIntervalPolicy(t *testing.T, repo settings.Repository, days, hours int) {
	t.Helper()
	require.NoError(t, repo.SetMultiple(context.Background(), map[string]string{
		identity.SettingKeyRegistrationEnabled:                 "true",
		identity.SettingKeyEmailVerifyEnabled:                  "false",
		identity.SettingKeyRegistrationEmailDomainQuotaEnabled: "false",
		identity.SettingKeyRegistrationEmailSuffixWhitelist:    "[]",
		identity.SettingKeyRegistrationIPIntervalDays:          strconv.Itoa(days),
		identity.SettingKeyRegistrationIPIntervalHours:         strconv.Itoa(hours),
	}))
}

// readIntervalTime reads the committed admission time for a specific address.
// Callers compare database round trips so PostgreSQL timestamp precision cannot
// turn an unchanged reservation into an apparent update.
func readIntervalTime(t *testing.T, db *sql.DB, ip string) time.Time {
	t.Helper()
	var timestamp time.Time
	require.NoError(t, db.QueryRow("SELECT last_registered_at FROM registration_ip_intervals WHERE ip_address=$1::inet", ip).Scan(&timestamp))
	return timestamp
}

// TestRegistrationIPIntervalConcurrent starts separate services at one barrier
// against an unused address. Exactly one user commits, while normalized duplicate
// addresses remain blocked and unrelated addresses retain registration access.
func TestRegistrationIPIntervalConcurrent(t *testing.T) {
	db, client := identityDatabase(t)
	repo := settingspostgres.NewSettingRepository(client)
	setIntervalPolicy(t, repo, 0, 6)
	first, _ := newIntervalAuth(client, db)
	second, _ := newIntervalAuth(client, db)
	start := make(chan struct{})
	results := make(chan error, 6)
	// Release all attempts together and collect every completion before assertions;
	// each service owns its transaction, exposing conflicts on an initially absent IP.
	for i := range 6 {
		go func() {
			<-start
			auth := first
			if i%2 == 1 {
				auth = second
			}
			_, _, err := auth.RegisterWithVerification(context.Background(), fmt.Sprintf("parallel-%d@example.com", i), "password123", "", "", "", "", "192.0.2.10")
			results <- err
		}()
	}
	close(start)
	successes := 0
	for range 6 {
		err := <-results
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, identity.ErrRegistrationIPInterval)
		}
	}
	require.Equal(t, 1, successes)
	count, err := client.User.Query().Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
	before := readIntervalTime(t, db, "192.0.2.10")
	_, _, err = second.RegisterWithVerification(context.Background(), "mapped@example.com", "password123", "", "", "", "", "::ffff:192.0.2.10")
	require.ErrorIs(t, err, identity.ErrRegistrationIPInterval)
	require.True(t, before.Equal(readIntervalTime(t, db, "192.0.2.10")))
	_, _, err = second.RegisterWithVerification(context.Background(), "different@example.com", "password123", "", "", "", "", "192.0.2.11")
	require.NoError(t, err)
	_, _, err = second.RegisterWithVerification(context.Background(), "ipv6@example.com", "password123", "", "", "", "", "2001:db8::1")
	require.NoError(t, err)
	_, _, err = first.RegisterWithVerification(context.Background(), "ipv6-again@example.com", "password123", "", "", "", "", "2001:0db8:0:0:0:0:0:1")
	require.ErrorIs(t, err, identity.ErrRegistrationIPInterval)
}

// TestRegistrationIPIntervalRollback injects a user-write failure after an IP
// reservation for both new and expired rows. Failure restores prior history,
// and a subsequent attempt can create the account immediately.
func TestRegistrationIPIntervalRollback(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(strconv.FormatBool(existing), func(t *testing.T) {
			db, client := identityDatabase(t)
			setIntervalPolicy(t, settingspostgres.NewSettingRepository(client), 0, 6)
			var before time.Time
			if existing {
				_, err := db.Exec("INSERT INTO registration_ip_intervals VALUES ('192.0.2.20', clock_timestamp() - INTERVAL '7 hours')")
				require.NoError(t, err)
				before = readIntervalTime(t, db, "192.0.2.20")
			}
			var fail atomic.Bool
			fail.Store(true)
			client.User.Use(func(next dbent.Mutator) dbent.Mutator {
				return dbent.MutateFunc(func(ctx context.Context, m dbent.Mutation) (dbent.Value, error) {
					if m.Op().Is(dbent.OpCreate) && fail.Swap(false) {
						return nil, errors.New("injected account write failure")
					}
					return next.Mutate(ctx, m)
				})
			})
			auth, _ := newIntervalAuth(client, db)
			_, _, err := auth.RegisterWithVerification(context.Background(), "retry@example.com", "password123", "", "", "", "", "192.0.2.20")
			require.ErrorIs(t, err, identity.ErrServiceUnavailable)
			count, err := client.User.Query().Count(context.Background())
			require.NoError(t, err)
			require.Zero(t, count)
			if existing {
				require.True(t, before.Equal(readIntervalTime(t, db, "192.0.2.20")))
			} else {
				require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM registration_ip_intervals").Scan(&count))
				require.Zero(t, count)
			}
			_, _, err = auth.RegisterWithVerification(context.Background(), "retry@example.com", "password123", "", "", "", "", "192.0.2.20")
			require.NoError(t, err)
		})
	}
}

// TestRegistrationIPIntervalPolicyChanges changes duration, expiry, and account
// deletion after a successful signup. The latest enabled registration determines
// admission, and disabled registrations leave that stored history untouched.
func TestRegistrationIPIntervalPolicyChanges(t *testing.T) {
	db, client := identityDatabase(t)
	repo := settingspostgres.NewSettingRepository(client)
	setIntervalPolicy(t, repo, 0, 6)
	auth, _ := newIntervalAuth(client, db)
	ctx := context.Background()
	_, _, err := auth.RegisterWithVerification(ctx, "initial@example.com", "password123", "", "", "", "", "192.0.2.30")
	require.NoError(t, err)
	_, err = db.Exec("UPDATE registration_ip_intervals SET last_registered_at=clock_timestamp()-INTERVAL '7 hours'")
	require.NoError(t, err)
	setIntervalPolicy(t, repo, 30, 0)
	_, _, err = auth.RegisterWithVerification(ctx, "longer@example.com", "password123", "", "", "", "", "192.0.2.30")
	require.ErrorIs(t, err, identity.ErrRegistrationIPInterval)
	setIntervalPolicy(t, repo, 0, 6)
	_, _, err = auth.RegisterWithVerification(ctx, "shorter@example.com", "password123", "", "", "", "", "192.0.2.30")
	require.NoError(t, err)
	before := readIntervalTime(t, db, "192.0.2.30")
	setIntervalPolicy(t, repo, 0, 0)
	for i := range 2 {
		_, _, err = auth.RegisterWithVerification(ctx, fmt.Sprintf("disabled-%d@example.com", i), "password123", "", "", "", "", "192.0.2.30")
		require.NoError(t, err)
	}
	require.True(t, before.Equal(readIntervalTime(t, db, "192.0.2.30")))
	setIntervalPolicy(t, repo, 30, 0)
	_, err = db.Exec("UPDATE users SET deleted_at=clock_timestamp()")
	require.NoError(t, err)
	_, _, err = auth.RegisterWithVerification(ctx, "after-delete@example.com", "password123", "", "", "", "", "192.0.2.30")
	require.ErrorIs(t, err, identity.ErrRegistrationIPInterval)
	_, err = db.Exec("UPDATE registration_ip_intervals SET last_registered_at=clock_timestamp()-INTERVAL '31 days'")
	require.NoError(t, err)
	_, _, err = auth.RegisterWithVerification(ctx, "expired@example.com", "password123", "", "", "", "", "192.0.2.30")
	require.NoError(t, err)
	require.NoError(t, repo.Set(ctx, identity.SettingKeyRegistrationIPIntervalHours, "broken"))
	_, _, err = auth.RegisterWithVerification(ctx, "corrupt@example.com", "password123", "", "", "", "", "192.0.2.31")
	require.ErrorIs(t, err, identity.ErrServiceUnavailable)
}

// TestRegistrationIPIntervalHTTP checks the ordinary signup handler after a
// committed registration. It verifies client address propagation and a localized
// 429 response before token issuance can create another account.
func TestRegistrationIPIntervalHTTP(t *testing.T) {
	db, client := identityDatabase(t)
	setIntervalPolicy(t, settingspostgres.NewSettingRepository(client), 0, 6)
	auth, readers := newIntervalAuth(client, db)
	_, _, err := auth.RegisterWithVerification(context.Background(), "first-http@example.com", "password123", "", "", "", "", "192.0.2.40")
	require.NoError(t, err)
	handler := identityhttp.NewSessionHandler(auth, nil, readers, nil, nil, nil, identityhttp.SessionHTTPOptions{})
	writer := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(writer)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"blocked-http@example.com","password":"password123"}`))
	ctx.Request.RemoteAddr = "192.0.2.40:12345"
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request = ctx.Request.WithContext(locale.WithLanguage(ctx.Request.Context(), "zh-CN"))
	handler.Register(ctx)
	require.Equal(t, http.StatusTooManyRequests, writer.Code)
	require.Contains(t, writer.Body.String(), "REGISTRATION_IP_INTERVAL")
	require.Contains(t, writer.Body.String(), "注册")
	count, err := client.User.Query().Count(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

// TestRegistrationIPIntervalMigrationRepeat replays the full runner over a migrated
// database and repeats the new SQL with an existing claim. Migration bookkeeping
// and registration history must survive both operations unchanged.
func TestRegistrationIPIntervalMigrationRepeat(t *testing.T) {
	db, _ := identityDatabase(t)
	_, err := db.Exec("INSERT INTO registration_ip_intervals VALUES ('192.0.2.50', clock_timestamp())")
	require.NoError(t, err)
	before := readIntervalTime(t, db, "192.0.2.50")
	require.NoError(t, postgresinfra.ApplyMigrations(context.Background(), db, migrations.FS))
	sqlBytes, err := migrations.FS.ReadFile("308_registration_ip_intervals.sql")
	require.NoError(t, err)
	for range 2 {
		_, err = db.Exec(string(sqlBytes))
		require.NoError(t, err)
	}
	require.True(t, before.Equal(readIntervalTime(t, db, "192.0.2.50")))
	var count int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE filename='308_registration_ip_intervals.sql'").Scan(&count))
	require.Equal(t, 1, count)
}
