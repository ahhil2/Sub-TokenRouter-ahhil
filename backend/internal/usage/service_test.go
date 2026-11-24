package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// service_test.go checks that interval rates and summary queries describe the
// same elapsed range. Captured repository bounds expose future clipping, while
// fixed counters distinguish text throughput from separately counted cache
// tokens and unbounded searches from measurable intervals.

// TestGetStatsWithPeriodRates measures historical, ongoing, empty and unbounded
// intervals. It checks matching query bounds, cache-token exclusion, absent
// unbounded averages, and preservation of the caller's selected end.
func TestGetStatsWithPeriodRates(t *testing.T) {
	start := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	asOf := start.Add(2 * time.Hour)
	historicalEnd := start.Add(time.Hour)
	futureEnd := start.Add(3 * time.Hour)
	futureStart := start.Add(4 * time.Hour)
	farFutureEnd := start.Add(5 * time.Hour)
	for _, tc := range []struct {
		name      string     // descriptive interval case name
		start     *time.Time // optional selected start instant
		end       *time.Time // optional selected end instant
		wantEnd   time.Time  // expected repository exclusive bound
		minutes   float64    // expected measured elapsed minutes
		unbounded bool       // absent duration selection case
	}{
		{name: "historical", start: &start, end: &historicalEnd, wantEnd: historicalEnd, minutes: 60},
		{name: "ongoing", start: &start, end: &futureEnd, wantEnd: asOf, minutes: 120},
		{name: "future", start: &futureStart, end: &farFutureEnd},
		{name: "reversed", start: &futureStart, end: &futureEnd},
		{name: "empty", start: &start, end: &start},
		{name: "unbounded request", unbounded: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &periodStatsRepo{stats: UsageStats{
				TotalRequests: 120, TotalInputTokens: 4000, TotalOutputTokens: 2000,
				TotalCacheTokens: 90000, TotalTokens: 96000,
			}}
			filters := UsageLogFilters{RequestID: "req-history", StartTime: tc.start, EndTime: tc.end}
			var selectedEnd time.Time
			if tc.end != nil {
				selectedEnd = *tc.end
			}
			result, err := NewUsageService(repo).GetStatsWithPeriodRates(context.Background(), filters, asOf)
			require.NoError(t, err)
			if tc.end != nil {
				require.Equal(t, selectedEnd, *tc.end)
			}
			if tc.unbounded {
				require.Nil(t, result.AverageRPM)
				require.Nil(t, result.AverageTPM)
				require.Equal(t, repo.stats, result.UsageStats)
				require.Equal(t, []UsageLogFilters{filters}, repo.filters)
				return
			}
			require.NotNil(t, result.AverageRPM)
			require.NotNil(t, result.AverageTPM)
			if tc.minutes == 0 {
				require.Empty(t, repo.filters)
				require.Equal(t, UsageStats{TotalProviderCost: new(float64)}, result.UsageStats)
				require.Zero(t, *result.AverageRPM)
				require.Zero(t, *result.AverageTPM)
				return
			}
			require.Len(t, repo.filters, 1)
			require.Equal(t, *tc.start, *repo.filters[0].StartTime)
			require.Equal(t, tc.wantEnd, *repo.filters[0].EndTime)
			require.Equal(t, repo.stats, result.UsageStats)
			require.InDelta(t, 120/tc.minutes, *result.AverageRPM, 1e-9)
			require.InDelta(t, 6000/tc.minutes, *result.AverageTPM, 1e-9)
		})
	}
}

// TestGetStatsWithPeriodRatesReturnsQueryError injects a failed summary query.
// It requires the service to return that error and no usable partial rates.
func TestGetStatsWithPeriodRatesReturnsQueryError(t *testing.T) {
	errQuery := errors.New("summary unavailable")
	repo := &periodStatsRepo{err: errQuery}
	result, err := NewUsageService(repo).GetStatsWithPeriodRates(context.Background(), UsageLogFilters{}, time.Now())
	require.ErrorIs(t, err, errQuery)
	require.Nil(t, result)
}
