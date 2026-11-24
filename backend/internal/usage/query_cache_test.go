package usage

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// query_cache_test.go verifies administrator statistics cache keys, shared
// cancellation, and independent interval samples. Concurrent statistics reads
// can share a query while the first caller cancels its own wait. The blocking
// repository announces entry and waits for
// an explicit release or shared-context cancellation, and the second context
// announces when querycache reaches its cancellation select.
//
// The test orders second-waiter registration before canceling the first
// caller, then releases the one repository read. Its observations distinguish
// cancellation of one HTTP-style waiter from cancellation of work still
// needed by another. Separate key cases check repeatability, while filter
// changes must load independent totals and rates for each administrator selector.
//
// Fixed current-day selections reuse the first sample's totals and rates when
// a later caller supplies another observation time. Mutating returned rate
// pointers and nested totals checks that the cache keeps an independent copy.

// statsCancellationRepo blocks shared statistics queries until the test
// releases or cancels them.
type statsCancellationRepo struct {
	UsageLogRepository // unused usage repository method slots
	// started closes when calls reaches one, and the test closes release to
	// finish the shared query. Configure both before dispatch and observe calls
	// atomically across waiters.
	started chan struct{} // first loader entry signal
	release chan struct{} // explicit shared loader completion signal
	calls   atomic.Int32  // atomic shared loader invocation count
}

// joinedStatsContext observes querycache waiter registration through the
// context's Done lookup.
type joinedStatsContext struct {
	context.Context // embedded live waiter context
	// joined closes when Done first announces a cancellation-channel check.
	// once prevents repeated Done calls from closing that publication channel
	// again while returning the embedded channel.
	joined chan struct{} // waiter cancellation check signal
	once   sync.Once     // single joined signal publication guard
}

// TestUsageStatsCacheKey_StableAndDistinct repeats one filter set and varies
// model, user, team and endpoint source. It requires stability and distinct
// keys for the exercised variations.
func TestUsageStatsCacheKey_StableAndDistinct(t *testing.T) {
	start := time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC)
	base := UsageLogFilters{StartTime: &start, EndTime: &end, Model: "claude-3"}

	k1 := usageStatsCacheKey(base)
	k2 := usageStatsCacheKey(base)
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "same filters must produce same key")

	other := base
	other.Model = "gpt-4o"
	require.NotEqual(t, k1, usageStatsCacheKey(other), "different model must change key")

	withUser := base
	withUser.UserID = 7
	require.NotEqual(t, k1, usageStatsCacheKey(withUser), "different user must change key")

	withTeam := base
	withTeam.TeamID = 11
	require.NotEqual(t, k1, usageStatsCacheKey(withTeam), "different team must change key")
	keys := map[string]bool{k1: true}
	for _, source := range []string{"inbound", "upstream", "path"} {
		filtered := base
		filtered.EndpointSource = source
		key := usageStatsCacheKey(filtered)
		require.False(t, keys[key])
		keys[key] = true
	}
}

// GetStatsWithFilters announces the first repository call and waits for
// release or shared-context cancellation. The fixture counter records how
// many loaders actually reach this method.
//
// Release supplies the fixed statistics result, while canceled shared work
// returns its context error. This select lets the test determine whether
// canceling the first cache waiter incorrectly cancels a loader still needed
// by another waiter. Callers configure channels before dispatch and wait for
// completion before changing them.
func (r *statsCancellationRepo) GetStatsWithFilters(ctx context.Context, _ UsageLogFilters) (*UsageStats, error) {
	// Publish initial entry before waiting so the test can add another cache
	// waiter to a known active load before canceling its first caller.
	if r.calls.Add(1) == 1 {
		close(r.started)
	}
	select {
	case <-r.release:
		return &UsageStats{TotalRequests: 17}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Done announces once that querycache has reached this waiter's
// cancellation-channel check. It returns the embedded context channel,
// allowing the test to order leader cancellation after the live waiter
// registered.
func (c *joinedStatsContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.joined) })
	return c.Context.Done()
}

