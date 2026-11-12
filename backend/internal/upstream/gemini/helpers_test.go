package gemini

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// executeBody wraps a response stream and counts explicit fixture close
// invocations.
type executeBody struct {
	io.ReadCloser // wrapped upstream response body
	// closes points to the nonnil shared atomic counter supplied by the fixture;
	// every Close invocation updates it before the wrapped body is closed.
	closes *atomic.Int32 // shared close invocation counter
}

// Close increments the shared close counter before delegating to the wrapped
// body. It counts every invocation, including repeated closes, and returns the
// wrapped close error to the fixture caller.
func (b *executeBody) Close() error { b.closes.Add(1); return b.ReadCloser.Close() }

// cancellationExchangeOptions supplies local request construction and inert
// diagnostics for exchange cancellation tests. Callers set Do for their failure
// scenario; the callback is the sole transport and performs no network access.
func cancellationExchangeOptions() ExchangeOptions {
	return ExchangeOptions{
		MaxRetries: 2,
		Build: func(ctx context.Context) (*http.Request, string, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://fixture.invalid/", nil)
			return req, "x-request-id", err
		},
		BuildError:  func(err error) error { return errors.New(err.Error()) },
		FinalError:  errors.New,
		Sanitize:    func(message string) string { return message },
		Observe:     func(ExchangeNotice) {},
		SetError:    func(int, string, string) {},
		CheckPolicy: func(_ context.Context, resp *http.Response) (bool, *http.Response) { return false, resp },
		ShouldRetry: func(status int) bool {
			return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
		},
		ReadError: func(resp *http.Response) []byte {
			body, _ := io.ReadAll(resp.Body)
			return body
		},
		Message:  func(body []byte) string { return string(body) },
		Detail:   func(body []byte) string { return string(body) },
		OnStatus: func(context.Context, int, http.Header, []byte) {},
	}
}

// testExchangeCancellation exercises one protocol's exchange with the shared
// cancellation cases. The supplied exchange uses synthetic callbacks and a
// virtual clock; callers place each invocation in its owning protocol test file.
//
// Cancellation before construction, during construction, during transport, and
// during retry delay must stop additional attempts. HTTP response bodies still
// close before interrupted waits, while an independent upstream timeout keeps
// its retry opportunity because the client context remains active.
func testExchangeCancellation(t *testing.T, exchange func(context.Context, ExchangeOptions) (ExchangeResult, error)) {
	t.Helper()
	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		options := cancellationExchangeOptions()
		options.Build = func(context.Context) (*http.Request, string, error) {
			t.Fatal("canceled request reached Build")
			return nil, "", nil
		}
		_, err := exchange(ctx, options)
		require.ErrorIs(t, err, context.Canceled)
	})
	t.Run("canceled during build", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var closes atomic.Int32
		options := cancellationExchangeOptions()
		options.Build = func(ctx context.Context) (*http.Request, string, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://fixture.invalid/", nil)
			require.NoError(t, err)
			req.Body = &executeBody{ReadCloser: io.NopCloser(strings.NewReader("fixture")), closes: &closes}
			cancel()
			return req, "", nil
		}
		options.Do = func(*http.Request) (*http.Response, error) {
			t.Fatal("canceled build reached Do")
			return nil, nil
		}
		_, err := exchange(ctx, options)
		require.ErrorIs(t, err, context.Canceled)
		require.EqualValues(t, 1, closes.Load())
	})
	t.Run("response arrives with cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var closes atomic.Int32
		options := cancellationExchangeOptions()
		options.Do = func(*http.Request) (*http.Response, error) {
			cancel()
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: &executeBody{ReadCloser: io.NopCloser(strings.NewReader("fixture")), closes: &closes}}, nil
		}
		_, err := exchange(ctx, options)
		require.ErrorIs(t, err, context.Canceled)
		require.EqualValues(t, 1, closes.Load())
	})
	for _, cancelParent := range []bool{false, true} {
		t.Run(fmt.Sprintf("transport cancellation parent=%t", cancelParent), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			options := cancellationExchangeOptions()
			calls := 0
			options.Do = func(*http.Request) (*http.Response, error) {
				calls++
				if cancelParent {
					cancel()
				}
				return nil, fmt.Errorf("transport: %w", context.Canceled)
			}
			options.Observe = func(ExchangeNotice) { t.Error("cancellation was recorded as an upstream failure") }
			options.FinalError = func(string) error {
				t.Error("cancellation was remapped")
				return nil
			}
			_, err := exchange(ctx, options)
			require.ErrorIs(t, err, context.Canceled)
			require.Equal(t, 1, calls)
		})
	}
	for _, status := range []int{0, http.StatusTooManyRequests, http.StatusBadGateway} {
		t.Run(fmt.Sprintf("cancel retry status=%d", status), func(t *testing.T) {
			// Virtual time advances only after both the retry and the cancellation
			// worker block, making cancellation during production backoff deterministic.
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				options := cancellationExchangeOptions()
				calls := 0
				var closes atomic.Int32
				options.Do = func(*http.Request) (*http.Response, error) {
					calls++
					if status == 0 {
						return nil, errors.New("connection reset")
					}
					return &http.Response{StatusCode: status, Header: make(http.Header), Body: &executeBody{ReadCloser: io.NopCloser(strings.NewReader("retry")), closes: &closes}}, nil
				}
				// Cancel well inside the first jittered backoff, whose minimum delay is
				// eight hundred milliseconds, so another dispatch proves a regression.
				go func() {
					time.Sleep(100 * time.Millisecond)
					cancel()
				}()
				started := time.Now()
				_, err := exchange(ctx, options)
				require.ErrorIs(t, err, context.Canceled)
				require.Equal(t, 100*time.Millisecond, time.Since(started))
				require.Equal(t, 1, calls)
				if status != 0 {
					require.EqualValues(t, 1, closes.Load())
				}
			})
		})
	}
	t.Run("parent deadline during transport", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			options := cancellationExchangeOptions()
			calls := 0
			options.Do = func(*http.Request) (*http.Response, error) {
				calls++
				// The simulated transport blocks on the request deadline so the exchange
				// receives the same timeout identity as a context-aware HTTP client.
				<-ctx.Done()
				return nil, fmt.Errorf("transport: %w", ctx.Err())
			}
			_, err := exchange(ctx, options)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.Equal(t, 1, calls)
		})
	})
	t.Run("independent upstream deadline retries", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			options := cancellationExchangeOptions()
			calls := 0
			options.Do = func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return nil, fmt.Errorf("upstream deadline: %w", context.DeadlineExceeded)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("success"))}, nil
			}
			result, err := exchange(context.Background(), options)
			require.NoError(t, err)
			require.Equal(t, 2, calls)
			require.Equal(t, http.StatusOK, result.Response.StatusCode)
			require.NoError(t, result.Response.Body.Close())
		})
	})
}
