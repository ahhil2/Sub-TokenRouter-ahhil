CREATE TABLE pelican_tasks (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups(id),
    model VARCHAR(255) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    interval_minutes INTEGER NOT NULL DEFAULT 360 CHECK (interval_minutes BETWEEN 5 AND 10080),
    public_consent BOOLEAN NOT NULL DEFAULT FALSE,
    manual_requested BOOLEAN NOT NULL DEFAULT FALSE,
    next_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, model),
    CHECK (NOT enabled OR public_consent)
);
CREATE INDEX pelican_tasks_due ON pelican_tasks(next_run_at) WHERE enabled OR manual_requested;

CREATE TABLE pelican_results (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES pelican_tasks(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL,
    group_name TEXT NOT NULL,
    model VARCHAR(255) NOT NULL,
    provider_id BIGINT,
    execution_model TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed', 'interrupted')),
    published BOOLEAN NOT NULL DEFAULT FALSE,
    raw_response TEXT NOT NULL DEFAULT '',
    html TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    latency_ms BIGINT NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    lease_token TEXT NOT NULL,
    CHECK (octet_length(raw_response) <= 1048576)
);
CREATE INDEX pelican_results_recent ON pelican_results(started_at DESC, id DESC);
CREATE INDEX pelican_results_task ON pelican_results(task_id, started_at DESC);
CREATE INDEX pelican_results_public ON pelican_results(started_at DESC, id DESC) WHERE published;
