package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"journeyin/internal/application"
	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
)

type Server struct {
	trips              *application.TripService
	web                fs.FS
	schema             fs.FS
	version            string
	logger             *slog.Logger
	mapRegistry        *journeymaps.Registry
	mapService         *application.MapService
	browserMapKey      string
	amapBrowserKey     string
	amapSecurityCode   string
	defaultMapProvider string
	shareService       *journeyshare.Service
	publicURL          string
	syncStore          *store.Store
	settingsStore      *store.Store
	auth               *Authenticator
}

func NewServer(trips *application.TripService, web, schema fs.FS, version string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{trips: trips, web: web, schema: schema, version: version, logger: logger}
}

func (s *Server) SetMapRegistry(registry *journeymaps.Registry, browserKey string) {
	s.mapRegistry = registry
	s.browserMapKey = browserKey
}
func (s *Server) SetAMapBrowserKey(browserKey string) {
	s.amapBrowserKey = strings.TrimSpace(browserKey)
}
func (s *Server) SetAMapSecurityJSCode(code string) { s.amapSecurityCode = strings.TrimSpace(code) }
func (s *Server) SetDefaultMapProvider(provider journeymaps.ProviderID) {
	if provider == journeymaps.ProviderAMap || provider == journeymaps.ProviderBaidu {
		s.defaultMapProvider = string(provider)
	}
}
func (s *Server) SetMapService(service *application.MapService) { s.mapService = service }

func (s *Server) SetShareService(service *journeyshare.Service, publicURL string) {
	s.shareService = service
	s.publicURL = strings.TrimRight(publicURL, "/")
}