// TestStatsCanceledLeaderKeepsLiveWaiter registers a second waiter before
// canceling the first. It requires first-caller cancellation, the second
// caller's statistics result and one repository load.
func TestStatsCanceledLeaderKeepsLiveWaiter(t *testing.T) {
	repo := &statsCancellationRepo{started: make(chan struct{}), release: make(chan struct{})}
	service := NewUsageService(repo)
	filters := UsageLogFilters{EndpointSource: "upstream"}
	asOf := time.Now()
	first, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstDone := make(chan error, 1)
	// Enter the repository before adding the second waiter so both requests join
	// the same active flight and the fixture can observe one shared query invocation.
	go func() {
		_, _, err := service.GetStatsCached(first, filters, asOf)
		firstDone <- err
	}()
	<-repo.started
	live := &joinedStatsContext{Context: context.Background(), joined: make(chan struct{})}
	nextDone := make(chan error, 1)
	nextValue := make(chan *UsagePeriodStats, 1)
	go func() {
		value, _, err := service.GetStatsCached(live, filters, asOf)
		nextValue <- value
		nextDone <- err
	}()
	// Done is queried after cache registration. Wait for that observation before
	// canceling the leader, leaving the second waiter active when shared work is evaluated.
	<-live.joined
	cancel()
	require.ErrorIs(t, <-firstDone, context.Canceled)
	// Complete shared loading after the first cancellation has returned. The live
	// waiter's result then demonstrates that its needed repository work remained available.
	close(repo.release)
	require.NoError(t, <-nextDone)
	require.EqualValues(t, 17, (<-nextValue).TotalRequests)
	require.EqualValues(t, 1, repo.calls.Load())
}

// TestStatsCachedKeepsIntervalSampleAndCopiesPayload advances the observation
// time for a fixed today selection. It requires one repository query, stable
// sampled averages, and independent embedded totals, slices and rate pointers.
func TestStatsCachedKeepsIntervalSampleAndCopiesPayload(t *testing.T) {
	start := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	asOf := start.Add(2 * time.Hour)
	cost := 2.5
	repo := &periodStatsRepo{stats: UsageStats{
		TotalRequests: 120, TotalInputTokens: 4000, TotalOutputTokens: 2000,
		TotalProviderCost: &cost,
		Endpoints:         []EndpointStat{{Endpoint: "responses", Requests: 120}},
		UpstreamEndpoints: []EndpointStat{{Endpoint: "upstream", Requests: 120}},
		EndpointPaths:     []EndpointStat{{Endpoint: "/v1/responses", Requests: 120}},
	}}
	service := NewUsageService(repo)
	filters := UsageLogFilters{StartTime: &start, EndTime: &end}
	first, hit, err := service.GetStatsCached(context.Background(), filters, asOf)
	require.NoError(t, err)
	require.False(t, hit)
	require.Equal(t, 1.0, *first.AverageRPM)
	require.Equal(t, 50.0, *first.AverageTPM)
	// Mutate every reference-bearing part of the returned summary before reading
	// the same entry again, exposing aliases through the newly embedded totals.
	*first.AverageRPM = -1
	*first.AverageTPM = -1
	*first.TotalProviderCost = -1
	first.Endpoints[0].Requests = -1
	first.UpstreamEndpoints[0].Requests = -1
	first.EndpointPaths[0].Requests = -1
	first.TotalRequests = -1
	second, hit, err := service.GetStatsCached(context.Background(), filters, asOf.Add(5*time.Second))
	require.NoError(t, err)
	require.True(t, hit)
	require.Len(t, repo.filters, 1)
	require.Equal(t, asOf, *repo.filters[0].EndTime)
	require.Equal(t, start.AddDate(0, 0, 1), end)
	require.Equal(t, repo.stats, second.UsageStats)
	require.Equal(t, 1.0, *second.AverageRPM)
	require.Equal(t, 50.0, *second.AverageTPM)
	*second.AverageRPM = -2
	third, hit, err := service.GetStatsCached(context.Background(), filters, asOf.Add(10*time.Second))
	require.NoError(t, err)
	require.True(t, hit)
	require.Equal(t, 1.0, *third.AverageRPM)
}

