package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the connector's Prometheus collectors.
type Metrics struct {
	OpsTotal      *prometheus.CounterVec
	OpErrorsTotal *prometheus.CounterVec
	OpDuration    *prometheus.HistogramVec
	registry      *prometheus.Registry
}

// New creates and registers the connector metrics on a private registry so the
// service does not pollute (or collide with) the global default registry.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)

	return &Metrics{
		registry: reg,
		OpsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "cbdc_operations_total",
			Help: "Total number of CBDC operations processed, by operation and status.",
		}, []string{"operation", "status"}),
		OpErrorsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "cbdc_operation_errors_total",
			Help: "Total number of failed CBDC operations, by operation and error code.",
		}, []string{"operation", "code"}),
		OpDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "cbdc_operation_duration_seconds",
			Help:    "Latency of CBDC operations in seconds, by operation.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation"}),
	}
}

// Handler returns the /metrics HTTP handler for this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