func (s *Server) SetSyncStore(syncStore *store.Store)         { s.syncStore = syncStore }
func (s *Server) SetSettingsStore(settingsStore *store.Store) { s.settingsStore = settingsStore }
func (s *Server) SetAuthenticator(auth *Authenticator)        { s.auth = auth }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", s.health)
	mux.HandleFunc("POST /api/v1/auth/login", s.login)
	mux.HandleFunc("POST /api/v1/auth/logout", s.logout)
	mux.HandleFunc("GET /api/v1/capabilities", s.capabilities)
	mux.HandleFunc("GET /api/v1/schema/trip/v1.json", s.schemaTrip)
	mux.HandleFunc("POST /api/v1/validate", s.validateTrip)
	mux.HandleFunc("GET /api/v1/trips", s.listTrips)
	mux.HandleFunc("POST /api/v1/trips", s.createTrip)
	mux.HandleFunc("POST /api/v1/import", s.importTrip)
	mux.HandleFunc("GET /api/v1/trips/{id}", s.getTrip)
	mux.HandleFunc("PUT /api/v1/trips/{id}", s.replaceTrip)
	mux.HandleFunc("POST /api/v1/trips/{id}/days/{dayID}/stops", s.addStop)
	mux.HandleFunc("POST /api/v1/trips/{id}/days/{dayID}/stops/{stopID}/move", s.moveStop)
	mux.HandleFunc("DELETE /api/v1/trips/{id}/days/{dayID}/stops/{stopID}", s.deleteStop)
	mux.HandleFunc("POST /api/v1/trips/{id}/days/{dayID}/stops/{stopID}/children", s.addSubStop)
	mux.HandleFunc("POST /api/v1/trips/{id}/days/{dayID}/stops/{stopID}/weather", s.refreshWeather)
	mux.HandleFunc("POST /api/v1/trips/{id}/plan", s.planTrip)
	mux.HandleFunc("POST /api/v1/trips/{id}/routes/refresh", s.planTrip)
	mux.HandleFunc("DELETE /api/v1/trips/{id}", s.deleteTrip)
	mux.HandleFunc("GET /api/v1/trips/{id}/export.json", s.exportTrip)
	mux.HandleFunc("POST /api/v1/maps/geocode", s.geocode)
	mux.HandleFunc("POST /api/v1/maps/pois/search", s.searchPOI)
	mux.HandleFunc("POST /api/v1/maps/reverse-geocode", s.reverseGeocode)
	mux.HandleFunc("POST /api/v1/maps/route", s.route)
	mux.HandleFunc("POST /api/v1/maps/weather", s.weather)
	mux.HandleFunc("POST /api/v1/maps/navigation", s.navigation)
	mux.HandleFunc("POST /api/v1/shares", s.createShare)
	mux.HandleFunc("POST /api/v1/shares/{id}/revoke", s.revokeShare)
	mux.HandleFunc("GET /s/{token}", s.publicShare)
	mux.HandleFunc("GET /api/v1/sync/pull", s.syncPull)
	mux.HandleFunc("POST /api/v1/sync/push", s.syncPush)
	mux.HandleFunc("GET /api/v1/settings", s.getSettings)
	mux.HandleFunc("PUT /api/v1/settings/map-keys", s.updateMapKeys)
	mux.HandleFunc("PUT /api/v1/settings/poi", s.updatePOIPreferences)
	mux.HandleFunc("DELETE /api/v1/settings/place-directory", s.clearPlaceDirectory)
	mux.HandleFunc("/_AMapService/", s.amapServiceProxy)
	mux.Handle("/", s.staticHandler())
	return requestLogger(mux, s.logger)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || !s.auth.UsernamePasswordEnabled() {
		writeError(w, http.StatusServiceUnavailable, "auth_not_configured", "username/password authentication is not configured", nil)
		return
	}
	var body loginRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	session, err := s.auth.Login(body.Username, body.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "账号或密码错误", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "auth_error", err.Error(), nil)
		return
	}
	s.auth.SetSessionCookie(w, session, r.TLS != nil)
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "username": strings.TrimSpace(body.Username), "expires_in": int(authSessionTTL.Seconds())})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if s.auth != nil {
		s.auth.ClearSession(w, r)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": s.version})
}
func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	providers := map[string]any{
		"baidu": map[string]any{"registered": false, "browser_key_configured": s.browserMapKey != "", "browser_key": s.browserMapKey},
		"amap":  map[string]any{"registered": false, "browser_key_configured": s.amapBrowserKey != "", "browser_key": s.amapBrowserKey, "security_proxy_path": "/_AMapService", "security_js_code_configured": s.amapSecurityCode != ""},
	}
	if s.mapRegistry != nil {
		if _, ok := s.mapRegistry.Get(journeymaps.ProviderBaidu); ok {
			providers["baidu"].(map[string]any)["registered"] = true
		}
		if _, ok := s.mapRegistry.Get(journeymaps.ProviderAMap); ok {
			providers["amap"].(map[string]any)["registered"] = true
		}
	}
	defaultProvider := s.defaultMapProvider
	if defaultProvider == "" {
		defaultProvider = string(journeymaps.ProviderBaidu)
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": s.version, "schema_versions": []int{1}, "default_map_provider": defaultProvider, "map_providers": providers, "mcp": map[string]any{"http_endpoint": "/mcp", "transports": []string{"streamable-http", "stdio"}}})
}

func (s *Server) schemaTrip(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.schema, "trip.v1.json")
	if err != nil {
		http.Error(w, "schema unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/schema+json; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) validateTrip(w http.ResponseWriter, r *http.Request) {
	document, err := readBody(r, 4<<20)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
		return
	}
	normalized, trip, issues, err := s.trips.Validate(document)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if hasValidationErrors(issues) {
		writeError(w, http.StatusBadRequest, "validation_error", "trip validation failed", map[string]any{"issues": issues})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"document": json.RawMessage(normalized), "summary": trip.Summary(), "warnings": validationWarnings(issues)})
}

func hasValidationErrors(issues []domain.ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}

func validationWarnings(issues []domain.ValidationIssue) []domain.ValidationIssue {
	result := make([]domain.ValidationIssue, 0)
	for _, issue := range issues {
		if issue.Level == "warning" {
			result = append(result, issue)
		}
	}
	return result
}

