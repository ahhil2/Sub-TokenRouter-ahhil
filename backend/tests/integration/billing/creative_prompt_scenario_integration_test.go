//go:build integration

package billing_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	billingpg "github.com/TokenFlux/TokenRouter/internal/billing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/creative"
	creativehttp "github.com/TokenFlux/TokenRouter/internal/creative/httpapi"
	"github.com/TokenFlux/TokenRouter/internal/creative/rediscache"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
	"github.com/TokenFlux/TokenRouter/internal/pkg/timezone"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerpg "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	providerexec "github.com/TokenFlux/TokenRouter/internal/provider/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
	"github.com/TokenFlux/TokenRouter/internal/settings"
	settingspg "github.com/TokenFlux/TokenRouter/internal/settings/postgres"
	"github.com/TokenFlux/TokenRouter/internal/testutil/rediscontainer"
)

// creative_prompt_scenario_integration_test.go exercises the public prompt
// endpoint across real settings, Redis recovery, provider SSE parsing, and
// PostgreSQL wallet settlement. The provider transport permits only a local
// HTTP fixture, so these cases cannot contact configured production providers.
// Admission supplies a fixture Key while the shared financial owner performs
// actual strict available-balance checks and durable request deduplication.
//
// Success establishes the fixed yuan charge and immutable conversion snapshot.
// Replays, changed inputs, disabled settings, invalid scope, expired output,
// upstream rejection, and incomplete text each verify upstream call counts and
// persisted balances. The scenario covers creative/prompt.go, its HTTP and Redis
// adapters, provider/testing_generation.go, and billing/postgres/settlement.go.

// promptFixtureTransport rejects destinations outside its isolated HTTP fixture.
type promptFixtureTransport struct {
	client *http.Client // fixture-owned connection and redirect policy
	root   string       // only permitted upstream scheme and authority
}

// DoWithTLS applies the fixture destination restriction before sending HTTP.
// Proxy, provider concurrency, and TLS-profile inputs are irrelevant to this
// isolated server; the real provider adapter still constructs its production payload.
func (transport promptFixtureTransport) DoWithTLS(request *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	if request.URL.Scheme+"://"+request.URL.Host != transport.root {
		return nil, fmt.Errorf("unexpected upstream destination")
	}
	return transport.client.Do(request)
}

