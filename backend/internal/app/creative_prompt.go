package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/apikey"
	keypostgres "github.com/TokenFlux/TokenRouter/internal/apikey/postgres"
	"github.com/TokenFlux/TokenRouter/internal/billing"
	billingpostgres "github.com/TokenFlux/TokenRouter/internal/billing/postgres"
	"github.com/TokenFlux/TokenRouter/internal/config"
	"github.com/TokenFlux/TokenRouter/internal/creative"
	creativeredis "github.com/TokenFlux/TokenRouter/internal/creative/rediscache"
	"github.com/TokenFlux/TokenRouter/internal/gateway/provider/selection"
	"github.com/TokenFlux/TokenRouter/internal/identity"
	identitypostgres "github.com/TokenFlux/TokenRouter/internal/identity/postgres"
	"github.com/TokenFlux/TokenRouter/internal/protocol"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	providerpostgres "github.com/TokenFlux/TokenRouter/internal/provider/postgres"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/scheduler"
	"github.com/TokenFlux/TokenRouter/internal/usage"
)

// creative_prompt.go assembles fixed-price text expansion with current user
// access, scheduler slots, provider transports, and wallet settlement. The text
// route belongs to administrator settings, while every caller still needs
// access to its group and enough available wallet funds for the displayed price.
// User and provider concurrency slots remain held until the operation returns.
//
// Provider text execution uses the route selected under group policy and feeds
// text into a bounded memory sink. The standard request-token settlement path
// is absent from this execution, so the prompt service supplies one fixed
// wallet charge. Usage recording receives identifiers and amounts after that
// charge; prompt bodies and generated text remain in transient storage.

