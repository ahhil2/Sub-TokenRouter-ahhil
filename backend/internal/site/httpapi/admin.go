package httpapi

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/pkg/locale"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

// admin.go serves administrator announcement operations and converts request
// schedules for the service.
// Administrator announcement JSON carries Unix-second schedule values, while
// the service uses optional time pointers for saved endpoints. Creation treats
// nonpositive values as absent; updates reserve zero for clearing and retain
// omission separately from a replacement timestamp.
//
// Authenticated actor identity supplies audit fields after request binding.
// Source-copy updates pass through localization revision checks in the service,
// while list requests retain the requested sort options for repository
// normalization. Search terms are trimmed and capped in bytes before querying,
// and schedule conversion keeps the wire format's creation and update rules
// distinct even when both endpoints are changed in one request.

// AdminAnnouncementHandler maps administrator announcement requests to domain publication and read-status operations.
type AdminAnnouncementHandler struct {
	announcementService *site.AnnouncementService // publication and reader operations
}

// CreateAnnouncementRequest supplies revisioned copy, publication fields, and Unix-second schedule input.
type CreateAnnouncementRequest struct {
	// Localization is required during JSON binding and supplies the revisioned
	// title/body copy used by the service before validating the base record.
	Localization *locale.Update[site.AnnouncementCopy] `json:"localization" binding:"required"`
	Title        string                                `json:"title"`
	Content      string                                `json:"content"`
	Status       string                                `json:"status" binding:"omitempty,oneof=draft active archived"`
	NotifyMode   string                                `json:"notify_mode" binding:"omitempty,oneof=silent popup banner"`
	Targeting    site.AnnouncementTargeting            `json:"targeting"`
	StartsAt     *int64                                `json:"starts_at"` // Unix seconds, 0/empty = immediate
	EndsAt       *int64                                `json:"ends_at"`   // Unix seconds, 0/empty = never
}

// UpdateAnnouncementRequest supplies selected edits and optional Unix-second schedule replacements or clears.
type UpdateAnnouncementRequest struct {
	// Localization carries revisioned copy edits; raw Title or Content edits
	// without it are rejected by Update's conflict check in the service.
	Localization *locale.Update[site.AnnouncementCopy] `json:"localization"`
	Title        *string                               `json:"title"`
	Content      *string                               `json:"content"`
	Status       *string                               `json:"status" binding:"omitempty,oneof=draft active archived"`
	NotifyMode   *string                               `json:"notify_mode" binding:"omitempty,oneof=silent popup banner"`
	Targeting    *site.AnnouncementTargeting           `json:"targeting"`
	StartsAt     *int64                                `json:"starts_at"` // Unix seconds, 0 = clear
	EndsAt       *int64                                `json:"ends_at"`   // Unix seconds, 0 = clear
}

// NewAdminAnnouncementHandler binds the service used for administrator announcement HTTP operations.
func NewAdminAnnouncementHandler(announcementService *site.AnnouncementService) *AdminAnnouncementHandler {
	return &AdminAnnouncementHandler{
		announcementService: announcementService,
	}
}

