package billing

import (
	"context"
	"errors"
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

func TestRedeemBonusPerUseAndDuplicate(t *testing.T) {
	store := &bonusRedeemStore{code: RedeemCode{ID: 1, Code: "test-code", Type: RedeemTypeBalance, Value: 100, MaxUses: 3, Status: StatusUnused}, usages: make(map[int64]*RedeemCodeUsage)}
	balance := &bonusRedeemBalance{}
	affiliate := &bonusRedeemAffiliate{}
	code := "bonus_5"
	service := NewRedeemService(store, balance, nil, nil, nil, balance, nil, affiliate, RedeemRuntime{Now: time.Now, RechargeIdentity: func(context.Context, int64) (string, error) { return code, nil }})
	result, err := service.Redeem(context.Background(), 1, "test-code")
	require.NoError(t, err)
	require.Equal(t, 100.0, result.Value)
	require.Equal(t, 105.0, result.RechargeSnapshot.CreditedAmount)
	code = "bonus_10"
	result, err = service.Redeem(context.Background(), 2, "test-code")
	require.NoError(t, err)
	require.Equal(t, 110.0, result.RechargeSnapshot.CreditedAmount)
	require.Equal(t, 105.0, store.usages[1].RechargeSnapshot.CreditedAmount)
	require.Equal(t, 110.0, store.usages[2].RechargeSnapshot.CreditedAmount)
	require.Equal(t, 215.0, balance.amount)
	require.Equal(t, []float64{100, 100}, affiliate.bases)
	_, err = service.Redeem(context.Background(), 1, "test-code")
	require.ErrorIs(t, err, ErrRedeemCodeAlreadyUsed)
	require.Equal(t, 215.0, balance.amount)
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
	service := &RedeemService{runtime: RedeemRuntime{RechargeIdentity: func(context.Context, int64) (string, error) {
		t.Fatal("payment must not query current identity")
		return "", nil
	}}}
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

func TestRechargeSnapshotIdentityFailureStopsCredit(t *testing.T) {
	failure := errors.New("identity unavailable")
	service := &RedeemService{runtime: RedeemRuntime{RechargeIdentity: func(context.Context, int64) (string, error) { return "", failure }}}
	_, err := service.rechargeSnapshot(context.Background(), 1, &RedeemCode{Type: RedeemTypeBalance, Value: 100})
	require.ErrorIs(t, err, failure)
	snapshot, err := service.rechargeSnapshot(context.Background(), 1, &RedeemCode{Type: RedeemTypeBalance, Value: -1})
	require.NoError(t, err)
	require.Equal(t, -1.0, snapshot.CreditedAmount)
}
