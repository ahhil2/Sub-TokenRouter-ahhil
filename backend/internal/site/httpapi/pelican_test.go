package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/site"
)

// pelicanHTTPRepository records HTTP service calls and supplies public result fixtures.
type pelicanHTTPRepository struct {
	site.PelicanRepository
	public bool
	calls  int
	saved  site.PelicanTask // Last task passed to storage.
}

// pelicanHTTPExecutor accepts model validation while leaving actual generation unused.
type pelicanHTTPExecutor struct {
	site.PelicanExecutor // Generation is unused by HTTP configuration tests.
}

// Validate accepts the model so HTTP tests can isolate input handling.
func (pelicanHTTPExecutor) Validate(context.Context, int64, string) error { return nil }

// SaveTask records settings that passed parsing and domain validation checks.
func (repository *pelicanHTTPRepository) SaveTask(_ context.Context, task site.PelicanTask) (site.PelicanTask, error) {
	repository.calls++
	repository.saved = task
	return task, nil
}

func (repository *pelicanHTTPRepository) Results(_ context.Context, filter site.PelicanFilter) (site.PelicanPage, error) {
	repository.calls++
	repository.public = filter.Public
	return site.PelicanPage{Items: []site.PelicanResult{}, Prompt: site.PelicanPrompt}, nil
}

func (repository *pelicanHTTPRepository) Result(_ context.Context, _ int64, public bool) (site.PelicanResult, error) {
	repository.public = public
	return site.PelicanResult{}, site.ErrPelicanNotFound
}

// TestPelicanPublicAndAdminHTTP checks showcase visibility and administrator
// admission using canonical management paths. Public reads must hide unpublished
// results, and invalid publication requests must fail before a service mutation.
func TestPelicanPublicAndAdminHTTP(test *testing.T) {
	repository := &pelicanHTTPRepository{}
	handler := NewPelicanHandler(site.NewPelicanService(repository, nil))
	router := gin.New()
	RegisterPelicanPublicRoutes(router.Group(""), handler, func(ctx *gin.Context) { ctx.Next() })
	admin := router.Group("/api/v1/admin", func(ctx *gin.Context) {
		if ctx.GetHeader("X-Test-Role") != "admin" {
			ctx.AbortWithStatus(403)
			return
		}
		ctx.Next()
	})
	RegisterPelicanAdminRoutes(admin, handler)
	for _, scenario := range []struct {
		method, path, role, body string
		status                   int
	}{
		{"GET", "/pelican-showcase", "", "", 200},
		{"GET", "/pelican-showcase?page=0", "", "", 400},
		{"GET", "/pelican-showcase?date=invalid", "", "", 400},
		{"GET", "/pelican-showcase/items/1", "", "", 404},
		{"GET", "/pelican-showcase/items/-1", "", "", 400},
		{"POST", "/pelican-showcase", "", "", 404},
		{"GET", "/api/v1/admin/pelican-tests", "", "", 403},
		{"POST", "/api/v1/admin/pelican-tests", "user", "{}", 403},
		{"GET", "/api/v1/admin/pelican-tests/results", "admin", "", 200},
		{"PATCH", "/api/v1/admin/pelican-tests/results/1", "admin", "{}", 400},
	} {
		request := httptest.NewRequest(scenario.method, scenario.path, strings.NewReader(scenario.body))
		request.Header.Set("X-Test-Role", scenario.role)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(test, scenario.status, response.Code, scenario.path+": "+response.Body.String())
		if scenario.path == "/pelican-showcase" && scenario.method == http.MethodGet {
			require.True(test, repository.public)
		}
		if scenario.path == "/api/v1/admin/pelican-tests/results" {
			require.False(test, repository.public)
		}
	}
	require.Equal(test, 2, repository.calls)
}

// TestPelicanTaskTimeoutHTTP submits task budgets through administrator routes.
// Integer limits round-trip through JSON, invalid inputs never reach storage,
// and omitted update limits reach the repository as a preservation request.
func TestPelicanTaskTimeoutHTTP(t *testing.T) {
	for _, scenario := range []struct {
		name, method, path, field string
		status, seconds           int
	}{
		{"creation default", "POST", "", "", 200, 300},
		{"custom budget", "POST", "", `,"timeout_seconds":600`, 200, 600},
		{"minimum budget", "POST", "", `,"timeout_seconds":1`, 200, 1},
		{"maximum budget", "PUT", "/7", `,"timeout_seconds":3600`, 200, 3600},
		{"omitted update", "PUT", "/7", "", 200, 0},
		{"null update", "PUT", "/7", `,"timeout_seconds":null`, 200, 0},
		{"zero update", "PUT", "/7", `,"timeout_seconds":0`, 200, 0},
		{"negative budget", "POST", "", `,"timeout_seconds":-1`, 400, 0},
		{"excessive budget", "PUT", "/7", `,"timeout_seconds":3601`, 400, 0},
		{"fractional budget", "POST", "", `,"timeout_seconds":1.5`, 400, 0},
		{"text budget", "POST", "", `,"timeout_seconds":"600"`, 400, 0},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			repository := &pelicanHTTPRepository{}
			handler := NewPelicanHandler(site.NewPelicanService(repository, pelicanHTTPExecutor{}))
			router := gin.New()
			RegisterPelicanAdminRoutes(router.Group("/api/v1/admin"), handler)
			body := `{"group_id":1,"model":"alias","interval_minutes":360` + scenario.field + `}`
			request := httptest.NewRequest(scenario.method, "/api/v1/admin/pelican-tests"+scenario.path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, scenario.status, response.Code, response.Body.String())
			if scenario.status != http.StatusOK {
				require.Zero(t, repository.calls)
				return
			}
			require.Equal(t, 1, repository.calls)
			require.Equal(t, scenario.seconds, repository.saved.TimeoutSeconds)
			var envelope struct {
				Data site.PelicanTask `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
			require.Equal(t, scenario.seconds, envelope.Data.TimeoutSeconds)
		})
	}
}
