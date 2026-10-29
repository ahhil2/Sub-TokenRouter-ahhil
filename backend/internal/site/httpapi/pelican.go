package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/server/adminapi"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

// pelican.go serves public showcase reads and administrator generation tasks.
// Public handlers restrict results to published entries, while administrator
// handlers inherit authentication and auditing from their route group. Management
// descriptions expose the same task and publication inputs to MCP clients.

// PublishPelicanResultRequest carries the requested visibility of a generated result.
type PublishPelicanResultRequest struct {
	// Published distinguishes a supplied false from omission; the handler rejects
	// absent and null values before asking the service to update visibility.
	Published *bool `json:"published"`
}

type PelicanHandler struct{ service *site.PelicanService }

func NewPelicanHandler(service *site.PelicanService) *PelicanHandler {
	return &PelicanHandler{service: service}
}

func RegisterPelicanPublicRoutes(group *gin.RouterGroup, handler *PelicanHandler, limit gin.HandlerFunc) {
	public := group.Group("/pelican-showcase", limit)
	public.GET("", func(ctx *gin.Context) { handler.results(ctx, true) })
	public.GET("/catalog", handler.catalog)
	public.GET("/items/:id", func(ctx *gin.Context) { handler.result(ctx, true) })
}

// RegisterPelicanAdminRoutes mounts task and result operations with MCP descriptions.
// The caller supplies the /api/v1/admin group with authentication and auditing
// already installed; all operations retain that middleware chain.
func RegisterPelicanAdminRoutes(admin *gin.RouterGroup, handler *PelicanHandler) {
	group := admin.Group("/pelican-tests")
	adminapi.Register(group, ManagementOperations["pelican.tasks.list"], handler.tasks)
	adminapi.Register(group, ManagementOperations["pelican.tasks.create"], handler.save)
	adminapi.Register(group, ManagementOperations["pelican.tasks.update"], handler.save)
	adminapi.Register(group, ManagementOperations["pelican.tasks.delete"], handler.deleteTask)
	adminapi.Register(group, ManagementOperations["pelican.tasks.run"], handler.run)
	adminapi.Register(group, ManagementOperations["pelican.results.list"], func(ctx *gin.Context) { handler.results(ctx, false) })
	adminapi.Register(group, ManagementOperations["pelican.results.get"], func(ctx *gin.Context) { handler.result(ctx, false) })
	adminapi.Register(group, ManagementOperations["pelican.results.publish"], handler.publish)
	adminapi.Register(group, ManagementOperations["pelican.results.delete"], handler.deleteResult)
}

func (handler *PelicanHandler) tasks(ctx *gin.Context) {
	items, err := handler.service.Tasks(ctx.Request.Context())
	if err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	httpx.Success(ctx, items)
}

func (handler *PelicanHandler) catalog(ctx *gin.Context) {
	items, err := handler.service.Catalog(ctx.Request.Context())
	if err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	httpx.Success(ctx, items)
}

func (handler *PelicanHandler) save(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 8192)
	var task site.PelicanTask
	if err := ctx.ShouldBindJSON(&task); err != nil {
		httpx.ErrorFrom(ctx, site.ErrPelicanInvalid)
		return
	}
	task.ID = 0
	if ctx.Param("id") != "" {
		var ok bool
		task.ID, ok = pelicanID(ctx)
		if !ok {
			return
		}
	}
	item, err := handler.service.SaveTask(ctx.Request.Context(), task)
	if err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	httpx.Success(ctx, item)
}

func (handler *PelicanHandler) deleteTask(ctx *gin.Context) {
	id, ok := pelicanID(ctx)
	if !ok {
		return
	}
	if err := handler.service.DeleteTask(ctx.Request.Context(), id); err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	httpx.Success(ctx, gin.H{"deleted": true})
}

func (handler *PelicanHandler) run(ctx *gin.Context) {
	id, ok := pelicanID(ctx)
	if !ok {
		return
	}
	if err := handler.service.RequestRun(ctx.Request.Context(), id); err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	httpx.Success(ctx, gin.H{"queued": true})
}

func (handler *PelicanHandler) results(ctx *gin.Context, public bool) {
	page, pageErr := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	groupID, groupErr := strconv.ParseInt(ctx.DefaultQuery("group_id", "0"), 10, 64)
	taskID, taskErr := strconv.ParseInt(ctx.DefaultQuery("task_id", "0"), 10, 64)
	if pageErr != nil || groupErr != nil || taskErr != nil {
		httpx.ErrorFrom(ctx, site.ErrPelicanInvalid)
		return
	}
	filter := site.PelicanFilter{Page: page, GroupID: groupID, TaskID: taskID, Model: ctx.Query("model"), Date: ctx.Query("date"), Public: public}
	items, err := handler.service.Results(ctx.Request.Context(), filter)
	if err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	httpx.Success(ctx, items)
}

func (handler *PelicanHandler) result(ctx *gin.Context, public bool) {
	id, ok := pelicanID(ctx)
	if !ok {
		return
	}
	item, err := handler.service.Result(ctx.Request.Context(), id, public)
	if err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	ctx.Header("Cache-Control", "no-store")
	httpx.Success(ctx, item)
}

// publish applies an explicit publication choice to an existing result.
// The identifier must be positive and the JSON body must contain a non-null
// published field; service validation controls which results may become public.
func (handler *PelicanHandler) publish(ctx *gin.Context) {
	id, ok := pelicanID(ctx)
	if !ok {
		return
	}
	var request PublishPelicanResultRequest
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 1024)
	if err := ctx.ShouldBindJSON(&request); err != nil || request.Published == nil {
		httpx.ErrorFrom(ctx, site.ErrPelicanInvalid)
		return
	}
	if err := handler.service.Publish(ctx.Request.Context(), id, *request.Published); err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	httpx.Success(ctx, gin.H{"updated": true})
}

func (handler *PelicanHandler) deleteResult(ctx *gin.Context) {
	id, ok := pelicanID(ctx)
	if !ok {
		return
	}
	if err := handler.service.DeleteResult(ctx.Request.Context(), id); err != nil {
		httpx.ErrorFrom(ctx, err)
		return
	}
	httpx.Success(ctx, gin.H{"deleted": true})
}

func pelicanID(ctx *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(ctx.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.ErrorFrom(ctx, site.ErrPelicanInvalid)
		return 0, false
	}
	return id, true
}
