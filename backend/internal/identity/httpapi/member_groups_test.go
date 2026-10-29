package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
)

type memberGroupHTTPStore struct {
	identity.MemberGroupRepository
	changed bool
	actor   int64
}

func (store *memberGroupHTTPStore) ChangeMembers(_ context.Context, actor int64, _ string, _ []int64, _ bool) error {
	store.changed, store.actor = true, actor
	return nil
}

// TestMemberGroupMutationRequiresSubject submits a membership change without
// an authenticated principal. The handler must reject the request before the
// repository records any membership update or actor.
func TestMemberGroupMutationRequiresSubject(t *testing.T) {
	store := &memberGroupHTTPStore{}
	handler := &MemberGroupHandler{Groups: &identity.MemberGroups{Repository: store}}
	router := gin.New()
	RegisterMemberGroupAdminRoutes(router.Group("/api/v1/admin"), handler)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/identity-groups/bonus_5/members", strings.NewReader(`{"user_ids":[2],"action":"add"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.False(t, store.changed)
}

// TestMemberGroupMutationUsesAuthenticatedActor submits a forged actor field
// alongside a valid membership change. The repository must receive the
// authenticated administrator identifier when the update succeeds.
func TestMemberGroupMutationUsesAuthenticatedActor(t *testing.T) {
	store := &memberGroupHTTPStore{}
	handler := &MemberGroupHandler{Groups: &identity.MemberGroups{Repository: store}}
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		authctx.SetPrincipal(ctx, identity.Principal{UserID: 42}, 1, "admin@test.local")
		ctx.Next()
	})
	RegisterMemberGroupAdminRoutes(router.Group("/api/v1/admin"), handler)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/identity-groups/bonus_5/members", strings.NewReader(`{"user_ids":[2],"action":"add","actor_id":999}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.True(t, store.changed)
	require.EqualValues(t, 42, store.actor)
}
