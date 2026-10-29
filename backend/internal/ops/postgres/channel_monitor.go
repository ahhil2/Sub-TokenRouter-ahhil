package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/ops"
)

const (
	channelMonitorSQL = `
WITH visible_groups AS MATERIALIZED (
 SELECT id, name, COALESCE(NULLIF(display_brand, ''), name) AS brand, rate_multiplier, sort_order, config.status AS status_override
 FROM groups
 JOIN jsonb_to_recordset(COALESCE((SELECT value::jsonb FROM settings WHERE key = 'channel_monitor_groups'), '[]'::jsonb))
 AS config(group_id bigint, enabled bool, status text) ON config.group_id = groups.id AND config.enabled
 WHERE groups.deleted_at IS NULL AND groups.status = 'active' AND NOT groups.is_exclusive
 AND EXISTS (SELECT 1 FROM provider_groups pg JOIN providers p ON p.id = pg.provider_id
             WHERE pg.group_id = groups.id AND p.deleted_at IS NULL AND p.status = 'active')
 ORDER BY sort_order, id LIMIT 200
), successful AS MATERIALIZED (
 SELECT DISTINCT ON (ul.group_id, ul.api_key_id, COALESCE(NULLIF(ul.request_id, ''), ul.id::text))
 ul.group_id, ul.api_key_id, ul.request_id, ul.created_at,
 ul.input_tokens, ul.cache_creation_tokens, ul.cache_read_tokens,
 %s AS ttft
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
 COALESCE(m.requests, 0), COALESCE(m.successes, 0), m.cache_rate, m.ttft, g.status_override
FROM visible_groups g LEFT JOIN metrics m ON m.group_id = g.id
ORDER BY g.sort_order, g.id, m.bucket`
	channelMonitorConfigSQL = `
SELECT g.id, g.name, COALESCE(NULLIF(g.display_brand, ''), g.name),
 COALESCE(config.enabled, false), COALESCE(config.status, 'auto'),
 g.status = 'active' AND NOT g.is_exclusive AND EXISTS (
 SELECT 1 FROM provider_groups pg JOIN providers p ON p.id = pg.provider_id
 WHERE pg.group_id = g.id AND p.deleted_at IS NULL AND p.status = 'active') AS eligible
FROM groups g
LEFT JOIN jsonb_to_recordset(COALESCE((SELECT value::jsonb FROM settings WHERE key = 'channel_monitor_groups'), '[]'::jsonb))
 AS config(group_id bigint, enabled bool, status text) ON config.group_id = g.id
WHERE g.deleted_at IS NULL ORDER BY g.sort_order, g.id`
	saveChannelMonitorConfigSQL = `
INSERT INTO settings (key, value, updated_at)
SELECT 'channel_monitor_groups', $1::text, NOW()
WHERE NOT EXISTS (
 SELECT 1 FROM jsonb_to_recordset($1::text::jsonb) AS config(group_id bigint)
 WHERE NOT EXISTS (SELECT 1 FROM groups WHERE id = config.group_id AND deleted_at IS NULL))
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`
)

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
	rows, err := store.db.QueryContext(ctx, fmt.Sprintf(channelMonitorSQL, usageTTFTSQL("ul"), sla), start, end, int(slotDuration/time.Second))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	positions := map[int64]int{}
	for rows.Next() {
		var channel ops.MonitoredChannel
		var bucket int
		var cacheRate, ttft sql.NullFloat64
		var metrics ops.ChannelMonitorMetrics
		if err := rows.Scan(&channel.ID, &channel.Name, &channel.Brand, &channel.RateMultiplier, &bucket,
			&metrics.Requests, &metrics.Successes, &cacheRate, &ttft, &channel.StatusOverride); err != nil {
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

func (store *Store) ReadChannelMonitorConfig(ctx context.Context) ([]ops.ChannelMonitorGroupOption, error) {
	rows, err := store.db.QueryContext(ctx, channelMonitorConfigSQL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	groups := []ops.ChannelMonitorGroupOption{}
	for rows.Next() {
		var group ops.ChannelMonitorGroupOption
		if err := rows.Scan(&group.GroupID, &group.Name, &group.Brand, &group.Enabled, &group.Status, &group.Eligible); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, rows.Err()
}

func (store *Store) SaveChannelMonitorConfig(ctx context.Context, groups []ops.ChannelMonitorGroupConfig) error {
	payload, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	result, err := store.db.ExecContext(ctx, saveChannelMonitorConfigSQL, string(payload))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ops.ErrInvalidMonitorConfig
	}
	return nil
}
