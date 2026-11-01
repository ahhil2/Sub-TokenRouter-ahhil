package billing

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type bonusRedeemStore struct {
	RedeemCodeRepository
	code   RedeemCode
	usages map[int64]*RedeemCodeUsage
}

type bonusRedeemBalance struct{ amount float64 }

type bonusRedeemAffiliate struct{ bases []float64 }

func (affiliate *bonusRedeemAffiliate) IsEnabled(context.Context) bool { return true }

func (affiliate *bonusRedeemAffiliate) AccrueInviteRebate(_ context.Context, _ int64, amount float64) (float64, error) {
	affiliate.bases = append(affiliate.bases, amount)
	return 0, nil
}

func (store *bonusRedeemStore) GetByCodeForUpdate(context.Context, string) (*RedeemCode, error) {
	value := store.code
	return &value, nil
}

func (store *bonusRedeemStore) GetByID(context.Context, int64) (*RedeemCode, error) {
	value := store.code
	value.RechargeSnapshot = nil
	return &value, nil
}

func (store *bonusRedeemStore) GetUsageByRedeemCodeAndUser(_ context.Context, _, userID int64) (*RedeemCodeUsage, error) {
	return store.usages[userID], nil
}

func (store *bonusRedeemStore) CreateUsage(_ context.Context, usage *RedeemCodeUsage) error {
	store.usages[usage.UserID] = usage
	return nil
}

func (store *bonusRedeemStore) Update(_ context.Context, code *RedeemCode) error {
	store.code = *code
	return nil
}

func (balance *bonusRedeemBalance) GetByID(context.Context, int64) (*UserSummary, error) {
	return &UserSummary{ID: 1}, nil
}

func (balance *bonusRedeemBalance) Within(ctx context.Context, run func(context.Context) error) error {
	return run(ctx)
}

func (balance *bonusRedeemBalance) ApplyBalance(_ context.Context, _ int64, amount float64) error {
	balance.amount += amount
	return nil
}

func (balance *bonusRedeemBalance) ApplyConcurrency(context.Context, int64, int) error { return nil }

// TestRedeemCreditsCodeValueWithoutIdentityGift redeems one multi-use balance code for two
// users after identity recharge gifts were removed. Each user receives exactly the code
// value with a no-gift snapshot, affiliate accrual uses that value, and a repeat
// redemption by the same user is still rejected without changing the balance.
func TestRedeemCreditsCodeValueWithoutIdentityGift(t *testing.T) {
	store := &bonusRedeemStore{code: RedeemCode{ID: 1, Code: "test-code", Type: RedeemTypeBalance, Value: 100, MaxUses: 3, Status: StatusUnused}, usages: make(map[int64]*RedeemCodeUsage)}
	balance := &bonusRedeemBalance{}
	affiliate := &bonusRedeemAffiliate{}
	service := NewRedeemService(store, balance, nil, nil, nil, balance, nil, affiliate, RedeemRuntime{Now: time.Now})
	for _, userID := range []int64{1, 2} {
		result, err := service.Redeem(context.Background(), userID, "test-code")
		require.NoError(t, err)
		require.Equal(t, 100.0, result.Value)
		require.Equal(t, 100.0, result.RechargeSnapshot.CreditedAmount)
		require.Zero(t, result.RechargeSnapshot.BonusAmount)
		require.Empty(t, result.RechargeSnapshot.IdentityCode)
	}
	require.Equal(t, 200.0, balance.amount)
	require.Equal(t, []float64{100, 100}, affiliate.bases)
	_, err := service.Redeem(context.Background(), 1, "test-code")
	require.ErrorIs(t, err, ErrRedeemCodeAlreadyUsed)
	require.Equal(t, 200.0, balance.amount)
}

func TestCalculateRecharge(t *testing.T) {
	cases := []struct {
		name         string
		base         float64
		code         string
		bonus, total float64
	}{
		{"ordinary", 100, "", 0, 100},
		{"five", 100, "bonus_5", 5, 105},
		{"ten", 100, "bonus_10", 10, 110},
		{"fraction", 0.0000001, "bonus_5", 0.00000001, 0.00000011},
		{"negative", -100, "bonus_10", 0, -100},
		{"zero", 0, "bonus_10", 0, 0},
	}
	for _, scenario := range cases {
		t.Run(scenario.name, func(t *testing.T) {
			snapshot, err := CalculateRecharge(scenario.base, scenario.code)
			require.NoError(t, err)
			require.Equal(t, scenario.bonus, snapshot.BonusAmount)
			require.Equal(t, scenario.total, snapshot.CreditedAmount)
		})
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_, err := CalculateRecharge(value, "bonus_5")
		require.Error(t, err)
	}
	_, err := CalculateRecharge(100, "unknown")
	require.Error(t, err)
}

func TestRechargeSnapshotRefundCredit(t *testing.T) {
	snapshot, err := CalculateRecharge(100, "bonus_10")
	require.NoError(t, err)
	require.Equal(t, 110.0, snapshot.RefundCredit(100))
	require.Equal(t, 27.5, snapshot.RefundCredit(25))
	require.Equal(t, 25.0, (*RechargeSnapshot)(nil).RefundCredit(25))
}

func TestRechargeSnapshotPaymentIsFixed(t *testing.T) {
	service := &RedeemService{}
	original, err := CalculateRecharge(100, "bonus_5")
	require.NoError(t, err)
	code := &RedeemCode{Type: RedeemTypeBalance, Value: 100}
	snapshot, err := service.rechargeSnapshot(ContextPaymentRecharge(context.Background(), original), 1, code)
	require.NoError(t, err)
	require.Equal(t, 105.0, snapshot.CreditedAmount)
	require.NotSame(t, original, snapshot)
	snapshot, err = service.rechargeSnapshot(ContextPaymentRecharge(context.Background(), nil), 1, code)
	require.NoError(t, err)
	require.Equal(t, 100.0, snapshot.CreditedAmount)
	code.Value = 200
	_, err = service.rechargeSnapshot(ContextPaymentRecharge(context.Background(), original), 1, code)
	require.Error(t, err)
}

// TestRechargeSnapshotOrdinaryCodeHasNoGift selects terms for ordinary code redemptions,
// which carry no payment snapshot. Positive and negative code values are credited as
// supplied with no identity and no gift.
func TestRechargeSnapshotOrdinaryCodeHasNoGift(t *testing.T) {
	service := &RedeemService{}
	for _, value := range []float64{100, -1} {
		snapshot, err := service.rechargeSnapshot(context.Background(), 1, &RedeemCode{Type: RedeemTypeBalance, Value: value})
		require.NoError(t, err)
		require.Equal(t, value, snapshot.CreditedAmount)
		require.Zero(t, snapshot.BonusAmount)
		require.Empty(t, snapshot.IdentityCode)
	}
}
