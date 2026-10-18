package creative_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/creative"
)

func TestIndependentPricingCaptureUsesSnapshot(t *testing.T) {
	run := &creative.CreativeRun{
		RunID: "independent-run", APIKeyID: 1, UserID: 1, GroupID: 1,
		BaseUnitPrice: 0.065, RequestedOutputCount: 2, EstimatedCost: 0.13, BalanceHoldAmount: 0.13,
		SubscriptionRateMultiplier: 1, BalanceRateMultiplier: 1, PlanGroupRateEnabled: false,
	}
	command, err := creative.BuildHoldCommand(run, creative.CreativeCaptureRequestID(run.RunID), 0.065)
	require.NoError(t, err)
	require.True(t, command.DisablePlanGroupRateMultiplier)
	require.Equal(t, 1.0, command.SubscriptionRateMultiplier)
	require.Equal(t, 1.0, command.BalanceRateMultiplier)
	plan, err := billing.PlanTaskCapture(command)
	require.NoError(t, err)
	require.InDelta(t, 0.065, plan.ActualAmountUSD, 1e-12)
	require.InDelta(t, 0.065, plan.BalanceAmountUSD, 1e-12)
}
