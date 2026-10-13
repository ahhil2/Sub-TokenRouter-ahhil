package site

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const (
	PelicanPrompt           = "创建一个HTML,内容是SVG绘制一个鹈鹕自行车的2D动画，你不需要任何测试"
	PelicanMaxResponseBytes = 1 << 20
	PelicanOutputTokens     = 16384
	PelicanTimeout          = 300 * time.Second
)

var (
	ErrPelicanInvalid  = apperror.BadRequest("PELICAN_INVALID", "Check the group, model, interval and publication consent.")
	ErrPelicanNotFound = apperror.NotFound("PELICAN_NOT_FOUND", "Pelican task or result not found.")
	ErrPelicanBusy     = apperror.Conflict("PELICAN_BUSY", "The task is already queued or running, or this group/model already exists.")
)

type PelicanTask struct {
	ID              int64      `json:"id"`
	GroupID         int64      `json:"group_id"`
	GroupName       string     `json:"group_name"`
	Model           string     `json:"model"`
	Enabled         bool       `json:"enabled"`
	IntervalMinutes int        `json:"interval_minutes"`
	PublicConsent   bool       `json:"public_consent"`
	ManualRequested bool       `json:"manual_requested"`
	NextRunAt       time.Time  `json:"next_run_at"`
	LeaseUntil      *time.Time `json:"lease_until"`
	LastStatus      string     `json:"last_status"`
	LastRunAt       *time.Time `json:"last_run_at"`
}

type PelicanResult struct {
	ID             int64      `json:"id"`
	TaskID         int64      `json:"task_id"`
	GroupID        int64      `json:"group_id"`
	GroupName      string     `json:"group_name"`
	Model          string     `json:"model"`
	Status         string     `json:"status"`
	Published      bool       `json:"published"`
	LatencyMs      int64      `json:"latency_ms"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	ProviderID     *int64     `json:"provider_id,omitempty"`
	ExecutionModel string     `json:"execution_model,omitempty"`
	RawResponse    string     `json:"raw_response,omitempty"`
	HTML           string     `json:"html,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
}

type PelicanFilter struct {
	Page    int
	GroupID int64
	TaskID  int64
	Model   string
	Date    string
	Public  bool
}

type PelicanPage struct {
	Items   []PelicanResult `json:"items"`
	HasMore bool            `json:"has_more"`
	Prompt  string          `json:"prompt"`
}

type PelicanCatalogItem struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Model     string `json:"model"`
}

type PelicanClaim struct {
	Task     PelicanTask
	ResultID int64
	Token    string
}

type PelicanExecution struct {
	Text       string
	ProviderID int64
	Model      string
}

type PelicanRepository interface {
	Catalog(context.Context) ([]PelicanCatalogItem, error)
	Tasks(context.Context) ([]PelicanTask, error)
	SaveTask(context.Context, PelicanTask) (PelicanTask, error)
	DeleteTask(context.Context, int64) error
	RequestRun(context.Context, int64) error
	Results(context.Context, PelicanFilter) (PelicanPage, error)
	Result(context.Context, int64, bool) (PelicanResult, error)
	Publish(context.Context, int64, bool) error
	DeleteResult(context.Context, int64) error
	Claim(context.Context) (*PelicanClaim, error)
	Finish(context.Context, PelicanClaim, PelicanResult) error
	Maintain(context.Context) error
}

type PelicanExecutor interface {
	Validate(context.Context, int64, string) error
	Generate(context.Context, PelicanTask) (PelicanExecution, error)
}

type PelicanService struct {
	repo     PelicanRepository
	executor PelicanExecutor
}

func NewPelicanService(repo PelicanRepository, executor PelicanExecutor) *PelicanService {
	return &PelicanService{repo: repo, executor: executor}
}

func (service *PelicanService) Tasks(ctx context.Context) ([]PelicanTask, error) {
	return service.repo.Tasks(ctx)
}

func (service *PelicanService) Catalog(ctx context.Context) ([]PelicanCatalogItem, error) {
	return service.repo.Catalog(ctx)
}

func (service *PelicanService) SaveTask(ctx context.Context, task PelicanTask) (PelicanTask, error) {
	task.Model = strings.TrimSpace(task.Model)
	if task.IntervalMinutes == 0 {
		task.IntervalMinutes = 360
	}
	if task.ID < 0 || task.GroupID <= 0 || task.Model == "" || len(task.Model) > 255 || strings.ContainsAny(task.Model, "*?\r\n") || task.IntervalMinutes < 5 || task.IntervalMinutes > 10080 || (task.Enabled && !task.PublicConsent) {
		return PelicanTask{}, ErrPelicanInvalid
	}
	if task.ID == 0 && task.Enabled {
		return PelicanTask{}, ErrPelicanInvalid
	}
	if err := service.executor.Validate(ctx, task.GroupID, task.Model); err != nil {
		return PelicanTask{}, err
	}
	return service.repo.SaveTask(ctx, task)
}

func (service *PelicanService) DeleteTask(ctx context.Context, id int64) error {
	return service.repo.DeleteTask(ctx, id)
}

func (service *PelicanService) RequestRun(ctx context.Context, id int64) error {
	return service.repo.RequestRun(ctx, id)
}

func (service *PelicanService) Results(ctx context.Context, filter PelicanFilter) (PelicanPage, error) {
	if filter.Page < 1 || filter.Page > 10000 || filter.GroupID < 0 || filter.TaskID < 0 || len(filter.Model) > 255 {
		return PelicanPage{}, ErrPelicanInvalid
	}
	if filter.Date != "" {
		if _, err := time.Parse("2006-01-02", filter.Date); err != nil {
			return PelicanPage{}, ErrPelicanInvalid
		}
	}
	return service.repo.Results(ctx, filter)
}

func (service *PelicanService) Result(ctx context.Context, id int64, public bool) (PelicanResult, error) {
	return service.repo.Result(ctx, id, public)
}

func (service *PelicanService) Publish(ctx context.Context, id int64, published bool) error {
	return service.repo.Publish(ctx, id, published)
}

func (service *PelicanService) DeleteResult(ctx context.Context, id int64) error {
	return service.repo.DeleteResult(ctx, id)
}

func ExtractPelicanHTML(raw string) (string, error) {
	if len(raw) > PelicanMaxResponseBytes || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return "", ErrPelicanInvalid
	}
	text := strings.TrimSpace(raw)
	if _, remaining, found := strings.Cut(text, "```"); found {
		if _, content, found := strings.Cut(remaining, "\n"); found {
			if body, _, closed := strings.Cut(content, "```"); closed {
				text = strings.TrimSpace(body)
			}
		}
	}
	lower := strings.ToLower(text)
	for _, tag := range []string{"html", "svg"} {
		start := strings.Index(lower, "<"+tag)
		end := strings.LastIndex(lower, "</"+tag+">")
		if start >= 0 && end < start {
			return "", ErrPelicanInvalid
		}
		if start >= 0 && end > start {
			return text[start : end+len(tag)+3], nil
		}
	}
	return "", ErrPelicanInvalid
}
