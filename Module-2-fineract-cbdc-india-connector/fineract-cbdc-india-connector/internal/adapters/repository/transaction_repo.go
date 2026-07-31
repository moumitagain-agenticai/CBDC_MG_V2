package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "github.com/lib/pq" // postgres driver (side-effect import)

	"github.com/fineract/cbdc/india-connector/internal/config"
	"github.com/fineract/cbdc/india-connector/internal/domain"
	"github.com/fineract/cbdc/india-connector/internal/ports"
)

// Repository is a PostgreSQL-backed implementation of the transaction port.
type Repository struct {
	db *sql.DB
}

var _ ports.TransactionRepository = (*Repository)(nil)

// OpenDB opens and configures a *sql.DB from database configuration.
func OpenDB(cfg config.DatabaseConfig) (*sql.DB, error) {
	db, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	return db, nil
}

// New builds a Repository over an existing *sql.DB.
func New(db *sql.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Save(ctx context.Context, tx *domain.Transaction) error {
	const q = `
INSERT INTO cbdc_transactions
  (id, reference_id, operation, status, source_wallet, destination_wallet,
   amount, currency, upstream_tx_id, failure_reason, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`
	_, err := r.db.ExecContext(ctx, q,
		tx.ID, tx.ReferenceID, tx.Operation, tx.Status, tx.SourceWallet,
		tx.DestWallet, tx.Amount, tx.Currency, tx.UpstreamTxID,
		tx.FailureReason, tx.CreatedAt, tx.UpdatedAt,
	)
	if err != nil {
		return domain.NewInternalError("persist transaction", err)
	}
	return nil
}

func (r *Repository) UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, upstreamTxID, failureReason string) error {
	const q = `
UPDATE cbdc_transactions
   SET status = $2, upstream_tx_id = $3, failure_reason = $4, updated_at = $5
 WHERE id = $1`
	res, err := r.db.ExecContext(ctx, q, id, status, upstreamTxID, failureReason, time.Now().UTC())
	if err != nil {
		return domain.NewInternalError("update transaction status", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NewNotFoundError("transaction not found: "+id, nil)
	}
	return nil
}

func (r *Repository) GetByReferenceID(ctx context.Context, referenceID string) (*domain.Transaction, error) {
	const q = baseSelect + ` WHERE reference_id = $1`
	return r.queryOne(ctx, q, referenceID)
}

func (r *Repository) GetByID(ctx context.Context, id string) (*domain.Transaction, error) {
	const q = baseSelect + ` WHERE id = $1`
	return r.queryOne(ctx, q, id)
}

func (r *Repository) Ping(ctx context.Context) error {
	if err := r.db.PingContext(ctx); err != nil {
		return domain.NewInternalError("database ping failed", err)
	}
	return nil
}

const baseSelect = `
SELECT id, reference_id, operation, status, source_wallet, destination_wallet,
       amount, currency, upstream_tx_id, failure_reason, created_at, updated_at
  FROM cbdc_transactions`

func (r *Repository) queryOne(ctx context.Context, q string, arg string) (*domain.Transaction, error) {
	row := r.db.QueryRowContext(ctx, q, arg)
	var t domain.Transaction
	err := row.Scan(
		&t.ID, &t.ReferenceID, &t.Operation, &t.Status, &t.SourceWallet,
		&t.DestWallet, &t.Amount, &t.Currency, &t.UpstreamTxID,
		&t.FailureReason, &t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.NewNotFoundError("transaction not found", err)
	}
	if err != nil {
		return nil, domain.NewInternalError("scan transaction", err)
	}
	return &t, nil
}
