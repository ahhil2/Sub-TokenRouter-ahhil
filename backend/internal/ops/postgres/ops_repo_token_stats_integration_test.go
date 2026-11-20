//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// TestGetTokenStatsMigratedUsageSchema queries text and media observations in
// the fully migrated usage table. It protects query compatibility, text-only
// speed averages, captured TTFT modes, nullable samples, complete request totals,
// and pagination through empty pages. Unused malformed offsets must stay uncast.
func TestGetTokenStatsMigratedUsageSchema(t *testing.T) {
	ctx := t.Context()
	start := time.Date(2043, 2, 3, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	var userID, keyID, providerID int64
	// Delete the owning user first so cascades remove usage and API keys before
	// provider cleanup. An independent deadline keeps cleanup usable after failure.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id = $1", userID)
		require.NoError(t, err)
		_, err = integrationDB.ExecContext(cleanupCtx, "DELETE FROM providers WHERE id = $1", providerID)
		require.NoError(t, err)
	})
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"INSERT INTO users(email, password_hash) VALUES('token-stats@test.local', 'fixture') RETURNING id").Scan(&userID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"INSERT INTO api_keys(user_id, key, name) VALUES($1, 'token-stats-key', 'token stats') RETURNING id",
		userID).Scan(&keyID))
	require.NoError(t, integrationDB.QueryRowContext(ctx,
		"INSERT INTO providers(name, platform, type) VALUES('token-stats-provider', 'openai', 'apikey') RETURNING id").Scan(&providerID))

	_, err := integrationDB.ExecContext(ctx, `
INSERT INTO usage_logs (
  user_id, billing_user_id, api_key_id, provider_id, request_id, platform,
  model, output_tokens, image_count, image_size, image_output_tokens, video_count,
  billing_mode, request_type, stream, duration_ms, timings, created_at
)
SELECT $1, $1, $2, $3, 'token-stats-' || sample, 'openai', model, output_tokens,
  image_count, CASE WHEN image_count > 0 THEN '1K' END,
  image_output_tokens, video_count, billing_mode, request_type,
  streaming, 1000, jsonb_build_object(
    'upstream_stream', streaming, 'upstream_complete', true,
    'usage_complete', complete, 'ttft_mode', 'first_output',
    'values', jsonb_build_object('upstream_started_ms', 100,
      'upstream_finished_ms', 1100, 'upstream_first_output_ms', 200)
  ), $4::timestamptz
FROM (VALUES
  ('stream', 'text-model', 100, 0, 0, 0, NULL, 2, true, true),
  ('buffered', 'text-model', 200, 0, 0, 0, NULL, 1, false, true),
  ('image-count', 'text-model', 500, 1, 0, 0, NULL, 2, true, true),
  ('image-tokens', 'text-model', 500, 0, 1, 0, NULL, 2, true, true),
  ('video-count', 'text-model', 500, 0, 0, 1, NULL, 2, true, true),
  ('image-billing', 'text-model', 500, 0, 0, 0, 'image', 2, true, true),
  ('video-billing', 'text-model', 500, 0, 0, 0, 'video', 2, true, true),
  ('live', 'text-model', 500, 0, 0, 0, NULL, 5, true, true),
  ('incomplete', 'text-model', 500, 0, 0, 0, NULL, 2, true, false),
  ('media-only', 'media-model', 500, 1, 0, 0, 'image', 2, true, true)
) AS samples(sample, model, output_tokens, image_count, image_output_tokens,
  video_count, billing_mode, request_type, streaming, complete)`, userID, keyID, providerID, start)
	require.NoError(t, err)

	// Each timing edge case shares a model so the response verifies independent
	// TPS and TTFT sample populations, including zero latency and per-row rounding.
	_, err = integrationDB.ExecContext(ctx, `
INSERT INTO usage_logs (
  user_id, billing_user_id, api_key_id, provider_id, request_id, platform,
  model, output_tokens, request_type, stream, duration_ms, timings, created_at
)
SELECT $1, $1, $2, $3, 'token-stats-timing-' || sample, 'openai', 'timing-model',
  output_tokens, 2, true, 1000, timings::jsonb, $4::timestamptz
FROM (VALUES
  ('zero-event', 333, '{"upstream_stream":true,"upstream_complete":true,"usage_complete":true,"ttft_mode":"first_event","values":{"upstream_started_ms":100,"upstream_finished_ms":1100,"upstream_first_event_ms":100,"upstream_first_output_ms":"unused"}}'),
  ('output-mode', 10, '{"upstream_stream":true,"upstream_complete":true,"usage_complete":true,"ttft_mode":"first_output","values":{"upstream_started_ms":100,"upstream_finished_ms":400,"upstream_first_event_ms":"unused","upstream_first_output_ms":300}}'),
  ('missing-timing', 20, NULL),
  ('non-object-offsets', 30, '{"values":[]}'),
  ('inverted-offsets', 50, '{"upstream_stream":true,"upstream_complete":true,"usage_complete":true,"values":{"upstream_started_ms":100,"upstream_finished_ms":99,"upstream_first_output_ms":99}}'),
  ('missing-start', 40, '{"upstream_stream":true,"upstream_complete":true,"usage_complete":true,"values":{"upstream_finished_ms":1100,"upstream_first_output_ms":200}}'),
  ('incomplete-usage', 60, '{"upstream_stream":true,"upstream_complete":true,"usage_complete":false,"values":{"upstream_started_ms":100,"upstream_finished_ms":"unused","upstream_first_output_ms":150}}'),
  ('buffered-output', 100, '{"upstream_stream":false,"upstream_complete":true,"usage_complete":true,"values":{"upstream_started_ms":100,"upstream_finished_ms":433,"upstream_first_output_ms":"unused"}}')
) AS samples(sample, output_tokens, timings)`, userID, keyID, providerID, start)
	require.NoError(t, err)

	repo := NewOpsRepository(integrationDB)
	filter := &ops.OpsTokenStatsFilter{StartTime: start, EndTime: end, Platform: "openai", TopN: 5}
	result, err := repo.GetTokenStats(ctx, filter)
	require.NoError(t, err)
	require.EqualValues(t, 3, result.Total)
	require.Len(t, result.Items, 3)
	require.Equal(t, "text-model", result.Items[0].Model)
	require.EqualValues(t, 9, result.Items[0].RequestCount)
	require.EqualValues(t, 3800, result.Items[0].TotalOutputTokens)
	require.NotNil(t, result.Items[0].AvgTokensPerSec)
	require.Equal(t, float64(150), *result.Items[0].AvgTokensPerSec)
	require.NotNil(t, result.Items[0].AvgTTFTMs)
	require.Equal(t, float64(100), *result.Items[0].AvgTTFTMs)
	require.EqualValues(t, 8, result.Items[0].RequestsWithFirstToken)

	timingStats := result.Items[1]
	require.Equal(t, "timing-model", timingStats.Model)
	require.EqualValues(t, 8, timingStats.RequestCount)
	require.EqualValues(t, 643, timingStats.TotalOutputTokens)
	require.EqualValues(t, 1000, timingStats.AvgDurationMs)
	require.NotNil(t, timingStats.AvgTokensPerSec)
	require.Equal(t, 222.2, *timingStats.AvgTokensPerSec)
	require.NotNil(t, timingStats.AvgTTFTMs)
	require.Equal(t, 83.33, *timingStats.AvgTTFTMs)
	require.EqualValues(t, 3, timingStats.RequestsWithFirstToken)
	require.Nil(t, result.Items[2].AvgTokensPerSec)

	filter.TopN = 0
	filter.PageSize = 1
	for page := 1; page <= 4; page++ {
		filter.Page = page
		result, err := repo.GetTokenStats(ctx, filter)
		require.NoError(t, err)
		require.EqualValues(t, 3, result.Total)
		if page == 4 {
			require.Empty(t, result.Items)
		} else {
			require.Len(t, result.Items, 1)
		}
	}
}
