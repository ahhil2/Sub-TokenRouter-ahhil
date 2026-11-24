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
}

// GetStatsWithFilters captures the query selectors and returns configured totals.
// Tests access captured filters after their synchronous service or cache call
// completes, and configure all fixture values before starting that call.
func (r *periodStatsRepo) GetStatsWithFilters(_ context.Context, filters UsageLogFilters) (*UsageStats, error) {
	r.filters = append(r.filters, filters)
	return &r.stats, r.err
}
