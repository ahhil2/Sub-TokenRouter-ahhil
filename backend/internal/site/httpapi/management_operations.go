package httpapi

import (
	"github.com/TokenFlux/TokenRouter/internal/server/adminapi"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

// management_operations.go describes registered administrative HTTP operations for
// MCP discovery and permission selection. Request schemas reference the same input
// types decoded by handlers; metadata records query arguments, side effects, and
// browser confirmation requirements before the shared executor admits a call.

// ManagementOperations supplies stable permission identities and parameter descriptions
// for this module. Registrars mount these descriptions together with existing HTTP handlers.
var ManagementOperations = map[string]*adminapi.Operation{
	"pelican.tasks.list": adminapi.Define(adminapi.Operation{
		ID: "pelican.tasks.list", Module: "pelican", Title: "pelican tasks list",
		Description: "List configured animation generation tasks.",
		Method:      "GET", Path: "/api/v1/admin/pelican-tests", ReadOnly: true,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests", []string{}, nil),
	}),
	"pelican.tasks.create": adminapi.Define(adminapi.Operation{
		ID: "pelican.tasks.create", Module: "pelican", Title: "pelican tasks create",
		Description: "Create a generation task. Enabling execution requires public consent; generation incurs platform provider costs. Body identifiers and stored execution state are managed by the service.",
		Method:      "POST", Path: "/api/v1/admin/pelican-tests", ReadOnly: false,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests", []string{}, (*site.PelicanTask)(nil)),
	}),
	"pelican.tasks.update": adminapi.Define(adminapi.Operation{
		ID: "pelican.tasks.update", Module: "pelican", Title: "pelican tasks update",
		Description: "Update a generation task using its path identifier. Public consent is required for enabled tasks; active tasks cannot be edited.",
		Method:      "PUT", Path: "/api/v1/admin/pelican-tests/:id", ReadOnly: false,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests/:id", []string{}, (*site.PelicanTask)(nil)),
	}),
	"pelican.tasks.delete": adminapi.Define(adminapi.Operation{
		ID: "pelican.tasks.delete", Module: "pelican", Title: "pelican tasks delete",
		Description: "Delete an inactive generation task and its stored results.",
		Method:      "DELETE", Path: "/api/v1/admin/pelican-tests/:id", ReadOnly: false,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests/:id", []string{}, nil),
	}),
	"pelican.tasks.run": adminapi.Define(adminapi.Operation{
		ID: "pelican.tasks.run", Module: "pelican", Title: "pelican tasks run",
		Description: "Queue a generation task with public consent. The response acknowledges queuing; provider execution occurs asynchronously and incurs platform costs.",
		Method:      "POST", Path: "/api/v1/admin/pelican-tests/:id/run", ReadOnly: false,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests/:id/run", []string{}, nil),
	}),
	"pelican.results.list": adminapi.Define(adminapi.Operation{
		ID: "pelican.results.list", Module: "pelican", Title: "pelican results list",
		Description: "List retained results, including unpublished records, with pagination and optional task, group, model, and Beijing-date filters.",
		Method:      "GET", Path: "/api/v1/admin/pelican-tests/results", ReadOnly: true,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests/results", []string{"page", "task_id", "group_id", "model", "date"}, nil),
	}),
	"pelican.results.get": adminapi.Define(adminapi.Operation{
		ID: "pelican.results.get", Module: "pelican", Title: "pelican results get",
		Description: "Read a retained result including its generated content and administrator diagnostics.",
		Method:      "GET", Path: "/api/v1/admin/pelican-tests/results/:id", ReadOnly: true,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests/results/:id", []string{}, nil),
	}),
	"pelican.results.publish": adminapi.Define(adminapi.Operation{
		ID: "pelican.results.publish", Module: "pelican", Title: "pelican results publish",
		Description: "Set result visibility using an explicit non-null published boolean. Running results cannot be published.",
		Method:      "PATCH", Path: "/api/v1/admin/pelican-tests/results/:id", ReadOnly: false,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests/results/:id", []string{}, (*PublishPelicanResultRequest)(nil)),
	}),
	"pelican.results.delete": adminapi.Define(adminapi.Operation{
		ID: "pelican.results.delete", Module: "pelican", Title: "pelican results delete",
		Description: "Delete a retained generation result.",
		Method:      "DELETE", Path: "/api/v1/admin/pelican-tests/results/:id", ReadOnly: false,
		Input: adminapi.Arguments("/api/v1/admin/pelican-tests/results/:id", []string{}, nil),
	}),
	"pages.list": adminapi.Define(adminapi.Operation{ID: "pages.list", Module: "pages", Title: "List administrator pages", Description: "List the Markdown pages available to the authenticated administrator. Page content and images remain available through their existing browser routes.", Method: "GET", Path: "/api/v1/pages", ReadOnly: true, Input: adminapi.Arguments("/api/v1/pages", nil, nil)}),
	"announcements.create": adminapi.Define(adminapi.Operation{
		ID:     "announcements.create",
		Module: "announcements",
		Title:  "Create",
		Description: "Create creates an announcement attributed to the authenticated actor. Localization is " +
			"required during binding, and actor identity must exist in authentication context. Positive " +
			"Unix-second endpoints become time pointers; omitted or nonpositive schedule values remain " +
			"absent for service validation. Omitted body fields follow the HTTP handler defaults; " +
			"explicit null and empty collections retain their documented meanings.",
		Method:      "POST",
		Path:        "/api/v1/admin/announcements",
		Response:    "json",
		BrowserPath: "/admin/announcements",
		Input:       adminapi.Arguments("/api/v1/admin/announcements", []string{}, (*CreateAnnouncementRequest)(nil)),
	}),
	"announcements.delete": adminapi.Define(adminapi.Operation{
		ID:      "announcements.delete",
		Module:  "announcements",
		Title:   "Delete announcement",
		TitleZH: "删除公告",
		Description: "Delete removes an announcement after requiring a positive decimal record identifier. Service " +
			"failures use the shared error response; successful deletion writes the endpoint's " +
			"acknowledgement object through the normal success envelope.",
		Method:      "DELETE",
		Path:        "/api/v1/admin/announcements/:id",
		Response:    "json",
		BrowserPath: "/admin/announcements",
		Input:       adminapi.Arguments("/api/v1/admin/announcements/:id", []string{}, nil),
	}),
	"announcements.get": adminapi.Define(adminapi.Operation{
		ID:      "announcements.get",
		Module:  "announcements",
		Title:   "Get announcement details",
		TitleZH: "查看公告详情",
		Description: "GetByID returns an administrator announcement after validating its positive decimal " +
			"identifier. Service failures use the shared error response. The service's detail read " +
			"archives expired records before returning the requested announcement representation. This " +
			"operation is available through read_operation.",
		Method:      "GET",
		Path:        "/api/v1/admin/announcements/:id",
		ReadOnly:    true,
		Response:    "json",
		BrowserPath: "/admin/announcements",
		Input:       adminapi.Arguments("/api/v1/admin/announcements/:id", []string{}, nil),
	}),
	"announcements.list": adminapi.Define(adminapi.Operation{
		ID:      "announcements.list",
		Module:  "announcements",
		Title:   "List announcement records",
		TitleZH: "查询公告",
		Description: "List returns announcements matching the requested filters and sort order. Search and status " +
			"are trimmed, and search is capped at two hundred bytes. Sort defaults are created_at " +
			"descending; the service archives expired records before the repository supplies a page and " +
			"its total. This operation is available through read_operation.",
		Method:      "GET",
		Path:        "/api/v1/admin/announcements",
		ReadOnly:    true,
		Response:    "json",
		BrowserPath: "/admin/announcements",
		Input:       adminapi.Arguments("/api/v1/admin/announcements", []string{"page", "page_size", "search", "sort_by", "sort_order", "status"}, nil),
	}),
	"announcements.read_status.list": adminapi.Define(adminapi.Operation{
		ID:     "announcements.read_status.list",
		Module: "announcements",
		Title:  "List Read Status",
		Description: "ListReadStatus returns an administrator user page with targeting eligibility and first-read " +
			"timestamps. It requires a positive announcement ID, trims and caps search in bytes, and " +
			"defaults user ordering to email ascending before delegating to the service. This operation " +
			"is available through read_operation.",
		Method:      "GET",
		Path:        "/api/v1/admin/announcements/:id/read-status",
		ReadOnly:    true,
		Response:    "json",
		BrowserPath: "/admin/announcements",
		Input:       adminapi.Arguments("/api/v1/admin/announcements/:id/read-status", []string{"page", "page_size", "search", "sort_by", "sort_order"}, nil),
	}),
	"announcements.update": adminapi.Define(adminapi.Operation{
		ID:      "announcements.update",
		Module:  "announcements",
		Title:   "Update announcement",
		TitleZH: "修改公告",
		Description: "Update applies selected fields while preserving schedule omission, clearing, and replacement " +
			"intent. It requires a positive ID and authenticated actor, then converts supplied " +
			"Unix-second endpoints into the service's double-pointer representation. Domain errors are " +
			"returned through the shared response mapper.  An omitted endpoint leaves its input pointer " +
			"nil. Zero supplies a pointer to a nil time, while any nonzero number supplies a pointer to " +
			"that instant, including negative timestamps. This keeps clearing distinct from both " +
			"preserving a saved endpoint and assigning a pre-epoch time. Omitted body fields follow the " +
			"HTTP handler defaults; explicit null and empty collections retain their documented meanings.",
		Method:      "PUT",
		Path:        "/api/v1/admin/announcements/:id",
		Response:    "json",
		BrowserPath: "/admin/announcements",
		Input:       adminapi.Arguments("/api/v1/admin/announcements/:id", []string{}, (*UpdateAnnouncementRequest)(nil)),
	}),
}
