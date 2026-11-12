package creative

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// prompt.go expands a user's image or video description and charges a fixed
// wallet price after a usable result exists. Each request belongs to one user
// and browser workspace, with an idempotency key distinguishing intentional
// repetitions. Administrator settings select the text model and group, and
// the site exchange rate converts the displayed yuan price into wallet dollars.
//
// A Redis lease serializes the request identity. The generated text and complete
// billing command enter temporary storage before settlement, allowing a retry
// after an uncertain database commit to reuse both. The persistent billing
// receipt prevents a second charge after temporary results expire. Text remains
// in Redis for thirty minutes, and an expired charged request returns an error
// with its persistent charge already recorded.

// PromptExpansionPriceCNY is the fixed yuan price shown before every expansion.
// Wallet conversion uses the administrator's exchange rate at first execution.
const PromptExpansionPriceCNY = 0.015

// Prompt expansion errors distinguish unavailable configuration, concurrent work,
// expired receipts, and failed upstream output without exposing provider payloads.
var (
	ErrPromptUnavailable = apperror.New(apperror.CategoryServiceUnavailable, "CREATIVE_PROMPT_UNAVAILABLE", "prompt expansion is not configured")
	ErrPromptBusy        = apperror.New(apperror.CategoryConflict, "CREATIVE_PROMPT_BUSY", "prompt expansion is already running; retry with the same key")
	ErrPromptExpired     = apperror.New(apperror.CategoryConflict, "CREATIVE_PROMPT_EXPIRED", "this expansion was charged but its temporary result has expired")
	ErrPromptFailed      = apperror.New(apperror.CategoryBadGateway, "CREATIVE_PROMPT_FAILED", "prompt expansion failed; no charge was applied")
)

// PromptConfig selects the administrator's text route and wallet exchange rate.
type PromptConfig struct {
	GroupID         int64   // configured text group identifier
	Model           string  // configured public text model
	USDExchangeRate float64 // yuan per wallet dollar
}

// PromptExpansionRequest contains user text and its intended generation medium.
type PromptExpansionRequest struct {
	Prompt    string `json:"prompt"`     // description sent for expansion
	MediaType string `json:"media_type"` // image or video intent
}

// PromptExpansionResponse contains the usable replacement text and displayed price.
type PromptExpansionResponse struct {
	Prompt  string  `json:"prompt"`   // expanded description for generation
	CostCNY float64 `json:"cost_cny"` // yuan charged per expansion
}

// PromptGeneration retains text and execution identity for fixed-price usage records.
type PromptGeneration struct {
	Text       string // generated expanded prompt text
	Model      string // actual response model name
	ProviderID int64  // selected upstream provider identifier
	Platform   string // selected upstream platform name
}

// PromptExpansionRecord keeps transient output with its exact recoverable charge.
type PromptExpansionRecord struct {
	// Fingerprint, Response, Command, Generation, and GroupID are saved together
	// before settlement. Retries keep this snapshot immutable so output and charge
	// retain their first execution identity through later configuration changes.
	Fingerprint string                      // hash of normalized input fields
	Response    PromptExpansionResponse     // transient returned text and price
	Command     billing.UsageBillingCommand // immutable first-execution financial snapshot
	Generation  PromptGeneration            // transient output execution evidence
	GroupID     int64                       // original configured text group
}

// PromptExpansionStore serializes requests and retains their short-lived output.
type PromptExpansionStore interface {
	// Acquire holds a request identity beyond its execution deadline. The returned
	// cleanup releases only its own lease; a false acquired result requires retry.
	Acquire(context.Context, string) (release func(), acquired bool, err error)
	// Load returns temporary output or nil when its retention has elapsed.
	// Storage failures remain errors because treating an uncertain read as absence
	// could restart generation while a recoverable result still exists.
	Load(context.Context, string) (*PromptExpansionRecord, error)
	// Save retains the full output and charge before financial settlement starts.
	// A successful return makes that snapshot available to retries until its
	// configured lifetime expires; failures prevent the caller from settling.
	Save(context.Context, string, *PromptExpansionRecord) error
}

// PromptExpansionFunds commits fixed wallet charges through the common billing owner.
type PromptExpansionFunds interface {
	// Settle applies the supplied command through the shared billing transaction.
	// Prompt commands request wallet funding and RequireAvailableBalance, and
	// matching financial identities return Applied false after an earlier commit.
	Settle(context.Context, *billing.UsageBillingCommand) (*billing.UsageBillingApplyResult, error)
}

