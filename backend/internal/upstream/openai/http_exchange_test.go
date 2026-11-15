package openai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// exchangeRequestBody records whether cancelled preparation releases its body.
type exchangeRequestBody struct {
	io.Reader      // prepared request payload
	closed    bool // request body release observed
}

// Close records local release when transport never receives this request body.
func (b *exchangeRequestBody) Close() error {
	b.closed = true
	return nil
}

// TestExchangeHTTP_CancelledPreparation cancels clients before detachment or
// during request preparation. No case may dispatch an upstream request or
// classify the cancellation as a transport failure, and acquired resources close.
func TestExchangeHTTP_CancelledPreparation(t *testing.T) {
	for _, phase := range []string{"before_detach", "build", "headers"} {
		for _, guarded := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "/unguarded", true: "/guarded"}[guarded], func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if phase == "before_detach" {
					cancel()
				}
				body := &exchangeRequestBody{Reader: strings.NewReader("{}")}
				releases := 0
				detachments := 0
				var sentContext context.Context
				options := HTTPExchangeOptions{
					StartedAt: time.Now(),
					RequestContext: func(ctx context.Context) (context.Context, context.CancelFunc) {
						detachments++
						return context.WithoutCancel(ctx), func() { releases++ }
					},
					Build: func(ctx context.Context, _ []byte) (*http.Request, error) {
						sentContext = ctx
						if phase == "build" {
							cancel()
						}
						return http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid", body)
					},
					ApplyHeaders: func(http.Header) {
						if phase == "headers" {
							cancel()
						}
					},
					Do: func(*http.Request) (*http.Response, error) {
						t.Fatal("cancelled preparation dispatched an upstream request")
						return nil, nil
					},
					TransportError: func(error) error {
						t.Fatal("client cancellation reached upstream failure classification")
						return nil
					},
				}
				if guarded {
					options.FirstOutputTimeout = time.Minute
				}
				response, err := ExchangeHTTP(ctx, nil, options)
				require.Nil(t, response)
				require.ErrorIs(t, err, context.Canceled)
				if phase == "before_detach" {
					require.Zero(t, detachments)
					require.Zero(t, releases)
				} else {
					require.Equal(t, 1, releases)
					require.True(t, body.closed)
					if guarded {
						require.ErrorIs(t, sentContext.Err(), context.Canceled)
					}
				}
			})
		}
	}
}

// TestExchangeHTTP_DispatchedResponseSurvivesClientCancellation disconnects the
// client after dispatch begins. Detached response reading must still collect
// the completed body, and closing it releases the first-output guard once.
func TestExchangeHTTP_DispatchedResponseSurvivesClientCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	releases := 0
	var sentContext context.Context
	response, err := ExchangeHTTP(ctx, nil, HTTPExchangeOptions{
		StartedAt: time.Now(), FirstOutputTimeout: time.Minute,
		RequestContext: func(ctx context.Context) (context.Context, context.CancelFunc) {
			return context.WithoutCancel(ctx), func() { releases++ }
		},
		Build: func(ctx context.Context, _ []byte) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid", nil)
		},
		ApplyHeaders: func(http.Header) {},
		Do: func(request *http.Request) (*http.Response, error) {
			sentContext = request.Context()
			cancel()
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("completed usage"))}, nil
		},
		Latency: func(time.Duration) {},
	})
	require.NoError(t, err)
	require.NoError(t, sentContext.Err())
	content, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "completed usage", string(content))
	require.Zero(t, releases)
	require.NoError(t, response.Body.Close())
	require.NoError(t, response.Body.Close())
	require.Equal(t, 1, releases)
	require.ErrorIs(t, sentContext.Err(), context.Canceled)
}

// TestExchangeHTTP_HeaderDeadline cancels a transport waiting for headers.
// Its late response body must close and the configured timeout classification
// must win over the transport cancellation returned at the same instant.
func TestExchangeHTTP_HeaderDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		body := &exchangeRequestBody{Reader: strings.NewReader("late response")}
		releases := 0
		response, err := ExchangeHTTP(context.Background(), nil, HTTPExchangeOptions{
			StartedAt: time.Now(), FirstOutputTimeout: time.Second,
			RequestContext: func(ctx context.Context) (context.Context, context.CancelFunc) {
				return ctx, func() { releases++ }
			},
			Build: func(ctx context.Context, _ []byte) (*http.Request, error) {
				return http.NewRequestWithContext(ctx, http.MethodPost, "https://example.invalid", nil)
			},
			ApplyHeaders: func(http.Header) {},
			Do: func(request *http.Request) (*http.Response, error) {
				// Let the configured timer cancel the blocked exchange, then return
				// a body as well to exercise ownership when the deadline wins header arrival.
				<-request.Context().Done()
				return &http.Response{Body: body}, request.Context().Err()
			},
			Latency:       func(time.Duration) {},
			HeaderTimeout: func() error { return context.DeadlineExceeded },
			TransportError: func(error) error {
				t.Fatal("header deadline lost to transport classification")
				return nil
			},
		})
		require.Nil(t, response)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.True(t, body.closed)
		require.Equal(t, 1, releases)
	})
}
