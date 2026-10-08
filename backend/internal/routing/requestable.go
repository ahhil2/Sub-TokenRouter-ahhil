package routing

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing/capability"
)

// RequestableModel 描述客户端可请求的模型，以及模型广场应使用的定价模型。
type RequestableModel struct {
	// UpstreamModels 保存已确认可请求的最终模型，供展示使用。
	UpstreamModels []string
	Protocols      []capability.ProtocolID
	// NativeProtocols 是 Protocols 里每个承接该模型的提供商都能直接处理的协议，请求按这些协议进入时不经过协议转换。
	NativeProtocols  []capability.ProtocolID
	ID               string
	PricingModel     string
	PricingAmbiguous bool
}

// RequestableModelsResult 是分组模型解析结果。
// Restricted 用于区分分组白名单后的空结果与旧版“没有显式模型”语义。
type RequestableModelsResult struct {
	Models                    []RequestableModel
	Restricted                bool
	HadExplicitProviderModels bool // 用于保持 /v1/models 的历史响应字段结构。
}

// CatalogueRules 提供平台资格、模型映射和执行时观测到的模型信息。
type CatalogueRules interface {
	ConfiguredModels() []string
	Mapping() map[string]string
	Supports(context.Context, string) bool
	UpstreamModels(context.Context, string) []string
}

// CatalogueProvider 只向目录编排提供可分组和排序的只读快照。
type CatalogueProvider struct {
	provider.ProviderSnapshot
	GroupIDs         []int64
	ProviderGroupIDs []int64
	Passthrough      bool
	Rules            CatalogueRules
}
type CatalogueDefaults struct {
	Platform func(string) []string
}
type CataloguePolicies interface {
	GetGroupPolicy(context.Context, int64) (*GroupPolicyView, error)
	GetPricingConfigForGroup(context.Context, int64) (*PricingConfig, error)
}

// RequestableResolver 只编排目录规则，缓存和数据取得均由现有唯一来源提供。
type RequestableResolver struct {
	GroupPolicies CataloguePolicies
	Defaults      CatalogueDefaults
	Warn          func(string, ...any)
}

// ResolveWithProviders 使用已预取提供商解析模型，供模型广场避免逐分组重复查询。
func (s *RequestableResolver) ResolveWithProviders(
	ctx context.Context,
	groupID *int64,
	platform string,
	baseModels []string,
	providers []CatalogueProvider,
) RequestableModelsResult {
	providers = filterRequestableModelProviders(providers, platform)
	// 提供商查询成功但没有平台匹配提供商时必须保持空结果；分组策略读取失败不能凭空补入默认模型。
	if len(providers) == 0 {
		return RequestableModelsResult{}
	}
	currentProviderModels := ConfiguredRequestModelsFromProviders(providers, platform)
	hadExplicitProviderModels := len(baseModels) > 0 || len(currentProviderModels) > 0
	// 缓存层可能暂时为空或滞后，当前查询成功时仍要纳入提供商白名单模型。
	providerCandidateModels := make([]string, 0, len(baseModels)+len(currentProviderModels))
	providerCandidateModels = append(providerCandidateModels, baseModels...)
	providerCandidateModels = append(providerCandidateModels, currentProviderModels...)

	var policy *GroupPolicyView
	billingSource := BillingModelSourceRequested
	policyPlatform := strings.TrimSpace(platform)
	var err error
	if groupID != nil && s.GroupPolicies != nil {
		policy, err = s.GroupPolicies.GetGroupPolicy(ctx, *groupID)
		if err != nil {
			if s.Warn != nil {
				s.Warn("failed to load group policy for requestable model resolution", "group_id", *groupID, "platform", platform, "error", err)
			}
			return RequestableModelsResult{Restricted: true, HadExplicitProviderModels: hadExplicitProviderModels}
		}
		// 每次目录查询预先读取分组策略与计费来源。
		billingSource = BillingModelSourceGroupMapped
		if config, err := s.GroupPolicies.GetPricingConfigForGroup(ctx, *groupID); err == nil && config != nil && config.BillingModelSource != "" {
			billingSource = config.BillingModelSource
		}
	}

	candidates := mergeRequestableModelCandidates(providerCandidateModels, providers, policy, policyPlatform, s.Defaults)
	result := RequestableModelsResult{
		Restricted:                policy != nil && policy.RestrictModels,
		HadExplicitProviderModels: hadExplicitProviderModels,
	}
	if len(candidates) == 0 || len(providers) == 0 {
		return result
	}

	result.Models = make([]RequestableModel, 0, len(candidates))
	for _, requestedModel := range candidates {
		if resolved, ok := resolveRequestableModel(ctx, policy, billingSource, providers, requestedModel); ok {
			result.Models = append(result.Models, resolved)
		}
	}
	return result
}

