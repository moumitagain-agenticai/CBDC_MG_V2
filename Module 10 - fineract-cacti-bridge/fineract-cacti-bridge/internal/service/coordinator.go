package service

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/fineract/cacti-bridge/internal/config"
	"github.com/fineract/cacti-bridge/internal/domain"
	"github.com/fineract/cacti-bridge/internal/ports"
	"github.com/fineract/cacti-bridge/pkg/metrics"
)

// Coordinator runs the cross-chain lock-release-burn saga across a source and a
// destination Cacti ledger connector. State is persisted after every step so the
// saga is crash-recoverable, and ledger operations are idempotent on transfer id
// so retries and recovery never double-apply.
//
// Flow:  INITIATED --lock--> LOCKED --release--> RELEASED --burn--> BURNED
// If release fails while LOCKED, the source lock is unwound:
//
//	LOCKED --(release fails)--> COMPENSATING --unlock--> COMPENSATED
//
// Once RELEASED (destination credited) the saga only rolls forward — burn is
// retried; if it cannot complete it stays RELEASED for recovery, never rolled back.
type Coordinator struct {
	source  ports.LedgerConnector
	dest    ports.LedgerConnector
	repo    ports.SettlementRepository
	cfg     config.SettlementConfig
	metrics *metrics.Metrics
	log     *zap.Logger
}

// NewCoordinator wires the saga. repo is always non-nil (in-memory or Postgres).
func NewCoordinator(source, dest ports.LedgerConnector, repo ports.SettlementRepository, cfg config.SettlementConfig, m *metrics.Metrics, log *zap.Logger) *Coordinator {
	return &Coordinator{source: source, dest: dest, repo: repo, cfg: cfg, metrics: m, log: log}
}

// Settle initiates (or, if the reference already exists, returns) a cross-chain
// settlement and drives it to a terminal state. The returned transfer carries
// the outcome (BURNED / COMPENSATED / FAILED, or RELEASED if burn is pending);
// a non-nil error indicates a validation or internal/persistence failure, not a
// business outcome.
func (c *Coordinator) Settle(ctx context.Context, req domain.SettleRequest) (*domain.Transfer, error) {
	defer c.observe("settle")()
	if err := req.Validate(); err != nil {
		return nil, c.fail("settle", err)
	}

	// Idempotent initiation: same reference returns the existing settlement.
	if existing, err := c.repo.GetByReference(ctx, req.ReferenceID); err == nil {
		return existing, nil
	}

	t := domain.NewTransfer(uuid.NewString(), req, time.Now().UTC())
	if err := c.repo.Save(ctx, t); err != nil {
		if domain.AsDomainError(err).Code == domain.CodeConflict {
			if existing, gerr := c.repo.GetByReference(ctx, req.ReferenceID); gerr == nil {
				return existing, nil
			}
		}
		return nil, c.fail("settle", err)
	}
	c.log.Info("settlement initiated", zap.String("id", t.ID), zap.String("reference_id", t.ReferenceID))
	return c.runForward(ctx, t)
}

// Rollback explicitly compensates an in-flight settlement. It is only possible
// before the value is released on the destination.
func (c *Coordinator) Rollback(ctx context.Context, id string) (*domain.Transfer, error) {
	defer c.observe("rollback")()
	t, err := c.repo.Get(ctx, id)
	if err != nil {
		return nil, c.fail("rollback", err)
	}
	switch t.Status {
	case domain.StatusInitiated:
		t.FailureReason = "cancelled before lock"
		_ = t.Transition(domain.StatusFailed)
		if err := c.persist(ctx, t); err != nil {
			return nil, c.fail("rollback", err)
		}
		return t, nil
	case domain.StatusLocked:
		t.FailureReason = "rollback requested"
		if err := t.Transition(domain.StatusCompensating); err != nil {
			return nil, c.fail("rollback", err)
		}
		if err := c.persist(ctx, t); err != nil {
			return nil, c.fail("rollback", err)
		}
		if err := c.stepUnlock(ctx, t); err != nil {
			return nil, c.fail("rollback", err)
		}
		return t, nil
	case domain.StatusCompensating:
		if err := c.stepUnlock(ctx, t); err != nil {
			return nil, c.fail("rollback", err)
		}
		return t, nil
	case domain.StatusReleased, domain.StatusBurned:
		return nil, c.fail("rollback", domain.NewConflictError("cannot roll back: value already released on the destination ledger", nil))
	default: // COMPENSATED, FAILED
		return t, nil
	}
}

// Get returns a settlement by id.
func (c *Coordinator) Get(ctx context.Context, id string) (*domain.Transfer, error) {
	t, err := c.repo.Get(ctx, id)
	if err != nil {
		return nil, domain.AsDomainError(err)
	}
	return t, nil
}

// runForward advances a transfer from its current state to a terminal (or
// burn-pending) state. Each stage is guarded by the current status so the same
// routine resumes a recovered transfer from wherever it left off.
func (c *Coordinator) runForward(ctx context.Context, t *domain.Transfer) (*domain.Transfer, error) {
	if t.Status == domain.StatusInitiated {
		if err := c.stepLock(ctx, t); err != nil {
			return t, c.fail("lock", err)
		}
		if t.Status == domain.StatusFailed {
			return t, nil
		}
	}
	if t.Status == domain.StatusLocked {
		if err := c.stepRelease(ctx, t); err != nil {
			return t, c.fail("release", err)
		}
	}
	if t.Status == domain.StatusCompensating {
		if err := c.stepUnlock(ctx, t); err != nil {
			return t, c.fail("unlock", err)
		}
		return t, nil
	}
	if t.Status == domain.StatusReleased {
		if err := c.stepBurn(ctx, t); err != nil {
			return t, c.fail("burn", err)
		}
	}
	return t, nil
}

