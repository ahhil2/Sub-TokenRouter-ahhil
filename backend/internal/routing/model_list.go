package routing

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	gocache "github.com/patrickmn/go-cache"
)

var sharedModelListMetrics ModelListMetrics

// ModelList 维护原模型列表短缓存，构造时不启动 janitor，由应用时间轮执行到期清理。
type ModelList struct {
	Version func() string
	Cache   *gocache.Cache
	TTL     time.Duration
	Read    func(context.Context, *int64) ([]CatalogueProvider, error)
}

type ModelListMetrics struct{ Hit, Miss, Store atomic.Int64 }

func NewModelList(read func(context.Context, *int64) ([]CatalogueProvider, error), ttl time.Duration) *ModelList {
	return &ModelList{Cache: gocache.New(ttl, 0), TTL: ttl, Read: read}
}

// Available 返回分组下可见的模型列表。
// 它会聚合每个提供商显式配置的“可请求模型”（model_mapping 的 key 或独立 model_whitelist）。
func (s *ModelList) Available(ctx context.Context, groupID *int64, platform string) []string {
	cacheKey := ModelListCacheKey(groupID, platform)
	if s.Version != nil {
		cacheKey += "|" + s.Version()
	}
	if s.Cache != nil {
		if cached, found := s.Cache.Get(cacheKey); found {
			if models, ok := cached.([]string); ok {
				sharedModelListMetrics.Hit.Add(1)
				return slices.Clone(models)
			}
		}
	}
	sharedModelListMetrics.Miss.Add(1)

	var providers []CatalogueProvider
	var err error

	providers, err = s.Read(ctx, groupID)

	if err != nil || len(providers) == 0 {
		return nil
	}

	models := ConfiguredRequestModelsFromProviders(providers, platform)
	// 具体配置为空时，后续解析仍会合并统一目录候选。
	if len(models) == 0 {
		if s.Cache != nil {
			s.Cache.Set(cacheKey, []string(nil), s.TTL)
			sharedModelListMetrics.Store.Add(1)
		}
		return nil
	}

	if s.Cache != nil {
		s.Cache.Set(cacheKey, slices.Clone(models), s.TTL)
		sharedModelListMetrics.Store.Add(1)
	}
	return slices.Clone(models)
}

func (s *ModelList) Invalidate(groupID *int64, platform string) {
	if s == nil || s.Cache == nil {
		return
	}

	normalizedPlatform := strings.TrimSpace(platform)
	// 配置变化需要清理对应分组和平台的所有目录版本。
	targetGroup := modelListGroupID(groupID)
	for key := range s.Cache.Items() {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) < 2 {
			continue
		}
		groupPart, parseErr := strconv.ParseInt(parts[0], 10, 64)
		if parseErr != nil {
			continue
		}
		if groupID != nil && groupPart != targetGroup {
			continue
		}
		if normalizedPlatform != "" && parts[1] != normalizedPlatform {
			continue
		}
		s.Cache.Delete(key)
	}
}

func ModelListCacheKey(groupID *int64, platform string) string {
	return fmt.Sprintf("%d|%s", modelListGroupID(groupID), strings.TrimSpace(platform))
}

func (s *ModelList) Expire() {
	if s != nil && s.Cache != nil {
		s.Cache.DeleteExpired()
	}
}

// SharedModelListMetrics 延续全进程唯一指标，旧 Ops 与测试只取得同一状态的引用。
func SharedModelListMetrics() *ModelListMetrics { return &sharedModelListMetrics }

func modelListGroupID(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}
