package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/ops"
	"github.com/TokenFlux/TokenRouter/internal/server/adminapi"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// channel_monitor.go serves public status snapshots and administrator monitor configuration.
// Administrator configuration uses the caller's protected route group and shared
// MCP request descriptions. Public reads return sanitized failures and disable
// browser caching so status refreshes reflect the monitor service.

// ChannelMonitorConfigRequest carries the complete requested monitor group configuration.
type ChannelMonitorConfigRequest struct {
	// Groups replaces the configured list; an empty array clears the list,
	// while omission or null is rejected by the monitor service.
	Groups []ops.ChannelMonitorGroupConfig `json:"groups"`
}

type ChannelMonitorHandler struct {
	monitor *ops.ChannelMonitor
}

func NewChannelMonitorHandler(monitor *ops.ChannelMonitor) *ChannelMonitorHandler {
	return &ChannelMonitorHandler{monitor: monitor}
}

func RegisterChannelMonitorRoutes(router *gin.RouterGroup, handler *ChannelMonitorHandler, guard gin.HandlerFunc) {
	router.GET("/channel-monitors", guard, handler.Snapshot)
}

// RegisterChannelMonitorAdminRoutes mounts described monitor configuration operations.
// The caller provides an authenticated, audited /api/v1/admin group. Both reads
// and writes use the existing handlers and their service validation.
func RegisterChannelMonitorAdminRoutes(router *gin.RouterGroup, handler *ChannelMonitorHandler) {
	adminapi.Register(router, ManagementOperations["channel_monitors.config.get"], handler.Configuration)
	adminapi.Register(router, ManagementOperations["channel_monitors.config.update"], handler.SaveConfiguration)
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

// SaveConfiguration validates and replaces the requested monitor group settings.
// Request bodies are limited to 64 KiB. Invalid configuration returns a client
// error, while persistence failures return a sanitized service-unavailable response.
func (handler *ChannelMonitorHandler) SaveConfiguration(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 64*1024)
	var request ChannelMonitorConfigRequest
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
