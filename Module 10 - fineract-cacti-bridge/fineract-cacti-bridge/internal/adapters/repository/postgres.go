package repository

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/lib/pq" // postgres driver + pq.Error (init registers the driver)

	"github.com/fineract/cacti-bridge/internal/config"
	"github.com/fineract/cacti-bridge/internal/domain"
	"github.com/fineract/cacti-bridge/internal/ports"
)

// Repository is a PostgreSQL-backed SettlementRepository.
type Repository struct {
	db *sql.DB
}

var _ ports.SettlementRepository = (*Repository)(nil)

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

const settlementCols = `id, reference_id, status, amount, asset, source_ledger, dest_ledger,
    sender, recipient, lock_tx_id, release_tx_id, burn_tx_id, unlock_tx_id,
    burn_attempts, failure_reason, created_at, updated_at`

func (r *Repository) Save(ctx context.Context, t *domain.Transfer) error {
	const q = `INSERT INTO settlements (` + settlementCols + `)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`
	_, err := r.db.ExecContext(ctx, q,
		t.ID, t.ReferenceID, t.Status, t.Amount, t.Asset, t.SourceLedger, t.DestLedger,
		t.Sender, t.Recipient, t.LockTxID, t.ReleaseTxID, t.BurnTxID, t.UnlockTxID,
		t.BurnAttempts, t.FailureReason, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return domain.NewConflictError("settlement with this reference_id already exists", err)
		}
		return domain.NewInternalError("persist settlement", err)
	}
	return nil
}

func (r *Repository) Update(ctx context.Context, t *domain.Transfer) error {
	const q = `UPDATE settlements SET
    status=$2, lock_tx_id=$3, release_tx_id=$4, burn_tx_id=$5, unlock_tx_id=$6,
    burn_attempts=$7, failure_reason=$8, updated_at=$9
  WHERE id=$1`
	res, err := r.db.ExecContext(ctx, q,
		t.ID, t.Status, t.LockTxID, t.ReleaseTxID, t.BurnTxID, t.UnlockTxID,
		t.BurnAttempts, t.FailureReason, t.UpdatedAt,
	)
	if err != nil {
		return domain.NewInternalError("update settlement", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.NewNotFoundError("settlement not found: "+t.ID, nil)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (*domain.Transfer, error) {
	const q = `SELECT ` + settlementCols + ` FROM settlements WHERE id=$1`
	return scanOne(r.db.QueryRowContext(ctx, q, id))
}

func (r *Repository) GetByReference(ctx context.Context, referenceID string) (*domain.Transfer, error) {
	const q = `SELECT ` + settlementCols + ` FROM settlements WHERE reference_id=$1`
	return scanOne(r.db.QueryRowContext(ctx, q, referenceID))
}

func (r *Repository) ListInFlight(ctx context.Context) ([]domain.Transfer, error) {
	const q = `SELECT ` + settlementCols + ` FROM settlements
  WHERE status IN ('INITIATED','LOCKED','RELEASED','COMPENSATING') ORDER BY created_at`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, domain.NewInternalError("query in-flight settlements", err)
	}
	defer rows.Close()
	var out []domain.Transfer
	for rows.Next() {
		t, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (r *Repository) Ping(ctx context.Context) error {
	if err := r.db.PingContext(ctx); err != nil {
		return domain.NewInternalError("database ping failed", err)
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

func scanOne(row scanner) (*domain.Transfer, error) {
	t, err := scanRows(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.NewNotFoundError("settlement not found", err)
	}
	return t, err
}

func scanRows(row scanner) (*domain.Transfer, error) {
	var t domain.Transfer
	if err := row.Scan(
		&t.ID, &t.ReferenceID, &t.Status, &t.Amount, &t.Asset, &t.SourceLedger, &t.DestLedger,
		&t.Sender, &t.Recipient, &t.LockTxID, &t.ReleaseTxID, &t.BurnTxID, &t.UnlockTxID,
		&t.BurnAttempts, &t.FailureReason, &t.CreatedAt, &t.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, domain.NewInternalError("scan settlement", err)
	}
	if strings.TrimSpace(string(t.Status)) == "" {
		t.Status = domain.StatusInitiated
	}
	return &t, nil
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_postgres.log at runtime.
var _ = func() bool { flog.For("10_postgres").Info("source file initialized"); return true }()
