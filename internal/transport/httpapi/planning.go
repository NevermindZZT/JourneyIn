package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"journeyin/internal/application"
	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

type addStopBody struct {
	ExpectedRevision int `json:"expected_revision,omitempty"`
	Stop             struct {
		ID                  string          `json:"id,omitempty"`
		Sequence            int             `json:"sequence,omitempty"`
		Kind                string          `json:"kind,omitempty"`
		Title               string          `json:"title"`
		Address             string          `json:"address,omitempty"`
		Location            json.RawMessage `json:"location"`
		TimeWindow          json.RawMessage `json:"time_window,omitempty"`
		DescriptionMarkdown string          `json:"description_markdown,omitempty"`
		Links               []domain.Link   `json:"links,omitempty"`
		Weather             json.RawMessage `json:"weather,omitempty"`
	} `json:"stop"`
}

type planTripBody struct {
	ExpectedRevision int                    `json:"expected_revision,omitempty"`
	Provider         journeymaps.ProviderID `json:"provider,omitempty"`
	Mode             journeymaps.TravelMode `json:"mode,omitempty"`
	DayID            string                 `json:"day_id,omitempty"`
	DepartureAt      string                 `json:"departure_at,omitempty"`
}

func (s *Server) addStop(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	var body addStopBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	stop := application.AddStopInput{ID: body.Stop.ID, Sequence: body.Stop.Sequence, Kind: body.Stop.Kind, Title: body.Stop.Title, Address: body.Stop.Address, Location: body.Stop.Location, TimeWindow: body.Stop.TimeWindow, DescriptionMarkdown: body.Stop.DescriptionMarkdown, Links: body.Stop.Links, Weather: body.Stop.Weather}
	record, err := s.trips.AddStop(r.Context(), r.PathValue("id"), expected, r.PathValue("dayID"), stop, "rest:add_stop")
	if err != nil {
		writePlanningError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(record))
}

func (s *Server) addSubStop(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	var body addStopBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	input := application.AddStopInput{ID: body.Stop.ID, Sequence: body.Stop.Sequence, Kind: body.Stop.Kind, Title: body.Stop.Title, Address: body.Stop.Address, Location: body.Stop.Location, TimeWindow: body.Stop.TimeWindow, DescriptionMarkdown: body.Stop.DescriptionMarkdown, Links: body.Stop.Links, Weather: body.Stop.Weather}
	record, err := s.trips.AddSubStop(r.Context(), r.PathValue("id"), expected, r.PathValue("dayID"), r.PathValue("stopID"), input, "rest:add_sub_stop")
	if err != nil {
		writePlanningError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(record))
}

type moveStopBody struct {
	Direction      string `json:"direction"`
	TargetSequence int    `json:"target_sequence,omitempty"`
}

func (s *Server) moveStop(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	var body moveStopBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	var record store.TripRecord
	if body.TargetSequence > 0 {
		record, err = s.trips.ReorderStop(r.Context(), r.PathValue("id"), expected, r.PathValue("dayID"), r.PathValue("stopID"), body.TargetSequence, "rest:reorder_stop")
	} else {
		record, err = s.trips.MoveStop(r.Context(), r.PathValue("id"), expected, r.PathValue("dayID"), r.PathValue("stopID"), body.Direction, "rest:move_stop")
	}
	if err != nil {
		writePlanningError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(record))
}

func (s *Server) deleteStop(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	record, err := s.trips.DeleteStop(r.Context(), r.PathValue("id"), expected, r.PathValue("dayID"), r.PathValue("stopID"), "rest:delete_stop")
	if err != nil {
		writePlanningError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(record))
}

type weatherRefreshBody struct {
	Provider  journeymaps.ProviderID `json:"provider,omitempty"`
	LocalDate string                 `json:"local_date,omitempty"`
}

func (s *Server) refreshWeather(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	var body weatherRefreshBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	record, err := s.trips.RefreshWeather(r.Context(), r.PathValue("id"), expected, r.PathValue("dayID"), r.PathValue("stopID"), application.WeatherInput{Provider: body.Provider, LocalDate: body.LocalDate}, "rest:weather_refresh")
	if err != nil {
		writePlanningError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(record))
}

func (s *Server) planTrip(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	var body planTripBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	departureAt, err := parseOptionalTime(body.DepartureAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_departure_at", err.Error(), nil)
		return
	}
	record, err := s.trips.PlanTrip(r.Context(), r.PathValue("id"), expected, application.PlanInput{Provider: body.Provider, Mode: body.Mode, DayID: body.DayID, DepartureAt: departureAt}, "rest:plan")
	if err != nil {
		writePlanningError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(record))
}

func parseOptionalTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}
func writePlanningError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "revision_conflict", err.Error(), nil)
		return
	}
	if errors.Is(err, store.ErrMapQuotaExceeded) {
		writeError(w, http.StatusTooManyRequests, "quota_exceeded", err.Error(), nil)
		return
	}
	if errors.Is(err, journeymaps.ErrProviderUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "provider_unavailable", err.Error(), nil)
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	writeError(w, http.StatusBadRequest, "planning_error", err.Error(), nil)
}
