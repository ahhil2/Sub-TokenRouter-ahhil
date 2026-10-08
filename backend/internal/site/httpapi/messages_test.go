package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/site"
)

func TestMessageHTTPAuthorization(t *testing.T) {
	tests := []struct {
		name, method, path, body, role string
		id                             int64
		status                         int
	}{
		{"anonymous", "GET", "/site-messages/summary", "", "", 0, http.StatusUnauthorized},
		{"read other thread", "GET", "/site-messages/conversations/3", "", "user", 2, http.StatusForbidden},
		{"mark other thread", "POST", "/site-messages/conversations/3/read", `{"through_id":1}`, "user", 2, http.StatusForbidden},
		{"enumerate contacts", "GET", "/site-messages/contacts", "", "user", 2, http.StatusForbidden},
		{"spoof admin", "POST", "/site-messages", `{"body":"hello","client_id":"019bb611-0872-7000-8000-000000000001","recipient_ids":[3],"is_admin":true,"sender_id":1}`, "user", 2, http.StatusForbidden},
		{"oversized body", "POST", "/site-messages", `{"body":"` + strings.Repeat("a", 33*1024) + `"}`, "user", 2, http.StatusBadRequest},
		{"malformed id", "GET", "/site-messages/conversations/not-a-number", "", "admin", 1, http.StatusBadRequest},
		{"negative cursor", "GET", "/site-messages/conversations/2?before=-1", "", "admin", 1, http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(context *gin.Context) {
				if test.id > 0 {
					context.Set(authctx.ContextKeyUser, authctx.AuthSubject{UserID: test.id})
					context.Set(authctx.ContextKeyUserRole, test.role)
				}
				context.Next()
			})
			noop := func(context *gin.Context) { context.Next() }
			RegisterMessageRoutes(router.Group(""), NewMessageHandler(site.NewMessageService(nil)), noop, noop)
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, test.status, response.Code, response.Body.String())
			assert.NotContains(t, response.Body.String(), "panic")
		})
	}
}
