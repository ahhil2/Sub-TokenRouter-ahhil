-- 311_pelican_timeout.sql stores each task's administrator-selected generation
-- limit in seconds. Existing tasks receive a five-minute budget, while the
-- constraint keeps future writes within the range accepted by the task service.
ALTER TABLE pelican_tasks
    ADD COLUMN IF NOT EXISTS timeout_seconds INTEGER NOT NULL DEFAULT 300
    CHECK (timeout_seconds BETWEEN 1 AND 3600);
