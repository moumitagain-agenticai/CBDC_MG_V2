package api

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"go.uber.org/zap"

	"github.com/fineract/cacti-bridge/pkg/metrics"
)

// NewRouter assembles the HTTP router with middleware, health, metrics and the
// versioned API surface.
func NewRouter(h *Handler, m *metrics.Metrics, log *zap.Logger, ratePerMin int) http.Handler {
	r := chi.NewRouter()

	r.Use(RequestID)
	r.Use(Recoverer(log))
	r.Use(AccessLog(log))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	if ratePerMin > 0 {
		r.Use(httprate.LimitByIP(ratePerMin, time.Minute))
	}

	r.Handle("/metrics", m.Handler())
	r.Get("/healthz", h.Liveness)
	r.Get("/readyz", h.Readiness)

	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/settlements", h.Settle)
		r.Get("/settlements/{id}", h.Get)
		r.Post("/settlements/{id}/rollback", h.Rollback)
	})

	return r
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_routes.log at runtime.
var _ = func() bool { flog.For("10_routes").Info("source file initialized"); return true }()
