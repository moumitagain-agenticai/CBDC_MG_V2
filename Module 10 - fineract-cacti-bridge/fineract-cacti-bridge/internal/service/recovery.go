package service

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"context"

	"go.uber.org/zap"
)

// Recover resumes every in-flight settlement to a terminal state. It is called
// on startup so a crash mid-saga is healed: LOCKED transfers roll forward
// (release, then burn) or compensate on release failure; RELEASED transfers
// complete their burn; COMPENSATING transfers finish unlocking. Because ledger
// operations are idempotent on transfer id, re-driving a step is safe.
func (c *Coordinator) Recover(ctx context.Context) (int, error) {
	inflight, err := c.repo.ListInFlight(ctx)
	if err != nil {
		return 0, c.fail("recover", err)
	}
	if len(inflight) == 0 {
		return 0, nil
	}
	c.log.Info("recovering in-flight settlements", zap.Int("count", len(inflight)))
	recovered := 0
	for i := range inflight {
		t := inflight[i]
		if _, err := c.runForward(ctx, &t); err != nil {
			c.log.Error("recovery of settlement did not complete", zap.String("id", t.ID), zap.Error(err))
			continue
		}
		c.log.Info("recovered settlement", zap.String("id", t.ID), zap.String("status", string(t.Status)))
		recovered++
	}
	return recovered, nil
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_recovery.log at runtime.
var _ = func() bool { flog.For("10_recovery").Info("source file initialized"); return true }()
