-- 001_initial.sql
-- Initial schema for Stratum.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    status      TEXT NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    left_hash   TEXT NOT NULL,
    right_hash  TEXT NOT NULL,
    language    TEXT,
    result      JSONB,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT unique_input_pair UNIQUE (left_hash, right_hash)
);

CREATE INDEX idx_jobs_status ON jobs (status);

CREATE TABLE cache_entries (
    hash        TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('parse', 'diff')),
    data        BYTEA NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (hash, kind)
);

CREATE INDEX idx_cache_entries_expires ON cache_entries (expires_at);
