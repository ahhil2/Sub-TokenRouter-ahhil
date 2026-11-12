package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	gatewayhttp "github.com/TokenFlux/TokenRouter/internal/gateway/httpapi"
	gatewayadapter "github.com/TokenFlux/TokenRouter/internal/gateway/provider"
	"github.com/TokenFlux/TokenRouter/internal/gateway/session/testkit"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
	openaiws "github.com/TokenFlux/TokenRouter/internal/upstream/openai/ws"
)

// staleIngressConn models a peer close waiting unread on an idle transport.
// Scripted output, when present, is delivered before the queued close.
type staleIngressConn struct {
	openAIWSCaptureConn // synchronized request and event fixture
}

// SupportsIdlePingWithoutReader exposes the transport's idle reader requirement
// to connection pool prewarming.
func (c *staleIngressConn) SupportsIdlePingWithoutReader() bool { return false }

// ReadMessage returns scripted events before the pending first-message close.
// The fixture shares its embedded mutex so assertions can inspect captured writes
// after the session handler has completed.
func (c *staleIngressConn) ReadMessage(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	hasEvent := len(c.events) > 0
	c.mu.Unlock()
	if hasEvent {
		return c.openAIWSCaptureConn.ReadMessage(ctx)
	}
	return nil, websocket.CloseError{Code: websocket.StatusPolicyViolation, Reason: "missing first response.create message"}
}