// PromptExpander coordinates text generation, transient recovery, and wallet billing.
type PromptExpander struct {
	// Store, Funds, Config, Enabled, MaxPromptChars, Admit, Generate, and
	// HasSettlement are installed before serving concurrent requests. Their
	// implementations retain application-owned dependencies for the service lifetime.
	Store          PromptExpansionStore               // temporary output and request leases
	Funds          PromptExpansionFunds               // shared financial settlement owner
	Config         func(context.Context) PromptConfig // current administrator route reader
	Enabled        func(context.Context) bool         // combined Creative Studio feature gate
	MaxPromptChars func() int                         // Unicode input and output limit
	// Admit checks current user/group access and funds before generation, and
	// returns a managed Key plus the user's concurrency lease cleanup callback.
	Admit func(context.Context, int64, PromptConfig, float64) (int64, func(), error)
	// Moderate optionally checks plaintext before generation. A denial or error
	// stops this request before any upstream result or wallet charge is produced.
	Moderate      func(context.Context, int64, PromptConfig, string) error              // review plaintext with retention disabled
	Generate      func(context.Context, PromptConfig, string) (PromptGeneration, error) // bounded upstream execution
	HasSettlement func(context.Context, string) (bool, error)                           // durable receipt existence check
	// Record writes idempotent usage metadata and invalidates cached user funds
	// after settlement. A nil callback skips metadata recording; configured
	// callbacks keep temporary prompt text out of persistent records.
	Record func(context.Context, int64, *PromptExpansionRecord, *billing.UsageBillingApplyResult)
}

// Available reports whether the feature and a complete administrator route are enabled.
// A nil service, absent setting callbacks, or a disabled feature returns false
// before model configuration can advertise this operation to clients.
func (s *PromptExpander) Available(ctx context.Context) bool {
	if s == nil || s.Config == nil || s.Enabled == nil || !s.Enabled(ctx) {
		return false
	}
	cfg := s.Config(ctx)
	return cfg.GroupID > 0 && strings.TrimSpace(cfg.Model) != ""
}

