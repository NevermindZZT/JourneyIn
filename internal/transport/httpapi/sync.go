package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	journeysync "journeyin/internal/sync"
)

type syncPushBody struct {
	Changes []journeysync.Change `json:"changes"`
}

func (s *Server) syncPush(w http.ResponseWriter, r *http.Request) {
	if s.syncStore == nil {
		writeError(w, http.StatusServiceUnavailable, "sync_unavailable", "sync store is not configured", nil)
		return
	}
	var body syncPushBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if len(body.Changes) == 0 {
		writeError(w, http.StatusBadRequest, "empty_changes", "changes must not be empty", nil)
		return
	}
	accepted := make([]journeysync.Change, 0, len(body.Changes))
	for _, change := range body.Changes {
		result, err := s.syncStore.PushChange(r.Context(), change)
		if err != nil {
			code := "sync_error"
			status := http.StatusBadRequest
			if errors.Is(err, journeysync.ErrConflict) {
				code = "revision_conflict"
				status = http.StatusConflict
			}
			if errors.Is(err, journeysync.ErrIdempotencyConflict) {
				code = "idempotency_conflict"
				status = http.StatusConflict
			}
			writeError(w, status, code, err.Error(), map[string]any{"accepted": accepted, "change_id": change.ChangeID})
			return
		}
		accepted = append(accepted, result)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": accepted})
}

func (s *Server) syncPull(w http.ResponseWriter, r *http.Request) {
	if s.syncStore == nil {
		writeError(w, http.StatusServiceUnavailable, "sync_unavailable", "sync store is not configured", nil)
		return
	}
	rawCursor := r.URL.Query().Get("cursor")
	var cursor journeysync.Cursor
	if rawCursor != "" {
		value, err := strconv.ParseInt(rawCursor, 10, 64)
		if err != nil || value < 0 {
			writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor must be a non-negative integer", nil)
			return
		}
		cursor = journeysync.Cursor(value)
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	changes, next, err := s.syncStore.PullChanges(r.Context(), r.URL.Query().Get("aggregate_id"), cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "sync_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"changes": changes, "next_cursor": next})
}
