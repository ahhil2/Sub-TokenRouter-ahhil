//go:build integration

package postgres

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// invalidation_migration_scenario_integration_test.go exercises the persisted
// cache policy and outbox events used by key_store.go and invalidation_outbox.go.
// A dedicated PostgreSQL fixture recreates the pre-312 API Key schema, upgrades
// an existing credential, and checks repeat migration and policy edits.

// TestClaudeCacheMigrationDefaultsAndInvalidation upgrades an existing API Key
// from the previous column layout. It verifies the disabled default, retained
// settings on repeated migration, and hashed invalidations for actual changes.
func TestClaudeCacheMigrationDefaultsAndInvalidation(t *testing.T) {
	ctx := t.Context()
	db := postgrescontainer.New(t)
	previous, err := migrations.FS.ReadFile("288_api_key_request_limits.sql")
	require.NoError(t, err)
	// Restore the preceding trigger before removing the new column in this isolated
	// database, so the fixture represents the schema encountered during an upgrade.
	_, err = db.ExecContext(ctx, string(previous))
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, "ALTER TABLE api_keys DROP COLUMN force_claude_1h_cache")
	require.NoError(t, err)
	var userID, keyID int64
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO users(email,password_hash) VALUES('cache@example.test','test') RETURNING id").Scan(&userID))
	const credential = "sk-cache-migration-fixture"
	require.NoError(t, db.QueryRowContext(ctx, "INSERT INTO api_keys(user_id,key,name) VALUES($1,$2,'cache') RETURNING id", userID, credential).Scan(&keyID))
	migration, err := migrations.FS.ReadFile("312_api_key_force_claude_1h_cache.sql")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	var enabled bool
	require.NoError(t, db.QueryRowContext(ctx, "SELECT force_claude_1h_cache FROM api_keys WHERE id=$1", keyID).Scan(&enabled))
	require.False(t, enabled)
	var baseline int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM auth_cache_invalidation_outbox").Scan(&baseline))
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(credential)))
	for _, change := range []struct {
		enabled bool // requested cache policy state
		events  int  // cumulative expected invalidation events
	}{
		{true, 1},
		{true, 1},
		{false, 2},
	} {
		_, err = db.ExecContext(ctx, "UPDATE api_keys SET force_claude_1h_cache=$1 WHERE id=$2", change.enabled, keyID)
		require.NoError(t, err)
		var count int
		require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM auth_cache_invalidation_outbox").Scan(&count))
		require.Equal(t, baseline+change.events, count)
		require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM auth_cache_invalidation_outbox WHERE cache_key=$1", hash).Scan(&count))
		require.Equal(t, change.events, count)
	}
	_, err = db.ExecContext(ctx, "UPDATE api_keys SET force_claude_1h_cache=true WHERE id=$1", keyID)
	require.NoError(t, err)
	// Reapplying the additive migration must retain an owner's enabled choice;
	// its default applies to missing values and newly created credentials.
	_, err = db.ExecContext(ctx, string(migration))
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT force_claude_1h_cache FROM api_keys WHERE id=$1", keyID).Scan(&enabled))
	require.True(t, enabled)
	_, err = db.ExecContext(ctx, "INSERT INTO api_keys(user_id,key,name) VALUES($1,'sk-cache-default-fixture','default')", userID)
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(ctx, "SELECT force_claude_1h_cache FROM api_keys WHERE key='sk-cache-default-fixture'").Scan(&enabled))
	require.False(t, enabled)
}
