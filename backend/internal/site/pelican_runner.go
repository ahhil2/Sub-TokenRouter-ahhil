package site

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

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
		for index := 0; index < 2; index++ {
			workers.Add(1)
			go func() { defer workers.Done(); runner.work(ctx) }()
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

func (runner *PelicanRunner) execute(parent context.Context, claim PelicanClaim) {
	started := time.Now()
	ctx, cancel := context.WithTimeout(parent, PelicanTimeout)
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
		if len(result.RawResponse) > PelicanMaxResponseBytes || !utf8.ValidString(result.RawResponse) || strings.ContainsRune(result.RawResponse, 0) {
			result.RawResponse = ""
			err = ErrPelicanInvalid
		}
		if err == nil {
			result.HTML, err = ExtractPelicanHTML(result.RawResponse)
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		result.Status = "failed"
		result.ErrorMessage = err.Error()
	}
	if parent.Err() != nil {
		result.Status = "interrupted"
	}
	result.LatencyMs = time.Since(started).Milliseconds()
	finished := time.Now()
	result.FinishedAt = &finished
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(parent), 10*time.Second)
	defer saveCancel()
	if err := runner.service.repo.Finish(saveCtx, claim, result); err != nil {
		log.Printf("[Pelican] finish result %d: %v", claim.ResultID, err)
	}
}
