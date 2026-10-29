package httpapi

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/server/adminapi"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// member_groups.go serves identity memberships and group-specific pricing rates.
// Administrator mutations use the authenticated subject for attribution and
// delegate membership and rate validation to their services. Shared request
// types keep HTTP binding and MCP input descriptions consistent.

// ChangeMemberGroupRequest selects users and the membership operation to apply.
type ChangeMemberGroupRequest struct {
	UserIDs []int64 `json:"user_ids" binding:"required,min=1,max=500"`  // selected user identifiers
	Action  string  `json:"action" binding:"required,oneof=add remove"` // membership change direction
}

// SetMemberGroupRatesRequest carries the identity pricing overrides to persist.
type SetMemberGroupRatesRequest struct {
	Rates []billing.IdentityRateInput `json:"rates" binding:"required,max=1000"` // requested pricing overrides
}

type MemberGroupRates interface {
	IdentityRates(context.Context) ([]billing.IdentityRate, error)
	UpdateIdentityRates(context.Context, int64, []billing.IdentityRateInput) error
}

type MemberGroupHandler struct {
	Groups          *identity.MemberGroups
	Rates           MemberGroupRates
	InvalidateRates func(context.Context, []billing.IdentityRateInput)
}

// RegisterMemberGroupAdminRoutes mounts identity membership and pricing operations.
// Callers provide an authenticated, audited /api/v1/admin group. The shared
// management descriptions retain the handlers' request validation and actor attribution.
func RegisterMemberGroupAdminRoutes(admin *gin.RouterGroup, handler *MemberGroupHandler) {
	groups := admin.Group("/identity-groups")
	adminapi.Register(groups, ManagementOperations["identity_groups.list"], handler.List)
	adminapi.Register(groups, ManagementOperations["identity_groups.users.list"], handler.Users)
	adminapi.Register(groups, ManagementOperations["identity_groups.members.list"], handler.Members)
	adminapi.Register(groups, ManagementOperations["identity_groups.members.update"], handler.Change)
	adminapi.Register(groups, ManagementOperations["identity_groups.rates.get"], handler.GetRates)
	adminapi.Register(groups, ManagementOperations["identity_groups.rates.update"], handler.SetRates)
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

// Change adds or removes the selected users from the requested identity group.
// The authenticated subject supplies the audit actor. Binding and service
// validation reject invalid requests before a successful acknowledgement is returned.
func (handler *MemberGroupHandler) Change(ctx *gin.Context) {
	var input ChangeMemberGroupRequest
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

// SetRates stores pricing overrides for the ahhilai identity using the authenticated actor.
// It rejects other identity codes and invalid request bodies, then invalidates
// configured rate readers after the service has persisted the update.
func (handler *MemberGroupHandler) SetRates(ctx *gin.Context) {
	if ctx.Param("code") != "ahhilai" {
		httpx.BadRequest(ctx, "pricing identity required")
		return
	}
	var input SetMemberGroupRatesRequest
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
