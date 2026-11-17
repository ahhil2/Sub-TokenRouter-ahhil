package site

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// pelican_runner.go executes claimed drawing tasks and saves their terminal
// results. Each worker uses the timeout captured in its claim, so administrator
// changes apply when the next execution is claimed. The repository grants extra
// lease time for result persistence after the generation context expires.
//
// Shutdown cancels active generation and waits for workers to record interrupted
// results. Saving uses a separate bounded context because the execution context
// may already have expired. Lease tokens let the repository reject late results
// after maintenance has recovered an abandoned task, preventing an expired
// worker from replacing the recovered result.

type PelicanRunner struct {
	service *PelicanService
	mu      sync.Mutex
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewPelicanRunner(service *PelicanService) *PelicanRunner {
	return &PelicanRunner{service: service}
}

func (runner *PelicanRunner) Start(ctx context.Context) error {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if runner.cancel != nil {
		return nil
	}
	ctx, runner.cancel = context.WithCancel(ctx)
	runner.done = make(chan struct{})
	go func() {
		defer close(runner.done)
		var workers sync.WaitGroup
		for range 2 {
			workers.Go(func() { runner.work(ctx) })
		}
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			maintenanceCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := runner.service.repo.Maintain(maintenanceCtx); err != nil && ctx.Err() == nil {
				log.Printf("[Pelican] maintenance: %v", err)
			}
			cancel()
			select {
			case <-ctx.Done():
				workers.Wait()
				return
			case <-ticker.C:
			}
		}
	}()
	return nil
}

func (runner *PelicanRunner) Stop(ctx context.Context) error {
	runner.mu.Lock()
	cancel, done := runner.cancel, runner.done
	runner.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runner *PelicanRunner) work(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		claimCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		claim, err := runner.service.repo.Claim(claimCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			log.Printf("[Pelican] claim: %v", err)
		}
		if claim != nil {
			runner.execute(ctx, *claim)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// execute generates one result within the claimed task's configured time limit.
// Claims come from persisted tasks with positive, validated timeout seconds.
// The same deadline covers validation, provider selection, and generated output.
//
// Expiration marks the result failed, while parent cancellation marks it
// interrupted because the application is stopping. Result persistence gets a
// separate ten-second window after either cancellation, and the repository
// accepts it only while the claim still owns an active lease.
func (runner *PelicanRunner) execute(parent context.Context, claim PelicanClaim) {
	started := time.Now()
	// Task budgets are stored in whole seconds for administrator inputs.
	ctx, cancel := context.WithTimeout(parent, time.Duration(claim.Task.TimeoutSeconds)*time.Second)
	defer cancel()
	result := PelicanResult{Status: "success"}
	err := runner.service.executor.Validate(ctx, claim.Task.GroupID, claim.Task.Model)
	if err == nil {
		var generated PelicanExecution
		generated, err = runner.service.executor.Generate(ctx, claim.Task)
		if generated.ProviderID > 0 {
			result.ProviderID = &generated.ProviderID
		}
		result.ExecutionModel = generated.Model
		result.RawResponse = generated.Text
		// Reject oversized or malformed text before saving because PostgreSQL
		// result columns require bounded content and valid text without null bytes.
		if len(result.RawResponse) > PelicanMaxResponseBytes || !utf8.ValidString(result.RawResponse) || strings.ContainsRune(result.RawResponse, 0) {
			result.RawResponse = ""
			err = ErrPelicanInvalid
		}
		if err == nil {
			result.HTML, err = ExtractPelicanHTML(result.RawResponse)
		}
	}
	// Deadline expiry takes precedence over returned output or adapter errors;
	// a provider that returns after cancellation cannot turn the run into success.
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		result.Status = "failed"
		result.ErrorMessage = err.Error()
	}
	// Application shutdown interrupts the run regardless of its provider outcome,
	// distinguishing stopped workers from requests that exhausted their generation budget.
	if parent.Err() != nil {
		result.Status = "interrupted"
	}
	result.LatencyMs = time.Since(started).Milliseconds()
	finished := time.Now()
	result.FinishedAt = &finished
	// A timeout or shutdown cancels generation first; saving needs its own
	// bounded context so the recorded status can still release this task's lease.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer saveCancel()
	if err := runner.service.repo.Finish(saveCtx, claim, result); err != nil {
		log.Printf("[Pelican] finish result %d: %v", claim.ResultID, err)
	}
}
