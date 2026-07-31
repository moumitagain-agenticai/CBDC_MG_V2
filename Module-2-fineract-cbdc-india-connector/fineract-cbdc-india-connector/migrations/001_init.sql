-- fineract-cbdc-india-connector: transaction audit trail.
-- Apply with: psql "$DATABASE_DSN" -f migrations/001_init.sql

CREATE TABLE IF NOT EXISTS cbdc_transactions (
    id                 UUID PRIMARY KEY,
    reference_id       TEXT        NOT NULL,
    operation          TEXT        NOT NULL,
    status             TEXT        NOT NULL,
    source_wallet      TEXT        NOT NULL DEFAULT '',
    destination_wallet TEXT        NOT NULL DEFAULT '',
    amount             TEXT        NOT NULL,
    currency           TEXT        NOT NULL,
    upstream_tx_id     TEXT        NOT NULL DEFAULT '',
    failure_reason     TEXT        NOT NULL DEFAULT '',
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL
);

-- Idempotency: a reference id maps to a single transaction.
CREATE UNIQUE INDEX IF NOT EXISTS idx_cbdc_transactions_reference_id
    ON cbdc_transactions (reference_id);

CREATE INDEX IF NOT EXISTS idx_cbdc_transactions_status
    ON cbdc_transactions (status);

CREATE INDEX IF NOT EXISTS idx_cbdc_transactions_created_at
    ON cbdc_transactions (created_at);
