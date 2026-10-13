package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

const channelMonitorSQL = `
WITH visible_groups AS MATERIALIZED (
 SELECT id, name, COALESCE(NULLIF(display_brand, ''), name) AS brand, rate_multiplier, sort_order
 FROM groups
 WHERE deleted_at IS NULL AND status = 'active' AND NOT is_exclusive
 AND EXISTS (SELECT 1 FROM provider_groups pg JOIN providers p ON p.id = pg.provider_id
             WHERE pg.group_id = groups.id AND p.deleted_at IS NULL AND p.status = 'active')
 ORDER BY sort_order, id LIMIT 200
), successful AS MATERIALIZED (
 SELECT DISTINCT ON (ul.group_id, ul.api_key_id, COALESCE(NULLIF(ul.request_id, ''), ul.id::text))
 ul.group_id, ul.api_key_id, ul.request_id, ul.created_at,
 ul.input_tokens, ul.cache_creation_tokens, ul.cache_read_tokens,
 CASE WHEN ul.stream AND ul.first_token_ms > 0 THEN ul.first_token_ms END AS ttft
 FROM usage_logs ul JOIN visible_groups g ON g.id = ul.group_id
 WHERE ul.created_at >= $1 AND ul.created_at < $2
 ORDER BY ul.group_id, ul.api_key_id, COALESCE(NULLIF(ul.request_id, ''), ul.id::text), ul.created_at DESC
), failed AS (
 SELECT DISTINCT ON (e.group_id, e.api_key_id, COALESCE(NULLIF(e.request_id, ''), e.id::text))
 e.group_id, e.created_at
 FROM ops_error_logs e JOIN visible_groups g ON g.id = e.group_id
 WHERE e.created_at >= $1 AND e.created_at < $2 AND NOT e.is_count_tokens
 AND COALESCE(e.status_code, 0) >= 400 AND %s
 AND NOT EXISTS (SELECT 1 FROM successful s WHERE s.group_id = e.group_id
                 AND s.api_key_id = e.api_key_id AND s.request_id = e.request_id AND s.request_id <> '')
 ORDER BY e.group_id, e.api_key_id, COALESCE(NULLIF(e.request_id, ''), e.id::text), e.created_at DESC
), events AS (
 SELECT group_id, created_at, 1::bigint AS succeeded, input_tokens::bigint AS input,
 cache_creation_tokens::bigint AS written, cache_read_tokens::bigint AS cached, ttft FROM successful
 UNION ALL
 SELECT group_id, created_at, 0, 0, 0, 0, NULL FROM failed
), bucketed AS (
 SELECT *, FLOOR(EXTRACT(EPOCH FROM (created_at - $1::timestamptz)) / $3)::int AS bucket FROM events
), metrics AS (
 SELECT group_id, COALESCE(bucket, -1) AS bucket, COUNT(*) AS requests, SUM(succeeded) AS successes,
 SUM(cached)::float8 / NULLIF(SUM(input + written + cached), 0) * 100 AS cache_rate,
 percentile_cont(0.5) WITHIN GROUP (ORDER BY ttft) FILTER (WHERE ttft IS NOT NULL) AS ttft
 FROM bucketed GROUP BY GROUPING SETS ((group_id, bucket), (group_id))
)
SELECT g.id, g.name, g.brand, g.rate_multiplier, COALESCE(m.bucket, -1),
 COALESCE(m.requests, 0), COALESCE(m.successes, 0), m.cache_rate, m.ttft
FROM visible_groups g LEFT JOIN metrics m ON m.group_id = g.id
ORDER BY g.sort_order, g.id, m.bucket`

func (store *Store) ReadChannelMonitor(ctx context.Context, end time.Time) (*ops.ChannelMonitorSnapshot, error) {
	snapshot := &ops.ChannelMonitorSnapshot{Channels: []ops.MonitoredChannel{}}
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE((SELECT value <> 'false' FROM settings WHERE key = 'ops_monitoring_enabled'), true)`).Scan(&snapshot.MonitoringEnabled); err != nil {
		return nil, err
	}
	if !snapshot.MonitoringEnabled {
		return snapshot, nil
	}
	start := end.Add(-ops.ChannelMonitorWindow)
	slotDuration := ops.ChannelMonitorWindow / ops.ChannelMonitorSlots
	sla := opsSLACountableSQL("e.status_code", "e.upstream_status_code", "e.upstream_errors", "e.error_owner", "e.is_business_limited", nil)
	rows, err := store.db.QueryContext(ctx, fmt.Sprintf(channelMonitorSQL, sla), start, end, int(slotDuration/time.Second))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	positions := map[int64]int{}
	for rows.Next() {
		var channel ops.MonitoredChannel
		var bucket int
		var cacheRate, ttft sql.NullFloat64
		var metrics ops.ChannelMonitorMetrics
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Brand, &channel.RateMultiplier, &bucket,
			&metrics.Requests, &metrics.Successes, &cacheRate, &ttft); err != nil {
			return nil, err
		}
		if metrics.Requests > 0 {
			availability := float64(metrics.Successes) / float64(metrics.Requests) * 100
			metrics.Availability = &availability
		}
		if cacheRate.Valid {
			metrics.CacheRate = &cacheRate.Float64
		}
		if ttft.Valid {
			metrics.TTFTP50MS = &ttft.Float64
		}
		position, exists := positions[channel.ID]
		if !exists {
			position = len(snapshot.Channels)
			positions[channel.ID] = position
			channel.Slots = make([]ops.ChannelMonitorSlot, ops.ChannelMonitorSlots)
			for slot := range channel.Slots {
				channel.Slots[slot].Start = start.Add(time.Duration(slot) * slotDuration)
				channel.Slots[slot].End = channel.Slots[slot].Start.Add(slotDuration)
			}
			snapshot.Channels = append(snapshot.Channels, channel)
		}
		if bucket == -1 {
			snapshot.Channels[position].ChannelMonitorMetrics = metrics
		} else if bucket >= 0 && bucket < ops.ChannelMonitorSlots {
			snapshot.Channels[position].Slots[bucket].ChannelMonitorMetrics = metrics
		}
	}
	return snapshot, rows.Err()
}
