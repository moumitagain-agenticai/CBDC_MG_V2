package api

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/fineract/cacti-bridge/internal/domain"
	"github.com/fineract/cacti-bridge/internal/service"
)

// Handler exposes the settlement coordinator over HTTP.
type Handler struct {
	svc      *service.Coordinator
	maxBytes int64
}

// NewHandler builds an HTTP handler around the coordinator.
func NewHandler(svc *service.Coordinator, maxBytes int64) *Handler {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return &Handler{svc: svc, maxBytes: maxBytes}
}

// Settle handles POST /api/v1/settlements — initiate a cross-chain settlement.
func (h *Handler) Settle(w http.ResponseWriter, r *http.Request) {
	var req domain.SettleRequest
	if !decodeJSON(w, r, h.maxBytes, &req) {
		return
	}
	t, err := h.svc.Settle(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	// A settlement that reaches a defined outcome is a successful call; the
	// status field carries the result (BURNED / COMPENSATED / FAILED / RELEASED).
	status := http.StatusCreated
	if t.Status == domain.StatusFailed {
		status = http.StatusOK
	}
	writeJSON(w, status, t)
}

// Get handles GET /api/v1/settlements/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Get(r.Context(), chi.URLParam(r, "id"))
	respond(w, http.StatusOK, t, err)
}

// Rollback handles POST /api/v1/settlements/{id}/rollback.
func (h *Handler) Rollback(w http.ResponseWriter, r *http.Request) {
	t, err := h.svc.Rollback(r.Context(), chi.URLParam(r, "id"))
	respond(w, http.StatusOK, t, err)
}

func (h *Handler) Liveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.svc.Liveness())
}

func (h *Handler) Readiness(w http.ResponseWriter, r *http.Request) {
	rep := h.svc.Readiness(r.Context())
	status := http.StatusOK
	if rep.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, rep)
}

// ---- helpers ----

func decodeJSON(w http.ResponseWriter, r *http.Request, max int64, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, max)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, domain.NewValidationError("invalid JSON body", err))
		return false
	}
	return true
}

func respond(w http.ResponseWriter, status int, payload any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, status, payload)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, err error) {
	de := domain.AsDomainError(err)
	var body errorBody
	body.Error.Code = string(de.Code)
	body.Error.Message = de.Message
	writeJSON(w, httpStatus(de.Code), body)
}

func httpStatus(code domain.ErrorCode) int {
	switch code {
	case domain.CodeValidation:
		return http.StatusBadRequest
	case domain.CodeNotFound:
		return http.StatusNotFound
	case domain.CodeConflict:
		return http.StatusConflict
	case domain.CodeUnauthorized:
		return http.StatusUnauthorized
	case domain.CodeTimeout:
		return http.StatusGatewayTimeout
	case domain.CodeUpstream, domain.CodeCircuitOpen:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_handlers.log at runtime.
var _ = func() bool { flog.For("10_handlers").Info("source file initialized"); return true }()
