package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"go.uber.org/zap"

	"github.com/fineract/cbdc/india-connector/pkg/metrics"
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
		AllowedHeaders:   []string{"Accept", "Content-Type", "X-API-Key", "X-Request-ID", "Authorization"},
		AllowCredentials: false,
		MaxAge:           300,
	}))
	if ratePerMin > 0 {
		r.Use(httprate.LimitByIP(ratePerMin, time.Minute))
	}

	// Observability & health.
	r.Handle("/metrics", m.Handler())
	r.Get("/healthz", h.Liveness)
	r.Get("/readyz", h.Readiness)

	// Versioned API.
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/issue", h.Issue)
		r.Post("/transfer", h.Transfer)
		r.Post("/lock", h.Lock)
		r.Post("/burn", h.Burn)
		r.Post("/redeem", h.Redeem)
		r.Get("/wallets/{walletID}/balance", h.Balance)
		r.Get("/transactions/{id}/status", h.UpstreamStatus)
		r.Get("/transactions/{id}", h.GetTransaction)
	})

	return r
}
