package creative

import (
	"sort"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// groupModelPolicy 固定一次目录查询或执行尝试的分组策略，价格配置不参与模型准入。
type groupModelPolicy struct {
	view *routing.GroupPolicyView
}

func newGroupModelPolicy(policy routing.GroupRoutingPolicy) groupModelPolicy {
	if !policy.Enabled {
		return groupModelPolicy{}
	}
	return groupModelPolicy{view: &routing.GroupPolicyView{
		GroupRoutingPolicy: policy.Clone(),
	}}
}

// resolve 只执行一次分组映射；上游阶段留给具体提供商解析完成后检查。
func (p groupModelPolicy) resolve(requested string) (string, bool) {
	mapped := p.view.ResolveModel(requested)
	if p.view.RestrictionSource() != routing.BillingModelSourceUpstream {
		model := routing.ModelForRestriction(p.view.RestrictionSource(), requested, mapped)
		if p.view.IsModelRestricted(model) {
			return "", false
		}
	}
	return mapped, true
}

func (p groupModelPolicy) allowsUpstream(model string) bool {
	return p.view.RestrictionSource() != routing.BillingModelSourceUpstream || !p.view.IsModelRestricted(model)
}

// candidates 合并可枚举的请求名称，返回可选模型列表。通配符留给后续匹配使用。
func (p groupModelPolicy) candidates(platform string, configured []string, providers []CatalogProvider) []string {
	models := make(map[string]struct{})
	add := func(values ...string) {
		for _, model := range values {
			model = strings.TrimSpace(model)
			if model != "" && !strings.ContainsAny(model, "*?") {
				models[model] = struct{}{}
			}
		}
	}
	add(configured...)
	if p.view != nil {
		add(p.view.AllowedModels...)
		for model := range p.view.ModelMapping {
			add(model)
		}
	}
	for _, provider := range providers {
		if provider == nil || !provider.IsSchedulable() {
			continue
		}
		add(provider.GetConfiguredRequestModels()...)
		for model := range provider.GetModelMapping() {
			add(model)
		}
	}
	result := make([]string, 0, len(models))
	for model := range models {
		result = append(result, model)
	}
	sort.Strings(result)
	return result
}
