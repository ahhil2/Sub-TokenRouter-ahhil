package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/infra/telemetry/timing"
	"github.com/TokenFlux/TokenRouter/internal/moderation"
	"github.com/TokenFlux/TokenRouter/internal/team"
)

// ingressModerationStub models an immediate policy bypass or a service failure.
type ingressModerationStub struct {
	ModerationPort       // unused warning operations
	err            error // check failure result
}

// TestBuildContentModerationInputTeamKeyUsesActorAndKeepsBillingAttribution
// supplies distinct team actor and owner records. It requires actor risk
// identity, owner billing identity and independently copied team ID storage in
// the check input.
func TestBuildContentModerationInputTeamKeyUsesActorAndKeepsBillingAttribution(t *testing.T) {
	c := newContentModerationHelperTestContext()
	teamID := int64(301)
	apiKey := &apikey.APIKey{
		ID:        401,
		UserID:    202,
		TeamID:    &teamID,
		User:      &identity.User{ID: 101, Email: "owner@example.com"},
		ActorUser: &identity.User{ID: 202, Email: "member@example.com"},
	}

	input := BuildContentModerationInput(GatewayModerationEndpoints{}, c, apiKey, authctx.AuthSubject{UserID: 101}, moderation.ContentModerationProtocolOpenAIChat, "gpt-5", []byte(`{"messages":[]}`))

	require.Equal(t, int64(202), input.UserID)
	require.Equal(t, "member@example.com", input.UserEmail)
	require.Equal(t, int64(101), input.BillingUserID)
	require.Equal(t, teamID, *input.TeamID)
	// The moderation snapshot owns its copy of the team ID.
	require.NotSame(t, apiKey.TeamID, input.TeamID)
}

// TestBuildOpenAICyberWarningInputTeamKeyUsesActorEmail supplies separate team
// actor and billing owner evidence. The warning input must retain the member
// email and actor ID while billing and team attribution remain distinct.
func TestBuildOpenAICyberWarningInputTeamKeyUsesActorEmail(t *testing.T) {
	c := newContentModerationHelperTestContext()
	teamID := int64(301)
	apiKey := &apikey.APIKey{
		ID:        401,
		UserID:    202,
		TeamID:    &teamID,
		User:      &identity.User{ID: 101, Email: "owner@example.com"},
		ActorUser: &identity.User{ID: 202, Email: "member@example.com"},
	}

	input := BuildOpenAICyberWarningInput(GatewayModerationEndpoints{}, c, apiKey, nil, "gpt-5", http.StatusBadRequest, nil, "cyber_policy", "prompt")

	require.Equal(t, int64(202), input.UserID)
	require.Equal(t, "member@example.com", input.UserEmail)
	require.Equal(t, int64(101), input.BillingUserID)
	require.Equal(t, teamID, *input.TeamID)
}

// TestResolveContentModerationIdentityPersonalKeyFallsBackToBillingUser
// supplies a personal Key and matching subject. It requires the same
// risk/billing identity with its user email and no team identifier.
func TestResolveContentModerationIdentityPersonalKeyFallsBackToBillingUser(t *testing.T) {
	apiKey := &apikey.APIKey{
		UserID: 303,
		User:   &identity.User{ID: 303, Email: "personal@example.com"},
	}

	identity := ResolveContentModerationIdentity(apiKey, authctx.AuthSubject{UserID: 303})

	require.Equal(t, int64(303), identity.UserID)
	require.Equal(t, int64(303), identity.BillingUserID)
	require.Equal(t, "personal@example.com", identity.UserEmail)
	require.Nil(t, identity.TeamID)
}

// TestResolveContentModerationIdentityTeamKeyWithoutActorUsesKeyOwnerAndMembership
// omits ActorUser but supplies Key user and membership evidence. It requires
// member risk identity/email with the separately loaded owner billing ID.
func TestResolveContentModerationIdentityTeamKeyWithoutActorUsesKeyOwnerAndMembership(t *testing.T) {
	teamID := int64(404)
	apiKey := &apikey.APIKey{
		UserID:         505,
		TeamID:         &teamID,
		User:           &identity.User{ID: 606, Email: "owner@example.com"},
		TeamMembership: &team.TeamMembership{UserID: 505, Email: "member@example.com"},
	}

	identity := ResolveContentModerationIdentity(apiKey, authctx.AuthSubject{UserID: 606})

	require.Equal(t, int64(505), identity.UserID)
	require.Equal(t, "member@example.com", identity.UserEmail)
	require.Equal(t, int64(606), identity.BillingUserID)
	require.Equal(t, teamID, *identity.TeamID)
}

// TestResolveContentModerationIdentityWithoutAPIKeyFallsBackToSubject supplies
// subject identity with no Key evidence. It requires that subject in both
// roles, empty email and no team attribution.
func TestResolveContentModerationIdentityWithoutAPIKeyFallsBackToSubject(t *testing.T) {
	identity := ResolveContentModerationIdentity(nil, authctx.AuthSubject{UserID: 707})

	require.Equal(t, int64(707), identity.UserID)
	require.Empty(t, identity.UserEmail)
	require.Equal(t, int64(707), identity.BillingUserID)
	require.Nil(t, identity.TeamID)
}

// newContentModerationHelperTestContext constructs a gateway request used for
// moderation attribution assertions.
func newContentModerationHelperTestContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

// Check simulates bounded policy work and returns its configured decision outcome.
func (s ingressModerationStub) Check(context.Context, moderation.ContentModerationCheckInput) (*moderation.ContentModerationDecision, error) {
	time.Sleep(time.Millisecond)
	return &moderation.ContentModerationDecision{Allowed: true}, s.err
}

// TestRunContentModerationSeparatesIngressDelay measures a long wait before policy evaluation.
// Both allow and error paths retain separate entry and completion times, and
// absent moderation services do not manufacture a check that never happened.
func TestRunContentModerationSeparatesIngressDelay(t *testing.T) {
	for _, checkErr := range []error{nil, context.Canceled} {
		synctest.Test(t, func(t *testing.T) {
			c := newContentModerationHelperTestContext()
			ctx, stages := timing.WithStages(c.Request.Context(), time.Now(), nil)
			c.Request = c.Request.WithContext(ctx)
			time.Sleep(215 * time.Second)
			svc := ingressModerationStub{err: checkErr}
			decision := RunContentModeration(GatewayModerationEndpoints{}, c, nil, svc, nil, authctx.AuthSubject{}, moderation.ContentModerationProtocolOpenAIChat, "model", []byte(`{"messages":[]}`))
			require.Equal(t, checkErr == nil, decision != nil)
			got := stages.FailureSnapshot()
			require.Equal(t, int64(215000), got["moderation_started_ms"])
			require.Equal(t, int64(215001), got["moderation_finished_ms"])
		})
	}
	c := newContentModerationHelperTestContext()
	ctx, stages := timing.WithStages(c.Request.Context(), time.Now(), nil)
	c.Request = c.Request.WithContext(ctx)
	require.Nil(t, RunContentModeration(nil, c, nil, nil, nil, authctx.AuthSubject{}, "", "", nil))
	require.NotContains(t, stages.Snapshot(), "moderation_started_ms")
}