func (s *Server) listTrips(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	records, err := s.trips.List(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	result := make([]map[string]any, 0, len(records))
	for _, record := range records {
		result = append(result, tripSummary(record))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result, "next_cursor": nil})
}

func (s *Server) importTrip(w http.ResponseWriter, r *http.Request) {
	document, err := readBody(r, 4<<20)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
		return
	}
	record, err := s.trips.Import(r.Context(), document, "rest:import")
	if err != nil {
		writeDomainError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+record.ID)
	writeJSON(w, http.StatusCreated, tripResponse(record))
}

func (s *Server) createTrip(w http.ResponseWriter, r *http.Request) {
	document, err := readBody(r, 4<<20)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
		return
	}
	record, err := s.trips.Create(r.Context(), document, "rest")
	if err != nil {
		writeDomainError(w, err)
		return
	}
	w.Header().Set("Location", "/api/v1/trips/"+record.ID)
	writeJSON(w, http.StatusCreated, tripResponse(record))
}

func (s *Server) getTrip(w http.ResponseWriter, r *http.Request) {
	record, err := s.trips.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(record))
}
func (s *Server) exportTrip(w http.ResponseWriter, r *http.Request) {
	record, err := s.trips.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=journeyin-trip.json")
	_, _ = w.Write(record.Document)
}

func (s *Server) replaceTrip(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	document, err := readBody(r, 4<<20)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
		return
	}
	record, err := s.trips.Replace(r.Context(), r.PathValue("id"), expected, document, "rest")
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "revision_conflict", "trip revision conflict", map[string]any{"expected_revision": expected})
		return
	}
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tripResponse(record))
}
func (s *Server) deleteTrip(w http.ResponseWriter, r *http.Request) {
	expected, err := parseRevision(r.Header.Get("If-Match"))
	if err != nil {
		writeError(w, http.StatusPreconditionRequired, "if_match_required", "If-Match must be revision-N", nil)
		return
	}
	err = s.trips.Delete(r.Context(), r.PathValue("id"), expected)
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "revision_conflict", "trip revision conflict", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) staticHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.web))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})
}
func tripSummary(r store.TripRecord) map[string]any {
	var document struct {
		Days []struct {
			Stops []any `json:"stops"`
		} `json:"days"`
	}
	_ = json.Unmarshal(r.Document, &document)
	stops := 0
	for _, day := range document.Days {
		stops += len(day.Stops)
	}
	return map[string]any{"id": r.ID, "title": r.Title, "status": r.Status, "start_date": r.StartDate, "end_date": r.EndDate, "timezone": r.Timezone, "revision": r.Revision, "days": len(document.Days), "stops": stops, "content_hash": r.ContentHash, "created_at": r.CreatedAt, "updated_at": r.UpdatedAt}
}
func tripResponse(r store.TripRecord) map[string]any {
	result := tripSummary(r)
	var document any
	if json.Unmarshal(r.Document, &document) == nil {
		result["document"] = document
	}
	return result
}
func parseRevision(value string) (int, error) {
	value = strings.Trim(value, "\"")
	if !strings.HasPrefix(value, "revision-") {
		return 0, fmt.Errorf("invalid revision")
	}
	return strconv.Atoi(strings.TrimPrefix(value, "revision-"))
}
func readBody(r *http.Request, max int64) ([]byte, error) {
	defer r.Body.Close()
	data, err := io.ReadAll(io.LimitReader(r.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("request body exceeds %d bytes", max)
	}
	return data, nil
}
func writeDomainError(w http.ResponseWriter, err error) {
	var validation application.ValidationError
	if errors.As(err, &validation) {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), map[string]any{"issues": validation.Issues})
		return
	}
	if errors.Is(err, store.ErrRevisionConflict) {
		writeError(w, http.StatusConflict, "revision_conflict", err.Error(), nil)
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_trip", err.Error(), nil)
}
func writeError(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "details": details}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func requestLogger(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if strings.HasPrefix(path, "/s/") {
			path = "/s/<redacted>"
		}
		logger.Info("http request", "method", r.Method, "path", path)
		next.ServeHTTP(w, r)
	})
}
