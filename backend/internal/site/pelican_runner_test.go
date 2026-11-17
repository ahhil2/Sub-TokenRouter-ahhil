package site

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

// pelican_runner_test.go checks generation outcomes and configurable execution
// deadlines. The runner uses in-memory repositories and a cancellable executor,
// while virtual time exercises requests lasting longer than the default budget
// without making upstream requests or delaying the suite.

// pelicanTimedExecutor simulates generation that either completes or observes cancellation.
type pelicanTimedExecutor struct {
	pelicanTestExecutor               // Reuses model validation and attempt recording.
	duration            time.Duration // Simulated generation latency.
}

// Generate waits for simulated provider output or the runner's context cancellation.
// The delay runs inside a synctest bubble so configured budgets can expire
// without consuming their duration in wall time.
func (executor *pelicanTimedExecutor) Generate(ctx context.Context, _ PelicanTask) (PelicanExecution, error) {
	executor.calls++
	select {
	case <-time.After(executor.duration):
		return PelicanExecution{Text: "<html><svg></svg></html>"}, nil
	case <-ctx.Done():
		return PelicanExecution{}, ctx.Err()
	}
}

// TestPelicanRunnerOneAttemptAndFailure exercises valid output, malformed output,
// and provider failures. Every scenario makes one attempt and retains execution
// model metadata with its terminal status.
func TestPelicanRunnerOneAttemptAndFailure(t *testing.T) {
	for _, test := range []struct {
		name, text, status string
		failure            error
	}{
		{"success", "<html><svg></svg></html>", "success", nil},
		{"format failure", "not html", "failed", nil},
		{"upstream failure", "", "failed", errors.New("429")},
	} {
		t.Run(test.name, func(testing *testing.T) {
			repository := &pelicanTestRepository{}
			executor := &pelicanTestExecutor{text: test.text, failure: test.failure}
			runner := NewPelicanRunner(NewPelicanService(repository, executor))
			runner.execute(context.Background(), PelicanClaim{ResultID: 1, Task: PelicanTask{GroupID: 1, Model: "alias", TimeoutSeconds: PelicanDefaultTimeoutSeconds}})
			require.Equal(testing, 1, executor.calls)
			require.Equal(testing, test.status, repository.result.Status)
			require.Equal(testing, "mapped-model", repository.result.ExecutionModel)
		})
	}
}

// TestPelicanRunnerConfiguredTimeout exercises shorter and longer generation
// budgets with a cancellable provider. Configured expiration records failure,
// generation beyond five minutes can succeed, and parent cancellation still
// records interruption through a usable persistence context.
func TestPelicanRunnerConfiguredTimeout(t *testing.T) {
	for _, scenario := range []struct {
		name                           string
		timeout                        int
		duration, parentLimit, elapsed time.Duration
		status                         string
	}{
		{"short budget expires", 1, 2 * time.Second, time.Hour, time.Second, "failed"},
		{"default budget expires", 300, 301 * time.Second, time.Hour, 300 * time.Second, "failed"},
		{"long generation succeeds", 600, 400 * time.Second, time.Hour, 400 * time.Second, "success"},
		{"custom budget expires", 600, 601 * time.Second, time.Hour, 600 * time.Second, "failed"},
		{"shutdown interrupts", 600, 400 * time.Second, 45 * time.Second, 45 * time.Second, "interrupted"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				repository := &pelicanTestRepository{}
				executor := &pelicanTimedExecutor{duration: scenario.duration}
				runner := NewPelicanRunner(NewPelicanService(repository, executor))
				parent, cancel := context.WithTimeout(t.Context(), scenario.parentLimit)
				defer cancel()
				runner.execute(parent, PelicanClaim{Task: PelicanTask{TimeoutSeconds: scenario.timeout}})
				require.Equal(t, 1, executor.calls)
				require.Equal(t, scenario.status, repository.result.Status)
				require.Equal(t, scenario.elapsed.Milliseconds(), repository.result.LatencyMs)
				if scenario.status == "failed" {
					require.Equal(t, context.DeadlineExceeded.Error(), repository.result.ErrorMessage)
				}
			})
		})
	}
}
