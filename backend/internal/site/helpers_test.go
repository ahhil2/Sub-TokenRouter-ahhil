package site

import "context"

// helpers_test.go supplies in-memory Pelican collaborators for service and runner
// tests. The fixtures capture task configuration and terminal results while
// allowing validation and generation to return controlled failures, so these
// tests exercise domain rules without provider requests.

// pelicanTestRepository captures saved settings and execution results for service tests.
type pelicanTestRepository struct {
	PelicanRepository               // Unused methods fail if called.
	saved             PelicanTask   // Last task saved.
	result            PelicanResult // Last execution finished.
}

// pelicanTestExecutor supplies deterministic validation and generated output to runner tests.
type pelicanTestExecutor struct {
	calls      int    // Number of generation attempts.
	validation error  // Validation failure to return.
	text       string // Generated response fixture.
	failure    error  // Generation failure to return.
}

// SaveTask records the normalized settings submitted by the service.
func (repository *pelicanTestRepository) SaveTask(_ context.Context, task PelicanTask) (PelicanTask, error) {
	repository.saved = task
	return task, nil
}

// Finish records the terminal result selected by the runner when saving is
// still permitted. A cancelled persistence context fails before recording,
// exposing accidental reuse of an expired generation context.
func (repository *pelicanTestRepository) Finish(ctx context.Context, _ PelicanClaim, result PelicanResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.result = result
	return nil
}

// Validate returns the configured model admission failure for this test.
func (executor *pelicanTestExecutor) Validate(context.Context, int64, string) error {
	return executor.validation
}

// Generate counts attempts and returns the configured provider output fixture.
func (executor *pelicanTestExecutor) Generate(context.Context, PelicanTask) (PelicanExecution, error) {
	executor.calls++
	return PelicanExecution{Text: executor.text, ProviderID: 12, Model: "mapped-model"}, executor.failure
}
