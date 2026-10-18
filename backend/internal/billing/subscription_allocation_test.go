package billing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIndependentPricingSubscriptionAllocation(t *testing.T) {
	now := time.Now()
	groupID := int64(1)
	limit := 0.02
	command := &UsageBillingCommand{
		BaseAmountUSD: 0.065, BillableAmountUSD: 0.065, GroupID: &groupID,
		SubscriptionRateMultiplier: 1, BalanceRateMultiplier: 1, DisablePlanGroupRateMultiplier: true,
	}
	plan, err := AllocateSubscriptions(command, []SettlementSubscription{{
		ID: 1, StartsAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour),
		DailyLimitUSD: &limit, PlanGroupRateMultipliers: map[int64]float64{groupID: 0.1},
	}}, now)
	require.NoError(t, err)
	require.InDelta(t, 0.02, plan.SubscriptionAmount, 1e-12)
	require.InDelta(t, 0.045, plan.Remaining*command.BalanceRateMultiplier, 1e-12)
	require.InDelta(t, 0.065, plan.SubscriptionAmount+plan.Remaining*command.BalanceRateMultiplier, 1e-12)
}
