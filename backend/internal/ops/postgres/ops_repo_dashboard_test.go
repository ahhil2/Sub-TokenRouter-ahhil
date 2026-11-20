package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

// ops_repo_dashboard_test.go protects dashboard fallback ownership, optional
// measurement degradation, cancellation, and aggregate weighting. SQL mocks
// distinguish attempted queries while context fixtures cover driver error
// classification without waiting for a wall-clock deadline.

// TestIsQueryTimeoutErr distinguishes local deadlines from parent cancellation
// and unrelated SQL failures. PostgreSQL cancellation permits degradation only
// when the optional query's own deadline has expired.
func TestIsQueryTimeoutErr(t *testing.T) {
	active := t.Context()
	expired, cancelExpired := context.WithDeadline(active, time.Unix(1, 0))
	defer cancelExpired()
	canceled, cancel := context.WithCancel(active)
	cancel()
	databaseCancel := &pq.Error{Code: "57014", Message: "canceling statement due to user request"}
	for _, tc := range []struct {
		name   string          // error and context combination
		parent context.Context // owning request lifetime
		query  context.Context // optional query lifetime
		err    error           // observed driver failure
		want   bool            // whether partial results remain usable
	}{
		{"nil", active, active, nil, false},
		{"deadline", active, active, context.DeadlineExceeded, true},
		{"wrapped_deadline", active, active, fmt.Errorf("wrapped: %w", context.DeadlineExceeded), true},
		{"pq_local_deadline", active, expired, databaseCancel, true},
		{"pq_wrapped_local_deadline", active, expired, fmt.Errorf("wrapped: %w", databaseCancel), true},
		{"pq_external_cancel", active, active, databaseCancel, false},
		{"pq_child_cancel", active, canceled, databaseCancel, false},
		{"parent_canceled", canceled, expired, databaseCancel, false},
		{"parent_expired", expired, expired, context.DeadlineExceeded, false},
		{"pq_parent_expired", expired, expired, databaseCancel, false},
		{"canceled", active, canceled, context.Canceled, false},
		{"wrapped_canceled", active, canceled, fmt.Errorf("wrapped: %w", context.Canceled), false},
		{"other_sql_error", active, expired, &pq.Error{Code: "42703"}, false},
		{"other_error", active, expired, errors.New("connection failed"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isQueryTimeoutErr(tc.parent, tc.query, tc.err))
		})
	}
}

// TestDashboardOverviewAutoAttemptsRawOnce supplies a range without stable full
// hours and a failing raw read. The original failure must survive without a
// second attempt replacing it.
func TestDashboardOverviewAutoAttemptsRawOnce(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2024, 1, 2, 10, 10, 0, 0, time.UTC)
	queryErr := errors.New("raw query failed")
	mock.ExpectQuery("SELECT .* AS success_count").WithArgs(start, start.Add(10*time.Minute)).WillReturnError(queryErr)

	overview, err := NewOpsRepository(db).GetDashboardOverview(t.Context(), &ops.OpsDashboardFilter{
		StartTime: start, EndTime: start.Add(10 * time.Minute), QueryMode: ops.OpsQueryModeAuto,
	})

	require.ErrorIs(t, err, queryErr)
	require.Nil(t, overview)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDashboardOverviewAutoHonorsParentCancellation prevents fallback when the
