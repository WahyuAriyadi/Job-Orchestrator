-- Job Orchestrator schema
-- Requires pgcrypto for gen_random_uuid() (available by default on most managed Postgres,
-- e.g. Cloud SQL >= PG13). If not available, run: CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name             TEXT NOT NULL,
    cron_expression  TEXT NOT NULL,
    http_method      TEXT NOT NULL DEFAULT 'POST',
    callback_url     TEXT NOT NULL,
    payload          JSONB NOT NULL DEFAULT '{}'::jsonb,
    max_retries      INT NOT NULL DEFAULT 3,
    timeout_seconds  INT NOT NULL DEFAULT 30,
    enabled          BOOLEAN NOT NULL DEFAULT true,
    next_run_at      TIMESTAMPTZ,
    last_run_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Speeds up the scheduler's "what's due" poll: only scans enabled jobs, ordered by due time.
CREATE INDEX IF NOT EXISTS idx_jobs_due ON jobs (next_run_at) WHERE enabled = true;

CREATE TABLE IF NOT EXISTS executions (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id         UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    status         TEXT NOT NULL, -- pending | running | success | failed | retrying
    attempt        INT NOT NULL DEFAULT 1,
    http_status    INT,
    response_body  TEXT,
    error_message  TEXT,
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ,
    duration_ms    INT
);

CREATE INDEX IF NOT EXISTS idx_executions_job_id ON executions (job_id, started_at DESC);
