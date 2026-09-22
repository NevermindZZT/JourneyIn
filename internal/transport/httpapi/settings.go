package httpapi

import (
	"context"
	"net/http"
	"os"
	"strings"

	journeymaps "journeyin/internal/maps"
	"journeyin/internal/photos"
	"journeyin/internal/weather"
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
		provider = journeymaps.ProviderAMap
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
	photosRootDir := ""
	if s.settingsStore != nil {
		if val, ok, err := s.settingsStore.GetSetting(r.Context(), "photos.root_dir"); err == nil && ok && strings.TrimSpace(val) != "" {
			photosRootDir = strings.TrimSpace(val)
		}
	}
	if photosRootDir == "" && s.photosService != nil {
		photosRootDir = s.photosService.RootDir()
	}
	defaultWeatherProvider := string(weather.ProviderAuto)
	caiyunTokenConfigured := false
	qweatherKeyConfigured := false
	qweatherHost := "api.qweather.com"
	if s.weatherService != nil {
		defaultWeatherProvider = string(s.weatherService.DefaultProvider())
		if p := s.weatherService.CaiyunProvider(); p != nil {
			caiyunTokenConfigured = p.TokenConfigured()
		}
		if p := s.weatherService.QWeatherProvider(); p != nil {
			qweatherKeyConfigured = p.KeyConfigured()
			qweatherHost = p.Host()
		}
	} else if s.settingsStore != nil {
		if val, ok, err := s.settingsStore.GetSetting(r.Context(), "weather.default_provider"); err == nil && ok && strings.TrimSpace(val) != "" {
			defaultWeatherProvider = strings.TrimSpace(val)
		}
		if _, ok, err := s.settingsStore.GetSetting(r.Context(), "weather.caiyun.token"); err == nil && ok {
			caiyunTokenConfigured = true
		}
		if _, ok, err := s.settingsStore.GetSetting(r.Context(), "weather.qweather.key"); err == nil && ok {
			qweatherKeyConfigured = true
		}
		if val, ok, err := s.settingsStore.GetSetting(r.Context(), "weather.qweather.host"); err == nil && ok && strings.TrimSpace(val) != "" {
			qweatherHost = strings.TrimSpace(val)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"map": map[string]any{
			"default_provider": defaultProvider,
			"baidu":            map[string]any{"browser_key_configured": browserOK, "server_key_configured": serverOK},
			"amap":             map[string]any{"js_key_configured": amapJSOK, "server_key_configured": amapServerOK, "security_js_code_configured": amapSecurityOK},
		},
		"poi": map[string]any{
			"provider_priority":     priority,
			"local_directory_count": directoryCount,
		},
		"photos": map[string]any{
			"root_dir":   photosRootDir,
			"configured": photosRootDir != "",
		},
		"weather": map[string]any{
			"default_provider": defaultWeatherProvider,
			"openmeteo": map[string]any{
				"available": true,
			},
			"qweather": map[string]any{
				"key_configured": qweatherKeyConfigured,
				"host":           qweatherHost,
			},
			"caiyun": map[string]any{
				"token_configured": caiyunTokenConfigured,
			},
		},
		"mcp": map[string]any{
			"http_endpoint":    "/mcp",
			"token_configured": s.mcpToken != "",
			"token":            s.mcpToken,
		},
	})
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
type photosSettingsBody struct {
	RootDir *string `json:"root_dir"`
}

func (s *Server) updatePhotosSettings(w http.ResponseWriter, r *http.Request) {
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "settings store is not configured", nil)
		return
	}
	var body photosSettingsBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	var newDir string
	if body.RootDir != nil {
		newDir = strings.TrimSpace(*body.RootDir)
	}
	var err error
	if newDir == "" {
		err = s.settingsStore.DeleteSetting(r.Context(), "photos.root_dir")
		fallbackDir := os.Getenv("JOURNEYIN_PHOTOS_DIR")
		if s.photosService != nil {
			s.photosService.SetRootDir(fallbackDir)
			if fallbackDir != "" {
				s.photosService.TriggerScan()
			}
		}
	} else {
		err = s.settingsStore.SetSetting(r.Context(), "photos.root_dir", newDir, false)
		if s.photosService == nil && s.settingsStore != nil && s.settingsStore.DB() != nil {
			s.photosService = photos.NewService(s.settingsStore.DB(), newDir, "", s.logger)
		}
		if s.photosService != nil {
			s.photosService.SetRootDir(newDir)
			s.photosService.TriggerScan()
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"saved":    true,
		"root_dir": newDir,
		"enabled":  s.photosService != nil && s.photosService.IsEnabled(),
	})
}

type weatherSettingsBody struct {
	DefaultProvider *string `json:"default_provider"`
	CaiyunToken     *string `json:"caiyun_token"`
	QWeatherKey     *string `json:"qweather_key"`
	QWeatherHost    *string `json:"qweather_host"`
}

func (s *Server) updateWeatherSettings(w http.ResponseWriter, r *http.Request) {
	if s.settingsStore == nil {
		writeError(w, http.StatusServiceUnavailable, "settings_unavailable", "settings store is not configured", nil)
		return
	}
	var body weatherSettingsBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if body.DefaultProvider != nil {
		val := strings.TrimSpace(*body.DefaultProvider)
		providerID := weather.ParseProviderID(val)
		if err := s.settingsStore.SetSetting(r.Context(), "weather.default_provider", string(providerID), false); err != nil {
			writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
			return
		}
		if s.weatherService != nil {
			s.weatherService.SetDefaultProvider(providerID)
		}
	}
	if body.CaiyunToken != nil {
		token := strings.TrimSpace(*body.CaiyunToken)
		var err error
		if token == "" {
			err = s.settingsStore.DeleteSetting(r.Context(), "weather.caiyun.token")
		} else {
			err = s.settingsStore.SetSetting(r.Context(), "weather.caiyun.token", token, true)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
			return
		}
		if s.weatherService != nil && s.weatherService.CaiyunProvider() != nil {
			s.weatherService.CaiyunProvider().SetToken(token)
		}
	}
	if body.QWeatherKey != nil {
		key := strings.TrimSpace(*body.QWeatherKey)
		var err error
		if key == "" {
			err = s.settingsStore.DeleteSetting(r.Context(), "weather.qweather.key")
		} else {
			err = s.settingsStore.SetSetting(r.Context(), "weather.qweather.key", key, true)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
			return
		}
		if s.weatherService != nil && s.weatherService.QWeatherProvider() != nil {
			s.weatherService.QWeatherProvider().SetKey(key)
		}
	}
	if body.QWeatherHost != nil {
		host := strings.TrimSpace(*body.QWeatherHost)
		var err error
		if host == "" {
			err = s.settingsStore.DeleteSetting(r.Context(), "weather.qweather.host")
		} else {
			err = s.settingsStore.SetSetting(r.Context(), "weather.qweather.host", host, false)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "settings_error", err.Error(), nil)
			return
		}
		if s.weatherService != nil && s.weatherService.QWeatherProvider() != nil {
			s.weatherService.QWeatherProvider().SetHost(host)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

