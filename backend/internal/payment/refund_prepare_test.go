package payment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

func TestRefundIdentityBonusDeduction(t *testing.T) {
	snapshot, err := billing.CalculateRecharge(100, "bonus_10")
	require.NoError(t, err)
	order := &Order{UserID: 1, OrderType: OrderTypeBalance, Amount: 100, RechargeSnapshot: snapshot}
	workflow := &RefundWorkflow{runtime: RefundRuntime{User: func(context.Context, int64) (*RefundUser, error) {
		return &RefundUser{Balance: 105}, nil
	}}}
	plan := &RefundPlan{Order: order, RefundAmount: 100}
	warning := workflow.PrepDeduct(context.Background(), order, plan, false)
	require.NotNil(t, warning)
	require.True(t, warning.RequireForce)
	plan.RefundAmount = 25
	warning = workflow.PrepDeduct(context.Background(), order, plan, false)
	require.Nil(t, warning)
	require.Equal(t, 27.5, plan.BalanceToDeduct)
	plan.RefundAmount = 100
	warning = workflow.PrepDeduct(context.Background(), order, plan, true)
	require.Nil(t, warning)
	require.Equal(t, 105.0, plan.BalanceToDeduct)
}