// provideCreativePrompt constructs prompt expansion with shared application services.
// Dependencies remain application-owned, and the returned service acquires and
// releases concurrency slots inside each bounded expansion request. User and
// group reads establish access before a managed Key is created, and provider
// selection keeps the admitted group protocol attached to text execution.
// Successful settlement triggers cache invalidation and idempotent usage
// recording using a short context that survives client cancellation. Replayed
// settlements repeat those completion actions to recover an interrupted response.
func provideCreativePrompt(public *creative.Public, settings *creative.RuntimeSettings, redisClient *redis.Client, funds *billing.Funds, settlement *billingpostgres.SettlementStore, users *identitypostgres.UserStore, keys *keypostgres.KeyStore, groups *routing.GroupAdmin, providers *providerpostgres.ProviderStore, tests *provider.TestService, generic *selection.Generic, compatible *selection.Compatible, gemini *selection.Gemini, concurrency *scheduler.ConcurrencyService, auth apikey.APIKeyAuthCacheInvalidator, cfg *config.Config, attributes *routing.ModelAttributeService) *creative.PromptExpander {
	probe := selection.NewProbe(tests, generic, compatible, gemini)
	probe.Attributes = attributes
	managed := apikey.ManagedKeys{Store: creativeManagedKeys{store: keys}, Prefix: cfg.Default.APIKeyPrefix, ManagedBy: creative.CreativeManagedBy, NamePrefix: "creative-studio"}
	return &creative.PromptExpander{
		Store: creativeredis.NewPromptStore(redisClient), Funds: funds, Config: settings.GetCreativePromptConfig,
		Enabled: public.Enabled, MaxPromptChars: public.MaxPromptChars, HasSettlement: settlement.HasSettlement,
		Admit: func(ctx context.Context, userID int64, config creative.PromptConfig, amount float64) (int64, func(), error) {
			user, err := users.GetByID(ctx, userID)
			if err != nil {
				return 0, nil, err
			}
			if user == nil || user.Status != identity.StatusActive {
				return 0, nil, identity.ErrUserNotFound
			}
			group, err := groups.GetGroup(ctx, config.GroupID)
			if err != nil {
				return 0, nil, creative.ErrPromptUnavailable
			}
			if group == nil || !group.IsActive() || group.ClaudeCodeOnly || !user.CanBindGroup(group.ID, group.IsExclusive) {
				return 0, nil, creative.ErrCreativeGroupForbidden
			}
			if user.Balance < amount {
				return 0, nil, creative.ErrCreativeInsufficientBalance
			}
			slot, err := concurrency.AcquireUserSlot(ctx, userID, user.Concurrency)
			if err != nil {
				return 0, nil, err
			}
			if slot == nil || !slot.Acquired {
				return 0, nil, creative.ErrPromptBusy
			}
			key, err := managed.Ensure(ctx, userID, group.ID)
			if err != nil {
				slot.ReleaseFunc()
				return 0, nil, err
			}
			return key.ID, slot.ReleaseFunc, nil
		},
		Generate: func(ctx context.Context, config creative.PromptConfig, prompt string) (creative.PromptGeneration, error) {
			group, err := groups.GetGroup(ctx, config.GroupID)
			if err != nil || group == nil || !group.IsActive() || group.ClaudeCodeOnly {
				return creative.PromptGeneration{}, creative.ErrPromptUnavailable
			}
			target, err := selectCreativePromptTarget(ctx, group, config.Model, probe.Select)
			if err != nil {
				return creative.PromptGeneration{}, err
			}

			selected, err := providers.GetByID(ctx, target.ProviderID)
			if err != nil {
				return creative.PromptGeneration{}, err
			}
			slot, err := concurrency.AcquireProviderSlot(ctx, selected.ID, selected.Concurrency)
			if err != nil {
				return creative.PromptGeneration{}, err
			}
			if slot == nil || !slot.Acquired {
				return creative.PromptGeneration{}, creative.ErrPromptBusy
			}
			defer slot.ReleaseFunc()
			if target.Attributes != nil {
				ctx = target.Attributes.WithContext(ctx)
			}
			generated, err := tests.GenerateTextRequest(ctx, provider.TestRequest{ProviderID: target.ProviderID, GroupID: target.GroupID, Model: target.ModelID, Prompt: prompt, ProtocolRoute: target.ProtocolRoute}, 4096, public.MaxPromptChars()*4)
			return creative.PromptGeneration{Text: generated.Text, Model: generated.Model, ProviderID: target.ProviderID, Platform: selected.Platform}, err
		},
		Moderate: func(ctx context.Context, userID int64, config creative.PromptConfig, prompt string) error {
			if public.Moderation == nil || !public.Moderation.RequestModerationEnabled(ctx) {
				return nil
			}
			body, _ := json.Marshal(map[string]any{"messages": []map[string]string{{"role": "user", "content": prompt}}})
			decision, err := public.Moderation.Check(ctx, creative.ModerationInput{UserID: userID, BillingUserID: userID, GroupID: &config.GroupID, Model: config.Model, Endpoint: "/api/v1/creative/prompt/expand", Protocol: "openai_chat", Body: body, NoMediaRetention: true})
			if err != nil {
				return errors.New("prompt moderation is unavailable")
			}
			if decision != nil && !decision.Allowed {
				return creative.ErrCreativeContentBlocked
			}
			return nil
		},
		Record: func(ctx context.Context, userID int64, record *creative.PromptExpansionRecord, result *billing.UsageBillingApplyResult) {
			// Completion metadata can be retried independently of its financial claim;
			// the usage writer deduplicates the stable Key and billing request identity.
			durable, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			auth.InvalidateAuthCacheByUserID(durable, userID)
			amount := record.Command.BillableAmountUSD
			endpoint := "/api/v1/creative/prompt/expand"
			public.Results.RecordUsage(durable, &usage.UsageLog{UserID: userID, BillingUserID: userID, APIKeyID: record.Command.APIKeyID, ProviderID: record.Generation.ProviderID, Platform: record.Generation.Platform, RequestID: record.Command.RequestID, BillingKey: record.Command.RequestID, Model: record.Command.Model, RequestedModel: record.Command.Model, UpstreamModel: &record.Generation.Model, GroupID: &record.GroupID, TotalCost: amount, ActualCost: amount, RateMultiplier: 1, InboundEndpoint: &endpoint, CreatedAt: time.Now(), RequestType: usage.RequestTypeSync})
		},
	}
}

// selectCreativePromptTarget finds an enabled text route with an eligible provider.
// Selection sends no upstream requests. A group may allow several client entries
// while its current providers support only one, so unavailable entries are tried
// in order until one target is selected or the request is cancelled. A
// successful target stops the search before generation begins, so subsequent
// entries cannot produce additional upstream work. A group with no usable
// candidate returns the same unavailable error used for disabled configuration.
func selectCreativePromptTarget(ctx context.Context, group *routing.Group, model string, selectTarget func(context.Context, routing.GroupAvailabilityProbeDueGroup, string) (routing.GroupProbeTarget, error)) (routing.GroupProbeTarget, error) {
	for _, candidate := range []protocol.ProtocolID{protocol.ProtocolOpenAIChatCompletions, protocol.ProtocolOpenAIResponses, protocol.ProtocolAnthropicMessages, protocol.ProtocolGeminiGenerateContent} {
		if !group.AllowsClientProtocol(candidate) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return routing.GroupProbeTarget{}, err
		}
		target, err := selectTarget(ctx, routing.GroupAvailabilityProbeDueGroup{GroupID: group.ID, Config: routing.GroupAvailabilityProbeConfig{Protocol: candidate}}, model)
		if err == nil && target.ProviderID > 0 {
			return target, nil
		}
		// Enabled client entries can lack a compatible provider. Selection performs
		// no generation, so trying another entry cannot duplicate a paid request.
	}
	return routing.GroupProbeTarget{}, creative.ErrPromptUnavailable
}
