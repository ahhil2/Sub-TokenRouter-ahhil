package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

type channelMonitorReader struct{ fail bool }

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
