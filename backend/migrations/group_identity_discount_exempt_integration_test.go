//go:build integration

package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// group_identity_discount_exempt_integration_test.go checks original-price
// schema defaults and repeatable upgrades. A dedicated fixture first applies
// every migration, then reconstructs the previous groups schema to verify that
// existing rows retain discounts and repeated SQL retains enabled policies.

// TestGroupIdentityDiscountExemptMigration upgrades an existing group and
// repeats both SQL and migration-runner execution. Defaults keep discounts
// enabled, saved original pricing survives repetition, and migration identity
// remains recorded once.
func TestGroupIdentityDiscountExemptMigration(t *testing.T) {
	database := postgrescontainer.New(t)
	ctx := t.Context()
	raw, err := migrations.FS.ReadFile("310_group_identity_discount_exempt.sql")
	require.NoError(t, err)
	// Reconstruct the previous column layout inside this isolated database so
	// applying the new SQL also exercises defaults for rows that already exist.
	_, err = database.ExecContext(ctx, `ALTER TABLE groups DROP COLUMN identity_discount_exempt`)
	require.NoError(t, err)
	var legacyID int64
	require.NoError(t, database.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES('legacy-price') RETURNING id`).Scan(&legacyID))
	_, err = database.ExecContext(ctx, string(raw))
	require.NoError(t, err)
	var exempt bool
	require.NoError(t, database.QueryRowContext(ctx, `SELECT identity_discount_exempt FROM groups WHERE id=$1`, legacyID).Scan(&exempt))
	require.False(t, exempt)
	require.NoError(t, database.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES('new-price') RETURNING identity_discount_exempt`).Scan(&exempt))
	require.False(t, exempt)
	_, err = database.ExecContext(ctx, `UPDATE groups SET identity_discount_exempt=true WHERE id=$1`, legacyID)
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, string(raw))
	require.NoError(t, err)
	require.NoError(t, postgres.ApplyMigrations(ctx, database, migrations.FS))
	require.NoError(t, database.QueryRowContext(ctx, `SELECT identity_discount_exempt FROM groups WHERE id=$1`, legacyID).Scan(&exempt))
	require.True(t, exempt)
	var count int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE filename='310_group_identity_discount_exempt.sql'`).Scan(&count))
	require.Equal(t, 1, count)
}
