-- fineract-cacti-bridge: durable state for the lock-release-burn settlement saga.
-- Apply with: psql "$DATABASE_DSN" -f migrations/001_init.sql
-- (The service also runs this automatically on startup when DATABASE_DSN is set.)

CREATE TABLE IF NOT EXISTS settlements (
    id             TEXT PRIMARY KEY,
    reference_id   TEXT        NOT NULL,
    status         TEXT        NOT NULL,
    amount         TEXT        NOT NULL,
    asset          TEXT        NOT NULL,
    source_ledger  TEXT        NOT NULL,
    dest_ledger    TEXT        NOT NULL,
    sender         TEXT        NOT NULL DEFAULT '',
    recipient      TEXT        NOT NULL DEFAULT '',
    lock_tx_id     TEXT        NOT NULL DEFAULT '',
    release_tx_id  TEXT        NOT NULL DEFAULT '',
    burn_tx_id     TEXT        NOT NULL DEFAULT '',
    unlock_tx_id   TEXT        NOT NULL DEFAULT '',
    burn_attempts  INTEGER     NOT NULL DEFAULT 0,
    failure_reason TEXT        NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_settlements_reference_id ON settlements (reference_id);
CREATE INDEX IF NOT EXISTS idx_settlements_status ON settlements (status);
