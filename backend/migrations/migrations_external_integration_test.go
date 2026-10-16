//go:build integration

package migrations_test

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/migrations"
)

func TestIdentityMigrationUpgrade(t *testing.T) {
	dsn := os.Getenv("TOKENROUTER_IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated identity database")
	}
	require.Contains(t, strings.ToLower(dsn), "identity_test")
	database, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	previous := fstest.MapFS{}
	entries, err := migrations.FS.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.Name() == "299_identity_groups.sql" {
			continue
		}
		contents, readErr := migrations.FS.ReadFile(entry.Name())
		require.NoError(t, readErr)
		previous[entry.Name()] = &fstest.MapFile{Data: contents}
	}
	require.NoError(t, postgres.ApplyMigrations(t.Context(), database, previous))
	var userID int64
	require.NoError(t, database.QueryRow(`INSERT INTO users(email,password_hash,username,balance) VALUES('legacy-identity@test.local','hash','legacy',123.45) RETURNING id`).Scan(&userID))
	require.NoError(t, postgres.ApplyMigrations(t.Context(), database, migrations.FS))
	require.NoError(t, postgres.ApplyMigrations(t.Context(), database, migrations.FS))
	contents, err := migrations.FS.ReadFile("299_identity_groups.sql")
	require.NoError(t, err)
	_, err = database.Exec(string(contents))
	require.NoError(t, err)
	var balance float64
	require.NoError(t, database.QueryRow(`SELECT balance FROM users WHERE id=$1`, userID).Scan(&balance))
	require.Equal(t, 123.45, balance)
	var count int
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM identity_groups`).Scan(&count))
	require.Equal(t, 3, count)
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM identity_group_rates`).Scan(&count))
	require.Zero(t, count)
	require.NoError(t, database.QueryRow(`SELECT count(*) FROM user_identity_memberships`).Scan(&count))
	require.Zero(t, count)
}
