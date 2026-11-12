package creative_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/creative"
)

// frameCaptureStore delegates financial allocation to the billing planner while
// retaining the command selected by creative funding for assertions.
type frameCaptureStore struct {
	creative.FundingStore                           // unused reservation and release callbacks
	command               *billing.TaskFundsCommand // capture command received from creative
}

// billing_test.go exercises creative funding with saved independent prices and
// verified animation counts. The fixtures delegate allocation to the billing
// planner while recording capture commands, so static pricing, frame overages,
// and unused reservations can be inspected without external services.

// TestIndependentPricingCaptureUsesSnapshot keeps independently priced image
// work at its saved amount. Later group multipliers must not change either
// the retained cost or the wallet share of this capture.
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

// Capture computes the retained charge from the supplied creative command.
// Persistence is omitted so frame-count scenarios exercise allocation without
// touching a database or changing any user wallet.
func (store *frameCaptureStore) Capture(_ context.Context, command *billing.TaskFundsCommand) (*billing.TaskFundsResult, error) {
	store.command = command
	plan, err := billing.PlanTaskCapture(command)
	if err != nil {
		return nil, err
	}
	return &billing.TaskFundsResult{ActualAmountUSD: plan.ActualAmountUSD, BalanceAmountUSD: plan.BalanceAmountUSD}, nil
}

// TestCreativeGIFFundingUsesActualFrames compares requested and observed output.
// Extra frames authorize a wallet overage, fewer frames release unused funding,
// and an animation returned for a PNG request follows its actual frame count.
func TestCreativeGIFFundingUsesActualFrames(t *testing.T) {
	for _, scenario := range []struct {
		name      string  // case describing requested and actual output
		requested int     // frames reserved when the task was created
		actual    int     // verified frames retained in output metadata
		amount    float64 // expected retained wallet charge in USD
		overage   bool    // additional wallet debit must be authorized
	}{
		{name: "nine_requested_twelve_returned", requested: 9, actual: 12, amount: 3, overage: true},
		{name: "nine_requested_seven_returned", requested: 9, actual: 7, amount: 1.75},
		{name: "png_requested_gif_returned", requested: 0, actual: 2, amount: 0.5, overage: true},
		{name: "static_output_preserved", requested: 0, actual: 0, amount: 0.25},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			reserved := float64(max(1, scenario.requested)) * 0.25
			run := &creative.CreativeRun{
				RunID: scenario.name, APIKeyID: 1, UserID: 1, GroupID: 1,
				BaseUnitPrice: 0.25, RequestedOutputCount: 1, EstimatedCost: reserved, BalanceHoldAmount: reserved,
				SubscriptionRateMultiplier: 1, BalanceRateMultiplier: 1, PlanGroupRateEnabled: false,
				FrameCount: scenario.requested, ActualFrameCount: scenario.actual,
			}
			store := &frameCaptureStore{}
			result, err := (creative.Funding{Store: store}).Capture(context.Background(), run, 1)
			require.NoError(t, err)
			require.InDelta(t, scenario.amount, result.ActualAmountUSD, 1e-12)
			require.InDelta(t, reserved, store.command.BaseAmountUSD, 1e-12)
			require.Equal(t, scenario.overage, store.command.OveragePolicy == billing.TaskOverageChargeBalance)
			require.NotNil(t, run.ActualCost)
			require.InDelta(t, scenario.amount, *run.ActualCost, 1e-12)
		})
	}
}
