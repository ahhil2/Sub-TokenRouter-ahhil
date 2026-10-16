package postgres

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

func TestEffectiveIdentityRateRead(t *testing.T) {
	database, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer database.Close()
	store := NewUserGroupRateRepository(database)
	mock.ExpectQuery("SELECT DISTINCT ON").WithArgs(int64(1), int64(2)).WillReturnRows(sqlmock.NewRows([]string{"group_id", "rate_multiplier"}).AddRow(2, 0))
	rate, err := store.EffectiveGroupRate(context.Background(), 1, 2)
	require.NoError(t, err)
	require.NotNil(t, rate)
	require.Zero(t, *rate)
	mock.ExpectQuery("SELECT DISTINCT ON").WithArgs(int64(1), int64(3)).WillReturnRows(sqlmock.NewRows([]string{"group_id", "rate_multiplier"}))
	rate, err = store.EffectiveGroupRate(context.Background(), 1, 3)
	require.NoError(t, err)
	require.Nil(t, rate)
	failure := errors.New("database unavailable")
	mock.ExpectQuery("SELECT DISTINCT ON").WillReturnError(failure)
	_, err = store.EffectiveGroupRate(context.Background(), 1, 4)
	require.ErrorIs(t, err, failure)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestIdentityRateValidation(t *testing.T) {
	store := &GroupRateStore{}
	for _, invalid := range []float64{-1, math.NaN(), math.Inf(1), 1e12} {
		require.Error(t, store.UpdateIdentityRates(context.Background(), 1, []billing.IdentityRateInput{{GroupID: 1, RateMultiplier: &invalid}}))
	}
	require.Error(t, store.UpdateIdentityRates(context.Background(), 1, []billing.IdentityRateInput{{GroupID: 0}}))
	require.Error(t, store.UpdateIdentityRates(context.Background(), 1, []billing.IdentityRateInput{{GroupID: 1}, {GroupID: 1}}))
}
