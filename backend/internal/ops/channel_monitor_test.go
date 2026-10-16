package ops

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type channelMonitorStub struct {
	calls atomic.Int64
	fail  bool
	gate  chan struct{}
	saved []ChannelMonitorGroupConfig
}

func (repository *channelMonitorStub) ReadChannelMonitorConfig(context.Context) ([]ChannelMonitorGroupOption, error) {
	return []ChannelMonitorGroupOption{}, nil
}

func (repository *channelMonitorStub) SaveChannelMonitorConfig(_ context.Context, groups []ChannelMonitorGroupConfig) error {
	if repository.fail {
		return errors.New("database unavailable")
	}
	repository.saved = groups
	return nil
}

func (repository *channelMonitorStub) ReadChannelMonitor(ctx context.Context, end time.Time) (*ChannelMonitorSnapshot, error) {
	repository.calls.Add(1)
	if repository.gate != nil {
		select {
		case <-repository.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if repository.fail {
		return nil, errors.New("database unavailable")
	}
	return &ChannelMonitorSnapshot{MonitoringEnabled: true}, nil
}

func TestChannelMonitorCacheAndExpiry(t *testing.T) {
	repository := &channelMonitorStub{}
	monitor := NewChannelMonitor(repository)
	first, err := monitor.Snapshot(t.Context())
	require.NoError(t, err)
	require.Equal(t, 90, first.WindowMinutes)
	require.Equal(t, 270, first.SlotSeconds)
	require.Equal(t, 30, first.RefreshSeconds)
	require.NotNil(t, first.Channels)
	second, err := monitor.Snapshot(t.Context())
	require.NoError(t, err)
	require.Same(t, first, second)
	require.Equal(t, int64(1), repository.calls.Load())
	monitor.mutex.Lock()
	monitor.expires = time.Now().Add(-time.Second)
	monitor.mutex.Unlock()
	_, err = monitor.Snapshot(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2), repository.calls.Load())
}

func TestChannelMonitorConcurrentReaders(t *testing.T) {
	repository := &channelMonitorStub{gate: make(chan struct{})}
	monitor := NewChannelMonitor(repository)
	var readers sync.WaitGroup
	failures := make(chan error, 12)
	for range 12 {
		readers.Go(func() { _, err := monitor.Snapshot(t.Context()); failures <- err })
	}
	require.Eventually(t, func() bool { return repository.calls.Load() == 1 }, time.Second, time.Millisecond)
	close(repository.gate)
	readers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	require.Equal(t, int64(1), repository.calls.Load())
}

func TestChannelMonitorFailureAndCancellation(t *testing.T) {
	monitor := NewChannelMonitor(&channelMonitorStub{fail: true})
	_, err := monitor.Snapshot(t.Context())
	require.Error(t, err)
	require.Nil(t, monitor.fresh())
	repository := &channelMonitorStub{gate: make(chan struct{})}
	monitor = NewChannelMonitor(repository)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = monitor.Snapshot(ctx)
	require.ErrorIs(t, err, context.Canceled)
	close(repository.gate)
	_, err = monitor.Snapshot(t.Context())
	require.NoError(t, err)
}

func TestChannelMonitorConfigurationValidation(t *testing.T) {
	valid := ChannelMonitorGroupConfig{GroupID: 1, Enabled: true, Status: "auto"}
	for name, groups := range map[string][]ChannelMonitorGroupConfig{
		"missing":        nil,
		"invalid id":     {{GroupID: 0, Status: "auto"}},
		"invalid status": {{GroupID: 1, Status: "healthy"}},
		"duplicate":      {valid, valid},
		"too many":       make([]ChannelMonitorGroupConfig, 201),
	} {
		t.Run(name, func(t *testing.T) {
			repository := &channelMonitorStub{}
			err := NewChannelMonitor(repository).SaveConfiguration(t.Context(), groups)
			require.ErrorIs(t, err, ErrInvalidMonitorConfig)
			require.Nil(t, repository.saved)
		})
	}
	for _, status := range []string{"auto", "operational", "degraded", "failed", "maintenance"} {
		valid.Status = status
		require.NoError(t, NewChannelMonitor(&channelMonitorStub{}).SaveConfiguration(t.Context(), []ChannelMonitorGroupConfig{valid}))
	}
}

func TestChannelMonitorSaveInvalidatesCache(t *testing.T) {
	repository := &channelMonitorStub{}
	monitor := NewChannelMonitor(repository)
	_, err := monitor.Snapshot(t.Context())
	require.NoError(t, err)
	require.NoError(t, monitor.SaveConfiguration(t.Context(), []ChannelMonitorGroupConfig{}))
	require.NotNil(t, repository.saved)
	require.Nil(t, monitor.fresh())
	_, err = monitor.Snapshot(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2), repository.calls.Load())
	repository.fail = true
	require.Error(t, monitor.SaveConfiguration(t.Context(), []ChannelMonitorGroupConfig{}))
	require.NotNil(t, monitor.fresh())
}

func TestChannelMonitorSaveWaitsForInflightSnapshot(t *testing.T) {
	repository := &channelMonitorStub{gate: make(chan struct{})}
	monitor := NewChannelMonitor(repository)
	readDone := make(chan error, 1)
	go func() { _, err := monitor.Snapshot(t.Context()); readDone <- err }()
	require.Eventually(t, func() bool { return repository.calls.Load() == 1 }, time.Second, time.Millisecond)
	saveDone := make(chan error, 1)
	go func() { saveDone <- monitor.SaveConfiguration(t.Context(), []ChannelMonitorGroupConfig{}) }()
	close(repository.gate)
	require.NoError(t, <-readDone)
	require.NoError(t, <-saveDone)
	require.Nil(t, monitor.fresh())
	_, err := monitor.Snapshot(t.Context())
	require.NoError(t, err)
	require.Equal(t, int64(2), repository.calls.Load())
}
