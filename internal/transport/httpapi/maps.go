package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

type geocodeBody struct {
	Provider journeymaps.ProviderID `json:"provider"`
	Address  string                 `json:"address"`
	City     string                 `json:"city,omitempty"`
}
type poiSearchBody struct {
	Provider journeymaps.ProviderID `json:"provider"`
	Query    string                 `json:"query"`
	Region   string                 `json:"region,omitempty"`
	Page     int                    `json:"page,omitempty"`
	PageSize int                    `json:"page_size,omitempty"`
	Category string                 `json:"category,omitempty"`
}
type routeBody struct {
	Provider journeymaps.ProviderID   `json:"provider"`
	Request  journeymaps.RouteRequest `json:"request"`
}
type weatherBody struct {
	Provider journeymaps.ProviderID     `json:"provider"`
	Request  journeymaps.WeatherRequest `json:"request"`
}
type navigationBody struct {
	Provider journeymaps.ProviderID `json:"provider"`
	Target   journeymaps.NavTarget  `json:"target"`
	Mode     journeymaps.TravelMode `json:"mode"`
	Platform journeymaps.Platform   `json:"platform"`
}

func (s *Server) provider(id journeymaps.ProviderID) (journeymaps.MapProvider, error) {
	if s.mapRegistry == nil {
		return nil, journeymaps.ErrProviderUnavailable
	}
	provider, ok := s.mapRegistry.Get(id)
	if !ok {
		return nil, fmt.Errorf("map provider %q is not registered", id)
	}
	return provider, nil
}

func (s *Server) geocode(w http.ResponseWriter, r *http.Request) {
	var body geocodeBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if body.Provider == "" {
		body.Provider = journeymaps.ProviderBaidu
	}
	provider, err := s.provider(body.Provider)
	if err != nil {
		writeMapError(w, err)
		return
	}
	var result []journeymaps.PlaceCandidate
	if s.mapService != nil {
		result, err = s.mapService.Geocode(r.Context(), body.Provider, body.Address, body.City)
	} else {
		result, err = provider.Geocode(r.Context(), body.Address, body.City)
	}
	if err != nil {
		writeMapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": body.Provider, "items": result})
}

func (s *Server) searchPOI(w http.ResponseWriter, r *http.Request) {
	var body poiSearchBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	preferred := body.Provider
	if preferred == "" && s.settingsStore != nil {
		if value, ok, err := s.settingsStore.GetSetting(r.Context(), "map.poi.provider_priority"); err == nil && ok {
			preferred = journeymaps.ProviderID(value)
		}
	}
	if preferred == "" {
		preferred = journeymaps.ProviderAMap
	}
	if preferred != journeymaps.ProviderAMap && preferred != journeymaps.ProviderBaidu {
		writeError(w, http.StatusBadRequest, "invalid_provider", "provider must be amap or baidu", nil)
		return
	}
	if s.mapService == nil {
		writeMapError(w, journeymaps.ErrProviderUnavailable)
		return
	}
	usedProvider, result, err := s.mapService.SearchPOIByPriority(r.Context(), preferred, body.Query, body.Region, body.Category, body.Page, body.PageSize)
	if err != nil {
		writeMapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": usedProvider, "items": result.Items, "total": result.Total, "page": result.Page, "page_size": result.PageSize})
}

func (s *Server) reverseGeocode(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider journeymaps.ProviderID `json:"provider"`
		Location journeymaps.GeoPoint   `json:"location"`
	}
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if body.Provider == "" {
		body.Provider = journeymaps.ProviderBaidu
	}
	provider, err := s.provider(body.Provider)
	if err != nil {
		writeMapError(w, err)
		return
	}
	var address string
	if s.mapService != nil {
		address, err = s.mapService.ReverseGeocode(r.Context(), body.Provider, body.Location)
	} else {
		address, err = provider.ReverseGeocode(r.Context(), body.Location)
	}
	if err != nil {
		writeMapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": body.Provider, "address": address})
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	var body routeBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if body.Provider == "" {
		body.Provider = journeymaps.ProviderBaidu
	}
	provider, err := s.provider(body.Provider)
	if err != nil {
		writeMapError(w, err)
		return
	}
	var result journeymaps.RouteSnapshot
	if s.mapService != nil {
		result, err = s.mapService.Route(r.Context(), body.Provider, body.Request)
	} else {
		result, err = provider.Route(r.Context(), body.Request)
	}
	if err != nil {
		writeMapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) weather(w http.ResponseWriter, r *http.Request) {
	var body weatherBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if body.Provider == "" {
		body.Provider = journeymaps.ProviderBaidu
	}
	provider, err := s.provider(body.Provider)
	if err != nil {
		writeMapError(w, err)
		return
	}
	var result journeymaps.WeatherSnapshot
	if s.mapService != nil {
		result, err = s.mapService.Weather(r.Context(), body.Provider, body.Request)
	} else {
		result, err = provider.Weather(r.Context(), body.Request)
	}
	if err != nil {
		writeMapError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) navigation(w http.ResponseWriter, r *http.Request) {
	var body navigationBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if body.Provider == "" {
		body.Provider = journeymaps.ProviderBaidu
	}
	provider, err := s.provider(body.Provider)
	if err != nil {
		writeMapError(w, err)
		return
	}
	raw, err := provider.NavigationURL(body.Target, body.Mode, body.Platform)
	if err != nil {
		writeMapError(w, err)
		return
	}
	if err := journeymaps.ValidateNavigationURL(raw); err != nil {
		writeMapError(w, err)
		return
	}
	response := map[string]any{"provider": body.Provider, "url": raw}
	if body.Platform == journeymaps.PlatformAndroid || body.Platform == journeymaps.PlatformIOS {
		if fallback, fallbackErr := provider.NavigationURL(body.Target, body.Mode, journeymaps.PlatformWeb); fallbackErr == nil && journeymaps.ValidateNavigationURL(fallback) == nil {
			response["fallback_url"] = fallback
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func decodeBody(r *http.Request, value any) error {
	data, err := readBody(r, 4<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}
func writeMapError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := "map_error"
	switch {
	case errors.Is(err, journeymaps.ErrProviderRateLimited):
		status = http.StatusTooManyRequests
		code = "provider_rate_limited"
	case errors.Is(err, journeymaps.ErrProviderQuotaExceeded):
		status = http.StatusTooManyRequests
		code = "provider_quota_exceeded"
	case errors.Is(err, journeymaps.ErrProviderTemporary):
		status = http.StatusServiceUnavailable
		code = "provider_temporary"
	case errors.Is(err, journeymaps.ErrProviderUnauthorized):
		status = http.StatusBadGateway
		code = "provider_unauthorized"
	case errors.Is(err, journeymaps.ErrProviderUnavailable):
		status = http.StatusServiceUnavailable
		code = "provider_unavailable"
	case errors.Is(err, store.ErrMapQuotaExceeded):
		status = http.StatusTooManyRequests
		code = "quota_exceeded"
	}
	writeError(w, status, code, err.Error(), nil)
}
