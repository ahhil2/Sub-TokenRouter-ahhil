package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

type channelMonitorReader struct{ fail bool }

func (reader channelMonitorReader) ReadChannelMonitorConfig(context.Context) ([]ops.ChannelMonitorGroupOption, error) {
	if reader.fail {
		return nil, errors.New("secret database credentials")
	}
	return []ops.ChannelMonitorGroupOption{}, nil
}

func (reader channelMonitorReader) SaveChannelMonitorConfig(context.Context, []ops.ChannelMonitorGroupConfig) error {
	if reader.fail {
		return errors.New("secret database credentials")
	}
	return nil
}

func (reader channelMonitorReader) ReadChannelMonitor(context.Context, time.Time) (*ops.ChannelMonitorSnapshot, error) {
	if reader.fail {
		return nil, errors.New("secret database credentials")
	}
	return &ops.ChannelMonitorSnapshot{MonitoringEnabled: true}, nil
}

func TestChannelMonitorPublicRoute(t *testing.T) {
	for _, fail := range []bool{false, true} {
		router := gin.New()
		guarded := false
		RegisterChannelMonitorRoutes(router.Group("/api/v1"), NewChannelMonitorHandler(ops.NewChannelMonitor(channelMonitorReader{fail: fail})), func(ctx *gin.Context) { guarded = true; ctx.Next() })
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/channel-monitors", nil))
		require.True(t, guarded)
		require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
		if fail {
			require.Equal(t, 503, recorder.Code)
			require.NotContains(t, recorder.Body.String(), "secret")
		} else {
			require.Equal(t, 200, recorder.Code)
			require.Contains(t, recorder.Body.String(), `"channels":[]`)
		}
	}
}

func TestChannelMonitorAdminConfiguration(t *testing.T) {
	for _, test := range []struct {
		name, role, method, body string
		fail                     bool
		status                   int
	}{
		{name: "guest read", method: http.MethodGet, status: 401},
		{name: "guest write", method: http.MethodPut, body: `{"groups":[]}`, status: 401},
		{name: "user read", role: "user", method: http.MethodGet, status: 403},
		{name: "user write", role: "user", method: http.MethodPut, body: `{"groups":[]}`, status: 403},
		{name: "admin read", role: "admin", method: http.MethodGet, status: 200},
		{name: "admin clear", role: "admin", method: http.MethodPut, body: `{"groups":[]}`, status: 200},
		{name: "admin configure", role: "admin", method: http.MethodPut, body: `{"groups":[{"group_id":1,"enabled":true,"status":"maintenance"}]}`, status: 200},
		{name: "missing groups", role: "admin", method: http.MethodPut, body: `{}`, status: 400},
		{name: "invalid status", role: "admin", method: http.MethodPut, body: `{"groups":[{"group_id":1,"status":"invalid"}]}`, status: 400},
		{name: "broken JSON", role: "admin", method: http.MethodPut, body: `{`, status: 400},
		{name: "read error", role: "admin", method: http.MethodGet, fail: true, status: 503},
		{name: "write error", role: "admin", method: http.MethodPut, body: `{"groups":[]}`, fail: true, status: 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			admin := router.Group("/api/v1/admin", func(ctx *gin.Context) {
				if test.role == "" {
					ctx.AbortWithStatus(http.StatusUnauthorized)
				} else if test.role != "admin" {
					ctx.AbortWithStatus(http.StatusForbidden)
				}
			})
			RegisterChannelMonitorAdminRoutes(admin, NewChannelMonitorHandler(ops.NewChannelMonitor(channelMonitorReader{fail: test.fail})))
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, "/api/v1/admin/channel-monitors/config", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			require.Equal(t, test.status, recorder.Code)
			require.NotContains(t, recorder.Body.String(), "secret")
		})
	}
}
