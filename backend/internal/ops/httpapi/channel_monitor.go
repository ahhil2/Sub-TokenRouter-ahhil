package httpapi

import (
	"errors"
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

func RegisterChannelMonitorAdminRoutes(router *gin.RouterGroup, handler *ChannelMonitorHandler) {
	router.GET("/channel-monitors/config", handler.Configuration)
	router.PUT("/channel-monitors/config", handler.SaveConfiguration)
}

func (handler *ChannelMonitorHandler) Configuration(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	groups, err := handler.monitor.Configuration(ctx.Request.Context())
	if err != nil {
		httpx.Error(ctx, http.StatusServiceUnavailable, "Channel monitor configuration is temporarily unavailable")
		return
	}
	httpx.Success(ctx, gin.H{"groups": groups})
}

func (handler *ChannelMonitorHandler) SaveConfiguration(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 64*1024)
	var request struct {
		Groups []ops.ChannelMonitorGroupConfig `json:"groups"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		httpx.Error(ctx, http.StatusBadRequest, "Invalid channel monitor configuration")
		return
	}
	if err := handler.monitor.SaveConfiguration(ctx.Request.Context(), request.Groups); err != nil {
		if errors.Is(err, ops.ErrInvalidMonitorConfig) {
			httpx.Error(ctx, http.StatusBadRequest, "Invalid channel monitor groups or status")
		} else {
			httpx.Error(ctx, http.StatusServiceUnavailable, "Could not save channel monitor configuration")
		}
		return
	}
	httpx.Success(ctx, gin.H{"saved": true})
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
