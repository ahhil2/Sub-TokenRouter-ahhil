package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/site"
)

// pelican.go stores drawing tasks, execution leases, and generated results in
// PostgreSQL. Task updates require an idle task so that an active execution
// retains the settings captured when its lease was claimed. Omitted timeout
// updates retain the saved value in the same statement that saves other fields.
//
// Claiming serializes the shared capacity check and task selection, then creates
// the result together with a lease lasting the configured timeout plus a saving
// allowance. Completion requires the matching active token. Maintenance recovers
// expired leases and marks their running results interrupted, preventing delayed
// workers from overwriting recovered executions.

const pelicanResultColumns = `result.id, result.task_id, result.group_id, result.group_name, result.model,
	result.status, result.published, result.latency_ms, result.started_at, result.finished_at`

type pelicanRepository struct{ db *sql.DB }

func NewPelicanRepository(db *sql.DB) site.PelicanRepository { return &pelicanRepository{db: db} }

func (repository *pelicanRepository) Catalog(ctx context.Context) ([]site.PelicanCatalogItem, error) {
	rows, err := repository.db.QueryContext(ctx, `SELECT DISTINCT result.group_id, groups.name, result.model
		FROM pelican_results result JOIN groups ON groups.id=result.group_id
		WHERE result.published AND result.status<>'running' AND groups.deleted_at IS NULL
		AND result.started_at>NOW()-INTERVAL '30 days' ORDER BY groups.name, result.model LIMIT 2000`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []site.PelicanCatalogItem{}
	for rows.Next() {
		var item site.PelicanCatalogItem
		if err := rows.Scan(&item.GroupID, &item.GroupName, &item.Model); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Tasks lists saved settings and recent execution state for administrator editing.
func (repository *pelicanRepository) Tasks(ctx context.Context) ([]site.PelicanTask, error) {
	rows, err := repository.db.QueryContext(ctx, `SELECT row_to_json(item) FROM (
		SELECT task.id, task.group_id, groups.name AS group_name, task.model, task.enabled, task.interval_minutes, task.timeout_seconds,
		task.public_consent, task.manual_requested, task.next_run_at, task.lease_until,
		COALESCE(last_result.status, '') AS last_status, last_result.started_at AS last_run_at
		FROM pelican_tasks task JOIN groups ON groups.id = task.group_id
		LEFT JOIN LATERAL (SELECT status, started_at FROM pelican_results WHERE task_id = task.id ORDER BY started_at DESC, id DESC LIMIT 1) last_result ON TRUE
		ORDER BY task.id DESC) item`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []site.PelicanTask{}
	for rows.Next() {
		var raw []byte
		var item site.PelicanTask
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// SaveTask persists validated task settings and returns the saved timeout and
// next execution time. Creation requires a positive timeout supplied by the
// service, and updates use zero to retain the existing value.
//
// The update checks idle state in the same statement as the configuration
// change, so queued or claimed tasks cannot change their execution budget.
// Interval and enablement changes also update the schedule atomically with the
// settings used by subsequent claims.
func (repository *pelicanRepository) SaveTask(ctx context.Context, task site.PelicanTask) (site.PelicanTask, error) {
	var err error
	if task.ID == 0 {
		err = repository.db.QueryRowContext(ctx, `INSERT INTO pelican_tasks(group_id, model, interval_minutes, public_consent, timeout_seconds)
			SELECT id, $2, $3, $4, $5 FROM groups WHERE id = $1 AND deleted_at IS NULL AND status = 'active' RETURNING id, next_run_at`,
			task.GroupID, task.Model, task.IntervalMinutes, task.PublicConsent, task.TimeoutSeconds).Scan(&task.ID, &task.NextRunAt)
	} else {
		err = repository.db.QueryRowContext(ctx, `UPDATE pelican_tasks SET group_id=$2, model=$3, enabled=$4,
			interval_minutes=$5, public_consent=$6, timeout_seconds=CASE WHEN $7::integer=0 THEN timeout_seconds ELSE $7 END, updated_at=NOW(),
			next_run_at=CASE WHEN $4 AND NOT enabled THEN NOW() WHEN interval_minutes<>$5 THEN NOW()+make_interval(mins=>$5) ELSE next_run_at END
			WHERE id=$1 AND lease_until IS NULL AND NOT manual_requested
			AND EXISTS (SELECT 1 FROM groups WHERE id=$2 AND deleted_at IS NULL AND status='active')
			RETURNING next_run_at, timeout_seconds`, task.ID, task.GroupID, task.Model, task.Enabled, task.IntervalMinutes, task.PublicConsent, task.TimeoutSeconds).Scan(&task.NextRunAt, &task.TimeoutSeconds)
		if errors.Is(err, sql.ErrNoRows) {
			return task, site.ErrPelicanBusy
		}
	}
	return task, pelicanStorageError(err)
}

func (repository *pelicanRepository) DeleteTask(ctx context.Context, id int64) error {
	result, err := repository.db.ExecContext(ctx, `DELETE FROM pelican_tasks WHERE id=$1 AND lease_until IS NULL AND NOT manual_requested`, id)
	return pelicanAffected(result, err)
}

func (repository *pelicanRepository) RequestRun(ctx context.Context, id int64) error {
	result, err := repository.db.ExecContext(ctx, `UPDATE pelican_tasks SET manual_requested=TRUE, updated_at=NOW()
		WHERE id=$1 AND public_consent AND lease_until IS NULL AND NOT manual_requested
		AND EXISTS (SELECT 1 FROM groups WHERE id=group_id AND deleted_at IS NULL AND status='active')`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return site.ErrPelicanBusy
	}
	return err
}

func (repository *pelicanRepository) Results(ctx context.Context, filter site.PelicanFilter) (site.PelicanPage, error) {
	page := site.PelicanPage{Items: []site.PelicanResult{}, Prompt: site.PelicanPrompt}
	columns := pelicanResultColumns
	if !filter.Public {
		columns += `, result.provider_id, result.execution_model, result.error_message`
	}
	rows, err := repository.db.QueryContext(ctx, `SELECT row_to_json(item) FROM (SELECT `+columns+`
		FROM pelican_results result JOIN groups ON groups.id=result.group_id
		WHERE result.started_at > NOW()-INTERVAL '30 days'
		AND (NOT $1 OR (result.published AND result.status<>'running' AND groups.deleted_at IS NULL))
		AND ($2::bigint=0 OR result.group_id=$2) AND ($3::bigint=0 OR result.task_id=$3)
		AND ($4='' OR result.model=$4)
		AND ($5='' OR to_char(result.started_at AT TIME ZONE 'Asia/Shanghai', 'YYYY-MM-DD')=$5)
		ORDER BY result.started_at DESC, result.id DESC LIMIT 25 OFFSET $6) item`, filter.Public, filter.GroupID, filter.TaskID, filter.Model, filter.Date, (filter.Page-1)*24)
	if err != nil {
		return page, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var raw []byte
		var item site.PelicanResult
		if err := rows.Scan(&raw); err != nil {
			return page, err
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	if len(page.Items) > 24 {
		page.HasMore = true
		page.Items = page.Items[:24]
	}
	return page, rows.Err()
}

func (repository *pelicanRepository) Result(ctx context.Context, id int64, public bool) (site.PelicanResult, error) {
	columns := pelicanResultColumns + `, result.html`
	if !public {
		columns += `, result.provider_id, result.execution_model, result.error_message, result.raw_response`
	}
	var raw []byte
	var item site.PelicanResult
	err := repository.db.QueryRowContext(ctx, `SELECT row_to_json(item) FROM (SELECT `+columns+`
		FROM pelican_results result JOIN groups ON groups.id=result.group_id WHERE result.id=$1
		AND result.started_at>NOW()-INTERVAL '30 days'
		AND (NOT $2 OR (result.published AND result.status<>'running' AND groups.deleted_at IS NULL))) item`, id, public).Scan(&raw)
	if err != nil {
		return item, pelicanStorageError(err)
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return item, err
	}
	return item, nil
}

func (repository *pelicanRepository) Publish(ctx context.Context, id int64, published bool) error {
	result, err := repository.db.ExecContext(ctx, `UPDATE pelican_results SET published=$2 WHERE id=$1 AND status<>'running' AND started_at>NOW()-INTERVAL '30 days'`, id, published)
	return pelicanAffected(result, err)
}

func (repository *pelicanRepository) DeleteResult(ctx context.Context, id int64) error {
	result, err := repository.db.ExecContext(ctx, `DELETE FROM pelican_results WHERE id=$1 AND status<>'running'`, id)
	return pelicanAffected(result, err)
}

// Claim reserves one eligible task and creates its running result atomically.
// It returns nil when two leases are active or no authorized task is due.
// The advisory lock serializes the capacity check across application instances,
// while the selected task row stays locked until its lease and result commit.
//
// The lease includes the task's generation timeout and a saving allowance, so
// maintenance cannot reclaim it merely because generation exceeds the default
// budget. The returned task carries the same timeout used for that lease.
func (repository *pelicanRepository) Claim(ctx context.Context) (*site.PelicanClaim, error) {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	// Any failure rolls back both the lease and running result; a successful
	// commit leaves this deferred rollback with no transaction left to change.
	defer func() { _ = transaction.Rollback() }()
	// Serialize the active-count check through commit so separate instances
	// cannot each admit tasks against the same two remaining execution slots.
	if _, err := transaction.ExecContext(ctx, `SELECT pg_advisory_xact_lock(710298)`); err != nil {
		return nil, err
	}
	var active int
	if err := transaction.QueryRowContext(ctx, `SELECT COUNT(*) FROM pelican_tasks WHERE lease_until>NOW()`).Scan(&active); err != nil {
		return nil, err
	}
	// All instances share the same two execution slots.
	if active >= 2 {
		return nil, nil
	}
	claim := &site.PelicanClaim{Token: uuid.NewString()}
	err = transaction.QueryRowContext(ctx, `SELECT task.id, task.group_id, groups.name, task.model, task.interval_minutes, task.public_consent, task.timeout_seconds
		FROM pelican_tasks task JOIN groups ON groups.id=task.group_id
		WHERE task.lease_until IS NULL AND task.public_consent AND groups.deleted_at IS NULL AND groups.status='active'
		AND (task.manual_requested OR (task.enabled AND task.next_run_at<=NOW()))
		ORDER BY task.manual_requested DESC, task.next_run_at, task.id FOR UPDATE OF task SKIP LOCKED LIMIT 1`).Scan(
		&claim.Task.ID, &claim.Task.GroupID, &claim.Task.GroupName, &claim.Task.Model, &claim.Task.IntervalMinutes, &claim.Task.PublicConsent, &claim.Task.TimeoutSeconds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// The grace period covers result saving after the generation context ends;
	// deriving the lease from the locked row keeps both limits consistent.
	_, err = transaction.ExecContext(ctx, `UPDATE pelican_tasks SET lease_token=$2, lease_until=NOW()+make_interval(secs=>timeout_seconds+$3), manual_requested=FALSE,
		next_run_at=NOW()+make_interval(mins=>interval_minutes) WHERE id=$1`, claim.Task.ID, claim.Token, site.PelicanLeaseGraceSeconds)
	if err != nil {
		return nil, err
	}
	err = transaction.QueryRowContext(ctx, `INSERT INTO pelican_results(task_id, group_id, group_name, model, status, lease_token)
		VALUES($1,$2,$3,$4,'running',$5) RETURNING id`, claim.Task.ID, claim.Task.GroupID, claim.Task.GroupName, claim.Task.Model, claim.Token).Scan(&claim.ResultID)
	if err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}
	return claim, nil
}

func (repository *pelicanRepository) Finish(ctx context.Context, claim site.PelicanClaim, result site.PelicanResult) error {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	var consent bool
	err = transaction.QueryRowContext(ctx, `UPDATE pelican_tasks SET lease_until=NULL, lease_token='',
		next_run_at=GREATEST(next_run_at,NOW()) WHERE id=$1 AND lease_token=$2 AND lease_until>NOW() RETURNING public_consent`, claim.Task.ID, claim.Token).Scan(&consent)
	if err != nil {
		return pelicanStorageError(err)
	}
	_, err = transaction.ExecContext(ctx, `UPDATE pelican_results SET status=$3, published=$4, raw_response=$5, html=$6,
		error_message=$7, latency_ms=$8, finished_at=$9, provider_id=$10, execution_model=$11
		WHERE id=$1 AND lease_token=$2 AND status='running'`, claim.ResultID, claim.Token, result.Status, consent, result.RawResponse,
		result.HTML, result.ErrorMessage, result.LatencyMs, result.FinishedAt, result.ProviderID, result.ExecutionModel)
	if err != nil {
		return err
	}
	return transaction.Commit()
}

func (repository *pelicanRepository) Maintain(ctx context.Context) error {
	transaction, err := repository.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	_, err = transaction.ExecContext(ctx, `WITH expired AS (
		UPDATE pelican_tasks SET lease_until=NULL, lease_token='', next_run_at=NOW()+make_interval(mins=>interval_minutes)
		WHERE lease_until<=NOW() RETURNING id, public_consent
	) UPDATE pelican_results result SET status='interrupted', published=expired.public_consent, finished_at=NOW(),
		error_message='Execution lease expired' FROM expired WHERE result.task_id=expired.id AND result.status='running'`)
	if err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM pelican_results WHERE started_at<NOW()-INTERVAL '30 days' AND status<>'running'`); err != nil {
		return err
	}
	return transaction.Commit()
}

func pelicanStorageError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return site.ErrPelicanNotFound
	}
	var postgresError *pq.Error
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return site.ErrPelicanBusy
	}
	return err
}

func pelicanAffected(result sql.Result, err error) error {
	if err != nil {
		return pelicanStorageError(err)
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return site.ErrPelicanNotFound
	}
	return err
}
