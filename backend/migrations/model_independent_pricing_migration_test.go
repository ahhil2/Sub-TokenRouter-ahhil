package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModelIndependentPricingMigration(t *testing.T) {
	content, err := FS.ReadFile("300_model_independent_pricing.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "independent_pricing BOOLEAN NOT NULL DEFAULT FALSE")
	require.Contains(t, sql, "CHECK (NOT independent_pricing OR billing_mode IN ('per_request', 'image', 'video'))")
	require.NotContains(t, sql, "UPDATE ")
	require.NotContains(t, sql, "provider_stats")
}
