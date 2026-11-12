package creative

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

// prompt_test.go checks prompt charging and recovery through the public service.
// In-memory adapters simulate output retention, concurrent leases, and an uncertain
// financial commit. Assertions cover usable output, strict fixed prices, failures,
// workspace isolation, and replay behavior without connecting to upstream providers.
//
// Financial and transient state remain separate so the tests can remove cached
// text while retaining its charge or report an error after a committed debit.
// The blocked-generation case coordinates two service requests through channels
// and checks that the second request cannot acquire the first request identity.
// The fixture mutex serializes shared counters and receipt maps while those
// requests run, and tests mutate failure controls before starting concurrent work.

// promptFixture stores test outputs and an atomic financial receipt for each request.
type promptFixture struct {
	// The mutex protects records, locks, charged, calls, and charges during
	// concurrent service execution. Error controls and service callbacks are
	// configured before requests start and remain stable while they run.
	mu              sync.Mutex                        // shared fixture state mutex
	records         map[string]*PromptExpansionRecord // transient generated output records
	locks           map[string]bool                   // currently acquired request identities
	charged         map[string]bool                   // durable financial receipt identities
	calls           int                               // upstream generation invocation count
	charges         int                               // newly committed wallet charge count
	saveError       error                             // injected output write failure
	generationError error                             // injected upstream execution failure
	settlementError error                             // injected financial failure
	ambiguous       bool                              // report failure after first charge
	service         *PromptExpander                   // public service under test
}

// newPromptFixture builds a configured service with deterministic text and funds.
// Tests mutate error controls before execution and own all fixture state for
// the duration of a test; concurrent execution uses the fixture's mutex.
func newPromptFixture() *promptFixture {
	f := &promptFixture{records: map[string]*PromptExpansionRecord{}, locks: map[string]bool{}, charged: map[string]bool{}}
	f.service = &PromptExpander{
		Store: f, Funds: f, Enabled: func(context.Context) bool { return true }, Config: func(context.Context) PromptConfig {
			return PromptConfig{GroupID: 2, Model: "text-model", USDExchangeRate: 7.5}
		}, MaxPromptChars: func() int { return 8000 },
		Admit: func(context.Context, int64, PromptConfig, float64) (int64, func(), error) { return 9, func() {}, nil },
		Generate: func(context.Context, PromptConfig, string) (PromptGeneration, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.calls++
			return PromptGeneration{Text: "expanded scene", ProviderID: 3, Model: "text-model"}, f.generationError
		},
		HasSettlement: func(_ context.Context, id string) (bool, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.charged[id], nil
		},
	}
	return f
}

// Acquire serializes fixture requests and returns cleanup for the acquired identity.
// A held identity returns acquired false and no cleanup callback. The fixture
// records ownership under its mutex before exposing a successful acquisition,
// so the blocked-generation test can send a second request while the first
// remains inside upstream execution. Successful callers release their identity
// through the returned callback after all cache and billing operations finish.
func (f *promptFixture) Acquire(_ context.Context, id string) (func(), bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locks[id] {
		return nil, false, nil
	}
	f.locks[id] = true
	return func() { f.mu.Lock(); defer f.mu.Unlock(); delete(f.locks, id) }, true, nil
}

// Load returns the fixture's temporary record for the requested identity.
// Missing records return nil with no error, and the service treats returned
// snapshots as immutable while the fixture retains their shared pointers.
func (f *promptFixture) Load(_ context.Context, id string) (*PromptExpansionRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.records[id], nil
}

// Save publishes the generated fixture output unless a failure is configured.
// The service supplies an immutable snapshot, and a configured write failure
// leaves the prior record unchanged so settlement cannot begin.
func (f *promptFixture) Save(_ context.Context, id string, r *PromptExpansionRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.saveError != nil {
		return f.saveError
	}
	f.records[id] = r
	return nil
}

// Settle models receipt deduplication and an optional uncertain first commit.
// A configured ambiguous response records the charge before returning an error,
// allowing the retry test to verify that cached output prevents regeneration.
// The mutex keeps receipt lookup and the debit counter update together, making
// one successful identity visible before a concurrent retry can inspect it.
// Ordinary configured failures return before either state changes; matching
// committed identities return Applied false while their cached text remains
// available to the caller for response recovery.
func (f *promptFixture) Settle(_ context.Context, c *billing.UsageBillingCommand) (*billing.UsageBillingApplyResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.settlementError != nil {
		return nil, f.settlementError
	}
	if f.charged[c.RequestID] {
		return &billing.UsageBillingApplyResult{}, nil
	}
	f.charged[c.RequestID] = true
	f.charges++
	if f.ambiguous {
		f.ambiguous = false
		return nil, errors.New("commit acknowledgement lost")
	}
	return &billing.UsageBillingApplyResult{Applied: true, BalanceAmountUSD: c.BillableAmountUSD}, nil
}

