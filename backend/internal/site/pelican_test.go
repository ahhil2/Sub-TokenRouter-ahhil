package site

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type pelicanTestRepository struct {
	PelicanRepository
	saved  PelicanTask
	result PelicanResult
}

type pelicanTestExecutor struct {
	calls      int
	validation error
	text       string
	failure    error
}

func (repository *pelicanTestRepository) SaveTask(_ context.Context, task PelicanTask) (PelicanTask, error) {
	repository.saved = task
	return task, nil
}

func (repository *pelicanTestRepository) Finish(_ context.Context, _ PelicanClaim, result PelicanResult) error {
	repository.result = result
	return nil
}

func (executor *pelicanTestExecutor) Validate(context.Context, int64, string) error {
	return executor.validation
}

func (executor *pelicanTestExecutor) Generate(context.Context, PelicanTask) (PelicanExecution, error) {
	executor.calls++
	return PelicanExecution{Text: executor.text, ProviderID: 12, Model: "mapped-model"}, executor.failure
}

func TestPelicanExtraction(t *testing.T) {
	for _, test := range []struct{ name, raw, want string }{
		{"document", "<!doctype html><html><body><svg></svg></body></html>", "<html><body><svg></svg></body></html>"},
		{"fence", "Here is the result:\n```html\n<html><svg></svg></html>\n```", "<html><svg></svg></html>"},
		{"svg", "<svg viewBox=\"0 0 100 100\"></svg>", "<svg viewBox=\"0 0 100 100\"></svg>"},
		{"interactive animation", "<!DOCTYPE html><html><body><svg id=\"bird\"></svg><button id=\"pause\">暂停</button><script>document.getElementById('pause').addEventListener('click', () => {});</script></body></html>", "<html><body><svg id=\"bird\"></svg><button id=\"pause\">暂停</button><script>document.getElementById('pause').addEventListener('click', () => {});</script></body></html>"},
		{"empty", "", ""},
		{"truncated", "<html><svg>", ""},
		{"prose", "hello", ""},
		{"oversize", strings.Repeat("a", PelicanMaxResponseBytes+1), ""},
	} {
		t.Run(test.name, func(testing *testing.T) {
			value, err := ExtractPelicanHTML(test.raw)
			if test.want == "" {
				require.Error(testing, err)
			} else {
				require.NoError(testing, err)
				require.Equal(testing, test.want, value)
			}
		})
	}
}

func TestPelicanPrompt(t *testing.T) {
	for _, requirement := range []string{
		"【场景】", "【服装】", "【车子】", "【视觉风格】", "独立选择", "喉囊", "蹼足", "相差半圈",
		"<!DOCTYPE html>", "1024×768", "暂停／继续", "prefers-reduced-motion", "addEventListener",
		"sandbox=\"allow-scripts\"", "不依赖外部图片", "直接输出 HTML",
	} {
		require.Contains(t, PelicanPrompt, requirement)
	}
}

func TestPelicanTaskValidation(t *testing.T) {
	repository := &pelicanTestRepository{}
	executor := &pelicanTestExecutor{}
	service := NewPelicanService(repository, executor)
	base := PelicanTask{GroupID: 1, Model: "model-alias"}
	value, err := service.SaveTask(context.Background(), base)
	require.NoError(t, err)
	require.Equal(t, 360, value.IntervalMinutes)
	require.False(t, value.Enabled)
	for _, change := range []func(*PelicanTask){
		func(task *PelicanTask) { task.IntervalMinutes = 4 }, func(task *PelicanTask) { task.IntervalMinutes = 10081 },
		func(task *PelicanTask) { task.Enabled = true }, func(task *PelicanTask) { task.Enabled = true; task.PublicConsent = true },
		func(task *PelicanTask) { task.Model = "*" }, func(task *PelicanTask) { task.GroupID = 0 },
	} {
		task := base
		change(&task)
		_, err := service.SaveTask(context.Background(), task)
		require.Error(t, err)
	}
	executor.validation = errors.New("model outside group")
	_, err = service.SaveTask(context.Background(), base)
	require.ErrorContains(t, err, "outside group")
}

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
			runner.execute(context.Background(), PelicanClaim{ResultID: 1, Task: PelicanTask{GroupID: 1, Model: "alias"}})
			require.Equal(testing, 1, executor.calls)
			require.Equal(testing, test.status, repository.result.Status)
			require.Equal(testing, "mapped-model", repository.result.ExecutionModel)
		})
	}
}
