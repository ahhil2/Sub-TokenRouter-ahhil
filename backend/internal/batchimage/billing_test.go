package batchimage_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/batchimage"
)

func TestIndependentPricingFundsSnapshot(t *testing.T) {
	job := testSettlingBatchImageJob("independent-funds")
	job.PricingSnapshotVersion = batchimage.IndependentPricingSnapshotVersion
	job.BaseUnitPrice = 0.065
	job.ItemCount = 2
	job.SubscriptionRateMultiplier = 1
	job.BalanceRateMultiplier = 1
	job.PlanGroupRateEnabled = false
	job.ProviderRateMultiplier = 3
	job.HoldMultiplier = 1
	job.BatchDiscountMultiplier = 1
	for _, actual := range []float64{0, 0.065, 0.13} {
		command, err := batchimage.BuildHoldCommand(job, "independent-funds", actual, "payload")
		require.NoError(t, err)
		require.Equal(t, 1.0, command.SubscriptionRateMultiplier)
		require.Equal(t, 1.0, command.SubscriptionRateMultiplierScale)
		require.Equal(t, 1.0, command.BalanceRateMultiplier)
		require.Equal(t, 1.0, command.SettlementRateScale)
		require.True(t, command.DisablePlanGroupRateMultiplier)
		require.InDelta(t, 0.13, command.BaseAmountUSD, 1e-12)
		require.Equal(t, actual, command.ActualBaseAmountUSD)
	}
}
