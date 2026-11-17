//go:build integration

package postgres

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	postgresinfra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/site"
	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
	"github.com/TokenFlux/TokenRouter/migrations"
)

// pelican_integration_test.go exercises task persistence and lease ownership in
// an isolated PostgreSQL container. Concurrent claims test shared capacity while
// configured generation budgets determine lease duration, and completed results
// follow the repository's publication and retention rules.

// TestPelicanRepository saves distinct task budgets and claims tasks concurrently.
// Stored limits survive partial updates, leases cover their execution budgets,
// and recovery prevents expired workers from replacing interrupted results.
func TestPelicanRepository(t *testing.T) {
	database := postgrescontainer.New(t)
	ctx := t.Context()
	require.NoError(t, postgresinfra.ApplyMigrations(ctx, database, migrations.FS))
	require.NoError(t, postgresinfra.ApplyMigrations(ctx, database, migrations.FS))
	var groupID int64
	err := database.QueryRowContext(ctx, `INSERT INTO groups(name) VALUES('pelican-integration-' || gen_random_uuid()::text) RETURNING id`).Scan(&groupID)
	require.NoError(t, err)
	repository := NewPelicanRepository(database)
	tasks := []site.PelicanTask{}
	for index, model := range []string{"first", "second", "third"} {
		task, saveErr := repository.SaveTask(ctx, site.PelicanTask{GroupID: groupID, Model: model, IntervalMinutes: 360, TimeoutSeconds: []int{1, 600, 3600}[index], PublicConsent: true})
		require.NoError(t, saveErr)
		tasks = append(tasks, task)
	}
	tasks[0].TimeoutSeconds = 900
	saved, err := repository.SaveTask(ctx, tasks[0])
	require.NoError(t, err)
	require.Equal(t, 900, saved.TimeoutSeconds)
	// A client can change the schedule while omitting the timeout; verify
	// the returned task combines that schedule with its previously saved budget.
	omitted := tasks[0]
	omitted.TimeoutSeconds = 0
	omitted.IntervalMinutes = 720
	saved, err = repository.SaveTask(ctx, omitted)
	require.NoError(t, err)
	require.Equal(t, 900, saved.TimeoutSeconds)
	require.Equal(t, 720, saved.IntervalMinutes)
	listed, err := repository.Tasks(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 3)
	require.Equal(t, []int{3600, 600, 900}, []int{listed[0].TimeoutSeconds, listed[1].TimeoutSeconds, listed[2].TimeoutSeconds})
	claim, err := repository.Claim(ctx)
	require.NoError(t, err)
	require.Nil(t, claim)
	for _, task := range tasks {
		require.NoError(t, repository.RequestRun(ctx, task.ID))
	}
	require.Error(t, repository.RequestRun(ctx, tasks[0].ID))
	_, err = repository.SaveTask(ctx, tasks[0])
	require.ErrorIs(t, err, site.ErrPelicanBusy)
	var workers sync.WaitGroup
	claims := make(chan *site.PelicanClaim, 8)
	// Concurrent transactions compete for two slots; collect committed claims
	// after all workers finish so the observed count includes every contender.
	for range 8 {
		workers.Go(func() {
			claimed, claimErr := repository.Claim(ctx)
			if claimErr != nil {
				t.Error(claimErr)
			}
			if claimed != nil {
				claims <- claimed
			}
		})
	}
	workers.Wait()
	close(claims)
	active := []*site.PelicanClaim{}
	for value := range claims {
		active = append(active, value)
	}
	require.Len(t, active, 2)
	require.NotEqual(t, active[0].Task.ID, active[1].Task.ID)
	for _, claimed := range active {
		var leaseSeconds float64
		require.NoError(t, database.QueryRowContext(ctx, `SELECT EXTRACT(EPOCH FROM task.lease_until-result.started_at)
			FROM pelican_tasks task JOIN pelican_results result ON result.task_id=task.id WHERE result.id=$1`, claimed.ResultID).Scan(&leaseSeconds))
		require.Equal(t, float64(claimed.Task.TimeoutSeconds+30), leaseSeconds)
		_, err = repository.SaveTask(ctx, claimed.Task)
		require.ErrorIs(t, err, site.ErrPelicanBusy)
	}
	// Simulate a run that has crossed the former lease duration but remains
	// within its custom budget; maintenance must leave its running record intact.
	_, err = database.ExecContext(ctx, `UPDATE pelican_tasks SET lease_until=lease_until-INTERVAL '340 seconds' WHERE id=$1`, active[0].Task.ID)
	require.NoError(t, err)
	require.NoError(t, repository.Maintain(ctx))
	running, err := repository.Result(ctx, active[0].ResultID, false)
	require.NoError(t, err)
	require.Equal(t, "running", running.Status)
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
