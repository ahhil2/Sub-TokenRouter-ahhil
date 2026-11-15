//go:build integration

package postgres

import (
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/timing"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// TestUsagePerformanceSQLMatchesRecordedMetrics compares PostgreSQL aggregation
// expressions with usage API derivation for persisted fields. Missing, invalid,
// incomplete and media observations must yield identical eligible samples and
// rounded average TPS without depending on removed schema columns.
func TestUsagePerformanceSQLMatchesRecordedMetrics(t *testing.T) {
	complete := timing.NewPerformance(timing.FirstOutput, 1614)
	complete.Stream, complete.Complete, complete.UsageComplete = true, true, true
	complete.Values["upstream_first_event_ms"] = 1614
	complete.Values["upstream_first_output_ms"] = 11699
	complete.Values["upstream_last_output_ms"] = 11700
	complete.Values["upstream_output_event_count"] = 8
	complete.Values["upstream_finished_ms"] = 11761
	image, video := "image", "video"
	for _, tc := range []struct {
		name     string                // independent sample label
		change   func(*usage.UsageLog) // sample-specific observation change
		eligible bool                  // expected average output rate availability
	}{
		{"burst", func(*usage.UsageLog) {}, true},
		{"zero_event", func(u *usage.UsageLog) { u.Timings.Mode = timing.FirstEvent }, true},
		{"missing_start", func(u *usage.UsageLog) { delete(u.Timings.Values, "upstream_started_ms") }, false},
		{"missing_finish", func(u *usage.UsageLog) { delete(u.Timings.Values, "upstream_finished_ms") }, false},
		{"inverted", func(u *usage.UsageLog) { u.Timings.Values["upstream_finished_ms"] = 1613 }, false},
		{"zero_duration", func(u *usage.UsageLog) { u.Timings.Values["upstream_finished_ms"] = 1614 }, false},
		{"single_chunk", func(u *usage.UsageLog) { u.Timings.Values["upstream_output_event_count"] = 1 }, true},
		{"partial_usage", func(u *usage.UsageLog) { u.Timings.UsageComplete = false }, false},
		{"failed", func(u *usage.UsageLog) { u.Timings.Complete = false }, false},
		{"nonstreaming", func(u *usage.UsageLog) { u.Timings.Stream = false }, true},
		{"historical", func(u *usage.UsageLog) { u.Timings = nil }, false},
		{"empty_timing", func(u *usage.UsageLog) { u.Timings = &timing.Performance{} }, false},
		{"zero_tokens", func(u *usage.UsageLog) { u.OutputTokens = 0 }, false},
		{"negative_tokens", func(u *usage.UsageLog) { u.OutputTokens = -1 }, false},
		{"image_count", func(u *usage.UsageLog) { u.ImageCount = 1 }, false},
		{"image_tokens", func(u *usage.UsageLog) { u.ImageOutputTokens = 1 }, false},
		{"image_billing", func(u *usage.UsageLog) { u.BillingMode = &image }, false},
		{"video_count", func(u *usage.UsageLog) { u.VideoCount = 1 }, false},
		{"video_billing", func(u *usage.UsageLog) { u.BillingMode = &video }, false},
		{"live", func(u *usage.UsageLog) { u.RequestType = usage.RequestTypeLive }, false},
		{"websocket", func(u *usage.UsageLog) { u.RequestType = usage.RequestTypeWSV2 }, true},
		{"cyber_completed", func(u *usage.UsageLog) { u.RequestType = usage.RequestTypeCyberBlocked }, true},
		{"legacy", func(u *usage.UsageLog) { u.RequestType = usage.RequestTypeUnknown }, true},
		{"invalid_type", func(u *usage.UsageLog) { u.RequestType = 99 }, true},
		{"image_input", func(u *usage.UsageLog) { u.ImageInputTokens = 100 }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := &usage.UsageLog{Timings: complete.Clone(), OutputTokens: 334, RequestType: usage.RequestTypeStream}
			tc.change(u)
			encoded, err := json.Marshal(u.Timings)
			require.NoError(t, err)
			for _, alias := range []string{"", "u"} {
				var latency sql.NullInt64
				var speed sql.NullFloat64
				query := "SELECT " + usageTTFTSQL(alias) + "," + usageTPSSQL(alias) + ` FROM (
 SELECT $1::jsonb AS timings,$2::int AS output_tokens,$3::int AS image_count,
 $4::int AS image_output_tokens,$5::int AS video_count,$6::text AS billing_mode,
 $7::smallint AS request_type) u`
				require.NoError(t, integrationDB.QueryRowContext(t.Context(), query, string(encoded), u.OutputTokens,
					u.ImageCount, u.ImageOutputTokens, u.VideoCount, u.BillingMode, u.RequestType).Scan(&latency, &speed))
				require.Equal(t, u.Timings.TTFTMs() != nil, latency.Valid)
				if latency.Valid {
					require.Equal(t, int64(*u.Timings.TTFTMs()), latency.Int64)
				}
				require.Equal(t, tc.eligible, speed.Valid)
				require.Equal(t, u.TPS() != nil, speed.Valid)
				if speed.Valid {
					require.Equal(t, 32.9, speed.Float64)
					require.Equal(t, *u.TPS(), speed.Float64)
				}
			}
		})
	}
}
