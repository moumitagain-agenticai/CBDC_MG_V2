package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/fineract/cbdc/india-connector/internal/domain"
	"github.com/fineract/cbdc/india-connector/internal/ports"
	"github.com/fineract/cbdc/india-connector/internal/service"
)

// Handler exposes the connector service over HTTP.
type Handler struct {
	svc *service.Connector
}

// NewHandler builds an HTTP handler around the connector service.
func NewHandler(svc *service.Connector) *Handler { return &Handler{svc: svc} }

func (h *Handler) Issue(w http.ResponseWriter, r *http.Request) {
	var req ports.IssueRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Issue(r.Context(), req)
	respond(w, http.StatusOK, res, err)
}

func (h *Handler) Transfer(w http.ResponseWriter, r *http.Request) {
	var req ports.TransferRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Transfer(r.Context(), req)
	respond(w, http.StatusOK, res, err)
}

func (h *Handler) Lock(w http.ResponseWriter, r *http.Request) {
	var req ports.LockRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Lock(r.Context(), req)
	respond(w, http.StatusOK, res, err)
}

func (h *Handler) Burn(w http.ResponseWriter, r *http.Request) {
	var req ports.BurnRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Burn(r.Context(), req)
	respond(w, http.StatusOK, res, err)
}

func (h *Handler) Redeem(w http.ResponseWriter, r *http.Request) {
	var req ports.RedeemRequest
	if !decode(w, r, &req) {
		return
	}
	res, err := h.svc.Redeem(r.Context(), req)
	respond(w, http.StatusOK, res, err)
}

func (h *Handler) Balance(w http.ResponseWriter, r *http.Request) {
	walletID := chi.URLParam(r, "walletID")
	res, err := h.svc.GetBalance(r.Context(), walletID)
	respond(w, http.StatusOK, res, err)
}

func (h *Handler) UpstreamStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.svc.GetUpstreamStatus(r.Context(), id)
	respond(w, http.StatusOK, res, err)
}

func (h *Handler) GetTransaction(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	res, err := h.svc.GetTransaction(r.Context(), id)
	respond(w, http.StatusOK, res, err)
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

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
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
	case domain.CodeCircuitOpen:
		return http.StatusServiceUnavailable
	case domain.CodeUpstream:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