// TestCreativePromptHTTPSettlement verifies endpoint responses against actual
// Redis and PostgreSQL state while a local SSE provider supplies successful,
// failed, blank, and interrupted outputs. Replays preserve their original charge
// even after administrator currency settings change.
func TestCreativePromptHTTPSettlement(t *testing.T) {
	ctx := t.Context()
	client := committedEntitlementClient(t)
	rdb := rediscontainer.New(t)
	user := mustCreateUser(t, client, &identity.User{Email: uuid.NewString() + "@example.com", Balance: 1})
	group := mustCreateGroup(t, client, &routing.Group{Name: "prompt-" + uuid.NewString()})
	key := mustCreateApiKey(t, client, &apikey.APIKey{UserID: user.ID, GroupID: &group.ID, Key: "sk-" + uuid.NewString(), Name: "prompt"})
	var previousReceipts int
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id=$1", key.ID).Scan(&previousReceipts))
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		assertion := assert.New(t)
		assertion.Equal("/v1/chat/completions", r.URL.Path)
		assertion.Equal("Bearer fixture-key", r.Header.Get("Authorization"))
		var payload map[string]any
		if !assertion.NoError(json.NewDecoder(r.Body).Decode(&payload)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assertion.Equal("fixture-text", payload["model"])
		assertion.Equal(float64(4096), payload["max_tokens"])
		messages, err := json.Marshal(payload["messages"])
		assertion.NoError(err)
		assertion.Contains(string(messages), "用户描述")
		if strings.Contains(string(messages), "private_auth_prompt") {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"echoed private_auth_prompt"}}`))
			return
		}
		if strings.Contains(string(messages), "upstream_failure") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"private upstream_failure"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		switch {
		case strings.Contains(string(messages), "empty_output"):
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case strings.Contains(string(messages), "unfinished_output"):
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"incomplete description\"}}]}\n\n"))
		case strings.Contains(string(messages), "truncated_output"):
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"limited description\"},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n"))
		default:
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"A lantern-lit garden at dusk, with soft cinematic lighting.\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
		}
	}))
	t.Cleanup(upstream.Close)
	value := mustCreateProvider(t, client, &provider.Record{
		Name: "prompt-" + uuid.NewString(), Platform: provider.PlatformOpenAI, Type: provider.ProviderTypeAPIKey,
		Credentials: map[string]any{"api_key": "fixture-key", "base_url": upstream.URL, provider.UpstreamProtocolsKey: []string{string(capability.ProtocolOpenAIChatCompletions)}},
	})
	transportClient := upstream.Client()
	transportClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	executor := &providerexec.OpenAIProviderTest{
		Store:     providerpg.NewProviderStore(client, integrationDB, providerpg.ProviderStoreOptions{}),
		Transport: promptFixtureTransport{client: transportClient, root: upstream.URL},
		ValidateURL: func(raw string) (string, error) {
			if raw != upstream.URL {
				return "", fmt.Errorf("unexpected provider root")
			}
			return raw, nil
		},
		ApplyRouting: func(*providerexec.TestRun, *provider.Record, *http.Request, bool) {},
		ResolveTLS:   func(*providerexec.TestRun, *provider.Record) *tlsfingerprint.Profile { return nil },
	}
	textService := provider.NewTestService(&providerexec.TestTargets{
		Read: func(context.Context, int64) (*provider.Record, error) { return value, nil }, OpenAI: executor,
	}, provider.TestOptions{})
	settingRepo := settingspg.NewSettingRepository(client)
	settingStore := settings.New(settingRepo)
	settingKeys := []string{creative.SettingKeyCreativePromptGroupID, creative.SettingKeyCreativePromptModel, creative.SettingKeyCreativeEnabled, billing.SettingKeyUSDExchangeRate}
	originalSettings, err := settingStore.GetMultiple(ctx, settingKeys)
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, settingKey := range settingKeys {
			if original, exists := originalSettings[settingKey]; exists {
				require.NoError(t, settingRepo.Set(context.Background(), settingKey, original))
			} else {
				require.NoError(t, settingRepo.Delete(context.Background(), settingKey))
			}
		}
	})
	require.NoError(t, settingStore.SetMultiple(ctx, map[string]string{
		creative.SettingKeyCreativePromptGroupID: strconv.FormatInt(group.ID, 10), creative.SettingKeyCreativePromptModel: "fixture-text",
		creative.SettingKeyCreativeEnabled: "true", billing.SettingKeyUSDExchangeRate: "7.5",
	}))
	runtimeSettings := creative.NewRuntimeSettings(settingStore, settings.ErrSettingNotFound)
	settlement := billingpg.NewSettlementStore(integrationDB, timezone.NewCalendar(time.UTC), nil)
	expander := &creative.PromptExpander{
		Store: rediscache.NewPromptStore(rdb), Funds: billing.NewFunds(settlement),
		Config: runtimeSettings.GetCreativePromptConfig, Enabled: runtimeSettings.IsCreativeEnabled,
		MaxPromptChars: func() int { return 8000 }, HasSettlement: settlement.HasSettlement,
		Admit: func(context.Context, int64, creative.PromptConfig, float64) (int64, func(), error) {
			return key.ID, func() {}, nil
		},
		Generate: func(ctx context.Context, cfg creative.PromptConfig, prompt string) (creative.PromptGeneration, error) {
			result, err := textService.GenerateTextRequest(ctx, provider.TestRequest{
				ProviderID: value.ID, GroupID: cfg.GroupID, Model: cfg.Model, Prompt: prompt,
				ProtocolRoute: provider.TestProtocolRoute{ClientProtocol: capability.ProtocolOpenAIChatCompletions, UpstreamProtocol: capability.ProtocolOpenAIChatCompletions},
			}, 4096, 32000)
			return creative.PromptGeneration{Text: result.Text, Model: result.Model, ProviderID: value.ID, Platform: value.Platform}, err
		},
	}
	endpoint := creativehttp.NewCreativeHandler(nil)
	endpoint.BindPromptExpansion(expander)
	engine := gin.New()
	authenticated := engine.Group("/api/v1", func(c *gin.Context) {
		authctx.SetPrincipal(c, identity.Principal{UserID: user.ID, Role: identity.RoleUser}, 5, user.Email)
		c.Next()
	})
	creativehttp.RegisterUserRoutes(authenticated, endpoint, func(c *gin.Context) { c.Next() })
	workspace := uuid.NewString()
	request := func(prompt, media, idempotencyKey, requestWorkspace string) *httptest.ResponseRecorder {
		t.Helper()
		body, err := json.Marshal(creative.PromptExpansionRequest{Prompt: prompt, MediaType: media})
		require.NoError(t, err)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/creative/prompt/expand", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)
		req.Header.Set(creative.CreativeWorkspaceHeader, requestWorkspace)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		return response
	}
	assertWallet := func(wantBalance float64, wantReceipts int) {
		t.Helper()
		var balance, frozen float64
		var receipts int
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT balance, frozen_balance FROM users WHERE id=$1", user.ID).Scan(&balance, &frozen))
		require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT COUNT(*) FROM usage_billing_dedup WHERE api_key_id=$1", key.ID).Scan(&receipts))
		require.InDelta(t, wantBalance, balance, 1e-9)
		require.Zero(t, frozen)
		require.Equal(t, previousReceipts+wantReceipts, receipts)
	}

	response := request("a garden at dusk", "image", "paid-once", workspace)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	var envelope struct {
		Data creative.PromptExpansionResponse `json:"data"` // public expansion and displayed price
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
	require.Equal(t, "A lantern-lit garden at dusk, with soft cinematic lighting.", envelope.Data.Prompt)
	require.Equal(t, 0.015, envelope.Data.CostCNY)
	assertWallet(0.998, 1)
	require.EqualValues(t, 1, calls.Load())

	// Administrator currency changes affect new requests only. The original Key
	// recovers its first stored command and returns the same output without a debit.
	require.NoError(t, settingStore.SetMultiple(ctx, map[string]string{billing.SettingKeyUSDExchangeRate: "15"}))
	replayed := request("a garden at dusk", "image", "paid-once", workspace)
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.JSONEq(t, response.Body.String(), replayed.Body.String())
	require.Equal(t, http.StatusConflict, request("different scene", "image", "paid-once", workspace).Code)
	require.EqualValues(t, 1, calls.Load())
	assertWallet(0.998, 1)

	for _, prompt := range []string{"upstream_failure", "empty_output", "unfinished_output", "truncated_output"} {
		failed := request(prompt, "video", uuid.NewString(), workspace)
		require.Equal(t, http.StatusBadGateway, failed.Code, failed.Body.String())
		require.NotContains(t, failed.Body.String(), "private upstream_failure")
		assertWallet(0.998, 1)
	}
	require.EqualValues(t, 5, calls.Load())
	require.Equal(t, http.StatusBadRequest, request("a garden", "image", "", workspace).Code)
	require.Equal(t, http.StatusBadRequest, request("a garden", "image", "invalid-workspace", "invalid").Code)
	require.NoError(t, settingStore.SetMultiple(ctx, map[string]string{creative.SettingKeyCreativeEnabled: "false"}))
	require.Equal(t, http.StatusServiceUnavailable, request("a garden", "image", "disabled", workspace).Code)
	require.NoError(t, settingStore.SetMultiple(ctx, map[string]string{creative.SettingKeyCreativeEnabled: "true"}))
	require.EqualValues(t, 5, calls.Load())
	assertWallet(0.998, 1)

	// Removing only transient results simulates their retention expiry. The
	// committed billing receipt must reject replay without another upstream call.
	resultKeys, err := rdb.Keys(ctx, "creative:prompt:result:*").Result()
	require.NoError(t, err)
	require.Len(t, resultKeys, 1)
	require.NoError(t, rdb.Del(ctx, resultKeys...).Err())
	expired := request("a garden at dusk", "image", "paid-once", workspace)
	require.Equal(t, http.StatusConflict, expired.Code, expired.Body.String())
	require.Contains(t, expired.Body.String(), "CREATIVE_PROMPT_EXPIRED")
	require.EqualValues(t, 5, calls.Load())
	assertWallet(0.998, 1)

	// A new operation uses the changed conversion and its own durable identity.
	// This second video request must debit exactly half the original wallet amount.
	response = request("a garden at dusk", "video", "new-video", workspace)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.EqualValues(t, 6, calls.Load())
	assertWallet(0.997, 2)

	// Authentication responses can echo user text. Provider health still changes,
	// but its durable diagnostic and the public response must omit that text.
	response = request("private_auth_prompt", "image", "auth-rejection", workspace)
	require.Equal(t, http.StatusBadGateway, response.Code, response.Body.String())
	require.NotContains(t, response.Body.String(), "private_auth_prompt")
	var diagnostic string
	require.NoError(t, integrationDB.QueryRowContext(ctx, "SELECT error_message FROM providers WHERE id=$1", value.ID).Scan(&diagnostic))
	require.Equal(t, "Text generation failed: provider authentication or access was rejected", diagnostic)
	require.EqualValues(t, 7, calls.Load())
	assertWallet(0.997, 2)
}