// owning request has ended. No raw database work is expected for an already
// canceled short-range request.
func TestDashboardOverviewAutoHonorsParentCancellation(t *testing.T) {
	db, mock := newSQLMock(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	start := time.Date(2024, 1, 2, 10, 10, 0, 0, time.UTC)
	overview, err := NewOpsRepository(db).GetDashboardOverview(ctx, &ops.OpsDashboardFilter{
		StartTime: start, EndTime: start.Add(10 * time.Minute), QueryMode: ops.OpsQueryModeAuto,
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, overview)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestDashboardOverviewOptionalTimeoutKeepsCounts supplies one failed optional
// metric per request. Counts remain usable, missing latency stays absent, and
// successfully observed zero rates never become estimates.
func TestDashboardOverviewOptionalTimeoutKeepsCounts(t *testing.T) {
	for _, mode := range []ops.OpsQueryMode{ops.OpsQueryModeRaw, ops.OpsQueryModeAuto} {
		for _, latencyTimeout := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/latency_timeout_%t", mode, latencyTimeout), func(t *testing.T) {
				db, mock := newSQLMock(t)
				start := time.Date(2024, 1, 2, 10, 10, 0, 0, time.UTC)
				end := start.Add(10 * time.Minute)
				mock.ExpectQuery("SELECT .* AS success_count").WithArgs(start, end).
					WillReturnRows(sqlmock.NewRows([]string{"count", "tokens"}).AddRow(600, 6000))
				latency := mock.ExpectQuery("WITH samples AS MATERIALIZED").WithArgs(start, end)
				if latencyTimeout {
					latency.WillReturnError(context.DeadlineExceeded)
				} else {
					latency.WillReturnRows(sqlmock.NewRows([]string{
						"d50", "d90", "d95", "d99", "davg", "dmax",
						"t50", "t90", "t95", "t99", "tavg", "tmax", "samples",
					}).AddRow(100, 100, 100, 100, 100, 100, 0, 0, 0, 0, 0, 0, 600))
				}
				mock.ExpectQuery("SELECT .*FROM ops_error_logs").WillReturnRows(dashboardEmptyErrorCounts())
				mock.ExpectQuery("SELECT .* AS success_count").WithArgs(end.Add(-time.Minute), end).
					WillReturnRows(sqlmock.NewRows([]string{"count", "tokens"}).AddRow(0, 0))
				mock.ExpectQuery("SELECT .*FROM ops_error_logs").WillReturnRows(dashboardEmptyErrorCounts())
				peak := mock.ExpectQuery("WITH usage_buckets AS").WithArgs(start, end, start, end)
				if latencyTimeout {
					peak.WillReturnRows(sqlmock.NewRows([]string{"requests", "tokens"}).AddRow(0, 0))
				} else {
					peak.WillReturnError(context.DeadlineExceeded)
				}

				overview, err := NewOpsRepository(db).GetDashboardOverview(t.Context(), &ops.OpsDashboardFilter{
					StartTime: start, EndTime: end, QueryMode: mode,
				})
				require.NoError(t, err)
				require.EqualValues(t, 600, overview.RequestCountTotal)
				require.EqualValues(t, 6000, overview.TokenConsumed)
				require.Equal(t, float64(1), overview.QPS.Avg)
				require.Equal(t, float64(10), overview.TPS.Avg)
				require.Zero(t, overview.QPS.Current)
				require.Zero(t, overview.TPS.Current)
				if latencyTimeout {
					require.Equal(t, ops.OpsPercentiles{}, overview.Duration)
					require.Equal(t, ops.OpsPercentiles{}, overview.TTFT)
					require.Zero(t, overview.QPS.Peak)
					require.Zero(t, overview.TPS.Peak)
				} else {
					require.Equal(t, intValuePtr(100), overview.Duration.P50)
					require.Equal(t, intValuePtr(0), overview.TTFT.P50)
					require.Equal(t, overview.QPS.Avg, overview.QPS.Peak)
					require.Equal(t, overview.TPS.Avg, overview.TPS.Peak)
				}
				require.NoError(t, mock.ExpectationsWereMet())
			})
		}
	}
}

// dashboardEmptyErrorCounts supplies all six zero-valued error classification
// counters for independent count queries.
func dashboardEmptyErrorCounts() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"total", "limited", "sla", "upstream", "429", "529"}).AddRow(0, 0, 0, 0, 0, 0)
}

