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

// TestIdentityRateDiscountTable checks the balance discount factors for each identity and
// the SQL fragments generated from them. The generated code list and CASE expression are
// pinned so the effective-rate query and the identity table cannot silently drift apart.
func TestIdentityRateDiscountTable(t *testing.T) {
	require.Equal(t, 0.95, IdentityRateDiscount("bonus_5"))
	require.Equal(t, 0.9, IdentityRateDiscount("bonus_10"))
	require.Equal(t, 1.0, IdentityRateDiscount("ahhilai"))
	require.Equal(t, 1.0, IdentityRateDiscount(""))
	codes, factor := IdentityRateDiscountSQL("m.identity_code")
	require.Equal(t, "'bonus_10','bonus_5'", codes)
	require.Equal(t, "CASE m.identity_code WHEN 'bonus_10' THEN 0.9 WHEN 'bonus_5' THEN 0.95 ELSE 1 END", factor)
}
