//go:build integration

package postgres

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/ent"
	_ "github.com/TokenFlux/TokenRouter/ent/runtime"
	postgresinfra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

// group_store_integration_test.go checks stored group settings through the Ent
// repository and PostgreSQL migrations. Fixtures use a dedicated database and
// replace scheduling notification with a callback so assertions observe group
// persistence independently of background scheduler delivery.

// TestGroupStoreOriginalPriceRoundTrip creates an original-price group and
// restores its discounts through repository updates. Fresh reads must retain
// each saved value, including false after a previously enabled setting.
func TestGroupStoreOriginalPriceRoundTrip(t *testing.T) {
	database := postgrescontainer.New(t)
	client := ent.NewClient(ent.Driver(sql.OpenDB(dialect.Postgres, database)))
	store := NewGroupStore(client, database, GroupStoreOptions{
		Enqueue: func(context.Context, postgresinfra.Executor, *int64) error { return nil },
	})
	ctx := t.Context()
	group := &routing.Group{
		Name: "original-price", RateMultiplier: 2, IdentityDiscountExempt: true,
		Status: routing.StatusActive,
	}
	require.NoError(t, routing.NormalizeGroupProtocolPolicy(group, nil))
	require.NoError(t, store.Create(ctx, group))
	saved, err := store.GetByID(ctx, group.ID)
	require.NoError(t, err)
	require.True(t, saved.IdentityDiscountExempt)
	saved.IdentityDiscountExempt = false
	require.NoError(t, store.Update(ctx, saved))
	saved, err = store.GetByID(ctx, group.ID)
	require.NoError(t, err)
	require.False(t, saved.IdentityDiscountExempt)
	saved.IdentityDiscountExempt = true
	require.NoError(t, store.Update(ctx, saved))
	saved, err = store.GetByID(ctx, group.ID)
	require.NoError(t, err)
	require.True(t, saved.IdentityDiscountExempt)
}
