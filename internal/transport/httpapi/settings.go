package httpapi

import (
	"context"
	"net/http"
	"strings"

	journeymaps "journeyin/internal/maps"
)

const defaultMapProviderSettingKey = "map.default_provider"

type mapKeysBody struct {
	BaiduBrowserKey    *string `json:"baidu_browser_key"`
	BaiduServerKey     *string `json:"baidu_server_key"`
	AMapJSKey          *string `json:"amap_js_key"`
	AMapServerKey      *string `json:"amap_server_key"`
	AMapSecurityJSCode *string `json:"amap_security_js_code"`
}

type mapPreferencesBody struct {
	DefaultProvider journeymaps.ProviderID `json:"default_provider"`
}

func isSupportedMapProvider(provider journeymaps.ProviderID) bool {
	return provider == journeymaps.ProviderAMap || provider == journeymaps.ProviderBaidu
}

func (s *Server) defaultMapProviderFor(ctx context.Context) (journeymaps.ProviderID, error) {
	s.defaultProviderMu.RLock()
	provider := journeymaps.ProviderID(strings.TrimSpace(s.defaultMapProvider))
	s.defaultProviderMu.RUnlock()
	if s.settingsStore != nil {
		value, ok, err := s.settingsStore.GetSetting(ctx, defaultMapProviderSettingKey)
		if err != nil {
			return "", err
		}
		configured := journeymaps.ProviderID(strings.TrimSpace(value))
		if ok && isSupportedMapProvider(configured) {
			provider = configured
		}
	}
	if !isSupportedMapProvider(provider) {
		provider = journeymaps.ProviderBaidu
	}
	return provider, nil
}

func (s *Server) resolveMapProvider(ctx context.Context, requested journeymaps.ProviderID) (journeymaps.ProviderID, error) {
	requested = journeymaps.ProviderID(strings.TrimSpace(string(requested)))
	if requested != "" {
		return requested, nil
	}
	return s.defaultMapProviderFor(ctx)
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "settings store is not configured", nil)
		return
	}
	defaultProvider, err := s.defaultMapProviderFor(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
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
	_, amapSecurityOK, err := s.settingsStore.GetSetting(r.Context(), "map.amap.security_js_code")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	if s.mapRegistry != nil {
		if provider, ok := s.mapRegistry.Get(journeymaps.ProviderAMap); ok {
			if amap, ok := provider.(*journeymaps.AMapProvider); ok {
				amapJSOK = amapJSOK || amap.BrowserKeyConfigured()
				amapServerOK = amapServerOK || amap.ServerKeyConfigured()
				amapSecurityOK = amapSecurityOK || amap.SecurityJSCodeConfigured()
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
		"default_provider": defaultProvider,
		"baidu":            map[string]any{"browser_key_configured": browserOK, "server_key_configured": serverOK},
		"amap":             map[string]any{"js_key_configured": amapJSOK, "server_key_configured": amapServerOK, "security_js_code_configured": amapSecurityOK},
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
		{"map.amap.security_js_code", body.AMapSecurityJSCode, true},
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
	if body.AMapJSKey != nil && s.mapRegistry != nil {
		if provider, ok := s.mapRegistry.Get(journeymaps.ProviderAMap); ok {
			if amap, ok := provider.(*journeymaps.AMapProvider); ok {
				amap.SetJSKey(strings.TrimSpace(*body.AMapJSKey))
			}
		}
	}
	if body.AMapJSKey != nil {
		s.amapBrowserKey = strings.TrimSpace(*body.AMapJSKey)
	}
	if body.AMapSecurityJSCode != nil {
		value := strings.TrimSpace(*body.AMapSecurityJSCode)
		s.amapSecurityCode = value
		if s.mapRegistry != nil {
			if provider, ok := s.mapRegistry.Get(journeymaps.ProviderAMap); ok {
				if amap, ok := provider.(*journeymaps.AMapProvider); ok {
					amap.SetSecurityJSCode(value)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (s *Server) updateMapPreferences(w http.ResponseWriter, r *http.Request) {
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "settings store is not configured", nil)
		return
	}
	var body mapPreferencesBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	provider := journeymaps.ProviderID(strings.TrimSpace(string(body.DefaultProvider)))
	if !isSupportedMapProvider(provider) {
		writeError(w, http.StatusBadRequest, "invalid_default_map_provider", "default_provider must be amap or baidu", nil)
		return
	}
	if err := s.settingsStore.SetSetting(r.Context(), defaultMapProviderSettingKey, string(provider), false); err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	s.SetDefaultMapProvider(provider)
	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "default_provider": provider})
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
