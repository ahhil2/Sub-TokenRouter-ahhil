package ops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// dashboard_test.go protects the service boundary around overview source
// selection. Repository failures must retain their original identity and must
// not cause another complete raw query after repository-owned fallback.

// dashboardFailureRepository records overview calls while embedding successful
// defaults for unrelated service diagnostics.
type dashboardFailureRepository struct {
	opsRepoMock              // default unrelated repository behavior
	calls       int          // attempted overview queries
	err         error        // repository failure to preserve
	mode        OpsQueryMode // last supplied source preference
}

// TestDashboardOverviewDoesNotRetryRepositoryFailure injects a failed automatic
// overview read. The service must make exactly one repository call and preserve
// its error for the API response.
func TestDashboardOverviewDoesNotRetryRepositoryFailure(t *testing.T) {
	queryErr := errors.New("overview unavailable")
	repo := &dashboardFailureRepository{err: queryErr}
	service := &OpsService{opsRepo: repo}
	start := time.Date(2024, 1, 2, 10, 0, 0, 0, time.UTC)

	result, err := service.GetDashboardOverview(t.Context(), &OpsDashboardFilter{
		StartTime: start, EndTime: start.Add(time.Hour), QueryMode: OpsQueryModeAuto,
	})

	require.ErrorIs(t, err, queryErr)
	require.Nil(t, result)
	require.Equal(t, 1, repo.calls)
	require.Equal(t, OpsQueryModeAuto, repo.mode)
}

// GetDashboardOverview records the requested source mode and returns its
// configured failure. Embedding the ordinary fixture keeps this regression
// focused on service-level overview retry behavior.
func (r *dashboardFailureRepository) GetDashboardOverview(_ context.Context, filter *OpsDashboardFilter) (*OpsDashboardOverview, error) {
	r.calls++
	r.mode = filter.QueryMode
	return nil, r.err
}
