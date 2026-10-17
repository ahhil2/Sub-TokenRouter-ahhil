package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/gateway/forward"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	provideradapter "github.com/TokenFlux/TokenRouter/internal/provider/provider"
)

type semanticHealthStore struct {
	providercore.HealthStore
	providerID int64
	until      time.Time
	reason     string
	fail       bool
}

func (store *semanticHealthStore) SetTempUnschedulable(_ context.Context, providerID int64, until time.Time, reason string) error {
	store.providerID, store.until, store.reason = providerID, until, reason
	if store.fail {
		return errors.New("storage unavailable")
	}
	return nil
}

func TestCheckAnthropicSemanticError(test *testing.T) {
	for _, persistFailure := range []bool{false, true} {
		store := &semanticHealthStore{fail: persistFailure}
		now := time.Now()
		blocked := false
		health := &provideradapter.UpstreamHealth{Core: providercore.NewHealthService(store, nil, providercore.HealthOptions{
			Now:   func() time.Time { return now },
			Block: func(_ *providercore.Record, _ time.Time, _ string) { blocked = true },
		})}
		target := &ExecutionProvider{Record: providercore.Record{ID: 7, Schedulable: true, Extra: map[string]any{
			providercore.SemanticErrorExtraKey: providercore.SemanticErrorPolicy{Enabled: true, Texts: []string{"Upstream unavailable"}, CooldownMinutes: 5},
		}}}
		response := &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": {"test-request"}}, Body: io.NopCloser(strings.NewReader(`{"type":"message","content":[{"type":"text","text":"Upstream unavailable"}]}`))}
		err := CheckAnthropicSemanticError(context.Background(), health, target, response)
		var failure *forward.UpstreamFailoverError
		require.ErrorAs(test, err, &failure)
		require.Equal(test, 503, failure.StatusCode)
		require.False(test, failure.RetryableOnSameProvider)
		require.Equal(test, forward.NextProviderRetry, failure.NextProviderAction)
		require.Contains(test, string(failure.ResponseBody), "upstream_semantic_error")
		require.Equal(test, int64(7), store.providerID)
		require.Equal(test, now.Add(5*time.Minute), store.until)
		require.Contains(test, store.reason, "upstream_semantic_error")
		require.True(test, blocked)
		require.True(test, target.Record.Schedulable)
		require.NoError(test, response.Body.Close())
	}
}

func TestCheckAnthropicSemanticErrorDisabled(test *testing.T) {
	body := `{"type":"message","content":[{"type":"text","text":"Upstream unavailable"}]}`
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
	require.NoError(test, CheckAnthropicSemanticError(context.Background(), nil, &ExecutionProvider{}, response))
	remaining, err := io.ReadAll(response.Body)
	require.NoError(test, err)
	require.Equal(test, body, string(remaining))
}
