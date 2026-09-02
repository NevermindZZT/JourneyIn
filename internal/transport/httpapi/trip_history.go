package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"journeyin/internal/application"
	"journeyin/internal/store"
)

type saveTripHistoryBody struct {
	Label string `json:"label,omitempty"`
}

func (s *Server) createTripHistory(w http.ResponseWriter, r *http.Request) {
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
	var body saveTripHistoryBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	version, alreadySaved, replayed, err := s.trips.SaveTripVersionIdempotent(r.Context(), r.PathValue("id"), expected, body.Label, idempotencyKey)
	if err != nil {
		writeTripHistoryError(w, err)
		return
	}
	response := tripHistoryResponse(version, false)
	response["already_saved"] = alreadySaved
	response["idempotency_replay"] = replayed
	status := http.StatusCreated
	if alreadySaved || replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, response)
}

func (s *Server) listTripHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	versions, err := s.trips.ListTripVersions(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		writeTripHistoryError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(versions))
	for _, version := range versions {
		items = append(items, tripHistoryResponse(version, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nil})
}

func (s *Server) getTripHistory(w http.ResponseWriter, r *http.Request) {
	version, err := s.trips.GetTripVersion(r.Context(), r.PathValue("id"), r.PathValue("historyID"))
	if err != nil {
		writeTripHistoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tripHistoryResponse(version, true))
}

func (s *Server) deleteTripHistory(w http.ResponseWriter, r *http.Request) {
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key is required", nil)
		return
	}
	_, err := s.trips.DeleteTripVersionIdempotent(r.Context(), r.PathValue("id"), r.PathValue("historyID"), idempotencyKey)
	if err != nil {
		writeTripHistoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func tripHistoryResponse(version store.SavedTripVersion, includeDocument bool) map[string]any {
	result := map[string]any{
		"id":              version.ID,
		"history_id":      version.ID,
		"trip_id":         version.TripID,
		"source_revision": version.SourceRevision,
		"title":           version.Title,
		"start_date":      version.StartDate,
		"end_date":        version.EndDate,
		"label":           version.Label,
		"content_hash":    version.ContentHash,
		"created_at":      version.CreatedAt,
		"read_only":       true,
	}
	if includeDocument {
		result["document"] = json.RawMessage(version.Document)
	}
	return result
}

func writeTripHistoryError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	}
	if errors.Is(err, store.ErrSavedTripVersionNotFound) {
		writeError(w, http.StatusNotFound, "history_not_found", "saved trip version not found", nil)
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
	if errors.Is(err, application.ErrSavedTripVersionLabelTooLong) {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), nil)
		return
	}
	if errors.Is(err, application.ErrIdempotencyKeyRequired) {
		writeError(w, http.StatusBadRequest, "idempotency_key_required", err.Error(), nil)
		return
	}
	writeError(w, http.StatusBadRequest, "history_error", err.Error(), nil)
}