// TestIngressTransportRetryBypassesOtherStaleConnections fills a real pool with
// two sockets carrying unread upstream closes. A pre-output retry dials fresh;
// output and strict continuation each prevent replay after that failure.
// Final errors retain their stage through ingress so entry records them in Ops.
func TestIngressTransportRetryBypassesOtherStaleConnections(t *testing.T) {
	for _, name := range []string{"before_output", "retry_exhausted", "after_output", "strict_continuation"} {
		t.Run(name, func(t *testing.T) {
			wroteOutput := name == "after_output"
			strictContinuation := name == "strict_continuation"
			wantRetry := !wroteOutput && !strictContinuation
			wantSuccess := name == "before_output"
			options := &wsFixtureOptions{}
			options.Request.URLPolicy.AllowInsecureHTTP = true
			options.Pool.MaxConnsPerProvider = 3
			options.Pool.MaxIdlePerProvider = 3
			options.Pool.PoolTargetUtilization = 1
			options.Pool.QueueLimitPerConn = 8
			options.WS.DialTimeoutSeconds = 3
			options.WS.ReadTimeoutSeconds = 3
			options.WS.WriteTimeoutSeconds = 3
			stale := []*staleIngressConn{{}, {}}
			if wroteOutput {
				for _, conn := range stale {
					conn.events = [][]byte{[]byte(`{"type":"response.created","response":{"id":"partial","model":"gpt-5.1"}}`)}
				}
			}
			freshCapture := &openAIWSCaptureConn{events: [][]byte{[]byte(`{"type":"response.completed","response":{"id":"recovered","model":"gpt-5.1","usage":{"input_tokens":1,"output_tokens":1}}}`)}}
			var fresh openai.WSClientConn = freshCapture
			if name == "retry_exhausted" {
				failedFresh := &staleIngressConn{}
				fresh = failedFresh
				freshCapture = &failedFresh.openAIWSCaptureConn
			}
			dialer := &openAIWSQueueDialer{conns: []openai.WSClientConn{stale[0], stale[1], fresh}}
			pool := newOpenAIWSConnPool(options)
			defer pool.Close()
			pool.SetClientDialerForTest(dialer)
			svc := newWSFixture(wsFixtureInputs{options: options, transport: &auxiliaryHTTPRecorder{}, cache: &testkit.StickyCache{}, corrector: openai.NewCodexToolCorrector(), pool: pool})
			provider := &gatewayadapter.ExecutionProvider{Record: providercore.Record{
				LoadLocation: time.LoadLocation, ID: 39, Name: "stale-pool",
				Platform: capability.PlatformOpenAI, Type: capability.ProviderTypeAPIKey,
				Status: billing.StatusActive, Schedulable: true, Concurrency: 3,
				Credentials: map[string]any{"api_key": "sk-test", "base_url": "https://upstream.example/v1"},
				Extra:       map[string]any{"responses_websockets_v2_enabled": true},
			}}
			// Seed distinct idle sockets before ingress starts, reproducing queued peer
			// closes that transport writes cannot detect until the first read.
			seed := openaiws.WSAcquireRequest{Provider: openAIWSPoolProviderView(provider), WSURL: "wss://upstream.example/v1/responses", ForceNewConn: true}
			for index := range 2 {
				lease, err := pool.Acquire(context.Background(), seed)
				require.NoError(t, err)
				if index == 0 && strictContinuation {
					svc.State.BindResponseConn("resp_prior", lease.ConnID(), time.Minute)
				}
				lease.Release()
			}
			require.Equal(t, 2, dialer.DialCount())
			// Publish final Ops snapshots before handler completion so assertions
			// observe the finished attempt and avoid concurrent reads of Gin state.
			serverErrCh := make(chan error, 1)
			serverOpsCh := make(chan []gatewayhttp.OpsStreamError, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					serverErrCh <- err
					return
				}
				defer conn.CloseNow()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r.Clone(r.Context())
				c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")
				_, first, err := conn.Read(r.Context())
				if err == nil {
					err = svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, provider, "sk-test", first, nil)
				}
				if err != nil {
					target := &openAIWSEntryTarget{
						root:     &openAIWSEntryAdapter{c: c, log: zap.NewNop()},
						provider: provider,
					}
					target.LogFailure(err)
				}
				serverOpsCh <- gatewayhttp.GetOpsStreamErrors(c)
				serverErrCh <- err
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer client.CloseNow()
			payload := `{"type":"response.create","model":"gpt-5.1","stream":true}`
			if strictContinuation {
				payload = `{"type":"response.create","model":"gpt-5.1","stream":true,"store":false,"previous_response_id":"resp_prior"}`
			}
			require.NoError(t, client.Write(ctx, websocket.MessageText, []byte(payload)))
			_, event, err := client.Read(ctx)
			if !wroteOutput && !wantSuccess {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if wroteOutput {
				require.Contains(t, string(event), "partial")
			} else if wantSuccess {
				require.Contains(t, string(event), "recovered")
				require.NoError(t, client.Close(websocket.StatusNormalClosure, "done"))
			}
			select {
			case sessionErr := <-serverErrCh:
				if !wantSuccess {
					require.Error(t, sessionErr)
					require.Equal(t, websocket.StatusPolicyViolation, websocket.CloseStatus(sessionErr))
				} else {
					require.NoError(t, sessionErr)
				}
			case <-ctx.Done():
				t.Fatal("ingress session did not finish")
			}
			observed := <-serverOpsCh
			if wantSuccess {
				require.Empty(t, observed)
			} else {
				require.Len(t, observed, 1)
				require.Equal(t, "upstream_websocket_error", observed[0].Code)
				require.Equal(t, http.StatusBadGateway, observed[0].IntendedStatus)
			}
			wantDials := 3
			if !wantRetry {
				wantDials = 2
			}
			require.Equal(t, wantDials, dialer.DialCount())
			staleWrites := 0
			for _, conn := range stale {
				conn.mu.Lock()
				staleWrites += len(conn.writes)
				conn.mu.Unlock()
			}
			require.Equal(t, 1, staleWrites, "retry must bypass the other stale connection")
			freshCapture.mu.Lock()
			freshWrites := len(freshCapture.writes)
			freshCapture.mu.Unlock()
			require.Equal(t, wantDials-2, freshWrites)
		})
	}
}
