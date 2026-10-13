package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/site"
)

type pelicanHTTPRepository struct {
	site.PelicanRepository
	public bool
	calls  int
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

func TestPelicanPublicAndAdminHTTP(test *testing.T) {
	repository := &pelicanHTTPRepository{}
	handler := NewPelicanHandler(site.NewPelicanService(repository, nil))
	router := gin.New()
	RegisterPelicanPublicRoutes(router.Group(""), handler, func(ctx *gin.Context) { ctx.Next() })
	admin := router.Group("/admin", func(ctx *gin.Context) {
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
		{"GET", "/admin/pelican-tests", "", "", 403},
		{"POST", "/admin/pelican-tests", "user", "{}", 403},
		{"GET", "/admin/pelican-tests/results", "admin", "", 200},
		{"PATCH", "/admin/pelican-tests/results/1", "admin", "{}", 400},
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
		if scenario.path == "/admin/pelican-tests/results" {
			require.False(test, repository.public)
		}
	}
	require.Equal(test, 2, repository.calls)
}
