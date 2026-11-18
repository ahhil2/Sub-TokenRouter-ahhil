package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// dashboard_snapshot_v2_handler_test.go verifies user-scoped minute rates in
// administrator snapshots. HTTP requests exercise parsed actor filters, separate
// cache entries, repeated reads, and statistics omission, while a shared global
// fixture detects accidental mutation of dashboard-wide rates.

// dashboardUserRatesRepoProbe supplies distinct global and actor minute rates.
type dashboardUserRatesRepoProbe struct {
	usage.UsageLogRepository                      // unused usage repository method slots
	global                   usage.DashboardStats // stable shared global metrics
	users                    []int64              // serial actor query observations
}

// GetDashboardStats returns a shared global result for response-copy assertions.
// Serial tests inspect this same fixture value after scoped requests to detect
// accidental replacement of its global rates.
func (r *dashboardUserRatesRepoProbe) GetDashboardStats(context.Context) (*usage.DashboardStats, error) {
	return &r.global, nil
}

// GetUserRealtimePerformance records the requested actor and returns distinct
// serial fixture values. Rates intentionally depend only on that actor, allowing
// chart dates and model filters to differ from the measurement scope.
func (r *dashboardUserRatesRepoProbe) GetUserRealtimePerformance(_ context.Context, userID int64) (int64, int64, error) {
	r.users = append(r.users, userID)
	return userID, userID * 100, nil
}

// TestDashboardHandler_SnapshotRatesSeparateUsers requests two actors and a
// global snapshot through the HTTP cache. Repeating the first actor must reuse
// its rates, while excluding statistics must skip the actor query.
func TestDashboardHandler_SnapshotRatesSeparateUsers(t *testing.T) {
	// This serial fixture owns the package snapshot cache until cleanup, so its
	// actor-specific entries cannot leak into other handler scenarios.
	dashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second)
	t.Cleanup(func() { dashboardSnapshotV2Cache = newSnapshotCache(30 * time.Second) })
	repo := &dashboardUserRatesRepoProbe{
		global: usage.DashboardStats{TotalRequests: 900, Rpm: 90, Tpm: 9000},
	}
	handler := NewDashboardHandler(usage.NewDashboardService(repo, nil, nil, nil), timezone.NewCalendar(time.UTC))
	router := gin.New()
	router.GET("/snapshot", handler.GetSnapshotV2)
	base := "/snapshot?start_date=2001-01-01&end_date=2001-01-02&model=filtered&include_trend=false&include_model_stats=false&include_group_stats=false&include_users_trend=false"
	for _, tc := range []struct {
		query string // requested actor query selector
		rpm   int64  // expected request rate value
		tpm   int64  // expected token rate value
		cache string // expected HTTP cache status
	}{
		{"&user_id=7", 7, 700, "miss"},
		{"&user_id=8", 8, 800, "miss"},
		{"", 90, 9000, "miss"},
		{"&user_id=7", 7, 700, "hit"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+tc.query, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, tc.cache, rec.Header().Get("X-Snapshot-Cache"))
		var envelope struct {
			Data dashboardSnapshotV2Response `json:"data"` // decoded snapshot response payload
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
		require.NotNil(t, envelope.Data.Stats)
		require.Equal(t, tc.rpm, envelope.Data.Stats.Rpm)
		require.Equal(t, tc.tpm, envelope.Data.Stats.Tpm)
		require.Equal(t, int64(900), envelope.Data.Stats.TotalRequests)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+"&user_id=9&include_stats=false", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.NotContains(t, rec.Body.String(), `"stats":`)
	require.Equal(t, []int64{7, 8}, repo.users)
	require.Equal(t, int64(90), repo.global.Rpm)
	require.Equal(t, int64(9000), repo.global.Tpm)
}
