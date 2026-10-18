package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"journeyin/internal/monitor"
	"journeyin/internal/store"
)

type createMonitorRequest struct {
	Title      string                  `json:"title"`
	PlaceName  string                  `json:"place_name"`
	Lat        float64                 `json:"lat"`
	Lng        float64                 `json:"lng"`
	CRS        string                  `json:"crs,omitempty"`
	Criteria   monitor.MonitorCriteria `json:"criteria"`
	OriginCity string                  `json:"origin_city,omitempty"`
	MaxBudget  float64                 `json:"max_budget,omitempty"`
}

type updateMonitorRequest struct {
	Title      *string                  `json:"title,omitempty"`
	PlaceName  *string                  `json:"place_name,omitempty"`
	Lat        *float64                 `json:"lat,omitempty"`
	Lng        *float64                 `json:"lng,omitempty"`
	CRS        *string                  `json:"crs,omitempty"`
	Criteria   *monitor.MonitorCriteria `json:"criteria,omitempty"`
	OriginCity *string                  `json:"origin_city,omitempty"`
	MaxBudget  *float64                 `json:"max_budget,omitempty"`
	Enabled    *bool                    `json:"enabled,omitempty"`
}

type convertTripRequest struct {
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
}

func (s *Server) listMonitors(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil {
		writeJSON(w, http.StatusOK, []monitor.Monitor{})
		return
	}
	enabledOnly := r.URL.Query().Get("enabled") == "true"
	items, err := s.monitorService.ListMonitors(r.Context(), enabledOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "monitor_error", err.Error(), nil)
		return
	}
	if items == nil {
		items = []monitor.Monitor{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *Server) createMonitor(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil {
		writeError(w, http.StatusServiceUnavailable, "monitor_unavailable", "monitor service is not configured", nil)
		return
	}
	var req createMonitorRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}

	req.PlaceName = strings.TrimSpace(req.PlaceName)
	if req.PlaceName == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "place_name is required", nil)
		return
	}
	if req.Lat == 0 && req.Lng == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "valid coordinates are required", nil)
		return
	}
	if req.Title == "" {
		req.Title = req.PlaceName
	}
	if req.CRS == "" {
		req.CRS = "gcj02"
	}

	m, err := s.monitorService.CreateMonitor(r.Context(), monitor.Monitor{
		Title:      req.Title,
		PlaceName:  req.PlaceName,
		Lat:        req.Lat,
		Lng:        req.Lng,
		CRS:        req.CRS,
		Criteria:   req.Criteria,
		OriginCity: req.OriginCity,
		MaxBudget:  req.MaxBudget,
		Enabled:    true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "create_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (s *Server) getMonitor(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil {
		writeError(w, http.StatusNotFound, "not_found", "monitor not found", nil)
		return
	}
	id := r.PathValue("id")
	m, err := s.monitorService.GetMonitor(r.Context(), id)
	if errors.Is(err, store.ErrMonitorNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "monitor not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "monitor_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) updateMonitor(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil {
		writeError(w, http.StatusServiceUnavailable, "monitor_unavailable", "monitor service is not configured", nil)
		return
	}
	id := r.PathValue("id")
	m, err := s.monitorService.GetMonitor(r.Context(), id)
	if errors.Is(err, store.ErrMonitorNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "monitor not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "monitor_error", err.Error(), nil)
		return
	}

	var req updateMonitorRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}

	if req.Title != nil {
		m.Title = strings.TrimSpace(*req.Title)
	}
	if req.PlaceName != nil {
		m.PlaceName = strings.TrimSpace(*req.PlaceName)
	}
	if req.Lat != nil {
		m.Lat = *req.Lat
	}
	if req.Lng != nil {
		m.Lng = *req.Lng
	}
	if req.CRS != nil {
		m.CRS = *req.CRS
	}
	if req.Criteria != nil {
		m.Criteria = *req.Criteria
	}
	if req.OriginCity != nil {
		m.OriginCity = strings.TrimSpace(*req.OriginCity)
	}
	if req.MaxBudget != nil {
		m.MaxBudget = *req.MaxBudget
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}

	if err := s.monitorService.UpdateMonitor(r.Context(), m); err != nil {
		writeError(w, http.StatusInternalServerError, "update_error", err.Error(), nil)
		return
	}
	if updated, err := s.monitorService.GetMonitor(r.Context(), id); err == nil {
		m = updated
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) deleteMonitor(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil {
		writeError(w, http.StatusNotFound, "not_found", "monitor not found", nil)
		return
	}
	id := r.PathValue("id")
	if err := s.monitorService.DeleteMonitor(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrMonitorNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "monitor not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "delete_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getMonitorEvaluations(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil {
		writeJSON(w, http.StatusOK, []monitor.Evaluation{})
		return
	}
	id := r.PathValue("id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 30
	}
	evals, err := s.monitorService.GetEvaluations(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "eval_error", err.Error(), nil)
		return
	}
	if evals == nil {
		evals = []monitor.Evaluation{}
	}
	writeJSON(w, http.StatusOK, evals)
}

func (s *Server) evaluateMonitor(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil {
		writeError(w, http.StatusServiceUnavailable, "monitor_unavailable", "monitor service is not configured", nil)
		return
	}
	id := r.PathValue("id")
	if err := s.monitorService.EvaluateOne(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "eval_error", err.Error(), nil)
		return
	}
	evals, _ := s.monitorService.GetEvaluations(r.Context(), id, 30)
	writeJSON(w, http.StatusOK, map[string]any{
		"message":     "evaluated successfully",
		"evaluations": evals,
	})
}

func (s *Server) getRadarSummary(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil {
		writeJSON(w, http.StatusOK, monitor.RadarSummary{})
		return
	}
	summary, err := s.monitorService.GetRadarSummary(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "summary_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) convertMonitorToTrip(w http.ResponseWriter, r *http.Request) {
	if s.monitorService == nil || s.trips == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "trip service is not configured", nil)
		return
	}
	id := r.PathValue("id")

	var req convertTripRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if req.WindowStart == "" || req.WindowEnd == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "window_start and window_end are required", nil)
		return
	}

	createdID, err := s.monitorService.ConvertToTrip(r.Context(), id, req.WindowStart, req.WindowEnd, func(ctx context.Context, doc []byte, src string) (string, error) {
		record, err := s.trips.Create(ctx, doc, src)
		if err != nil {
			return "", err
		}
		return record.ID, nil
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "convert_error", err.Error(), nil)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trip_id": createdID,
		"message": "trip draft created successfully",
	})
}
