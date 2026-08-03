package repository

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"context"
	"sync"

	"github.com/fineract/cacti-bridge/internal/domain"
	"github.com/fineract/cacti-bridge/internal/ports"
)

// MemoryRepository is a process-local SettlementRepository used when no database
// is configured. The saga is fully functional; state is not durable across
// restarts (so crash-recovery only has effect with the Postgres repository).
type MemoryRepository struct {
	mu    sync.RWMutex
	byID  map[string]*domain.Transfer
	byRef map[string]string // reference_id -> id
}

var _ ports.SettlementRepository = (*MemoryRepository)(nil)

// NewMemory builds an empty in-memory repository.
func NewMemory() *MemoryRepository {
	return &MemoryRepository{byID: make(map[string]*domain.Transfer), byRef: make(map[string]string)}
}

func (r *MemoryRepository) Save(_ context.Context, t *domain.Transfer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byRef[t.ReferenceID]; ok {
		return domain.NewConflictError("settlement with this reference_id already exists", nil)
	}
	cp := *t
	r.byID[t.ID] = &cp
	r.byRef[t.ReferenceID] = t.ID
	return nil
}

func (r *MemoryRepository) Update(_ context.Context, t *domain.Transfer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[t.ID]; !ok {
		return domain.NewNotFoundError("settlement not found", nil)
	}
	cp := *t
	r.byID[t.ID] = &cp
	return nil
}

func (r *MemoryRepository) Get(_ context.Context, id string) (*domain.Transfer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byID[id]
	if !ok {
		return nil, domain.NewNotFoundError("settlement not found", nil)
	}
	cp := *t
	return &cp, nil
}

func (r *MemoryRepository) GetByReference(_ context.Context, referenceID string) (*domain.Transfer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byRef[referenceID]
	if !ok {
		return nil, domain.NewNotFoundError("settlement not found", nil)
	}
	cp := *r.byID[id]
	return &cp, nil
}

func (r *MemoryRepository) ListInFlight(_ context.Context) ([]domain.Transfer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Transfer
	for _, t := range r.byID {
		if t.Status.InFlight() {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (r *MemoryRepository) Ping(_ context.Context) error { return nil }

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_memory.log at runtime.
var _ = func() bool { flog.For("10_memory").Info("source file initialized"); return true }()
