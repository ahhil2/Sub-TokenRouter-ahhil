//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	postgresinfra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/site"
	"github.com/TokenFlux/TokenRouter/migrations"
)

func TestPelicanRepository(t *testing.T) {
	dsn := os.Getenv("PELICAN_TEST_DSN")
	if dsn == "" {
		t.Skip("PELICAN_TEST_DSN must point to an isolated pelican_test database")
	}
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	require.Equal(t, "/pelican_test", parsed.Path)
	database, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	ctx := context.Background()
	require.NoError(t, postgresinfra.ApplyMigrations(ctx, database, migrations.FS))
	require.NoError(t, postgresinfra.ApplyMigrations(ctx, database, migrations.FS))
	_, err = database.ExecContext(ctx, `TRUNCATE pelican_results,pelican_tasks RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
	var groupID int64
	err = database.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES('pelican-integration-' || gen_random_uuid()::text) RETURNING id`).Scan(&groupID)
	require.NoError(t, err)
	repository := NewPelicanRepository(database)
	tasks := []site.PelicanTask{}
	for _, model := range []string{"first", "second", "third"} {
		task, saveErr := repository.SaveTask(ctx, site.PelicanTask{GroupID: groupID, Model: model, IntervalMinutes: 360, PublicConsent: true})
		require.NoError(t, saveErr)
		tasks = append(tasks, task)
	}
	claim, err := repository.Claim(ctx)
	require.NoError(t, err)
	require.Nil(t, claim)
	for _, task := range tasks {
		require.NoError(t, repository.RequestRun(ctx, task.ID))
	}
	require.Error(t, repository.RequestRun(ctx, tasks[0].ID))
	var workers sync.WaitGroup
	claims := make(chan *site.PelicanClaim, 8)
	for index := 0; index < 8; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			claimed, claimErr := repository.Claim(ctx)
			if claimErr != nil {
				t.Error(claimErr)
			}
			if claimed != nil {
				claims <- claimed
			}
		}()
	}
	workers.Wait()
	close(claims)
	active := []*site.PelicanClaim{}
	for value := range claims {
		active = append(active, value)
	}
	require.Len(t, active, 2)
	require.NotEqual(t, active[0].Task.ID, active[1].Task.ID)
	require.Error(t, repository.DeleteTask(ctx, active[0].Task.ID))
	finished := time.Now()
	result := site.PelicanResult{Status: "success", RawResponse: "<svg></svg>", HTML: "<svg></svg>", ErrorMessage: "private detail", ExecutionModel: "private model", FinishedAt: &finished}
	require.NoError(t, repository.Finish(ctx, *active[0], result))
	require.Error(t, repository.Finish(ctx, *active[0], result))
	public, err := repository.Result(ctx, active[0].ResultID, true)
	require.NoError(t, err)
	require.Empty(t, public.RawResponse)
	require.Empty(t, public.ErrorMessage)
	require.Empty(t, public.ExecutionModel)
	require.Equal(t, "<svg></svg>", public.HTML)
	admin, err := repository.Result(ctx, active[0].ResultID, false)
	require.NoError(t, err)
	require.Equal(t, "private detail", admin.ErrorMessage)
	page, err := repository.Results(ctx, site.PelicanFilter{Page: 1, Public: true})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Empty(t, page.Items[0].HTML)
	require.NoError(t, repository.Publish(ctx, active[0].ResultID, false))
	_, err = repository.Result(ctx, active[0].ResultID, true)
	require.ErrorIs(t, err, site.ErrPelicanNotFound)
	page, err = repository.Results(ctx, site.PelicanFilter{Page: 1, Public: true})
	require.NoError(t, err)
	require.Empty(t, page.Items)
	require.NoError(t, repository.Publish(ctx, active[0].ResultID, true))
	_, err = database.ExecContext(ctx, `UPDATE pelican_tasks SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, active[1].Task.ID)
	require.NoError(t, err)
	require.NoError(t, repository.Maintain(ctx))
	interrupted, err := repository.Result(ctx, active[1].ResultID, true)
	require.NoError(t, err)
	require.Equal(t, "interrupted", interrupted.Status)
	require.Error(t, repository.Finish(ctx, *active[1], result))
	_, err = database.ExecContext(ctx, `UPDATE pelican_results SET started_at=NOW()-INTERVAL '31 days' WHERE id=$1`, active[0].ResultID)
	require.NoError(t, err)
	_, err = repository.Result(ctx, active[0].ResultID, true)
	require.ErrorIs(t, err, site.ErrPelicanNotFound)
	require.NoError(t, repository.Maintain(ctx))
	_, err = repository.Result(ctx, active[0].ResultID, false)
	require.ErrorIs(t, err, site.ErrPelicanNotFound)
	third, err := repository.Claim(ctx)
	require.NoError(t, err)
	require.NotNil(t, third)
	require.NoError(t, repository.Finish(ctx, *third, result))
	require.NoError(t, repository.DeleteTask(ctx, third.Task.ID))
	_, err = repository.Result(ctx, third.ResultID, false)
	require.ErrorIs(t, err, site.ErrPelicanNotFound)
}
