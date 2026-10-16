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