// List returns announcements matching the requested filters and sort order.
// Search and status are trimmed, and search is capped at two hundred bytes.
// Sort defaults are created_at descending; the service archives expired records
// before the repository supplies a page and its total.
// GET /api/v1/admin/announcements
func (h *AdminAnnouncementHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	status := strings.TrimSpace(c.Query("status"))
	search := strings.TrimSpace(c.Query("search"))
	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")
	if len(search) > 200 {
		// Cap source-search bytes before handing them to storage filters.
		search = search[:200]
	}

	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	items, paginationResult, err := h.announcementService.List(
		c.Request.Context(),
		params,
		site.AnnouncementListFilters{Status: status, Search: search},
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	out := make([]Announcement, 0, len(items))
	for i := range items {
		out = append(out, *AnnouncementFromService(&items[i]))
	}
	response.Paginated(c, out, paginationResult.Total, page, pageSize)
}

// GetByID returns an administrator announcement after validating its positive decimal identifier.
// Service failures use the shared error response. The service's detail read
// archives expired records before returning the requested announcement representation.
// GET /api/v1/admin/announcements/:id
func (h *AdminAnnouncementHandler) GetByID(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	item, err := h.announcementService.GetByID(c.Request.Context(), announcementID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, AnnouncementFromService(item))
}

// Create creates an announcement attributed to the authenticated actor.
// Localization is required during binding, and actor identity must exist in
// authentication context. Positive Unix-second endpoints become time pointers;
// omitted or nonpositive schedule values remain absent for service validation.
// POST /api/v1/admin/announcements
func (h *AdminAnnouncementHandler) Create(c *gin.Context) {
	var req CreateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	input := &site.CreateAnnouncementInput{
		Localization: req.Localization,
		Title:        req.Title,
		Content:      req.Content,
		Status:       req.Status,
		NotifyMode:   req.NotifyMode,
		Targeting:    req.Targeting,
		ActorID:      &subject.UserID,
	}

	// Creation treats both omitted and nonpositive endpoint numbers as absence.
	// Convert the positive start/end values together before the service validates their resulting interval.
	if req.StartsAt != nil && *req.StartsAt > 0 {
		t := time.Unix(*req.StartsAt, 0)
		input.StartsAt = &t
	}
	if req.EndsAt != nil && *req.EndsAt > 0 {
		t := time.Unix(*req.EndsAt, 0)
		input.EndsAt = &t
	}

	created, err := h.announcementService.Create(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, AnnouncementFromService(created))
}

// Update applies selected fields while preserving schedule omission, clearing, and replacement intent.
// It requires a positive ID and authenticated actor, then converts supplied
// Unix-second endpoints into the service's double-pointer representation. Domain
// errors are returned through the shared response mapper.
//
// An omitted endpoint leaves its input pointer nil. Zero supplies a pointer
// to a nil time, while any nonzero number supplies a pointer to that instant,
// including negative timestamps. This keeps clearing distinct from both
// preserving a saved endpoint and assigning a pre-epoch time.
// PUT /api/v1/admin/announcements/:id
func (h *AdminAnnouncementHandler) Update(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	var req UpdateAnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	input := &site.UpdateAnnouncementInput{
		Localization: req.Localization,
		Title:        req.Title,
		Content:      req.Content,
		Status:       req.Status,
		NotifyMode:   req.NotifyMode,
		Targeting:    req.Targeting,
		ActorID:      &subject.UserID,
	}

	// Preserve the start endpoint's wire-level intent when converting to the
	// service type: omission, explicit zero, and nonzero instants require distinct pointer states.
	if req.StartsAt != nil {
		if *req.StartsAt == 0 {
			var cleared *time.Time
			input.StartsAt = &cleared
		} else {
			t := time.Unix(*req.StartsAt, 0)
			ptr := &t
			input.StartsAt = &ptr
		}
	}

	// End edits use the same three pointer states independently of start edits,
	// so changing one endpoint cannot implicitly clear or replace the other endpoint.
	if req.EndsAt != nil {
		if *req.EndsAt == 0 {
			var cleared *time.Time
			input.EndsAt = &cleared
		} else {
			t := time.Unix(*req.EndsAt, 0)
			ptr := &t
			input.EndsAt = &ptr
		}
	}

	updated, err := h.announcementService.Update(c.Request.Context(), announcementID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, AnnouncementFromService(updated))
}

// Delete removes an announcement after requiring a positive decimal record identifier.
// Service failures use the shared error response; successful deletion writes
// the endpoint's acknowledgement object through the normal success envelope.
// DELETE /api/v1/admin/announcements/:id
func (h *AdminAnnouncementHandler) Delete(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	if err := h.announcementService.Delete(c.Request.Context(), announcementID); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Announcement deleted successfully"})
}

// ListReadStatus returns an administrator user page with targeting eligibility and first-read timestamps.
// It requires a positive announcement ID, trims and caps search in bytes,
// and defaults user ordering to email ascending before delegating to the service.
// GET /api/v1/admin/announcements/:id/read-status
func (h *AdminAnnouncementHandler) ListReadStatus(c *gin.Context) {
	announcementID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || announcementID <= 0 {
		response.BadRequest(c, "Invalid announcement ID")
		return
	}

	page, pageSize := response.ParsePagination(c)
	params := pagination.PaginationParams{
		Page:      page,
		PageSize:  pageSize,
		SortBy:    c.DefaultQuery("sort_by", "email"),
		SortOrder: c.DefaultQuery("sort_order", "asc"),
	}
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 200 {
		// User search shares the administrator endpoint's byte-length query cap.
		search = search[:200]
	}

	items, paginationResult, err := h.announcementService.ListUserReadStatus(
		c.Request.Context(),
		announcementID,
		params,
		search,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}
