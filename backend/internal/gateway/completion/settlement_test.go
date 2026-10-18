package completion

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

func TestApplyRateMultipliersPreservesZeroIdentityRate(t *testing.T) {
	input := &SettlementInput{
		Cost:                       &CostBreakdown{TotalCost: 100, ActualCost: 50},
		SubscriptionRateMultiplier: 0.5,
		BalanceRateMultiplier:      0,
		RateMultipliersResolved:    true,
	}
	command := &billing.UsageBillingCommand{}
	ApplyRateMultipliers(command, input)
	require.Equal(t, 0.5, command.SubscriptionRateMultiplier)
	require.Zero(t, command.BalanceRateMultiplier)
	input.RateMultipliersResolved = false
	ApplyRateMultipliers(command, input)
	require.Equal(t, 0.5, command.BalanceRateMultiplier)
}

func TestApplyRateMultipliersIndependentPricing(t *testing.T) {
	for _, amount := range []float64{0, 0.065} {
		input := &SettlementInput{
			Cost:                       &CostBreakdown{TotalCost: amount, ActualCost: amount, IndependentPricing: true},
			SubscriptionRateMultiplier: 0.18, BalanceRateMultiplier: 0,
			SubscriptionRateMultiplierScale: 2, RateMultipliersResolved: true,
		}
		command := &billing.UsageBillingCommand{}
		ApplyRateMultipliers(command, input)
		require.Equal(t, 1.0, command.SubscriptionRateMultiplier)
		require.Equal(t, 1.0, command.BalanceRateMultiplier)
		require.Equal(t, 1.0, command.SubscriptionRateMultiplierScale)
		require.True(t, command.DisablePlanGroupRateMultiplier)
	}
}