// ConfiguredRequestModelsFromProviders 复用 GetAvailableModels 的显式模型聚合规则。
func ConfiguredRequestModelsFromProviders(providers []CatalogueProvider, platform string) []string {
	modelSet := make(map[string]struct{})
	hasConfiguredModels := false
	for i := range providers {
		provider := &providers[i]
		if platform != "" && provider.Platform != platform {
			continue
		}
		requestModels := provider.Rules.ConfiguredModels()
		if len(requestModels) == 0 {
			continue
		}
		hasConfiguredModels = true
		for _, model := range requestModels {
			modelSet[model] = struct{}{}
		}
	}
	if !hasConfiguredModels {
		return nil
	}
	models := make([]string, 0, len(modelSet))
	for model := range modelSet {
		models = append(models, model)
	}
	sort.Strings(models)
	return models
}

func filterRequestableModelProviders(providers []CatalogueProvider, platform string) []CatalogueProvider {
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return providers
	}
	filtered := make([]CatalogueProvider, 0, len(providers))
	for i := range providers {
		if matchesCataloguePlatform(&providers[i], platform) {
			filtered = append(filtered, providers[i])
		}
	}
	return filtered
}

// mergeRequestableModelCandidates 按既有候选、分组策略、提供商配置和默认模型的顺序合并候选。
// 通配符用于后续匹配，返回列表包含具体的模型 ID。
func mergeRequestableModelCandidates(baseModels []string, providers []CatalogueProvider, policy *GroupPolicyView, platform string, defaults CatalogueDefaults) []string {
	candidates := make([]string, 0, len(baseModels)+16)
	seen := make(map[string]struct{}, len(baseModels)+16)
	appendModels := func(models ...string) {
		for _, model := range models {
			model = strings.TrimSpace(model)
			if model == "" || strings.Contains(model, "*") {
				continue
			}
			key := strings.ToLower(model)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, model)
		}
	}

	appendModels(baseModels...)
	if policy != nil {
		appendModels(policy.AllowedModels...)
		if mapping := policy.ModelMapping; len(mapping) > 0 {
			appendModels(sortedModelMappingSources(mapping)...)
		}
	}

	for i := range providers {
		appendModels(sortedModelMappingSources(providers[i].Rules.Mapping())...)
	}
	if defaults.Platform != nil {
		appendModels(defaults.Platform(platform)...)
	}

	return candidates
}

func sortedModelMappingSources(mapping map[string]string) []string {
	if len(mapping) == 0 {
		return nil
	}
	models := make([]string, 0, len(mapping))
	for model := range mapping {
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool {
		left := strings.ToLower(models[i])
		right := strings.ToLower(models[j])
		if left != right {
			return left < right
		}
		return models[i] < models[j]
	})
	return models
}

