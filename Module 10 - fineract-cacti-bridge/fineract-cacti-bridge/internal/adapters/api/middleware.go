package api

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/fineract/cacti-bridge/internal/domain"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// RequestID ensures every request carries a correlation id, echoing it back in
// the response and making it available on the context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext returns the correlation id, or "" if unset.
func RequestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// AccessLog logs one structured line per request with method, path, status and
// latency.
func AccessLog(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			log.Info("http_request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rec.status),
				zap.Duration("duration", time.Since(start)),
				zap.String("request_id", RequestIDFromContext(r.Context())),
			)
		})
	}
}

// Recoverer converts panics into a clean 500 response instead of crashing the
// server, logging the stack context.
func Recoverer(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic_recovered",
						zap.Any("panic", rec),
						zap.String("request_id", RequestIDFromContext(r.Context())),
					)
					writeError(w, domain.NewInternalError("internal server error", nil))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_middleware.log at runtime.
var _ = func() bool { flog.For("10_middleware").Info("source file initialized"); return true }()
