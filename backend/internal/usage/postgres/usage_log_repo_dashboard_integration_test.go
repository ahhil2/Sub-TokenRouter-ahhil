//go:build integration

package postgres

import (
	"time"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/settings/preaggregation"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// usage_log_repo_dashboard_integration_test.go verifies dashboard totals and
// recent rates against persisted usage records. Administrator scenarios distinguish
// the preceding minute from older and future rows, while a Kathmandu key fixture
// compares raw and analytics totals across local dates and retained-history changes.

// TestAdminDashboardOneMinuteRates records recent, older, and future usage with
// cache tokens. Both administrator query paths must return global sixty-second
// request and text-token counts, independently of the totals' selected date range.
func (s *UsageLogRepoSuite) TestAdminDashboardOneMinuteRates() {
	user := mustCreateUser(s.T(), s.client, &identity.User{Email: "admin-minute@test.local"})
	key := mustCreateApiKey(s.T(), s.client, &apikey.APIKey{UserID: user.ID, Key: "admin-minute"})
	p := mustCreateProvider(s.T(), s.client, &provider.Record{Name: "admin-minute"})
	// Reset observations within this rollback-owned transaction so the global
	// measurement is independent of committed records from other repository scenarios.
	_, err := s.tx.ExecContext(s.ctx, "DELETE FROM usage_logs")
	s.Require().NoError(err)
	now := time.Now().UTC()
	recent := s.createUsageLog(user, key, p, 100, 50, 0, now.Add(-30*time.Second))
	s.createUsageLog(user, key, p, 200, 75, 0, now.Add(-15*time.Second))
	s.createUsageLog(user, key, p, 1000, 2000, 0, now.Add(-2*time.Minute))
	s.createUsageLog(user, key, p, 4000, 5000, 0, now.Add(2*time.Minute))
	_, err = s.tx.ExecContext(s.ctx, "UPDATE usage_logs SET cache_creation_tokens=3000, cache_read_tokens=7000 WHERE id=$1", recent.ID)
	s.Require().NoError(err)

	aggregated, err := s.repo.GetDashboardStats(s.ctx)
	s.Require().NoError(err)
	s.Require().Equal(int64(2), aggregated.Rpm)
	s.Require().Equal(int64(425), aggregated.Tpm)
	raw, err := s.repo.GetDashboardStatsWithRange(s.ctx, now.Add(-24*time.Hour), now.Add(-time.Hour))
	s.Require().NoError(err)
	s.Require().Equal(int64(2), raw.Rpm)
	s.Require().Equal(int64(425), raw.Tpm)
}

// TestAPIKeyDashboardAnalyticsParity records usage around Kathmandu dates with
// null timings, zero costs, and future instants. It compares complete
// raw/analytics dashboards, then checks parity after retained raw history
// shrinks.
func (s *UsageLogRepoSuite) TestAPIKeyDashboardAnalyticsParity() {
	user := mustCreateUser(s.T(), s.client, &identity.User{Email: "key-analytics@test.local"})
	key := mustCreateApiKey(s.T(), s.client, &apikey.APIKey{UserID: user.ID, Key: "key-analytics"})
	p := mustCreateProvider(s.T(), s.client, &provider.Record{Name: "key-analytics"})
	loc, err := time.LoadLocation("Asia/Kathmandu")
	s.Require().NoError(err)
	s.repo.calendar = timezone.NewCalendar(loc)
	now := time.Now().UTC().Truncate(time.Second)
	for i, at := range []time.Time{now.Add(-120 * time.Hour), now.Add(-53 * time.Hour), now.Add(-12 * time.Hour), now.Add(-2 * time.Hour), now.Add(48 * time.Hour)} {
		log := s.createUsageLog(user, key, p, 10+i, 5+i, float64(i), at)
		if i%2 == 0 {
			_, err = s.tx.ExecContext(s.ctx, "UPDATE usage_logs SET duration_ms=$1 WHERE id=$2", i*100, log.ID)
			s.Require().NoError(err)
		}
	}
	want, err := s.repo.GetAPIKeyDashboardStats(s.ctx, key.ID)
	s.Require().NoError(err)
	start := now.Add(-72 * time.Hour).Truncate(time.Hour)
	watermark := now.Add(-3 * time.Hour)
	aggregation := NewAggregationStoreWithSQL(s.tx, s.repo.calendar)
	s.Require().NoError(aggregation.AggregateUsageAnalyticsRange(s.ctx, start, watermark))
	s.Require().NoError(aggregation.SaveUsageAnalyticsAggregationState(s.ctx, &usage.UsageAnalyticsAggregationState{
		LiveWatermark: watermark, CoverageStart: &start, Phase: "idle",
	}))
	s.repo.preAggregation = preaggregation.NewPreAggregationSettingsService(nil, &preaggregation.Options{
		Usage: preaggregation.UsageOptions{Enabled: true, IntervalSeconds: 60},
	})
	_, ok, err := s.repo.getAPIKeyDashboardStatsFromAnalytics(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().True(ok)
	got, err := s.repo.GetAPIKeyDashboardStats(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Equal(want, got)
	// Delete retained history without rebuilding old buckets, then compare readers;
	// the key's oldest remaining raw record must bound its retained analytics
	// total.
	_, err = s.tx.ExecContext(s.ctx, "DELETE FROM usage_logs WHERE api_key_id=$1 AND created_at<$2", key.ID, now.Add(-24*time.Hour))
	s.Require().NoError(err)
	got, err = s.repo.GetAPIKeyDashboardStats(s.ctx, key.ID)
	s.Require().NoError(err)
	s.repo.preAggregation = nil
	want, err = s.repo.GetAPIKeyDashboardStats(s.ctx, key.ID)
	s.Require().NoError(err)
	s.Require().Equal(want, got)
}
