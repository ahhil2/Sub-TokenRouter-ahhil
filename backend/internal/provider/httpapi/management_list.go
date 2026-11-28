package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// management_list.go assembles paginated provider responses and computes
// entity tags for conditional requests.
// Provider list responses pair paginated configuration with loaded runtime
// observations and a JSON-derived entity tag. The list use case owns database
// sorting and optional scheduler scoring, while this handler converts its
// loaded states, enriches parent hints and hashes the exact response items.
// Search normalization also becomes part of the tag input.
//
// Conditional requests compare against that assembled response after the list
// query completes. The lite flag is parsed into the tag input but the same item
// view is returned for either value. Encoding completes before any validator is sent.

// providerListGroupUngroupedQueryValue names the list query spelling for
// selecting providers with no routing group association.
const providerListGroupUngroupedQueryValue = "ungrouped"

// List queries a paginated provider selection and presents loaded runtime
// observations with parent hints. Search is trimmed and truncated to one
// hundred bytes, group=ungrouped maps to its sentinel, and negative or
// malformed group identifiers return 400. Optional scheduler scoring is
// requested by its separate query flag.
//
// The ETag hashes the assembled item view, pagination and selected query values
// after loading. A matching conditional header returns 304. Lite changes the
// fingerprint value but
// leaves the returned item shape unchanged, so callers should treat it as a tag
// input rather than a DTO omission rule.
// GET /api/v1/admin/providers
func (h *ManagementHandler) List(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
	platform := c.Query("platform")
	providerType := c.Query("type")
	status := c.Query("status")
	search := c.Query("search")
	privacyMode := strings.TrimSpace(c.Query("privacy_mode"))
	sortBy := c.DefaultQuery("sort_by", "name")
	sortOrder := c.DefaultQuery("sort_order", "asc")
	// Storage search receives a bounded byte string. Apply the same hundred-byte
	// budget as archive export, retaining byte truncation even across UTF-8 characters.
	search = strings.TrimSpace(search)
	if len(search) > 100 {
		search = search[:100]
	}
	lite := response.ParseBoolQueryWithDefault(c.Query("lite"), false)
	// Scheduler scores require a batch load lookup; compute them when the client requests the score column.
	includeSchedulerScore := response.ParseBoolQueryWithDefault(c.Query("include_scheduler_score"), false)

	var groupID int64
	if groupIDStr := c.Query("group"); groupIDStr != "" {
		if groupIDStr == providerListGroupUngroupedQueryValue {
			groupID = providercore.ProviderListGroupUngrouped
		} else {
			parsedGroupID, parseErr := strconv.ParseInt(groupIDStr, 10, 64)
			if parseErr != nil {
				response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter"))
				return
			}
			if parsedGroupID < 0 {
				response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter"))
				return
			}
			groupID = parsedGroupID
		}
	}

	listing, err := h.listing.List(c.Request.Context(), providercore.ManagementListInput{Page: page, PageSize: pageSize, Platform: platform, ProviderType: providerType, Status: status, Search: search, GroupID: groupID, PrivacyMode: privacyMode, SortBy: sortBy, SortOrder: sortOrder, IncludeSchedulerScore: includeSchedulerScore})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	total := listing.Total
	result := make([]ProviderWithConcurrency, len(listing.Items))
	for i, state := range listing.Items {
		result[i] = h.runtimePresenter.Project(state)
	}
	h.runtimePresenter.EnrichShadowParents(c.Request.Context(), result)

	body, err := json.Marshal(response.PaginatedBody(result, total, page, pageSize))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	etag := providerListETag(body, platform, providerType, status, search, lite)
	c.Header("ETag", etag)
	c.Header("Vary", "If-None-Match")
	if response.IfNoneMatchMatched(c.GetHeader("If-None-Match"), etag) {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// BuildProvidersListETag returns the validator used by List for the supplied page.
// Callers keep items and their reachable values stable while encoding. An
// unencodable page returns an empty tag, leaving conditional matching unavailable.
func BuildProvidersListETag(
	items []ProviderWithConcurrency,
	total int64,
	page, pageSize int,
	platform, providerType, status, search string,
	lite bool,
) string {
	body, err := json.Marshal(response.PaginatedBody(items, total, page, pageSize))
	if err != nil {
		return ""
	}
	return providerListETag(body, platform, providerType, status, search, lite)
}

// providerListETag includes the filter identity alongside the exact transmitted body.
func providerListETag(body []byte, platform, providerType, status, search string, lite bool) string {
	// The selector contains only strings, and SHA-256 writes cannot fail; these
	// operations have no recoverable encoding or write error to propagate.
	selector, _ := json.Marshal([5]string{platform, providerType, status, search, strconv.FormatBool(lite)})
	hash := sha256.New()
	_, _ = hash.Write(selector)
	_, _ = hash.Write(body)
	return `"` + hex.EncodeToString(hash.Sum(nil)) + `"`
}
