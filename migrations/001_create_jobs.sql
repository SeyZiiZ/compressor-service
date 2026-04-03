CREATE TABLE IF NOT EXISTS jobs (
    id              VARCHAR(36) PRIMARY KEY,
    status          VARCHAR(20) NOT NULL DEFAULT 'pending',
    type            VARCHAR(10) NOT NULL,
    original_filename VARCHAR(255) NOT NULL DEFAULT '',
    input_path      TEXT NOT NULL DEFAULT '',
    output_path     TEXT NOT NULL DEFAULT '',
    progress        INTEGER NOT NULL DEFAULT 0,
    input_size      BIGINT NOT NULL DEFAULT 0,
    output_size     BIGINT NOT NULL DEFAULT 0,
    compression_ratio DOUBLE PRECISION NOT NULL DEFAULT 0,
    options_json    JSONB NOT NULL DEFAULT '{}',
    error           TEXT,
    webhook_url     TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at DESC);
