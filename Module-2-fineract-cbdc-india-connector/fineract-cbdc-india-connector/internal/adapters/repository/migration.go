package repository

import (
	"context"
	"database/sql"
	"fmt"

	"go.uber.org/zap"
)

// Migration is a single versioned schema change with forward (Up) and reverse
// (Down) SQL.
type Migration struct {
	Version int
	Name    string
	Up      string
	Down    string
}

// GetMigrations returns all migrations in ascending version order. To evolve the
// schema, append a new Migration with the next version number.
func GetMigrations() []Migration {
	return []Migration{
		{
			Version: 1,
			Name:    "create_cbdc_transactions",
			Up: `
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
CREATE UNIQUE INDEX IF NOT EXISTS idx_cbdc_transactions_reference_id ON cbdc_transactions (reference_id);
CREATE INDEX IF NOT EXISTS idx_cbdc_transactions_status ON cbdc_transactions (status);
CREATE INDEX IF NOT EXISTS idx_cbdc_transactions_created_at ON cbdc_transactions (created_at);
`,
			Down: `DROP TABLE IF EXISTS cbdc_transactions;`,
		},
	}
}

// Migrate creates the tracking table if needed and applies every pending
// migration in a transaction. It is safe to call on every startup.
func Migrate(ctx context.Context, db *sql.DB, log *zap.Logger) error {
	if _, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	current, err := currentVersion(ctx, db)
	if err != nil {
		return err
	}

	applied := 0
	for _, m := range GetMigrations() {
		if m.Version <= current {
			continue
		}
		if err := applyOne(ctx, db, m, true); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", m.Version, m.Name, err)
		}
		applied++
		if log != nil {
			log.Info("applied migration", zap.Int("version", m.Version), zap.String("name", m.Name))
		}
	}
	if log != nil && applied == 0 {
		log.Info("database schema up to date", zap.Int("version", current))
	}
	return nil
}

// Rollback reverts the last n applied migrations (n<=0 reverts one).
func Rollback(ctx context.Context, db *sql.DB, log *zap.Logger, n int) error {
	if n <= 0 {
		n = 1
	}
	byVersion := map[int]Migration{}
	for _, m := range GetMigrations() {
		byVersion[m.Version] = m
	}

	for i := 0; i < n; i++ {
		current, err := currentVersion(ctx, db)
		if err != nil {
			return err
		}
		if current == 0 {
			break
		}
		m, ok := byVersion[current]
		if !ok {
			return fmt.Errorf("no migration definition for version %d", current)
		}
		if err := applyOne(ctx, db, m, false); err != nil {
			return fmt.Errorf("rollback migration %d (%s): %w", m.Version, m.Name, err)
		}
		if log != nil {
			log.Info("rolled back migration", zap.Int("version", m.Version), zap.String("name", m.Name))
		}
	}
	return nil
}

func currentVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v sql.NullInt64
	if err := db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("read current migration version: %w", err)
	}
	if !v.Valid {
		return 0, nil
	}
	return int(v.Int64), nil
}

// applyOne runs a single migration's Up or Down inside a transaction and updates
// the tracking table atomically.
func applyOne(ctx context.Context, db *sql.DB, m Migration, up bool) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt := m.Up
	if !up {
		stmt = m.Down
	}
	if _, err := tx.ExecContext(ctx, stmt); err != nil {
		return err
	}

	if up {
		_, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, name) VALUES ($1, $2)`, m.Version, m.Name)
	} else {
		_, err = tx.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = $1`, m.Version)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
