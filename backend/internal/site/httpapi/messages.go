package httpapi

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

type MessageHandler struct {
	service *site.MessageService
}

func NewMessageHandler(service *site.MessageService) *MessageHandler {
	return &MessageHandler{service: service}
}

func RegisterMessageRoutes(authenticated *gin.RouterGroup, handler *MessageHandler, admin, heavy gin.HandlerFunc) {
	messages := authenticated.Group("/site-messages")
	messages.GET("/summary", handler.Summary)
	messages.GET("/conversations", handler.Conversations)
	messages.GET("/contacts", admin, handler.Contacts)
	messages.GET("/conversations/:user_id", handler.Thread)
	messages.POST("/conversations/:user_id/read", handler.MarkRead)
	messages.POST("", heavy, handler.Send)
}

func messageActor(context *gin.Context) (site.MessageActor, bool) {
	subject, authenticated := authctx.GetAuthSubjectFromContext(context)
	role, hasRole := authctx.GetUserRoleFromContext(context)
	if !authenticated || !hasRole || subject.UserID <= 0 {
		httpx.Unauthorized(context, "Authentication required")
		return site.MessageActor{}, false
	}
	return site.MessageActor{ID: subject.UserID, Admin: role == "admin"}, true
}

func (handler *MessageHandler) Summary(context *gin.Context) {
	actor, ok := messageActor(context)
	if !ok {
		return
	}
	count, err := handler.service.Unread(context.Request.Context(), actor)
	if err != nil {
		httpx.ErrorFrom(context, err)
		return
	}
	httpx.Success(context, gin.H{"unread_count": count})
}

func (handler *MessageHandler) Conversations(context *gin.Context) {
	actor, ok := messageActor(context)
	if !ok {
		return
	}
	page, err := strconv.Atoi(context.DefaultQuery("page", "1"))
	if err != nil {
		httpx.ErrorFrom(context, site.ErrMessageInvalid)
		return
	}
	result, err := handler.service.Conversations(context.Request.Context(), actor, context.Query("search"), page)
	if err != nil {
		httpx.ErrorFrom(context, err)
		return
	}
	httpx.Success(context, result)
}

func (handler *MessageHandler) Contacts(context *gin.Context) {
	actor, ok := messageActor(context)
	if !ok {
		return
	}
	page, err := strconv.Atoi(context.DefaultQuery("page", "1"))
	if err != nil {
		httpx.ErrorFrom(context, site.ErrMessageInvalid)
		return
	}
	result, err := handler.service.Contacts(context.Request.Context(), actor, context.Query("search"), page)
	if err != nil {
		httpx.ErrorFrom(context, err)
		return
	}
	httpx.Success(context, result)
}

func (handler *MessageHandler) Thread(context *gin.Context) {
	actor, ok := messageActor(context)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(context.Param("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		httpx.ErrorFrom(context, site.ErrMessageInvalid)
		return
	}
	before, err := strconv.ParseInt(context.DefaultQuery("before", "0"), 10, 64)
	if err != nil {
		httpx.ErrorFrom(context, site.ErrMessageInvalid)
		return
	}
	result, err := handler.service.Thread(context.Request.Context(), actor, userID, before)
	if err != nil {
		httpx.ErrorFrom(context, err)
		return
	}
	httpx.Success(context, result)
}

func (handler *MessageHandler) MarkRead(context *gin.Context) {
	actor, ok := messageActor(context)
	if !ok {
		return
	}
	userID, err := strconv.ParseInt(context.Param("user_id"), 10, 64)
	if err != nil || userID <= 0 {
		httpx.ErrorFrom(context, site.ErrMessageInvalid)
		return
	}
	var request struct {
		ThroughID int64 `json:"through_id"`
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 1024)
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.ErrorFrom(context, site.ErrMessageInvalid)
		return
	}
	if err := handler.service.MarkRead(context.Request.Context(), actor, userID, request.ThroughID); err != nil {
		httpx.ErrorFrom(context, err)
		return
	}
	httpx.Success(context, gin.H{"message": "ok"})
}

func (handler *MessageHandler) Send(context *gin.Context) {
	actor, ok := messageActor(context)
	if !ok {
		return
	}
	var request site.SendMessageRequest
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 32*1024)
	if err := context.ShouldBindJSON(&request); err != nil {
		httpx.ErrorFrom(context, site.ErrMessageInvalid)
		return
	}
	count, err := handler.service.Send(context.Request.Context(), actor, request)
	if err != nil {
		httpx.ErrorFrom(context, err)
		return
	}
	httpx.Success(context, gin.H{"recipient_count": count})
}
