package ops

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	ChannelMonitorSlots   = 20
	ChannelMonitorWindow  = 90 * time.Minute
	ChannelMonitorRefresh = 30 * time.Second
)

var ErrInvalidMonitorConfig = errors.New("invalid channel monitor configuration")

type ChannelMonitorGroupConfig struct {
	GroupID int64  `json:"group_id"`
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
}

type ChannelMonitorGroupOption struct {
	ChannelMonitorGroupConfig
	Name     string `json:"name"`
	Brand    string `json:"brand"`
	Eligible bool   `json:"eligible"`
}

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
	StatusOverride string  `json:"status_override"`
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
	ReadChannelMonitorConfig(context.Context) ([]ChannelMonitorGroupOption, error)
	SaveChannelMonitorConfig(context.Context, []ChannelMonitorGroupConfig) error
}

type ChannelMonitor struct {
	repository ChannelMonitorRepository
	mutex      sync.RWMutex
	cached     *ChannelMonitorSnapshot
	expires    time.Time
	load       singleflight.Group
	configLock sync.RWMutex
}

func NewChannelMonitor(repository ChannelMonitorRepository) *ChannelMonitor {
	return &ChannelMonitor{repository: repository}
}

func (monitor *ChannelMonitor) Snapshot(ctx context.Context) (*ChannelMonitorSnapshot, error) {
	if snapshot := monitor.fresh(); snapshot != nil {
		return snapshot, nil
	}
	result := monitor.load.DoChan("snapshot", func() (any, error) {
		monitor.configLock.RLock()
		defer monitor.configLock.RUnlock()
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
		snapshot, ok := value.Val.(*ChannelMonitorSnapshot)
		if !ok {
			return nil, errors.New("invalid channel monitor snapshot")
		}
		return snapshot, nil
	}
}

func (monitor *ChannelMonitor) Configuration(ctx context.Context) ([]ChannelMonitorGroupOption, error) {
	return monitor.repository.ReadChannelMonitorConfig(ctx)
}

func (monitor *ChannelMonitor) SaveConfiguration(ctx context.Context, groups []ChannelMonitorGroupConfig) error {
	if groups == nil || len(groups) > 200 {
		return ErrInvalidMonitorConfig
	}
	seen := make(map[int64]bool, len(groups))
	for _, group := range groups {
		if group.GroupID <= 0 || seen[group.GroupID] {
			return ErrInvalidMonitorConfig
		}
		seen[group.GroupID] = true
		switch group.Status {
		case "auto", "operational", "degraded", "failed", "maintenance":
		default:
			return ErrInvalidMonitorConfig
		}
	}
	monitor.configLock.Lock()
	defer monitor.configLock.Unlock()
	if err := monitor.repository.SaveChannelMonitorConfig(ctx, groups); err != nil {
		return err
	}
	monitor.mutex.Lock()
	monitor.cached = nil
	monitor.expires = time.Time{}
	monitor.mutex.Unlock()
	monitor.load.Forget("snapshot")
	return nil
}

func (monitor *ChannelMonitor) fresh() *ChannelMonitorSnapshot {
	monitor.mutex.RLock()
	defer monitor.mutex.RUnlock()
	if time.Now().Before(monitor.expires) {
		return monitor.cached
	}
	return nil
}
