package service

import "github.com/fineract/cacti-bridge/pkg/flog"
import "context"

// HealthReport is the aggregated health of the bridge.
type HealthReport struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Liveness reports process health with no I/O.
func (c *Coordinator) Liveness() HealthReport {
	return HealthReport{Status: "ok", Checks: map[string]string{"process": "ok"}}
}

// Readiness pings both ledger connectors and the repository.
func (c *Coordinator) Readiness(ctx context.Context) HealthReport {
	checks := map[string]string{}
	status := "ok"
	if err := c.source.Health(ctx); err != nil {
		checks["source_ledger"] = "error: " + err.Error()
		status = "degraded"
	} else {
		checks["source_ledger"] = "ok"
	}
	if err := c.dest.Health(ctx); err != nil {
		checks["dest_ledger"] = "error: " + err.Error()
		status = "degraded"
	} else {
		checks["dest_ledger"] = "ok"
	}
	if err := c.repo.Ping(ctx); err != nil {
		checks["repository"] = "error: " + err.Error()
		status = "degraded"
	} else {
		checks["repository"] = "ok"
	}
	return HealthReport{Status: status, Checks: checks}
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_health.log at runtime.
var _ = func() bool { flog.For("10_health").Info("source file initialized"); return true }()
