package postgres

import (
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestReadChannelMonitorFillsEmptyPeriods(t *testing.T) {
	database, mock := newSQLMock(t)
	store := NewOpsRepository(database)
	end := time.Date(2038, 1, 1, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(true))
	mock.ExpectQuery("WITH visible_groups").WithArgs(end.Add(-90*time.Minute), end, 270).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "brand", "rate", "bucket", "requests", "successes", "cache", "ttft"}).
			AddRow(2, "GPT", "OpenAI", 0.35, -1, 4, 3, 80.0, 1500.0).
			AddRow(2, "GPT", "OpenAI", 0.35, 19, 4, 3, 80.0, 1500.0).
			AddRow(3, "Claude", "Anthropic", 1.3, -1, 0, 0, nil, nil))
	snapshot, err := store.ReadChannelMonitor(t.Context(), end)
	require.NoError(t, err)
	require.Len(t, snapshot.Channels, 2)
	channel := snapshot.Channels[0]
	require.Equal(t, 75.0, *channel.Availability)
	require.Equal(t, 80.0, *channel.CacheRate)
	require.Equal(t, 1500.0, *channel.TTFTP50MS)
	require.Len(t, channel.Slots, 20)
	require.Nil(t, channel.Slots[0].Availability)
	require.Nil(t, channel.Slots[0].CacheRate)
	require.Equal(t, int64(4), channel.Slots[19].Requests)
	require.Equal(t, end.Add(-90*time.Minute), channel.Slots[0].Start)
	require.Equal(t, end, channel.Slots[19].End)
	require.Nil(t, snapshot.Channels[1].Availability)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReadChannelMonitorDisabled(t *testing.T) {
	database, mock := newSQLMock(t)
	mock.ExpectQuery("SELECT COALESCE").WillReturnRows(sqlmock.NewRows([]string{"enabled"}).AddRow(false))
	snapshot, err := NewOpsRepository(database).ReadChannelMonitor(t.Context(), time.Now())
	require.NoError(t, err)
	require.False(t, snapshot.MonitoringEnabled)
	require.Empty(t, snapshot.Channels)
	require.NoError(t, mock.ExpectationsWereMet())
}
