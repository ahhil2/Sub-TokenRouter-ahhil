package routing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// versionRules 返回缓存测试使用的模型名称。
type versionRules struct {
	CatalogueRules
	id string
}

// TestModelListVersionsAndInvalidation 目录换版和分组失效均清除旧候选的读取机会。
func TestModelListVersionsAndInvalidation(t *testing.T) {
	version, id, reads := "v1", "first", 0
	list := NewModelList(func(context.Context, *int64) ([]CatalogueProvider, error) {
		reads++
		return []CatalogueProvider{{Rules: versionRules{id: id}}}, nil
	}, time.Minute)
	list.Version = func() string { return version }
	group := int64(2)
	require.Equal(t, []string{"first"}, list.Available(context.Background(), &group, ""))
	id = "second"
	require.Equal(t, []string{"first"}, list.Available(context.Background(), &group, ""))
	version = "v2"
	require.Equal(t, []string{"second"}, list.Available(context.Background(), &group, ""))
	require.Equal(t, 2, reads)
	list.Invalidate(&group, "")
	require.Empty(t, list.Cache.Items())
	list.Cache.Set(ModelListCacheKey(&group, "openai")+"|v2", []string{"old"}, time.Minute)
	list.Invalidate(&group, "openai")
	require.Empty(t, list.Cache.Items())
}

func (r versionRules) ConfiguredModels() []string { return []string{r.id} }
