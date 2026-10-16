package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type identityRateTestReader struct {
	UserGroupRateRepository
	rate *float64
	err  error
}

func (reader *identityRateTestReader) EffectiveGroupRate(context.Context, int64, int64) (*float64, error) {
	return reader.rate, reader.err
}

func (reader *identityRateTestReader) EffectiveGroupRates(context.Context, int64) (map[int64]float64, error) {
	if reader.err != nil {
		return nil, reader.err
	}
	if reader.rate == nil {
		return map[int64]float64{}, nil
	}
	return map[int64]float64{1: *reader.rate}, nil
}

func TestIdentityRateResolverReadsCurrentConfiguration(t *testing.T) {
	rate := 0.2
	reader := &identityRateTestReader{rate: &rate}
	resolver := NewGroupRateResolver(reader, nil, 0, nil, "test", nil)
	actual, err := resolver.ResolveChecked(context.Background(), 1, 1, 1)
	require.NoError(t, err)
	require.Equal(t, 0.2, actual)
	rate = 0
	actual, err = resolver.ResolveChecked(context.Background(), 1, 1, 1)
	require.NoError(t, err)
	require.Zero(t, actual)
	reader.rate = nil
	actual, err = resolver.ResolveChecked(context.Background(), 1, 1, 1)
	require.NoError(t, err)
	require.Equal(t, 1.0, actual)
	reader.err = errors.New("unavailable")
	_, err = resolver.ResolveChecked(context.Background(), 1, 1, 1)
	require.ErrorIs(t, err, reader.err)
}