// TestPromptExpansionChargesOnceAndRecoversCommit retries an uncertain committed
// charge. The saved result returns with one upstream call and one fixed wallet
// debit, even though the first response reported a failed commit acknowledgement.
func TestPromptExpansionChargesOnceAndRecoversCommit(t *testing.T) {
	f := newPromptFixture()
	f.ambiguous = true
	scope := CreativeRunScope{UserID: 1, WorkspaceID: "workspace-a"}
	input := PromptExpansionRequest{Prompt: "scene", MediaType: "image"}
	_, err := f.service.Expand(t.Context(), scope, input, "same-key")
	require.Error(t, err)
	got, err := f.service.Expand(t.Context(), scope, input, "same-key")
	require.NoError(t, err)
	require.Equal(t, "expanded scene", got.Prompt)
	require.Equal(t, 0.015, got.CostCNY)
	require.Equal(t, 1, f.calls)
	require.Equal(t, 1, f.charges)
	for _, record := range f.records {
		require.Equal(t, 0.002, record.Command.BillableAmountUSD)
		require.True(t, record.Command.RequireAvailableBalance)
		require.Equal(t, billing.APIKeyBillingModeBalance, record.Command.APIKeyBillingMode)
	}
	input.Prompt = "different scene"
	_, err = f.service.Expand(t.Context(), scope, input, "same-key")
	require.ErrorIs(t, err, ErrCreativeRunIdempotencyConflict)
}

// TestPromptExpansionFailuresDoNotCharge supplies upstream, storage and balance
// failures. Every rejected expansion leaves the fixture's committed debit count
// zero, including empty output and a result that exceeds the configured prompt limit.
func TestPromptExpansionFailuresDoNotCharge(t *testing.T) {
	for _, kind := range []string{"upstream", "storage", "balance", "empty", "oversized", "moderation"} {
		t.Run(kind, func(t *testing.T) {
			f := newPromptFixture()
			switch kind {
			case "upstream":
				f.generationError = errors.New("upstream failed")
			case "storage":
				f.saveError = errors.New("redis failed")
			case "balance":
				f.settlementError = billing.ErrTaskInsufficientBalance
			case "empty":
				f.service.Generate = func(context.Context, PromptConfig, string) (PromptGeneration, error) { return PromptGeneration{}, nil }
			case "oversized":
				f.service.MaxPromptChars = func() int { return 5 }
			case "moderation":
				f.service.Moderate = func(context.Context, int64, PromptConfig, string) error { return ErrCreativeContentBlocked }
			}
			_, err := f.service.Expand(t.Context(), CreativeRunScope{UserID: 1, WorkspaceID: "a"}, PromptExpansionRequest{Prompt: "scene", MediaType: "image"}, "key")
			require.Error(t, err)
			require.Zero(t, f.charges)
		})
	}
}

// TestPromptExpansionIsolatesScopesAndRejectsExpiredReceipt reuses a client key
// across users and workspaces. Each scope charges once, and removing its cached
// text leaves the durable receipt blocking another generation under that identity.
func TestPromptExpansionIsolatesScopesAndRejectsExpiredReceipt(t *testing.T) {
	f := newPromptFixture()
	input := PromptExpansionRequest{Prompt: "scene", MediaType: "video"}
	for _, scope := range []CreativeRunScope{{UserID: 1, WorkspaceID: "a"}, {UserID: 1, WorkspaceID: "b"}, {UserID: 2, WorkspaceID: "a"}} {
		_, err := f.service.Expand(t.Context(), scope, input, "key")
		require.NoError(t, err)
	}
	require.Equal(t, 3, f.charges)
	f.records = map[string]*PromptExpansionRecord{}
	_, err := f.service.Expand(t.Context(), CreativeRunScope{UserID: 1, WorkspaceID: "a"}, input, "key")
	require.ErrorIs(t, err, ErrPromptExpired)
	require.Equal(t, 3, f.calls)
}

// TestPromptExpansionConcurrentReplayRejectsDuplicate holds upstream work open
// while an identical request arrives. The duplicate receives the busy error
// before any second generation or wallet debit can occur for that request identity.
func TestPromptExpansionConcurrentReplayRejectsDuplicate(t *testing.T) {
	f := newPromptFixture()
	started := make(chan struct{})
	finish := make(chan struct{})
	completed := make(chan error, 1)
	f.service.Generate = func(context.Context, PromptConfig, string) (PromptGeneration, error) {
		close(started)
		<-finish
		return PromptGeneration{Text: "expanded"}, nil
	}
	scope := CreativeRunScope{UserID: 1, WorkspaceID: "a"}
	input := PromptExpansionRequest{Prompt: "scene", MediaType: "image"}
	// The started signal confirms lease ownership before the second request.
	// The buffered completion channel lets the first request finish after release.
	go func() { _, err := f.service.Expand(t.Context(), scope, input, "key"); completed <- err }()
	<-started
	_, err := f.service.Expand(t.Context(), scope, input, "key")
	require.ErrorIs(t, err, ErrPromptBusy)
	close(finish)
	require.NoError(t, <-completed)
	require.Equal(t, 1, f.charges)
}

// TestPromptExpansionRejectsInvalidExchangeRate supplies unusable currency rates.
// Every rejected conversion stops before upstream execution or any committed
// wallet debit, covering nonfinite observations and a rate that rounds the charge to zero.
func TestPromptExpansionRejectsInvalidExchangeRate(t *testing.T) {
	for _, rate := range []float64{0, -1, math.Inf(1), math.NaN(), 1e100} {
		f := newPromptFixture()
		f.service.Config = func(context.Context) PromptConfig {
			return PromptConfig{GroupID: 2, Model: "text", USDExchangeRate: rate}
		}
		_, err := f.service.Expand(t.Context(), CreativeRunScope{UserID: 1, WorkspaceID: "a"}, PromptExpansionRequest{Prompt: "scene", MediaType: "image"}, "key")
		require.ErrorIs(t, err, ErrPromptUnavailable)
		require.Zero(t, f.calls)
		require.Zero(t, f.charges)
	}
}
