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

func TestMemberGroupMutationRequiresSubject(t *testing.T) {
	store := &memberGroupHTTPStore{}
	handler := &MemberGroupHandler{Groups: &identity.MemberGroups{Repository: store}}
	router := gin.New()
	RegisterMemberGroupAdminRoutes(router.Group("/admin"), handler)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/admin/identity-groups/bonus_5/members", strings.NewReader(`{"user_ids":[2],"action":"add"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.False(t, store.changed)
}

func TestMemberGroupMutationUsesAuthenticatedActor(t *testing.T) {
	store := &memberGroupHTTPStore{}
	handler := &MemberGroupHandler{Groups: &identity.MemberGroups{Repository: store}}
	router := gin.New()
	router.Use(func(ctx *gin.Context) {
		authctx.SetPrincipal(ctx, identity.Principal{UserID: 42}, 1, "admin@test.local")
		ctx.Next()
	})
	RegisterMemberGroupAdminRoutes(router.Group("/admin"), handler)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/admin/identity-groups/bonus_5/members", strings.NewReader(`{"user_ids":[2],"action":"add","actor_id":999}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.True(t, store.changed)
	require.EqualValues(t, 42, store.actor)
}