// TestDashboardRawFragmentTimeoutKeepsCounts protects automatic-mode tail
// aggregation after an optional latency deadline. Missing TTFT observations
// carry zero weight while usage and error totals remain available.
func TestDashboardRawFragmentTimeoutKeepsCounts(t *testing.T) {
	db, mock := newSQLMock(t)
	start := time.Date(2024, 1, 2, 10, 10, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)
	mock.ExpectQuery("SELECT .* AS success_count").WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count", "tokens"}).AddRow(600, 6000))
	mock.ExpectQuery("WITH samples AS MATERIALIZED").WithArgs(start, end).
		WillReturnError(context.DeadlineExceeded)
	mock.ExpectQuery("SELECT .*FROM ops_error_logs").WillReturnRows(dashboardEmptyErrorCounts())

	partial, err := NewOpsRepository(db).queryRawPartial(t.Context(), &ops.OpsDashboardFilter{}, start, end)

	require.NoError(t, err)
	require.EqualValues(t, 600, partial.successCount)
	require.EqualValues(t, 6000, partial.tokenConsumed)
	require.Zero(t, partial.ttftSampleCount)
	require.Equal(t, ops.OpsPercentiles{}, partial.duration)
	require.Equal(t, ops.OpsPercentiles{}, partial.ttft)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestAggregateHourlyRowsWeightsTTFTBySampleCount supplies hours with opposite
// success and first-token sample weights. It protects distinct rounded
// duration/first-token weighting and retention of the greatest first-token 95th
// percentile.
func TestAggregateHourlyRowsWeightsTTFTBySampleCount(t *testing.T) {
	rows := []opsHourlyMetricsRow{
		{
			bucketStart:     time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
			successCount:    100,
			ttftSampleCount: 1,
			durationP50:     sql.NullInt64{Int64: 100, Valid: true},
			durationP90:     sql.NullInt64{Int64: 200, Valid: true},
			durationAvg:     sql.NullFloat64{Float64: 150, Valid: true},
			durationP95:     sql.NullInt64{Int64: 220, Valid: true},
			durationP99:     sql.NullInt64{Int64: 250, Valid: true},
			durationMax:     sql.NullInt64{Int64: 300, Valid: true},
			ttftP50:         sql.NullInt64{Int64: 1000, Valid: true},
			ttftP90:         sql.NullInt64{Int64: 1200, Valid: true},
			ttftAvg:         sql.NullFloat64{Float64: 1100, Valid: true},
			ttftP95:         sql.NullInt64{Int64: 1300, Valid: true},
			ttftP99:         sql.NullInt64{Int64: 1400, Valid: true},
			ttftMax:         sql.NullInt64{Int64: 1500, Valid: true},
		},
		{
			bucketStart:     time.Date(2026, 6, 1, 1, 0, 0, 0, time.UTC),
			successCount:    1,
			ttftSampleCount: 99,
			durationP50:     sql.NullInt64{Int64: 1000, Valid: true},
			durationP90:     sql.NullInt64{Int64: 2000, Valid: true},
			durationAvg:     sql.NullFloat64{Float64: 1500, Valid: true},
			durationP95:     sql.NullInt64{Int64: 2200, Valid: true},
			durationP99:     sql.NullInt64{Int64: 2500, Valid: true},
			durationMax:     sql.NullInt64{Int64: 3000, Valid: true},
			ttftP50:         sql.NullInt64{Int64: 10, Valid: true},
			ttftP90:         sql.NullInt64{Int64: 20, Valid: true},
			ttftAvg:         sql.NullFloat64{Float64: 15, Valid: true},
			ttftP95:         sql.NullInt64{Int64: 25, Valid: true},
			ttftP99:         sql.NullInt64{Int64: 30, Valid: true},
			ttftMax:         sql.NullInt64{Int64: 35, Valid: true},
		},
	}

	got := aggregateHourlyRows(rows)

	require.Equal(t, int64(101), got.successCount)
	require.Equal(t, int64(100), got.ttftSampleCount)

	// Request duration is weighted by success_count.
	require.NotNil(t, got.duration.P50)
	require.Equal(t, 109, *got.duration.P50)

	// Use one versus ninety-nine measured first-token samples despite the opposite
	// success counts, so the fixture distinguishes latency weights from total
	// usage observations.
	require.NotNil(t, got.ttft.P50)
	require.Equal(t, 20, *got.ttft.P50)
	require.NotNil(t, got.ttft.P90)
	require.Equal(t, 32, *got.ttft.P90)
	require.NotNil(t, got.ttft.Avg)
	require.Equal(t, 26, *got.ttft.Avg)
	require.NotNil(t, got.ttft.P95)
	require.Equal(t, 1300, *got.ttft.P95)
}

// TestCombineApproxPercentilesUsesProvidedWeightsForTTFT combines two supplied
// percentile segments with weights one and ninety-nine. It protects rounded
// median, 90th percentile, and mean plus weight-independent 95th percentile
// maximum.
func TestCombineApproxPercentilesUsesProvidedWeightsForTTFT(t *testing.T) {
	got := combineApproxPercentiles([]opsPercentileSegment{
		{
			weight: 1,
			p: ops.OpsPercentiles{
				P50: intValuePtr(1000),
				P90: intValuePtr(1200),
				Avg: intValuePtr(1100),
				P95: intValuePtr(1300),
			},
		},
		{
			weight: 99,
			p: ops.OpsPercentiles{
				P50: intValuePtr(10),
				P90: intValuePtr(20),
				Avg: intValuePtr(15),
				P95: intValuePtr(25),
			},
		},
	})

	require.NotNil(t, got.P50)
	require.Equal(t, 20, *got.P50)
	require.NotNil(t, got.P90)
	require.Equal(t, 32, *got.P90)
	require.NotNil(t, got.Avg)
	require.Equal(t, 26, *got.Avg)
	require.NotNil(t, got.P95)
	require.Equal(t, 1300, *got.P95)
}

// intValuePtr allocates a scalar pointer for supplied fixture millisecond
// measurements.
func intValuePtr(v int) *int {
	return &v
}

// TestOpsHourlyMetricsCoverageRequiresEveryBucket supplies twenty-three then
// twenty-four global rows for a full-day range. It protects row-count coverage
// rejection and acceptance for the aligned-hour assumption used by aggregation.
func TestOpsHourlyMetricsCoverageRequiresEveryBucket(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &Store{db: db}
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	query := regexp.QuoteMeta(`
		SELECT COUNT(*)
		FROM ops_metrics_hourly
		WHERE bucket_start >= $1 AND bucket_start < $2
		  AND platform IS NULL AND group_id IS NULL
	`)

	mock.ExpectQuery(query).WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(23)))
	covered, err := repo.hasCompleteHourlyMetricsCoverage(context.Background(), start, end)
	require.NoError(t, err)
	require.False(t, covered)

	mock.ExpectQuery(query).WithArgs(start, end).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(24)))
	covered, err = repo.hasCompleteHourlyMetricsCoverage(context.Background(), start, end)
	require.NoError(t, err)
	require.True(t, covered)
	require.NoError(t, mock.ExpectationsWereMet())
}
