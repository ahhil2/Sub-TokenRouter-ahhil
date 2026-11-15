-- 308_registration_ip_intervals.sql stores the latest enabled email signup time
-- for each IP. PostgreSQL serializes claims alongside account creation, keeping
-- intervals durable across instances and restarts while failed transactions
-- restore the previous history.
-- The table remains independent of account deletion. Existing accounts have no
-- historical registration IP to backfill, so upgrades begin with empty history.
CREATE TABLE IF NOT EXISTS registration_ip_intervals (
    ip_address INET PRIMARY KEY,
    last_registered_at TIMESTAMPTZ NOT NULL
);
