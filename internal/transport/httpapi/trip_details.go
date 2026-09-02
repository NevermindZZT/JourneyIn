package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"journeyin/internal/application"
	"journeyin/internal/domain"
	"journeyin/internal/store"
)

type updateTripDetailsBody struct {
	Title     *string           `json:"title"`
	DateRange *domain.DateRange `json:"date_range"`
}

func (s *Server) updateTripDetails(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required", nil)
		return
	}
	var body updateTripDetailsBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	record, changes, replayed, err := s.trips.UpdateTripDetailsIdempotent(r.Context(), r.PathValue("id"), expected, application.UpdateTripDetailsInput{Title: body.Title, DateRange: body.DateRange}, idempotencyKey, "rest:update_trip_details")
	if err != nil {
		writeTripDetailsError(w, err)
		return
	}
	response := tripResponse(record)
	response["changes"] = changes
	response["idempotency_replay"] = replayed
	writeJSON(w, http.StatusOK, response)
}

func writeTripDetailsError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "revision_conflict", "trip revision conflict", nil)
		return
	}
	if errors.Is(err, store.ErrIdempotencyConflict) {
		writeError(w, http.StatusConflict, "idempotency_conflict", err.Error(), nil)
		return
	}
	var conflict *application.DateRangeConflictError
	if errors.As(err, &conflict) {
		writeError(w, http.StatusConflict, "date_range_conflict", err.Error(), map[string]any{"days": conflict.Days})
		return
	}
	if errors.Is(err, application.ErrTripDetailsEmpty) {
		writeError(w, http.StatusBadRequest, "empty_update", err.Error(), nil)
		return
	}
	writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
}
