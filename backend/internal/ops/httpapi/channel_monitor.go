package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/ops"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

type ChannelMonitorHandler struct {
	monitor *ops.ChannelMonitor
}

func NewChannelMonitorHandler(monitor *ops.ChannelMonitor) *ChannelMonitorHandler {
	return &ChannelMonitorHandler{monitor: monitor}
}

func RegisterChannelMonitorRoutes(router *gin.RouterGroup, handler *ChannelMonitorHandler, guard gin.HandlerFunc) {
	router.GET("/channel-monitors", guard, handler.Snapshot)
}

func (handler *ChannelMonitorHandler) Snapshot(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	snapshot, err := handler.monitor.Snapshot(ctx.Request.Context())
	if err != nil {
		ctx.Header("Retry-After", "30")
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"code": 503, "message": "Channel monitoring is temporarily unavailable"})
		return
	}
	httpx.Success(ctx, snapshot)
}
