package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// service_test.go checks selected-range totals alongside recent throughput.
// Captured repository bounds distinguish the summary's dates from the preceding
// minute, while separate counters expose accidental use of historical totals
// or cache tokens in recent rates.

// TestGetStatsWithRecentRates measures historical, ongoing, empty and unbounded
// selections. Every selection uses the preceding minute for rates while keeping
// the original summary scope and excluding cache tokens from TPM.
func TestGetStatsWithRecentRates(t *testing.T) {
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
			}, recentStats: &UsageStats{TotalRequests: 7, TotalInputTokens: 200, TotalOutputTokens: 100, TotalCacheTokens: 9000}}
			filters := UsageLogFilters{RequestID: "req-history", StartTime: tc.start, EndTime: tc.end}
			var selectedEnd time.Time
			if tc.end != nil {
				selectedEnd = *tc.end
			}
			result, err := NewUsageService(repo).GetStatsWithRecentRates(context.Background(), filters, asOf)
			require.NoError(t, err)
			if tc.end != nil {
				require.Equal(t, selectedEnd, *tc.end)
			}
			require.Equal(t, 7.0, *result.AverageRPM)
			require.Equal(t, 300.0, *result.AverageTPM)
			require.Len(t, repo.recentFilters, 1)
			require.Equal(t, asOf.Add(-time.Minute), *repo.recentFilters[0].StartTime)
			require.Equal(t, asOf, *repo.recentFilters[0].EndTime)
			require.Equal(t, filters.RequestID, repo.recentFilters[0].RequestID)
			if tc.unbounded {
				require.Equal(t, repo.stats, result.UsageStats)
				require.Equal(t, []UsageLogFilters{filters}, repo.filters)
				return
			}
			require.NotNil(t, result.AverageRPM)
			require.NotNil(t, result.AverageTPM)
			if tc.minutes == 0 {
				require.Empty(t, repo.filters)
				require.Equal(t, UsageStats{TotalProviderCost: new(float64)}, result.UsageStats)
				return
			}
			require.Len(t, repo.filters, 1)
			require.Equal(t, *tc.start, *repo.filters[0].StartTime)
			require.Equal(t, tc.wantEnd, *repo.filters[0].EndTime)
			require.Equal(t, repo.stats, result.UsageStats)
		})
	}
}

// TestGetStatsWithRecentRatesReturnsQueryError injects a failed summary query.
// It requires the service to return that error and no usable partial rates.
func TestGetStatsWithRecentRatesReturnsQueryError(t *testing.T) {
	errQuery := errors.New("summary unavailable")
	repo := &periodStatsRepo{err: errQuery}
	result, err := NewUsageService(repo).GetStatsWithRecentRates(context.Background(), UsageLogFilters{}, time.Now())
	require.ErrorIs(t, err, errQuery)
	require.Nil(t, result)
	repo.err = nil
	repo.recentErr = errQuery
	result, err = NewUsageService(repo).GetStatsWithRecentRates(context.Background(), UsageLogFilters{}, time.Now())
	require.ErrorIs(t, err, errQuery)
	require.Nil(t, result)
}
