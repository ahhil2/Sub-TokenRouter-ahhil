//go:build integration

package migrations_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// pelican_timeout_integration_test.go checks task defaults during schema upgrades.
// An isolated database first applies all migrations, then reconstructs the
// preceding task layout so an upgrade can populate existing tasks and preserve
// administrator budgets across repeated migration execution.

// TestPelicanTimeoutMigration upgrades an existing task and repeats migration
// execution after saving a custom limit. Both existing and newly inserted tasks
// receive the default, invalid limits fail, and repeated SQL retains saved limits.
func TestPelicanTimeoutMigration(t *testing.T) {
	database := postgrescontainer.New(t)
	ctx := t.Context()
	// Reconstruct the previous schema only inside this dedicated container;
	// removing its migration marker makes the runner exercise a real pending upgrade.
	_, err := database.ExecContext(ctx, `ALTER TABLE pelican_tasks DROP COLUMN timeout_seconds;
		DELETE FROM schema_migrations WHERE filename='311_pelican_timeout.sql'`)
	require.NoError(t, err)
	var groupID, taskID int64
	require.NoError(t, database.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES('pelican-timeout') RETURNING id`).Scan(&groupID))
	require.NoError(t, database.QueryRowContext(ctx, `INSERT INTO pelican_tasks(group_id,model) VALUES($1,'legacy') RETURNING id`, groupID).Scan(&taskID))
	require.NoError(t, postgres.ApplyMigrations(ctx, database, migrations.FS))
	var seconds int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT timeout_seconds FROM pelican_tasks WHERE id=$1`, taskID).Scan(&seconds))
	require.Equal(t, 300, seconds)
	require.NoError(t, database.QueryRowContext(ctx, `INSERT INTO pelican_tasks(group_id,model) VALUES($1,'new') RETURNING timeout_seconds`, groupID).Scan(&seconds))
	require.Equal(t, 300, seconds)
	for _, limit := range []int{0, -1, 3601} {
		_, err = database.ExecContext(ctx, `UPDATE pelican_tasks SET timeout_seconds=$1 WHERE id=$2`, limit, taskID)
		require.Error(t, err)
	}
	for _, limit := range []int{1, 3600, 900} {
		_, err = database.ExecContext(ctx, `UPDATE pelican_tasks SET timeout_seconds=$1 WHERE id=$2`, limit, taskID)
		require.NoError(t, err)
	}
	raw, err := migrations.FS.ReadFile("311_pelican_timeout.sql")
	require.NoError(t, err)
	_, err = database.ExecContext(ctx, string(raw))
	require.NoError(t, err)
	require.NoError(t, postgres.ApplyMigrations(ctx, database, migrations.FS))
	require.NoError(t, database.QueryRowContext(ctx, `SELECT timeout_seconds FROM pelican_tasks WHERE id=$1`, taskID).Scan(&seconds))
	require.Equal(t, 900, seconds)
	var count int
	require.NoError(t, database.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE filename='311_pelican_timeout.sql'`).Scan(&count))
	require.Equal(t, 1, count)
}
