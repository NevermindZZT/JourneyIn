package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"journeyin/internal/application"
	"journeyin/internal/store"
)

type updatePlanningPointBody struct {
	Title       *string
	Address     *string
	Location    json.RawMessage
	LocationSet bool
}

func (body *updatePlanningPointBody) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("planning point update must be a JSON object")
	}
	for key := range raw {
		if key != "title" && key != "address" && key != "location" {
			return fmt.Errorf("planning point update.%s is not supported", key)
		}
	}
	if value, ok := raw["title"]; ok {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("planning point update.title must not be null")
		}
		var title string
		if err := json.Unmarshal(value, &title); err != nil {
			return fmt.Errorf("planning point update.title must be a string: %w", err)
		}
		body.Title = &title
	}
	if value, ok := raw["address"]; ok {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("planning point update.address must not be null")
		}
		var address string
		if err := json.Unmarshal(value, &address); err != nil {
			return fmt.Errorf("planning point update.address must be a string: %w", err)
		}
		body.Address = &address
	}
	if value, ok := raw["location"]; ok {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("planning point update.location must not be null")
		}
		body.Location = append(json.RawMessage(nil), value...)
		body.LocationSet = true
	}
	return nil
}

func (s *Server) updatePlanningPoint(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	var body updatePlanningPointBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	record, changes, err := s.trips.UpdatePlanningPoint(r.Context(), r.PathValue("id"), expected, r.PathValue("dayID"), r.PathValue("stopID"), application.UpdatePlanningPointInput{Title: body.Title, Address: body.Address, Location: body.Location, LocationSet: body.LocationSet}, "rest:update_planning_point")
	if err != nil {
		writePlanningPointError(w, err)
		return
	}
	response := tripResponse(record)
	response["changes"] = changes
	writeJSON(w, http.StatusOK, response)
}

func writePlanningPointError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "revision_conflict", err.Error(), nil)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	if errors.Is(err, application.ErrPlanningLocationRequired) {
		writeError(w, http.StatusUnprocessableEntity, "location_required", "planning point must have a reliable location", map[string]any{"requires_user_consent": true})
		return
	}
	if errors.Is(err, application.ErrPlanningPointUpdateEmpty) {
		writeError(w, http.StatusBadRequest, "empty_update", err.Error(), nil)
		return
	}
	writeError(w, http.StatusBadRequest, "planning_point_update_error", err.Error(), nil)
}
