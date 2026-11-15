package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	gatewayprovider "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/session"
	"github.com/TokenFlux/TokenRouter/internal/pkg/requestcontext"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// responseBindingCache records shared bindings while enforcing caller cancellation.
type responseBindingCache struct {
	session.GatewayCache                            // unused session cache operations
	values               map[int64]map[string]int64 // shared group and response bindings
	// beforeWrite observes or delays each storage call before cancellation is
	// checked, allowing tests to model a write that exhausts its cleanup budget.
	beforeWrite func(context.Context)
}

// SetSessionProviderID stores one shared identifier after the optional write
// hook completes. Cancelled contexts reject the write, matching the behavior
// that prevents a cancelled request from publishing its response bindings.
func (s *responseBindingCache) SetSessionProviderID(ctx context.Context, groupID int64, key string, value int64, _ time.Duration) error {
	if s.beforeWrite != nil {
		s.beforeWrite(ctx)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.values == nil {
		s.values = make(map[int64]map[string]int64)
	}
	if s.values[groupID] == nil {
		s.values[groupID] = make(map[string]int64)
	}
	s.values[groupID][key] = value
	return nil
}

// GetSessionProviderID reads a binding published by another fixture store.
// Cancelled callers receive their context error; an absent group or key returns
// zero so a fresh session store observes a cache miss.
func (s *responseBindingCache) GetSessionProviderID(ctx context.Context, groupID int64, key string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return s.values[groupID][key], nil
}

// TestOpenAIGatewayService_BindHTTPResponseProvider stores one response
// provider and HTTP owner in the fixture state store. It checks affinity,
// same-user access across keys, different-user denial and denial of an unknown
// response.
func TestOpenAIGatewayService_BindHTTPResponseProvider(t *testing.T) {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	groupID := int64(4201)
	c.Set("api_key", &apikey.APIKey{ID: 501, GroupID: &groupID})
	SetHTTPResponseOwner(c, 601, 501)

	svc := newResponsesFixture(responsesFixtureInputs{})
	provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{LoadLocation: time.LoadLocation, ID: 37001, Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey}}
	svc.Output.BindResponseProvider(context.Background(), c, provider, "resp_http_001")

	got, err := svc.Lineage.Store.GetResponseProvider(context.Background(), groupID, "resp_http_001")
	require.NoError(t, err)
	require.Equal(t, provider.Record.ID, got)

	owned, err := session.ValidateHTTPResponseOwner(context.Background(), func() session.HTTPResponseOwnerReader { return svc.Lineage.Store }, groupID, "resp_http_001", 601, 501)
	require.NoError(t, err)
	require.True(t, owned)

	owned, err = session.ValidateHTTPResponseOwner(context.Background(), func() session.HTTPResponseOwnerReader { return svc.Lineage.Store }, groupID, "resp_http_001", 601, 502)
	require.NoError(t, err)
	require.True(t, owned, "API keys owned by the same downstream user remain interoperable")

	owned, err = session.ValidateHTTPResponseOwner(context.Background(), func() session.HTTPResponseOwnerReader { return svc.Lineage.Store }, groupID, "resp_http_001", 602, 501)
	require.NoError(t, err)
	require.False(t, owned)

	owned, err = session.ValidateHTTPResponseOwner(context.Background(), func() session.HTTPResponseOwnerReader { return svc.Lineage.Store }, groupID, "resp_unknown", 601, 501)
	require.NoError(t, err)
	require.False(t, owned)
}

