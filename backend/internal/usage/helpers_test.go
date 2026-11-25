package usage

import "context"

// helpers_test.go supplies repository fixtures shared by usage service and query
// cache tests. Captured filters expose the interval and selection used for each
// sample, while configured counters and failures let callers check rates and
// cached copies independently of database storage.

// periodStatsRepo captures summary bounds and supplies configured usage totals.
type periodStatsRepo struct {
	UsageLogRepository                   // unused repository method slots
	stats              UsageStats        // configured filtered summary counters
	filters            []UsageLogFilters // completed summary query selectors
	err                error             // configured summary query failure
	// recentStats supplies separate minute counters when present; nil uses the
	// summary counters so existing fixtures can share their configured values.
	recentStats   *UsageStats       // optional recent counter override
	recentFilters []UsageLogFilters // completed recent query selectors
	recentErr     error             // configured recent query failure
}

// GetStatsWithFilters captures the query selectors and returns configured totals.
// Tests access captured filters after their synchronous service or cache call
// completes, and configure all fixture values before starting that call.
func (r *periodStatsRepo) GetStatsWithFilters(_ context.Context, filters UsageLogFilters) (*UsageStats, error) {
	if filters.EndpointSource == "none" {
		r.recentFilters = append(r.recentFilters, filters)
		if r.recentStats != nil {
			return r.recentStats, r.recentErr
		}
		return &r.stats, r.recentErr
	}
	r.filters = append(r.filters, filters)
	return &r.stats, r.err
}
