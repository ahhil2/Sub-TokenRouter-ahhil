package ops

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	ChannelMonitorSlots   = 20
	ChannelMonitorWindow  = 90 * time.Minute
	ChannelMonitorRefresh = 30 * time.Second
)

type ChannelMonitorMetrics struct {
	Requests     int64    `json:"requests"`
	Successes    int64    `json:"successes"`
	Availability *float64 `json:"availability"`
	CacheRate    *float64 `json:"cache_rate"`
	TTFTP50MS    *float64 `json:"ttft_p50_ms"`
}

type ChannelMonitorSlot struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	ChannelMonitorMetrics
}

type MonitoredChannel struct {
	ID             int64   `json:"id"`
	Name           string  `json:"name"`
	Brand          string  `json:"brand"`
	RateMultiplier float64 `json:"rate_multiplier"`
	ChannelMonitorMetrics
	Slots []ChannelMonitorSlot `json:"slots"`
}

type ChannelMonitorSnapshot struct {
	GeneratedAt       time.Time          `json:"generated_at"`
	WindowMinutes     int                `json:"window_minutes"`
	SlotSeconds       int                `json:"slot_seconds"`
	RefreshSeconds    int                `json:"refresh_seconds"`
	MonitoringEnabled bool               `json:"monitoring_enabled"`
	Channels          []MonitoredChannel `json:"channels"`
}

type ChannelMonitorRepository interface {
	ReadChannelMonitor(context.Context, time.Time) (*ChannelMonitorSnapshot, error)
}

type ChannelMonitor struct {
	repository ChannelMonitorRepository
	mutex      sync.RWMutex
	cached     *ChannelMonitorSnapshot
	expires    time.Time
	load       singleflight.Group
}

func NewChannelMonitor(repository ChannelMonitorRepository) *ChannelMonitor {
	return &ChannelMonitor{repository: repository}
}

func (monitor *ChannelMonitor) Snapshot(ctx context.Context) (*ChannelMonitorSnapshot, error) {
	if snapshot := monitor.fresh(); snapshot != nil {
		return snapshot, nil
	}
	result := monitor.load.DoChan("snapshot", func() (any, error) {
		if snapshot := monitor.fresh(); snapshot != nil {
			return snapshot, nil
		}
		queryContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		end := time.Now().UTC().Truncate(ChannelMonitorRefresh)
		snapshot, err := monitor.repository.ReadChannelMonitor(queryContext, end)
		if err != nil {
			return nil, err
		}
		snapshot.GeneratedAt = end
		snapshot.WindowMinutes = int(ChannelMonitorWindow / time.Minute)
		snapshot.SlotSeconds = int(ChannelMonitorWindow / ChannelMonitorSlots / time.Second)
		snapshot.RefreshSeconds = int(ChannelMonitorRefresh / time.Second)
		if snapshot.Channels == nil {
			snapshot.Channels = []MonitoredChannel{}
		}
		monitor.mutex.Lock()
		monitor.cached = snapshot
		monitor.expires = time.Now().Add(ChannelMonitorRefresh)
		monitor.mutex.Unlock()
		return snapshot, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case value := <-result:
		if value.Err != nil {
			return nil, value.Err
		}
		return value.Val.(*ChannelMonitorSnapshot), nil
	}
}

func (monitor *ChannelMonitor) fresh() *ChannelMonitorSnapshot {
	monitor.mutex.RLock()
	defer monitor.mutex.RUnlock()
	if time.Now().Before(monitor.expires) {
		return monitor.cached
	}
	return nil
}
