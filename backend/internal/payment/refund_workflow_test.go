package payment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type identityRefundRights struct {
	RefundRights
	actual      float64
	requested   float64
	compensated float64
}

type identityConfirmedRefundStore struct {
	RefundStore
	rights *identityRefundRights
}

func (store *identityConfirmedRefundStore) CompleteRefund(ctx context.Context, plan *RefundPlan, _ *RefundReceipt, apply RefundMutation) (*RefundResult, error) {
	if err := apply(ctx, store.rights, plan); err != nil {
		return nil, err
	}
	return &RefundResult{Success: true, BalanceDeducted: plan.BalanceToDeduct}, nil
}

func (rights *identityRefundRights) DeductBalance(_ context.Context, _ int64, amount float64) (float64, error) {
	rights.requested = amount
	return rights.actual, nil
}

func (rights *identityRefundRights) CompensateBalance(_ context.Context, _ int64, amount float64) error {
	rights.compensated = amount
	return nil
}

func TestIdentityRefundDeductionAndCompensation(t *testing.T) {
	plan := &RefundPlan{Order: &Order{UserID: 1}, DeductionType: DeductionTypeBalance, BalanceToDeduct: 110}
	rights := &identityRefundRights{actual: 110}
	require.NoError(t, ApplyRefundDeduction(context.Background(), rights, plan))
	require.Equal(t, 110.0, rights.requested)
	require.NoError(t, CompensateRefundDeduction(context.Background(), rights, plan))
	require.Equal(t, 110.0, rights.compensated)
	rights.actual = 105
	require.Error(t, ApplyRefundDeduction(context.Background(), rights, plan))
	plan.Force = true
	require.NoError(t, ApplyRefundDeduction(context.Background(), rights, plan))
	require.NoError(t, CompensateRefundDeduction(context.Background(), rights, plan))
	require.Equal(t, 105.0, rights.compensated)
	receipt := &RefundReceipt{RefundPendingDetail: RefundPendingDetail{DeductionType: DeductionTypeBalance, BalanceDeducted: 105}}
	require.Equal(t, 105.0, receipt.Plan(plan.Order).BalanceToDeduct)
}

func TestIdentityConfirmedRefundUsesAvailableBalance(t *testing.T) {
	for _, available := range []float64{110, 105, 0} {
		plan := &RefundPlan{Order: &Order{UserID: 1}, DeductionType: DeductionTypeBalance, BalanceToDeduct: 110}
		rights := &identityRefundRights{actual: available}
		workflow := &RefundWorkflow{store: &identityConfirmedRefundStore{rights: rights}}
		result, err := workflow.FinalizePendingRefundSuccess(context.Background(), plan)
		require.NoError(t, err)
		require.True(t, result.Success)
		require.Equal(t, 110.0, rights.requested)
		require.Equal(t, available, result.BalanceDeducted)
		require.False(t, plan.Force)
	}
}
