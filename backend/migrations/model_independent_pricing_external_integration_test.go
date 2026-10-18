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

func TestModelIndependentPricingUpgrade(t *testing.T) {
	dsn := os.Getenv("TOKENROUTER_PRICING_TEST_DSN")
	if dsn == "" {
		t.Skip("requires an isolated pricing test database")
	}
	require.Contains(t, strings.ToLower(dsn), "independent_pricing_test")
	database, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	previous := fstest.MapFS{}
	entries, err := migrations.FS.ReadDir(".")
	require.NoError(t, err)
	for _, entry := range entries {
		if entry.Name() >= "300_" {
			continue
		}
		contents, readErr := migrations.FS.ReadFile(entry.Name())
		require.NoError(t, readErr)
		previous[entry.Name()] = &fstest.MapFile{Data: contents}
	}
	require.NoError(t, postgres.ApplyMigrations(t.Context(), database, previous))
	_, err = database.Exec(`INSERT INTO pricing_configs(id, name) VALUES (90001, 'independent pricing test');
INSERT INTO pricing_config_model_pricing(id, pricing_config_id, models, billing_mode, per_request_price)
VALUES (90001, 90001, '["custom-image"]', 'per_request', 0.065);`)
	require.NoError(t, err)
	require.NoError(t, postgres.ApplyMigrations(t.Context(), database, migrations.FS))
	require.NoError(t, postgres.ApplyMigrations(t.Context(), database, migrations.FS))
	var independent bool
	var price float64
	require.NoError(t, database.QueryRow(`SELECT independent_pricing, per_request_price FROM pricing_config_model_pricing WHERE id=90001`).Scan(&independent, &price))
	require.False(t, independent)
	require.InDelta(t, 0.065, price, 1e-12)
	_, err = database.Exec(`UPDATE pricing_config_model_pricing SET independent_pricing=TRUE WHERE id=90001`)
	require.NoError(t, err)
	require.NoError(t, database.QueryRow(`SELECT independent_pricing FROM pricing_config_model_pricing WHERE id=90001`).Scan(&independent))
	require.True(t, independent)
	_, err = database.Exec(`UPDATE pricing_config_model_pricing SET billing_mode='token' WHERE id=90001`)
	require.Error(t, err)
	_, err = database.Exec(`UPDATE pricing_config_model_pricing SET independent_pricing=FALSE, billing_mode='token' WHERE id=90001`)
	require.NoError(t, err)
}
