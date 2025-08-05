
CREATE TABLE IF NOT EXISTS ollama_models (
    id VARCHAR(255) PRIMARY KEY,
    model VARCHAR(512) NOT NULL UNIQUE,

    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS llm_pool (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(512) NOT NULL UNIQUE,
    purpose_type VARCHAR(512) NOT NULL,

    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS llm_backends (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(512) NOT NULL UNIQUE,
    base_url VARCHAR(512) NOT NULL UNIQUE,
    type VARCHAR(512) NOT NULL,

    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS llm_pool_backend_assignments (
    pool_id VARCHAR(255) NOT NULL REFERENCES llm_pool(id) ON DELETE CASCADE,
    backend_id VARCHAR(255) NOT NULL REFERENCES llm_backends(id) ON DELETE CASCADE,
    PRIMARY KEY (pool_id, backend_id),
    assigned_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS ollama_model_assignments (
    model_id VARCHAR(255) NOT NULL REFERENCES ollama_models(id) ON DELETE CASCADE,
    llm_pool_id VARCHAR(255) NOT NULL REFERENCES llm_pool(id) ON DELETE CASCADE,
    PRIMARY KEY (model_id, llm_pool_id),

    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS job_queue_v2 (
    id VARCHAR(255) PRIMARY KEY,
    task_type VARCHAR(512) NOT NULL,
    payload JSONB NOT NULL,

    scheduled_for INT,
    valid_until INT,
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS entity_events (
    id VARCHAR(255) PRIMARY KEY,
    entity_id VARCHAR(255) NOT NULL,
    entity_type VARCHAR(255) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,
    error TEXT
);

CREATE TABLE IF NOT EXISTS kv (
    key VARCHAR(255) PRIMARY KEY,
    value JSONB NOT NULL,

    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS remote_hooks (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL UNIQUE,
    endpoint_url VARCHAR(512) NOT NULL,
    method VARCHAR(10) NOT NULL DEFAULT 'POST',
    timeout_ms INT NOT NULL DEFAULT 5000,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);


CREATE INDEX IF NOT EXISTS idx_job_queue_v2_task_type ON job_queue_v2 USING hash(task_type);

CREATE OR REPLACE FUNCTION estimate_row_count(table_name TEXT)
RETURNS BIGINT AS $$
DECLARE
    result BIGINT;
BEGIN
    SELECT reltuples::BIGINT
    INTO result
    FROM pg_class
    WHERE relname = table_name;

    RETURN COALESCE(result, 0);
END;
$$ LANGUAGE plpgsql STABLE;