// TestBindResponseProvider_AfterClientCancellation publishes completed response
// metadata after cancellation or deadline expiry. A fresh store must recover
// both provider affinity and ownership from shared storage for continuation.
func TestBindResponseProvider_AfterClientCancellation(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancelled", true: "expired"}[expired], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			if expired {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			}
			cancel()
			requestCtx, abort := requestcontext.WithAbort(ctx)
			defer abort()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			groupID := int64(4201)
			c.Set("api_key", &apikey.APIKey{ID: 501, GroupID: &groupID})
			SetHTTPResponseOwner(c, 601, 501)
			var writtenContexts []context.Context
			cache := &responseBindingCache{beforeWrite: func(ctx context.Context) {
				require.NoError(t, ctx.Err())
				deadline, ok := ctx.Deadline()
				require.True(t, ok)
				require.Positive(t, time.Until(deadline))
				require.LessOrEqual(t, time.Until(deadline), session.StateStoreRedisTimeout)
				writtenContexts = append(writtenContexts, ctx)
			}}
			output := &OpenAIResponseOutput{
				Responses:   session.NewOpenAIWSStateStore(cache),
				ResponseTTL: func() time.Duration { return time.Hour },
			}
			provider := &gatewayprovider.ExecutionProvider{Record: providercore.Record{ID: 37001}}
			output.BindResponseProvider(requestCtx, c, provider, " resp_completed ")
			require.Len(t, writtenContexts, 3)
			for _, writeCtx := range writtenContexts {
				require.ErrorIs(t, writeCtx.Err(), context.Canceled)
			}

			// A fresh store has empty local maps, so these reads prove the shared
			// bindings survive beyond the instance that drained the cancelled request.
			reader := session.NewOpenAIWSStateStore(cache)
			providerID, err := reader.GetResponseProvider(context.Background(), groupID, "resp_completed")
			require.NoError(t, err)
			require.Equal(t, provider.Record.ID, providerID)
			userID, keyID, found, err := reader.GetHTTPResponseOwner(context.Background(), groupID, "resp_completed")
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, int64(601), userID)
			require.Equal(t, int64(501), keyID)
		})
	}
}

// TestBindResponseProvider_CleanupDeadline bounds cancellation-aware storage
// after client disconnection. A blocked provider write consumes one shared
// deadline, and the subsequent owner write cannot begin another full budget.
func TestBindResponseProvider_CleanupDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		SetHTTPResponseOwner(c, 601, 501)
		writes := 0
		cache := &responseBindingCache{beforeWrite: func(ctx context.Context) {
			writes++
			// Model storage that honors cancellation while waiting for admission;
			// virtual time reaches the cleanup deadline even though the client already ended.
			<-ctx.Done()
			require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
		}}
		output := &OpenAIResponseOutput{
			Responses:   session.NewOpenAIWSStateStore(cache),
			ResponseTTL: func() time.Duration { return time.Minute },
		}
		started := time.Now()
		output.BindResponseProvider(ctx, c, &gatewayprovider.ExecutionProvider{Record: providercore.Record{ID: 7}}, "resp_delayed")
		require.Equal(t, session.StateStoreRedisTimeout, time.Since(started))
		require.Equal(t, 2, writes)
		require.Empty(t, cache.values)
	})
}

// TestBindResponseProvider_ServerAbortStopsStorage interrupts metadata cleanup
// with the server-owned abort signal. Detachment must retain that signal so
// service shutdown can reject remaining writes even after client disconnection.
func TestBindResponseProvider_ServerAbortStopsStorage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, abort := requestcontext.WithAbort(context.Background())
		defer abort()
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		SetHTTPResponseOwner(c, 601, 501)
		cache := &responseBindingCache{beforeWrite: func(ctx context.Context) {
			// Publish server shutdown while a detached storage call is pending and
			// wait for context propagation before observing whether the write can proceed.
			abort()
			synctest.Wait()
			require.ErrorIs(t, ctx.Err(), context.Canceled)
		}}
		output := &OpenAIResponseOutput{
			Responses:   session.NewOpenAIWSStateStore(cache),
			ResponseTTL: func() time.Duration { return time.Minute },
		}
		output.BindResponseProvider(ctx, c, &gatewayprovider.ExecutionProvider{Record: providercore.Record{ID: 7}}, "resp_aborted")
		require.Empty(t, cache.values)
	})
}

// TestProtocolError_AfterResponsesHeartbeat rejects a malformed upstream result
// after waiting has committed SSE. The response must contain one failed event,
// and outer fallback handling must leave that terminal response unchanged.
func TestProtocolError_AfterResponsesHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c, rec := newPassthroughKeepaliveTestContext(t)
		stop := StartOpenAISSEKeepalive(c, time.Second)
		defer stop()
		// Advance the heartbeat worker before invoking the synchronous error path;
		// the stopped controller hands writer ownership to terminal response output.
		time.Sleep(time.Second)
		synctest.Wait()
		output := &OpenAIResponseOutput{}
		err := output.ProtocolError(&http.Response{Header: make(http.Header)}, c, "invalid upstream result")
		require.Error(t, err)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
		require.Contains(t, rec.Body.String(), ": keepalive\n\n")
		require.Contains(t, rec.Body.String(), "event: response.failed\n")
		require.Contains(t, rec.Body.String(), "invalid upstream result")
		before := rec.Body.String()
		require.False(t, DefaultOpenAIErrorOutput().EnsureFallback(c, true))
		require.Equal(t, before, rec.Body.String())
	})
}
