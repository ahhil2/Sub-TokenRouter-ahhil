package httpapi

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

type MemberGroupRates interface {
	IdentityRates(context.Context) ([]billing.IdentityRate, error)
	UpdateIdentityRates(context.Context, int64, []billing.IdentityRateInput) error
}

type MemberGroupHandler struct {
	Groups          *identity.MemberGroups
	Rates           MemberGroupRates
	InvalidateRates func(context.Context, []billing.IdentityRateInput)
}

func RegisterMemberGroupAdminRoutes(admin *gin.RouterGroup, handler *MemberGroupHandler) {
	groups := admin.Group("/identity-groups")
	groups.GET("", handler.List)
	groups.GET("/users", handler.Users)
	groups.GET("/:code/members", handler.Members)
	groups.PUT("/:code/members", handler.Change)
	groups.GET("/:code/rates", handler.GetRates)
	groups.PUT("/:code/rates", handler.SetRates)
}

func (handler *MemberGroupHandler) List(ctx *gin.Context) {
	groups, err := handler.Groups.Repository.ListGroups(ctx.Request.Context())
	if httpx.ErrorFrom(ctx, err) {
		return
	}
	httpx.Success(ctx, groups)
}

func (handler *MemberGroupHandler) Users(ctx *gin.Context) {
	handler.listMembers(ctx, "")
}

func (handler *MemberGroupHandler) Members(ctx *gin.Context) {
	handler.listMembers(ctx, ctx.Param("code"))
}

func (handler *MemberGroupHandler) listMembers(ctx *gin.Context, code string) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	members, total, err := handler.Groups.Members(ctx.Request.Context(), code, ctx.Query("search"), page, size)
	if httpx.ErrorFrom(ctx, err) {
		return
	}
	httpx.Success(ctx, gin.H{"items": members, "total": total})
}

func (handler *MemberGroupHandler) Change(ctx *gin.Context) {
	var input struct {
		UserIDs []int64 `json:"user_ids" binding:"required,min=1,max=500"`
		Action  string  `json:"action" binding:"required,oneof=add remove"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(ctx, "invalid membership change")
		return
	}
	subject, ok := authctx.GetAuthSubjectFromContext(ctx)
	if !ok {
		httpx.Unauthorized(ctx, "authentication required")
		return
	}
	err := handler.Groups.Change(ctx.Request.Context(), subject.UserID, ctx.Param("code"), input.UserIDs, input.Action == "add")
	if httpx.ErrorFrom(ctx, err) {
		return
	}
	httpx.Success(ctx, gin.H{"updated": true})
}

func (handler *MemberGroupHandler) GetRates(ctx *gin.Context) {
	if ctx.Param("code") != "ahhilai" {
		httpx.BadRequest(ctx, "pricing identity required")
		return
	}
	rates, err := handler.Rates.IdentityRates(ctx.Request.Context())
	if httpx.ErrorFrom(ctx, err) {
		return
	}
	httpx.Success(ctx, rates)
}

func (handler *MemberGroupHandler) SetRates(ctx *gin.Context) {
	if ctx.Param("code") != "ahhilai" {
		httpx.BadRequest(ctx, "pricing identity required")
		return
	}
	var input struct {
		Rates []billing.IdentityRateInput `json:"rates" binding:"required,max=1000"`
	}
	if err := ctx.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(ctx, "invalid identity rates")
		return
	}
	subject, ok := authctx.GetAuthSubjectFromContext(ctx)
	if !ok {
		httpx.Unauthorized(ctx, "authentication required")
		return
	}
	if httpx.ErrorFrom(ctx, handler.Rates.UpdateIdentityRates(ctx.Request.Context(), subject.UserID, input.Rates)) {
		return
	}
	if handler.InvalidateRates != nil {
		handler.InvalidateRates(ctx.Request.Context(), input.Rates)
	}
	httpx.Success(ctx, gin.H{"updated": true})
}

func (handler *MemberGroupHandler) Self(ctx *gin.Context) {
	subject, ok := authctx.GetAuthSubjectFromContext(ctx)
	if !ok {
		httpx.Unauthorized(ctx, "authentication required")
		return
	}
	groups, err := handler.Groups.Repository.Memberships(ctx.Request.Context(), []int64{subject.UserID})
	if httpx.ErrorFrom(ctx, err) {
		return
	}
	codes := groups[subject.UserID]
	if codes == nil {
		codes = []string{}
	}
	rate := 0.0
	for _, code := range codes {
		if code == "bonus_5" {
			rate = 0.05
		}
		if code == "bonus_10" {
			rate = 0.10
		}
	}
	httpx.Success(ctx, gin.H{"identity_groups": codes, "bonus_rate": rate})
}