func (c *Coordinator) stepLock(ctx context.Context, t *domain.Transfer) error {
	rcpt, err := c.source.Lock(ctx, c.op(t, t.Sender))
	if err != nil {
		t.FailureReason = "lock failed: " + err.Error()
		_ = t.Transition(domain.StatusFailed)
		c.count("lock", "failed")
		return c.persist(ctx, t)
	}
	t.LockTxID = rcpt.TxID
	if err := t.Transition(domain.StatusLocked); err != nil {
		return err
	}
	c.count("lock", "ok")
	c.log.Info("locked on source", zap.String("id", t.ID), zap.String("tx", rcpt.TxID))
	return c.persist(ctx, t)
}

func (c *Coordinator) stepRelease(ctx context.Context, t *domain.Transfer) error {
	rcpt, err := c.dest.Release(ctx, c.op(t, t.Recipient))
	if err != nil {
		// Release failed while only the source is locked -> compensate.
		t.FailureReason = "release failed: " + err.Error()
		if terr := t.Transition(domain.StatusCompensating); terr != nil {
			return terr
		}
		c.count("release", "failed")
		c.log.Warn("release failed, compensating", zap.String("id", t.ID), zap.Error(err))
		return c.persist(ctx, t)
	}
	t.ReleaseTxID = rcpt.TxID
	t.FailureReason = ""
	if err := t.Transition(domain.StatusReleased); err != nil {
		return err
	}
	c.count("release", "ok")
	c.log.Info("released on destination", zap.String("id", t.ID), zap.String("tx", rcpt.TxID))
	return c.persist(ctx, t)
}

func (c *Coordinator) stepUnlock(ctx context.Context, t *domain.Transfer) error {
	rcpt, err := c.source.Unlock(ctx, c.op(t, t.Sender))
	if err != nil {
		t.FailureReason = "compensation (unlock) failed: " + err.Error()
		_ = t.Transition(domain.StatusFailed)
		c.count("unlock", "failed")
		return c.persist(ctx, t)
	}
	t.UnlockTxID = rcpt.TxID
	if err := t.Transition(domain.StatusCompensated); err != nil {
		return err
	}
	c.count("unlock", "ok")
	c.log.Info("compensated (source unlocked)", zap.String("id", t.ID), zap.String("tx", rcpt.TxID))
	return c.persist(ctx, t)
}

func (c *Coordinator) stepBurn(ctx context.Context, t *domain.Transfer) error {
	// The loop is bounded per invocation; BurnAttempts is a cumulative total, so
	// a later recovery run gets a fresh budget of attempts.
	for attempt := 0; attempt < c.cfg.BurnMaxAttempts; attempt++ {
		t.BurnAttempts++
		rcpt, err := c.source.Burn(ctx, c.op(t, t.Sender))
		if err == nil {
			t.BurnTxID = rcpt.TxID
			t.FailureReason = ""
			if terr := t.Transition(domain.StatusBurned); terr != nil {
				return terr
			}
			c.count("burn", "ok")
			c.log.Info("burned on source; settlement complete", zap.String("id", t.ID), zap.String("tx", rcpt.TxID))
			return c.persist(ctx, t)
		}
		t.FailureReason = "burn attempt failed: " + err.Error()
		c.count("burn", "retry")
		if perr := c.persist(ctx, t); perr != nil {
			return perr
		}
	}
	// Exhausted: destination is already credited, so we do NOT roll back. The
	// transfer stays RELEASED and is completed by recovery.
	t.FailureReason = "burn pending after max attempts (destination already credited)"
	c.count("burn", "pending")
	c.log.Warn("burn pending; will be retried by recovery", zap.String("id", t.ID))
	return c.persist(ctx, t)
}

func (c *Coordinator) op(t *domain.Transfer, account string) ports.LedgerOp {
	return ports.LedgerOp{
		TransferID:  t.ID,
		ReferenceID: t.ReferenceID,
		Amount:      t.Amount,
		Asset:       t.Asset,
		Account:     account,
	}
}

func (c *Coordinator) persist(ctx context.Context, t *domain.Transfer) error {
	t.UpdatedAt = time.Now().UTC()
	if err := c.repo.Update(ctx, t); err != nil {
		c.log.Error("failed to persist settlement state", zap.String("id", t.ID), zap.Error(err))
		return err
	}
	return nil
}

func (c *Coordinator) observe(op string) func() {
	start := time.Now()
	return func() { c.metrics.OpDuration.WithLabelValues(op).Observe(time.Since(start).Seconds()) }
}

func (c *Coordinator) count(step, result string) {
	c.metrics.OpsTotal.WithLabelValues(step, result).Inc()
}

func (c *Coordinator) fail(op string, err error) error {
	de := domain.AsDomainError(err)
	c.metrics.OpErrorsTotal.WithLabelValues(op, string(de.Code)).Inc()
	return de
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_coordinator.log at runtime.
var _ = func() bool { flog.For("10_coordinator").Info("source file initialized"); return true }()
