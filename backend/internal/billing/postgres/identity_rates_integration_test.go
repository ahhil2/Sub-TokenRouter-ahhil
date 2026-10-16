//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

func TestIdentityRatePrioritySQL(t *testing.T) {
	dsn := os.Getenv("TOKENROUTER_IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated identity database")
	}
	require.Contains(t, strings.ToLower(dsn), "identity_test")
	database, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	store := NewUserGroupRateRepository(database)
	var userID, groupID int64
	require.NoError(t, database.QueryRow(`INSERT INTO users(email,password_hash,username) VALUES('rates@test.local','hash','rates') RETURNING id`).Scan(&userID))
	require.NoError(t, database.QueryRow(`INSERT INTO groups(name,rate_multiplier) VALUES('identity-test-group',1) RETURNING id`).Scan(&groupID))
	_, err = database.Exec(`INSERT INTO user_identity_memberships(user_id,category,identity_code) VALUES($1,'pricing','ahhilai')`, userID)
	require.NoError(t, err)
	rate := 0.2
	require.NoError(t, store.UpdateIdentityRates(ctx, userID, []billing.IdentityRateInput{{GroupID: groupID, RateMultiplier: &rate}}))
	actual, err := store.EffectiveGroupRate(ctx, userID, groupID)
	require.NoError(t, err)
	require.Equal(t, rate, *actual)
	_, err = database.Exec(`INSERT INTO user_group_rate_multipliers(user_id,group_id,rate_multiplier) VALUES($1,$2,0.1)`, userID, groupID)
	require.NoError(t, err)
	actual, err = store.EffectiveGroupRate(ctx, userID, groupID)
	require.NoError(t, err)
	require.Equal(t, 0.1, *actual)
	personal, err := store.GetByUserID(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, 0.1, personal[groupID])
	_, err = database.Exec(`DELETE FROM user_group_rate_multipliers WHERE user_id=$1 AND group_id=$2`, userID, groupID)
	require.NoError(t, err)
	rate = 0
	require.NoError(t, store.UpdateIdentityRates(ctx, userID, []billing.IdentityRateInput{{GroupID: groupID, RateMultiplier: &rate}}))
	actual, err = store.EffectiveGroupRate(ctx, userID, groupID)
	require.NoError(t, err)
	require.Zero(t, *actual)
	require.NoError(t, store.UpdateIdentityRates(ctx, userID, []billing.IdentityRateInput{{GroupID: groupID}}))
	actual, err = store.EffectiveGroupRate(ctx, userID, groupID)
	require.NoError(t, err)
	require.Nil(t, actual)
	rate = 0.06
	require.NoError(t, store.UpdateIdentityRates(ctx, userID, []billing.IdentityRateInput{{GroupID: groupID, RateMultiplier: &rate}}))
	_, err = database.Exec(`DELETE FROM user_identity_memberships WHERE user_id=$1 AND category='pricing'`, userID)
	require.NoError(t, err)
	actual, err = store.EffectiveGroupRate(ctx, userID, groupID)
	require.NoError(t, err)
	require.Nil(t, actual)
	var newGroupID int64
	require.NoError(t, database.QueryRow(`INSERT INTO groups(name,rate_multiplier) VALUES('identity-new-group',0.4) RETURNING id`).Scan(&newGroupID))
	listed, err := store.IdentityRates(ctx)
	require.NoError(t, err)
	found := false
	for _, listedRate := range listed {
		if listedRate.GroupID == newGroupID {
			found = true
			require.Nil(t, listedRate.RateMultiplier)
			require.Equal(t, 0.4, listedRate.DefaultMultiplier)
		}
	}
	require.True(t, found)
	for _, invalid := range []string{"-1", "NaN", "Infinity", "-Infinity"} {
		_, err = database.Exec(`INSERT INTO identity_group_rates(identity_code,group_id,rate_multiplier) VALUES('ahhilai',$1,$2::numeric)`, newGroupID, invalid)
		require.Error(t, err)
	}
	_, err = database.Exec(`INSERT INTO user_identity_memberships(user_id,category,identity_code) VALUES($1,'recharge','bonus_5'),($1,'recharge','bonus_10')`, userID)
	require.Error(t, err)
}
