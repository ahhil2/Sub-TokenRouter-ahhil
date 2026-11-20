//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// ops_repo_dashboard_integration_test.go verifies overview measurements and
// cancellation against the migrated PostgreSQL fixture. It covers nullable and
// zero timing samples, continuous percentiles, and lib/pq's actual cancellation
// response without accessing production data.

// TestDashboardUsageLatencyPreservesSamples queries measured, missing, and zero
// timing values from the migrated schema. Percentiles retain eligible sample
// counts and an empty window keeps every measurement absent.
func TestDashboardUsageLatencyPreservesSamples(t *testing.T) {
	ctx := t.Context()
	start := time.Date(2044, 3, 4, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	var userID, keyID, providerID int64
	// Remove the user before its provider so cascades clean dependent usage and
	// keys. Cleanup has an independent lifetime after the test context ends.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", userID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(cleanupCtx, "DELETE FROM providers WHERE id = $1", providerID)
		require.NoError(t, err)
	})
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"INSERT INTO users(email, password_hash) VALUES('dashboard-latency@test.local', 'fixture') RETURNING id").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"INSERT INTO api_keys(user_id, key, name) VALUES($1, 'dashboard-latency-key', 'dashboard latency') RETURNING id",
		userID).Scan(&keyID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"INSERT INTO providers(name, platform, type) VALUES('dashboard-latency-provider', 'openai', 'apikey') RETURNING id").Scan(&providerID))
	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_logs (
  user_id, billing_user_id, api_key_id, provider_id, request_id, platform,
  model, duration_ms, timings, created_at
)
SELECT $1, $1, $2, $3, 'dashboard-latency-' || sample, 'openai', 'latency-model',
  duration_ms, CASE WHEN sample = 'historical' THEN NULL ELSE jsonb_build_object(
    'upstream_stream', streaming, 'ttft_mode', mode,
    'values', jsonb_build_object('upstream_started_ms', 100,
      'upstream_first_event_ms', event_ms, 'upstream_first_output_ms', output_ms)
  ) END, $4::timestamptz
FROM (VALUES
  ('zero', 100, true, 'first_event', 100, 200),
  ('measured', 200, true, 'first_output', 100, 200),
  ('buffered', 300, false, 'first_output', 100, 300),
  ('historical', NULL, true, 'first_output', NULL, NULL)
) AS samples(sample, duration_ms, streaming, mode, event_ms, output_ms)`, userID, keyID, providerID, start)
	require.NoError(t, err)

	repo := NewOpsRepository(integrationDB)
	filter := &ops.OpsDashboardFilter{StartTime: start, EndTime: end, Platform: "openai"}
	duration, ttft, sampleCount, err := repo.queryUsageLatency(ctx, filter, start, end)
	require.NoError(t, err)
	require.Equal(t, ops.OpsPercentiles{
		P50: intValuePtr(200), P90: intValuePtr(280), P95: intValuePtr(290),
		P99: intValuePtr(298), Avg: intValuePtr(200), Max: intValuePtr(300),
	}, duration)
	require.Equal(t, ops.OpsPercentiles{
		P50: intValuePtr(50), P90: intValuePtr(90), P95: intValuePtr(95),
		P99: intValuePtr(99), Avg: intValuePtr(50), Max: intValuePtr(100),
	}, ttft)
	require.EqualValues(t, 2, sampleCount)

	duration, ttft, sampleCount, err = repo.queryUsageLatency(ctx, filter, end, end.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, ops.OpsPercentiles{}, duration)
	require.Equal(t, ops.OpsPercentiles{}, ttft)
	require.Zero(t, sampleCount)
}

// TestDashboardQueryTimeoutRecognizesPostgresCancellation exercises the actual
// lib/pq cancellation response. A spent child deadline permits partial results,
// whereas expiry of the owning request remains fatal.
func TestDashboardQueryTimeoutRecognizesPostgresCancellation(t *testing.T) {
	parent := t.Context()
	child, cancel := context.WithTimeout(parent, 100*time.Millisecond)
	defer cancel()
	_, err := integrationDB.ExecContext(child, "SELECT pg_sleep(10)")
	require.Error(t, err)
	var databaseError *pq.Error
	require.ErrorAs(t, err, &databaseError)
	require.Equal(t, pq.ErrorCode("57014"), databaseError.Code)
	require.True(t, isQueryTimeoutErr(parent, child, err))
	require.NoError(t, parent.Err())
	require.NoError(t, integrationDB.PingContext(parent))

	request, cancelRequest := context.WithTimeout(parent, 100*time.Millisecond)
	defer cancelRequest()
	child, cancelChild := context.WithTimeout(request, time.Second)
	defer cancelChild()
	_, err = integrationDB.ExecContext(child, "SELECT pg_sleep(10)")
	require.Error(t, err)
	require.ErrorIs(t, request.Err(), context.DeadlineExceeded)
	require.False(t, isQueryTimeoutErr(request, child, err))
}
