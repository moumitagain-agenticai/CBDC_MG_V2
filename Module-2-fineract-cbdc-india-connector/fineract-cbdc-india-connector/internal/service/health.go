package service

import "context"

// HealthReport is the aggregated health of the connector and its dependencies.
type HealthReport struct {
	Status string            `json:"status"` // "ok" | "degraded"
	Checks map[string]string `json:"checks"`
}

// Liveness reports whether the process itself is up. It performs no I/O.
func (c *Connector) Liveness() HealthReport {
	return HealthReport{Status: "ok", Checks: map[string]string{"process": "ok"}}
}

// Readiness reports whether the connector can serve traffic: the sponsor-bank
// API is reachable and (if enabled) the database responds.
func (c *Connector) Readiness(ctx context.Context) HealthReport {
	checks := map[string]string{}
	status := "ok"

	if err := c.client.HealthCheck(ctx); err != nil {
		checks["cbdc_api"] = "error: " + err.Error()
		status = "degraded"
	} else {
		checks["cbdc_api"] = "ok"
	}

	if c.repo != nil {
		if err := c.repo.Ping(ctx); err != nil {
			checks["database"] = "error: " + err.Error()
			status = "degraded"
		} else {
			checks["database"] = "ok"
		}
	}

	return HealthReport{Status: status, Checks: checks}
}
