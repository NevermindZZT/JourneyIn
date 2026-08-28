package httpapi

import (
	"net/http"
	"strings"

	journeymaps "journeyin/internal/maps"
)

type mapKeysBody struct {
	BaiduBrowserKey *string `json:"baidu_browser_key"`
	BaiduServerKey  *string `json:"baidu_server_key"`
	AMapJSKey       *string `json:"amap_js_key"`
	AMapServerKey   *string `json:"amap_server_key"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "settings store is not configured", nil)
		return
	}
	_, browserOK, err := s.settingsStore.GetSetting(r.Context(), "map.baidu.browser_key")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	browserOK = browserOK || strings.TrimSpace(s.browserMapKey) != ""
	_, serverOK, err := s.settingsStore.GetSetting(r.Context(), "map.baidu.server_key")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	if s.mapRegistry != nil {
		if provider, ok := s.mapRegistry.Get(journeymaps.ProviderBaidu); ok {
			if baidu, ok := provider.(*journeymaps.BaiduProvider); ok {
				serverOK = serverOK || baidu.ServerAKConfigured()
			}
		}
	}
	_, amapJSOK, err := s.settingsStore.GetSetting(r.Context(), "map.amap.js_key")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	_, amapServerOK, err := s.settingsStore.GetSetting(r.Context(), "map.amap.server_key")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	if s.mapRegistry != nil {
		if provider, ok := s.mapRegistry.Get(journeymaps.ProviderAMap); ok {
			if amap, ok := provider.(*journeymaps.AMapProvider); ok {
				amapServerOK = amapServerOK || amap.ServerKeyConfigured()
			}
		}
	}
	priority, priorityOK, err := s.settingsStore.GetSetting(r.Context(), "map.poi.provider_priority")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	if !priorityOK || (priority != string(journeymaps.ProviderAMap) && priority != string(journeymaps.ProviderBaidu)) {
		priority = string(journeymaps.ProviderAMap)
	}
	directoryCount, err := s.settingsStore.PlaceDirectoryCount(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"map": map[string]any{
		"baidu": map[string]any{"browser_key_configured": browserOK, "server_key_configured": serverOK},
		"amap":  map[string]any{"js_key_configured": amapJSOK, "server_key_configured": amapServerOK},
	}, "poi": map[string]any{"provider_priority": priority, "local_directory_count": directoryCount}})
}

func (s *Server) updateMapKeys(w http.ResponseWriter, r *http.Request) {
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "settings store is not configured", nil)
		return
	}
	var body mapKeysBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	updates := []struct {
		key    string
		value  *string
		secret bool
	}{
		{"map.baidu.browser_key", body.BaiduBrowserKey, false},
		{"map.baidu.server_key", body.BaiduServerKey, true},
		{"map.amap.js_key", body.AMapJSKey, false},
		{"map.amap.server_key", body.AMapServerKey, true},
	}
	for _, update := range updates {
		if update.value == nil {
			continue
		}
		value := strings.TrimSpace(*update.value)
		var err error
		if value == "" {
			err = s.settingsStore.DeleteSetting(r.Context(), update.key)
		} else {
			err = s.settingsStore.SetSetting(r.Context(), update.key, value, update.secret)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
			return
		}
	}
	if body.BaiduBrowserKey != nil {
		s.browserMapKey = strings.TrimSpace(*body.BaiduBrowserKey)
	}
	if body.BaiduServerKey != nil && s.mapRegistry != nil {
		if provider, ok := s.mapRegistry.Get(journeymaps.ProviderBaidu); ok {
			if baidu, ok := provider.(*journeymaps.BaiduProvider); ok {
				baidu.SetServerAK(strings.TrimSpace(*body.BaiduServerKey))
			}
		}
	}
	if body.AMapServerKey != nil && s.mapRegistry != nil {
		if provider, ok := s.mapRegistry.Get(journeymaps.ProviderAMap); ok {
			if amap, ok := provider.(*journeymaps.AMapProvider); ok {
				amap.SetServerKey(strings.TrimSpace(*body.AMapServerKey))
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

type poiPreferencesBody struct {
	ProviderPriority string `json:"provider_priority"`
}

func (s *Server) updatePOIPreferences(w http.ResponseWriter, r *http.Request) {
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "settings store is not configured", nil)
		return
	}
	var body poiPreferencesBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	priority := strings.TrimSpace(body.ProviderPriority)
	if priority != string(journeymaps.ProviderAMap) && priority != string(journeymaps.ProviderBaidu) {
		writeError(w, http.StatusBadRequest, "invalid_provider_priority", "provider_priority must be amap or baidu", nil)
		return
	}
	if err := s.settingsStore.SetSetting(r.Context(), "map.poi.provider_priority", priority, false); err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "provider_priority": priority})
}
func (s *Server) clearPlaceDirectory(w http.ResponseWriter, r *http.Request) {
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "settings store is not configured", nil)
		return
	}
	if err := s.settingsStore.ClearPlaceDirectory(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
}