// Expand produces a bounded expanded prompt and charges only a usable generated
// result. Callers retain the same idempotency key when retrying an interrupted
// request, and supply the authenticated user with a normalized workspace UUID.
//
// The request lease covers cache lookup, generation, and billing. Cached output
// carries its first charge snapshot across setting changes and uncertain commits.
// A persistent receipt with no temporary output ends the request as expired.
// Successful upstream output is saved before settlement, so retries recover the
// result while the billing owner prevents repeated debits under concurrency.
func (s *PromptExpander) Expand(ctx context.Context, scope CreativeRunScope, input PromptExpansionRequest, idempotencyKey string) (*PromptExpansionResponse, error) {
	if !s.Available(ctx) {
		return nil, ErrPromptUnavailable
	}
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.MediaType = strings.ToLower(strings.TrimSpace(input.MediaType))
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if scope.UserID <= 0 || scope.WorkspaceID == "" || input.Prompt == "" || utf8.RuneCountInString(input.Prompt) > s.MaxPromptChars() || (input.MediaType != "image" && input.MediaType != "video") || idempotencyKey == "" || len(idempotencyKey) > 255 {
		return nil, ErrCreativeInvalidParams
	}
	if s.Store == nil || s.Funds == nil || s.Admit == nil || s.Generate == nil || s.HasSettlement == nil {
		return nil, ErrPromptUnavailable
	}
	// The request deadline leaves thirty seconds before the lease expires.
	// Upstream and database adapters receive this context to enforce cancellation
	// before another owner can acquire the same financial identity.
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	// These identity and input records contain only integer and string values,
	// so JSON encoding has no unsupported types or cycles that could fail.
	identity, _ := json.Marshal([]any{scope.UserID, scope.WorkspaceID, idempotencyKey})
	digest := sha256.Sum256(identity)
	requestID := "creative_prompt:" + hex.EncodeToString(digest[:])
	payload, _ := json.Marshal(input)
	fingerprintBytes := sha256.Sum256(payload)
	fingerprint := hex.EncodeToString(fingerprintBytes[:])
	release, acquired, err := s.Store.Acquire(ctx, requestID)
	if err != nil {
		return nil, ErrCreativeTransientFailed.WithCause(err)
	}
	if !acquired {
		return nil, ErrPromptBusy
	}
	defer release()
	record, err := s.Store.Load(ctx, requestID)
	if err != nil {
		return nil, ErrCreativeTransientFailed.WithCause(err)
	}
	if record != nil && record.Fingerprint != fingerprint {
		return nil, ErrCreativeRunIdempotencyConflict
	}
	if record == nil {
		charged, err := s.HasSettlement(ctx, requestID)
		if err != nil {
			return nil, ErrCreativeSettlementBillingFail.WithCause(err)
		}
		if charged {
			return nil, ErrPromptExpired
		}
		cfg := s.Config(ctx)
		// Invalid exchange observations cannot create zero, infinite, or negative
		// charges. Missing settings are normalized by the runtime settings reader.
		if cfg.USDExchangeRate <= 0 || math.IsNaN(cfg.USDExchangeRate) || math.IsInf(cfg.USDExchangeRate, 0) {
			return nil, ErrPromptUnavailable
		}
		// Convert yuan to the wallet's eight-decimal dollar precision.
		amount := billing.QuantizeUsageBillingAmount(PromptExpansionPriceCNY / cfg.USDExchangeRate)
		if amount <= 0 || math.IsInf(amount, 0) || math.IsNaN(amount) {
			return nil, ErrPromptUnavailable
		}
		keyID, releaseUser, err := s.Admit(ctx, scope.UserID, cfg, amount)
		if err != nil {
			return nil, err
		}
		defer releaseUser()
		if s.Moderate != nil {
			if err := s.Moderate(ctx, scope.UserID, cfg, input.Prompt); err != nil {
				return nil, err
			}
		}
		generated, err := s.Generate(ctx, cfg, expansionInstruction(input))
		if err != nil {
			return nil, ErrPromptFailed
		}
		generated.Text = normalizeExpandedPrompt(generated.Text)
		if generated.Text == "" || utf8.RuneCountInString(generated.Text) > s.MaxPromptChars() {
			return nil, ErrPromptFailed
		}
		record = &PromptExpansionRecord{Fingerprint: fingerprint, Response: PromptExpansionResponse{Prompt: generated.Text, CostCNY: PromptExpansionPriceCNY}, Generation: generated, GroupID: cfg.GroupID, Command: billing.UsageBillingCommand{
			RequestID: requestID, APIKeyID: keyID, UserID: scope.UserID, ActorUserID: scope.UserID,
			RequestPayloadHash: fingerprint, APIKeyBillingMode: billing.APIKeyBillingModeBalance,
			BillableAmountUSD: amount, RequireAvailableBalance: true, ProviderID: generated.ProviderID,
			Model: cfg.Model, GroupID: &cfg.GroupID,
		}}
		// Persist the generated text with the exact charge first. A later interrupted
		// settlement can replay this same command and return the cached successful text.
		if err := s.Store.Save(ctx, requestID, record); err != nil {
			return nil, ErrCreativeTransientFailed.WithCause(err)
		}
	}
	command := record.Command
	applied, err := s.Funds.Settle(ctx, &command)
	if err != nil {
		if errors.Is(err, billing.ErrTaskInsufficientBalance) {
			return nil, ErrCreativeInsufficientBalance
		}
		return nil, ErrCreativeSettlementBillingFail.WithCause(err)
	}
	if s.Record != nil {
		s.Record(ctx, scope.UserID, record, applied)
	}
	return &record.Response, nil
}

// expansionInstruction supplies media-specific editing rules with the user's description.
// Expand validates image or video intent before calling this helper, which keeps
// the user text separate from the chosen editing instructions with blank lines.
func expansionInstruction(input PromptExpansionRequest) string {
	instruction := "你是专业的文生图提示词编辑。保留用户的主体、动作、关系和核心意图，补充主体细节、构图、镜头、环境、光线、色彩、材质和艺术风格，让提示词可以直接用于生图模型。"
	if input.MediaType == "video" {
		instruction = "你是专业的文生视频提示词编辑。保留用户的主题和叙事意图，补充主体动作、镜头运动、时间顺序、场景氛围、光线、画面风格和节奏，让提示词可以直接用于视频模型。"
	}
	return instruction + "只返回最终提示词，不要解释、标题、引号或 Markdown。保留用户使用的语言。\n\n用户描述：\n" + input.Prompt
}

// normalizeExpandedPrompt removes optional Markdown fences and trims returned text.
// Blank content remains blank so expansion fails before any wallet charge begins.
// Unfenced output keeps its internal line breaks and formatting.
func normalizeExpandedPrompt(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "```") {
		if newline := strings.IndexByte(value, '\n'); newline >= 0 && strings.HasSuffix(value, "```") {
			value = strings.TrimSpace(strings.TrimSuffix(value[newline+1:], "```"))
		}
	}
	return value
}
