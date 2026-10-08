package completion

import (
	"context"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

func (s *Recorder) WriteUsage(ctx context.Context, usageLog *UsageLog, logKey string) {
	if s.logs == nil || usageLog == nil {
		return
	}
	applyClientModel(ctx, usageLog)
	usageCtx, cancel := detachedBillingContext(ctx)
	defer cancel()

	if writer, ok := s.logs.(BestEffortLogWriter); ok {
		if err := writer.CreateBestEffort(usageCtx, usageLog); err != nil {
			s.printf(logKey, "Create usage log failed: %v", err)
			// 队列超时丢弃的用量转为同步写入，已结算和待对账的请求都需要 usage_log。
			// 结算失败记录用 ActualCost=0 表示未扣费，重复写入由 ON CONFLICT (request_id, api_key_id) DO NOTHING 处理。
			fallbackCtx := usageCtx
			if usageCtx.Err() != nil {
				// 入队等待已耗尽 usageCtx，使用新的独立超时窗口同步写入。
				var fallbackCancel context.CancelFunc
				fallbackCtx, fallbackCancel = detachedBillingContext(context.Background())
				defer fallbackCancel()
			}
			if _, syncErr := s.logs.Create(fallbackCtx, usageLog); syncErr != nil {
				s.printf(logKey, "Create usage log sync fallback failed: %v", syncErr)
			}
		}
		return
	}

	if _, err := s.logs.Create(usageCtx, usageLog); err != nil {
		s.printf(logKey, "Create usage log failed: %v", err)
	}
}

// applyProviderStatsCost 查询价格并计算提供商基础成本，用户实扣金额单独计算。
func (s *Recorder) applyProviderStatsCost(ctx context.Context, row *UsageLog, providerID, groupID int64, upstream, requested, mapped string, tokens UsageTokens) {
	if upstream == "" {
		upstream = requested
	}
	count := 1
	if row.ImageCount > 0 {
		count = row.ImageCount
	}
	source := s.stats
	if s.resolver != nil {
		source = s.resolver
	}
	if source == nil {
		return
	}
	row.ProviderStatsCost = source.ResolveProviderStats(ctx, billing.ProviderStatsCostInput{
		PreferRequestedModel: row.Platform == "qoder", ProviderID: providerID, GroupID: groupID, UpstreamModel: upstream, RequestedModel: requested, MappedModel: mapped, Tokens: tokens, RequestCount: count, ServiceTier: stringValueOrEmpty(row.ServiceTier), ReasoningEffort: stringValueOrEmpty(row.ReasoningEffort),
	})
}