// resolveRequestableModel 用本次查询的策略快照解析单个候选。
func resolveRequestableModel(
	ctx context.Context,
	policy *GroupPolicyView,
	billingSource string,
	providers []CatalogueProvider,
	requestedModel string,
) (RequestableModel, bool) {
	groupMappedModel := requestedModel
	if mapped := strings.TrimSpace(policy.ResolveModel(requestedModel)); mapped != "" {
		groupMappedModel = mapped
	}

	if policy != nil && policy.RestrictModels && policy.RestrictionSource() != BillingModelSourceUpstream {
		pricingModel := ModelForRestriction(policy.RestrictionSource(), requestedModel, groupMappedModel)
		if policy.IsModelRestricted(pricingModel) {
			return RequestableModel{}, false
		}
	}

	upstreamModels := make([]string, 0, len(providers))
	var protocols []capability.ProtocolID
	// nativeByProtocol 记录协议是否在每个承接该模型的提供商上都走原生路线。
	nativeByProtocol := map[capability.ProtocolID]bool{}
	for i := range providers {
		provider := &providers[i]
		var candidateProtocols []capability.ProtocolID
		var nativeCandidates []capability.ProtocolID
		if policy != nil && policy.AllowedProtocols != nil {
			if policy.RequireOAuthOnly && provider.Type == capability.ProviderTypeAPIKey {
				continue
			}
			for _, source := range policy.AllowedProtocols {
				target, ok := capability.ResolveRoute(provider.Protocols(), source, policy.ProtocolFallbacks)
				if !ok {
					continue
				}
				if aware, ok := provider.Rules.(interface {
					SupportsClientProtocol(string, capability.ProtocolID) bool
				}); ok && !aware.SupportsClientProtocol(groupMappedModel, source) {
					continue
				}
				candidateProtocols = append(candidateProtocols, source)
				if target == source {
					nativeCandidates = append(nativeCandidates, source)
				}
			}
			if len(candidateProtocols) == 0 {
				continue
			}
		}
		if !provider.Rules.Supports(ctx, groupMappedModel) {
			continue
		}
		contributed := false
		for _, upstreamModel := range provider.Rules.UpstreamModels(ctx, groupMappedModel) {
			if policy != nil && policy.RestrictModels && policy.RestrictionSource() == BillingModelSourceUpstream &&
				policy.IsModelRestricted(upstreamModel) {
				continue
			}
			upstreamModels = append(upstreamModels, upstreamModel)
			contributed = true
		}
		if !contributed {
			continue
		}
		for _, source := range candidateProtocols {
			native := slices.Contains(nativeCandidates, source)
			if previous, seen := nativeByProtocol[source]; seen {
				nativeByProtocol[source] = previous && native
				continue
			}
			nativeByProtocol[source] = native
			protocols = append(protocols, source)
		}
	}
	if len(upstreamModels) == 0 {
		return RequestableModel{}, false
	}

	var nativeProtocols []capability.ProtocolID
	for _, source := range protocols {
		if nativeByProtocol[source] {
			nativeProtocols = append(nativeProtocols, source)
		}
	}
	slices.Sort(upstreamModels)
	resolved := RequestableModel{ID: requestedModel, Protocols: protocols, NativeProtocols: nativeProtocols, UpstreamModels: slices.Compact(upstreamModels)}
	switch billingSource {
	case BillingModelSourceRequested:
		resolved.PricingModel = requestedModel
	case BillingModelSourceUpstream:
		resolved.PricingModel, resolved.PricingAmbiguous = uniquePricingModel(upstreamModels)
	default:
		resolved.PricingModel = groupMappedModel
	}
	return resolved, true
}

func uniquePricingModel(models []string) (string, bool) {
	var selected string
	selectedKey := ""
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		key := strings.ToLower(model)
		if selectedKey == "" {
			selected = model
			selectedKey = key
			continue
		}
		if key != selectedKey {
			return "", true
		}
	}
	return selected, false
}

// RequestableModelIDs 返回保持解析顺序的客户端模型 ID。
func RequestableModelIDs(models []RequestableModel) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}
	return ids
}

// matchesCataloguePlatform 只把平台参数用于专用入口的强制过滤。
func matchesCataloguePlatform(provider *CatalogueProvider, platform string) bool {
	return platform == "" || provider.Platform == platform
}
