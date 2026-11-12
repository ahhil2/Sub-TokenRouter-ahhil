//go:build integration

package billing_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/billing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
)

// prompt_settlement_scenario_integration_test.go checks concurrent fixed-price
// wallet charges on PostgreSQL. Multiple request identities compete for enough
// money to fund one expansion, and the receipt table is inspected with the
// final balance to verify that rejected operations leave no financial claim.

// TestPromptStrictSettlementConcurrentBalance sends competing paid prompt
// settlements to separate PostgreSQL transactions. Exactly one charge commits and replay cannot deduct the wallet again.
func TestPromptStrictSettlementConcurrentBalance(t *testing.T) {
	client := committedEntitlementClient(t)
	user := mustCreateUser(t, client, &identity.User{Email: "prompt-" + uuid.NewString() + "@example.com", Balance: 0.002})
	store := postgres.NewSettlementStore(integrationDB, timezone.NewCalendar(time.UTC), nil)
	prefix := "prompt-concurrent-" + uuid.NewString()
	start := make(chan struct{})
	outcomes := make(chan error, 8)
	// Each worker has a distinct financial identity and waits on the same start
	// signal, making the shared wallet row the serialization point under test.
	for i := range 8 {
		go func(i int) {
			<-start
			_, err := store.Apply(t.Context(), &billing.UsageBillingCommand{RequestID: fmt.Sprintf("%s-%d", prefix, i), UserID: user.ID, APIKeyID: user.ID, BillableAmountUSD: 0.002, APIKeyBillingMode: billing.APIKeyBillingModeBalance, RequireAvailableBalance: true})
			outcomes <- err
		}(i)
	}
	// Release all callers together so transaction locks determine which single
	// fixed charge can consume the user's available wallet amount before delivery.
	close(start)
	successes := 0
	for range 8 {
		err := <-outcomes
		if err == nil {
			successes++
		} else {
			require.True(t, errors.Is(err, billing.ErrTaskInsufficientBalance), "%v", err)
		}
	}
	require.Equal(t, 1, successes)
	var balance float64
	require.NoError(t, integrationDB.QueryRowContext(t.Context(), "SELECT balance FROM users WHERE id=$1", user.ID).Scan(&balance))
	require.Zero(t, balance)
	var count int
	var requestID string
	require.NoError(t, integrationDB.QueryRowContext(t.Context(), "SELECT count(*),min(request_id) FROM usage_billing_dedup WHERE request_id LIKE $1", prefix+"%").Scan(&count, &requestID))
	require.Equal(t, 1, count)
	replay, err := store.Apply(t.Context(), &billing.UsageBillingCommand{RequestID: requestID, UserID: user.ID, APIKeyID: user.ID, BillableAmountUSD: 0.002, APIKeyBillingMode: billing.APIKeyBillingModeBalance, RequireAvailableBalance: true})
	require.NoError(t, err)
	require.False(t, replay.Applied)
}
