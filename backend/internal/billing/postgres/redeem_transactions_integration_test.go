//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/billing"
)

type identityRedeemReader struct {
	database *sql.DB
}

type identityFailingUsage struct {
	billing.RedeemCodeRepository
}

func (reader identityRedeemReader) GetByID(ctx context.Context, userID int64) (*billing.UserSummary, error) {
	var balance float64
	err := reader.database.QueryRowContext(ctx, `SELECT balance FROM users WHERE id=$1`, userID).Scan(&balance)
	return &billing.UserSummary{ID: userID, Balance: balance}, err
}

func (store identityFailingUsage) CreateUsage(context.Context, *billing.RedeemCodeUsage) error {
	return errors.New("injected usage failure")
}

func TestIdentityRedeemTransactions(t *testing.T) {
	dsn := os.Getenv("TOKENROUTER_IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated identity database")
	}
	require.Contains(t, strings.ToLower(dsn), "identity_test")
	database, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, database)))
	repository := NewRedeemCodeRepository(client)
	transactions := NewRedeemMutations(client, RedeemWriters{Balances: NewBalanceStore(client)})
	runtime := billing.RedeemRuntime{Now: time.Now, RechargeIdentity: func(ctx context.Context, userID int64) (string, error) {
		rows, queryErr := dbent.TxFromContext(ctx).Client().QueryContext(ctx, `SELECT identity_code FROM user_identity_memberships WHERE user_id=$1 AND category='recharge'`, userID)
		if queryErr != nil {
			return "", queryErr
		}
		defer func() { _ = rows.Close() }()
		var code string
		if rows.Next() {
			queryErr = rows.Scan(&code)
		}
		if queryErr != nil {
			return "", queryErr
		}
		return code, rows.Err()
	}}
	service := billing.NewRedeemService(repository, identityRedeemReader{database}, nil, nil, nil, transactions, nil, nil, runtime)
	ctx := t.Context()
	code := &billing.RedeemCode{Code: "identity-shared-transaction", Type: billing.RedeemTypeBalance, Value: 100, MaxUses: 3, Status: billing.StatusUnused}
	require.NoError(t, repository.Create(ctx, code))
	var userIDs []int64
	for index, identityCode := range []string{"", "bonus_5", "bonus_10"} {
		var userID int64
		require.NoError(t, database.QueryRow(`INSERT INTO users(email,password_hash,username) VALUES($1,'hash','redeemer') RETURNING id`, "redeemer-"+identityCode+"@test.local").Scan(&userID))
		userIDs = append(userIDs, userID)
		if identityCode != "" {
			_, err = database.Exec(`INSERT INTO user_identity_memberships(user_id,category,identity_code) VALUES($1,'recharge',$2)`, userID, identityCode)
			require.NoError(t, err)
		}
		result, redeemErr := service.Redeem(ctx, userID, code.Code)
		require.NoError(t, redeemErr)
		require.Equal(t, 100+float64(index)*5, result.RechargeSnapshot.CreditedAmount)
		usage, usageErr := repository.GetUsageByRedeemCodeAndUser(ctx, code.ID, userID)
		require.NoError(t, usageErr)
		require.Equal(t, result.RechargeSnapshot, usage.RechargeSnapshot)
	}
	concurrent := &billing.RedeemCode{Code: "identity-concurrent-transaction", Type: billing.RedeemTypeBalance, Value: 100, MaxUses: 2, Status: billing.StatusUnused}
	require.NoError(t, repository.Create(ctx, concurrent))
	failures := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			_, redeemErr := service.Redeem(ctx, userIDs[2], concurrent.Code)
			failures <- redeemErr
		})
	}
	workers.Wait()
	close(failures)
	successes := 0
	for failure := range failures {
		if failure == nil {
			successes++
		} else {
			require.ErrorIs(t, failure, billing.ErrRedeemCodeAlreadyUsed)
		}
	}
	require.Equal(t, 1, successes)
	failing := billing.NewRedeemService(identityFailingUsage{repository}, identityRedeemReader{database}, nil, nil, nil, transactions, nil, nil, runtime)
	_, err = failing.Redeem(ctx, userIDs[1], concurrent.Code)
	require.ErrorContains(t, err, "injected usage failure")
	member, err := (identityRedeemReader{database}).GetByID(ctx, userIDs[1])
	require.NoError(t, err)
	require.Equal(t, 105.0, member.Balance)
	usage, err := repository.GetUsageByRedeemCodeAndUser(ctx, concurrent.ID, userIDs[1])
	require.NoError(t, err)
	require.Nil(t, usage)
	stored, err := repository.GetByID(ctx, concurrent.ID)
	require.NoError(t, err)
	require.Equal(t, 1, stored.UsedCount)
	member, err = (identityRedeemReader{database}).GetByID(ctx, userIDs[2])
	require.NoError(t, err)
	require.Equal(t, 220.0, member.Balance)
}
