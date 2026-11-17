//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// identity_rates_integration_test.go verifies identity pricing against PostgreSQL.
// The migrated fixtures exercise stored overrides, default discount eligibility,
// and both single-group billing and all-group display queries so administrative
// pricing changes produce consistent customer rates across those consumers.

// TestIdentityRateOriginalPrice toggles original pricing for both discount
// memberships in a migrated database. Billing and price-display reads must agree,
// other groups must retain discounts, and explicit overrides must keep precedence.
func TestIdentityRateOriginalPrice(t *testing.T) {
	database := postgrescontainer.New(t)
	ctx := t.Context()
	store := NewUserGroupRateRepository(database)
	resolver := billing.NewGroupRateResolver(store, nil, time.Minute, nil, "test", nil)
	var groupID, otherGroupID int64
	require.NoError(t, database.QueryRowContext(ctx, `INSERT INTO groups(name,rate_multiplier) VALUES('original-price',2) RETURNING id`).Scan(&groupID))
	require.NoError(t, database.QueryRowContext(ctx, `INSERT INTO groups(name,rate_multiplier) VALUES('discounted-price',2) RETURNING id`).Scan(&otherGroupID))
	var exempt bool
	require.NoError(t, database.QueryRowContext(ctx, `SELECT identity_discount_exempt FROM groups WHERE id=$1`, groupID).Scan(&exempt))
	require.False(t, exempt)
	for code, discounted := range map[string]float64{"bonus_5": 1.9, "bonus_10": 1.8} {
		t.Run(code, func(t *testing.T) {
			var userID int64
			require.NoError(t, database.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,username) VALUES($1,'hash','original-price') RETURNING id`, code+"@original-price.test").Scan(&userID))
			_, err := database.ExecContext(ctx, `INSERT INTO user_identity_memberships(user_id,category,identity_code) VALUES($1,'recharge',$2)`, userID, code)
			require.NoError(t, err)
			actual, err := resolver.ResolveChecked(ctx, userID, groupID, 2)
			require.NoError(t, err)
			require.Equal(t, discounted, actual)

			_, err = database.ExecContext(ctx, `UPDATE groups SET identity_discount_exempt=true WHERE id=$1`, groupID)
			require.NoError(t, err)
			actual, err = resolver.ResolveChecked(ctx, userID, groupID, 2)
			require.NoError(t, err)
			require.Equal(t, 2.0, actual)
			all, err := store.EffectiveGroupRates(ctx, userID)
			require.NoError(t, err)
			require.NotContains(t, all, groupID)
			require.Equal(t, discounted, all[otherGroupID])

			_, err = database.ExecContext(ctx, `INSERT INTO user_identity_memberships(user_id,category,identity_code) VALUES($1,'pricing','ahhilai')`, userID)
			require.NoError(t, err)
			_, err = database.ExecContext(ctx, `INSERT INTO identity_group_rates(identity_code,group_id,rate_multiplier) VALUES('ahhilai',$1,0.4)`, groupID)
			require.NoError(t, err)
			actual, err = resolver.ResolveChecked(ctx, userID, groupID, 2)
			require.NoError(t, err)
			require.Equal(t, 0.4, actual)
			// A personal rate above both other candidates still wins by priority;
			// changing it to zero must then retain the explicit free-rate selection.
			_, err = database.ExecContext(ctx, `INSERT INTO user_group_rate_multipliers(user_id,group_id,rate_multiplier) VALUES($1,$2,3)`, userID, groupID)
			require.NoError(t, err)
			actual, err = resolver.ResolveChecked(ctx, userID, groupID, 2)
			require.NoError(t, err)
			require.Equal(t, 3.0, actual)
			all, err = store.EffectiveGroupRates(ctx, userID)
			require.NoError(t, err)
			require.Equal(t, 3.0, all[groupID])
			_, err = database.ExecContext(ctx, `UPDATE user_group_rate_multipliers SET rate_multiplier=0 WHERE user_id=$1 AND group_id=$2`, userID, groupID)
			require.NoError(t, err)
			actual, err = resolver.ResolveChecked(ctx, userID, groupID, 2)
			require.NoError(t, err)
			require.Zero(t, actual)
			all, err = store.EffectiveGroupRates(ctx, userID)
			require.NoError(t, err)
			require.Contains(t, all, groupID)
			require.Zero(t, all[groupID])

			_, err = database.ExecContext(ctx, `DELETE FROM user_group_rate_multipliers WHERE user_id=$1`, userID)
			require.NoError(t, err)
			_, err = database.ExecContext(ctx, `DELETE FROM identity_group_rates WHERE group_id=$1`, groupID)
			require.NoError(t, err)
			_, err = database.ExecContext(ctx, `UPDATE groups SET identity_discount_exempt=false WHERE id=$1`, groupID)
			require.NoError(t, err)
			actual, err = resolver.ResolveChecked(ctx, userID, groupID, 2)
			require.NoError(t, err)
			require.Equal(t, discounted, actual)
			all, err = store.EffectiveGroupRates(ctx, userID)
			require.NoError(t, err)
			require.Equal(t, discounted, all[groupID])
		})
	}
}

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

func TestIdentityRateBatchUpdate(t *testing.T) {
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
	groupIDs := make([]int64, 3)
	for index := range groupIDs {
		require.NoError(t, database.QueryRow(`INSERT INTO groups(name,rate_multiplier) VALUES($1,1) RETURNING id`, "identity-batch-"+strconv.Itoa(index)).Scan(&groupIDs[index]))
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DELETE FROM identity_group_rates WHERE group_id = ANY($1)`, pq.Array(groupIDs))
	})
	count := func(query string, args ...any) int {
		var value int
		require.NoError(t, database.QueryRow(query, args...).Scan(&value))
		return value
	}
	first, second := 0.3, 0.7
	require.NoError(t, store.UpdateIdentityRates(ctx, 1, []billing.IdentityRateInput{{GroupID: groupIDs[0], RateMultiplier: &first}, {GroupID: groupIDs[1], RateMultiplier: &second}}))
	changes := count(`SELECT COUNT(*) FROM identity_group_changes WHERE identity_code='ahhilai' AND action='rates'`)
	updated := 0.5
	require.NoError(t, store.UpdateIdentityRates(ctx, 1, []billing.IdentityRateInput{{GroupID: groupIDs[0]}, {GroupID: groupIDs[1], RateMultiplier: &updated}, {GroupID: groupIDs[2], RateMultiplier: &first}}))
	assert.Zero(t, count(`SELECT COUNT(*) FROM identity_group_rates WHERE identity_code='ahhilai' AND group_id=$1`, groupIDs[0]))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM identity_group_rates WHERE identity_code='ahhilai' AND group_id=$1 AND rate_multiplier=0.5`, groupIDs[1]))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM identity_group_rates WHERE identity_code='ahhilai' AND group_id=$1 AND rate_multiplier=0.3`, groupIDs[2]))
	assert.Equal(t, changes+1, count(`SELECT COUNT(*) FROM identity_group_changes WHERE identity_code='ahhilai' AND action='rates'`))
	changes++
	err = store.UpdateIdentityRates(ctx, 1, []billing.IdentityRateInput{{GroupID: groupIDs[1], RateMultiplier: &first}, {GroupID: 2147483000}})
	require.Error(t, err)
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM identity_group_rates WHERE identity_code='ahhilai' AND group_id=$1 AND rate_multiplier=0.5`, groupIDs[1]))
	assert.Equal(t, changes, count(`SELECT COUNT(*) FROM identity_group_changes WHERE identity_code='ahhilai' AND action='rates'`))
	require.NoError(t, store.UpdateIdentityRates(ctx, 1, nil))
}

// TestIdentityRateDiscountPriority checks balance rate resolution for a bonus_10 member.
// Without other overrides a group is charged at 90% of its default rate, an AHHilAI
// override wins over the discount, a personal override wins over both, and a user with
// no identity has no effective rate. The all-groups read applies the same sources.
func TestIdentityRateDiscountPriority(t *testing.T) {
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
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	var memberID, plainID int64
	require.NoError(t, database.QueryRow(`INSERT INTO users(email,password_hash,username) VALUES($1,'hash','discount') RETURNING id`, "discount-"+suffix+"@test.local").Scan(&memberID))
	require.NoError(t, database.QueryRow(`INSERT INTO users(email,password_hash,username) VALUES($1,'hash','plain') RETURNING id`, "plain-"+suffix+"@test.local").Scan(&plainID))
	groupIDs := make([]int64, 3)
	for index, rate := range []float64{2, 1, 1.5} {
		require.NoError(t, database.QueryRow(`INSERT INTO groups(name,rate_multiplier) VALUES($1,$2) RETURNING id`, "discount-"+suffix+"-"+strconv.Itoa(index), rate).Scan(&groupIDs[index]))
	}
	t.Cleanup(func() {
		_, _ = database.Exec(`DELETE FROM identity_group_rates WHERE group_id = ANY($1)`, pq.Array(groupIDs))
	})
	_, err = database.Exec(`INSERT INTO user_identity_memberships(user_id,category,identity_code) VALUES($1,'recharge','bonus_10')`, memberID)
	require.NoError(t, err)
	rate := func(userID, groupID int64) *float64 {
		value, readErr := store.EffectiveGroupRate(ctx, userID, groupID)
		require.NoError(t, readErr)
		return value
	}
	require.InDelta(t, 1.8, *rate(memberID, groupIDs[0]), 1e-9)
	require.Nil(t, rate(plainID, groupIDs[0]))

	_, err = database.Exec(`INSERT INTO user_identity_memberships(user_id,category,identity_code) VALUES($1,'pricing','ahhilai')`, memberID)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO identity_group_rates(identity_code,group_id,rate_multiplier) VALUES('ahhilai',$1,0.5)`, groupIDs[0])
	require.NoError(t, err)
	require.InDelta(t, 0.5, *rate(memberID, groupIDs[0]), 1e-9)

	_, err = database.Exec(`INSERT INTO user_group_rate_multipliers(user_id,group_id,rate_multiplier) VALUES($1,$2,0.3)`, memberID, groupIDs[1])
	require.NoError(t, err)
	require.InDelta(t, 0.3, *rate(memberID, groupIDs[1]), 1e-9)

	all, err := store.EffectiveGroupRates(ctx, memberID)
	require.NoError(t, err)
	assert.InDelta(t, 0.5, all[groupIDs[0]], 1e-9)
	assert.InDelta(t, 0.3, all[groupIDs[1]], 1e-9)
	assert.InDelta(t, 1.35, all[groupIDs[2]], 1e-9)
	plain, err := store.EffectiveGroupRates(ctx, plainID)
	require.NoError(t, err)
	assert.Empty(t, plain)
}
