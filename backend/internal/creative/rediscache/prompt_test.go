package rediscache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/creative"
)

// prompt_test.go verifies prompt lease ownership and temporary result expiry.
// Miniredis advances time independently of request execution so stale cleanup
// can be exercised against a replacement lease and paid text can be confirmed
// unavailable once its documented retention period has elapsed.

// TestPromptLeaseAndResultExpiry replaces an expired lease before its old owner
// releases it. Ownership fencing protects the new lease and results expire after thirty minutes.
func TestPromptLeaseAndResultExpiry(t *testing.T) {
	server, client := newCreativeTestRedis(t)
	store := NewPromptStore(client)
	release, acquired, err := store.Acquire(t.Context(), "request")
	require.NoError(t, err)
	require.True(t, acquired)
	_, acquired, err = store.Acquire(t.Context(), "request")
	require.NoError(t, err)
	require.False(t, acquired)
	server.FastForward(2 * time.Minute)
	next, acquired, err := store.Acquire(t.Context(), "request")
	require.NoError(t, err)
	require.True(t, acquired)
	release()
	_, acquired, err = store.Acquire(t.Context(), "request")
	require.NoError(t, err)
	require.False(t, acquired)
	next()
	record := &creative.PromptExpansionRecord{Fingerprint: "hash", Response: creative.PromptExpansionResponse{Prompt: "private output", CostCNY: 0.015}}
	require.NoError(t, store.Save(t.Context(), "request", record))
	got, err := store.Load(t.Context(), "request")
	require.NoError(t, err)
	require.Equal(t, record, got)
	server.FastForward(30 * time.Minute)
	got, err = store.Load(t.Context(), "request")
	require.NoError(t, err)
	require.Nil(t, got)
}