// TestStatsCachedUnboundedRequestKeepsNullRates repeats a retained-history
// request lookup. It requires a cache hit to preserve nil averages alongside
// the original totals and unrestricted query bounds.
func TestStatsCachedUnboundedRequestKeepsNullRates(t *testing.T) {
	repo := &periodStatsRepo{stats: UsageStats{TotalRequests: 3}}
	service := NewUsageService(repo)
	filters := UsageLogFilters{RequestID: "req-history"}
	for index := range 2 {
		stats, hit, err := service.GetStatsCached(context.Background(), filters, time.Now())
		require.NoError(t, err)
		require.Equal(t, index > 0, hit)
		require.EqualValues(t, 3, stats.TotalRequests)
		require.Nil(t, stats.AverageRPM)
		require.Nil(t, stats.AverageTPM)
	}
	require.Equal(t, []UsageLogFilters{filters}, repo.filters)
}

// TestStatsCachedIsolatesFilterSelections changes each administrator selector
// after caching a baseline summary. It requires a new matching rate sample and
// prevents later reads from mixing the two selections' totals or averages.
func TestStatsCachedIsolatesFilterSelections(t *testing.T) {
	start := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	for _, tc := range []struct {
		name         string                 // administrator selector under test
		selectFilter func(*UsageLogFilters) // supplied selector mutation function
	}{
		{name: "request", selectFilter: func(f *UsageLogFilters) { f.RequestID = "req-selected" }},
		{name: "endpoint", selectFilter: func(f *UsageLogFilters) { f.EndpointSource = "inbound" }},
		{name: "user", selectFilter: func(f *UsageLogFilters) { f.UserID = 7 }},
		{name: "api key", selectFilter: func(f *UsageLogFilters) { f.APIKeyID = 11 }},
		{name: "provider", selectFilter: func(f *UsageLogFilters) { f.ProviderID = 13 }},
		{name: "group", selectFilter: func(f *UsageLogFilters) { f.GroupID = 17 }},
		{name: "team", selectFilter: func(f *UsageLogFilters) { f.TeamID = 19 }},
		{name: "model", selectFilter: func(f *UsageLogFilters) { f.Model = "selected-model" }},
		{name: "request type", selectFilter: func(f *UsageLogFilters) { f.RequestType = new(int16) }},
		{name: "nonstream", selectFilter: func(f *UsageLogFilters) { f.Stream = new(bool) }},
		{name: "compaction false", selectFilter: func(f *UsageLogFilters) { f.NativeCompactionV2 = new(bool) }},
		{name: "billing type zero", selectFilter: func(f *UsageLogFilters) { f.BillingType = new(int8) }},
		{name: "billing mode", selectFilter: func(f *UsageLogFilters) { f.BillingMode = "token" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &periodStatsRepo{stats: UsageStats{
				TotalRequests: 120, TotalInputTokens: 4000, TotalOutputTokens: 2000,
			}}
			service := NewUsageService(repo)
			base := UsageLogFilters{StartTime: &start, EndTime: &end}
			first, hit, err := service.GetStatsCached(context.Background(), base, end)
			require.NoError(t, err)
			require.False(t, hit)
			require.Equal(t, 2.0, *first.AverageRPM)
			require.Equal(t, 100.0, *first.AverageTPM)

			selected := base
			tc.selectFilter(&selected)
			repo.stats = UsageStats{TotalRequests: 300, TotalInputTokens: 12000, TotalOutputTokens: 6000}
			filtered, hit, err := service.GetStatsCached(context.Background(), selected, end)
			require.NoError(t, err)
			require.False(t, hit)
			require.Equal(t, 5.0, *filtered.AverageRPM)
			require.Equal(t, 300.0, *filtered.AverageTPM)
			require.Equal(t, []UsageLogFilters{base, selected}, repo.filters)

			// Replace the fixture values before reading both cached scopes again.
			// A leaked or missing cache dimension would publish the other selection's sample.
			repo.stats = UsageStats{TotalRequests: 600}
			baseAgain, hit, err := service.GetStatsCached(context.Background(), base, end)
			require.NoError(t, err)
			require.True(t, hit)
			require.Equal(t, first, baseAgain)
			selectedAgain, hit, err := service.GetStatsCached(context.Background(), selected, end)
			require.NoError(t, err)
			require.True(t, hit)
			require.Equal(t, filtered, selectedAgain)
			require.Len(t, repo.filters, 2)
		})
	}
}
