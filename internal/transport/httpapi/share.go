package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
)

const sharePageContentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://api.map.baidu.com http://api.map.baidu.com https://*.baidu.com http://*.baidu.com https://*.bdimg.com http://*.bdimg.com https://*.bdstatic.com http://*.bdstatic.com https://webapi.amap.com https://*.amap.com https://*.autonavi.com; " +
	"style-src 'self' 'unsafe-inline' data: blob: https://*.baidu.com http://*.baidu.com https://*.bdimg.com http://*.bdimg.com https://*.bdstatic.com http://*.bdstatic.com https://*.amap.com https://*.autonavi.com; " +
	"img-src 'self' data: blob: https://*.baidu.com http://*.baidu.com https://*.bdimg.com http://*.bdimg.com https://*.bdstatic.com http://*.bdstatic.com https://*.amap.com https://*.autonavi.com; " +
	"connect-src 'self' https://*.baidu.com http://*.baidu.com https://*.bdimg.com http://*.bdimg.com https://*.bdstatic.com http://*.bdstatic.com https://*.amap.com https://*.autonavi.com; " +
	"font-src 'self' data: https://*.baidu.com http://*.baidu.com https://*.bdimg.com http://*.bdimg.com https://*.bdstatic.com http://*.bdstatic.com https://*.amap.com https://*.autonavi.com; " +
	"worker-src 'self' blob: https://*.baidu.com http://*.baidu.com https://*.bdimg.com http://*.bdimg.com https://*.bdstatic.com http://*.bdstatic.com https://*.amap.com https://*.autonavi.com; " +
	"child-src 'self' blob: https://*.baidu.com http://*.baidu.com https://*.bdimg.com http://*.bdimg.com https://*.bdstatic.com http://*.bdstatic.com https://*.amap.com https://*.autonavi.com; " +
	"frame-src 'self' https://*.baidu.com http://*.baidu.com https://*.bdimg.com http://*.bdimg.com https://*.bdstatic.com http://*.bdstatic.com https://*.amap.com https://*.autonavi.com; " +
	"base-uri 'self'; object-src 'none'; form-action 'none'; frame-ancestors 'none'"

type createShareBody struct {
	TripID        string `json:"trip_id"`
	TTLSeconds    int    `json:"ttl_seconds,omitempty"`
	ExistingToken string `json:"existing_token,omitempty"`
}

func (s *Server) createShare(w http.ResponseWriter, r *http.Request) {
	if s.shareService == nil {
		writeError(w, http.StatusServiceUnavailable, "sharing_unavailable", "share service is not configured", nil)
		return
	}
	var body createShareBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	record, err := s.trips.Get(r.Context(), body.TripID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	ttl := 7 * 24 * time.Hour
	if body.TTLSeconds > 0 {
		ttl = time.Duration(body.TTLSeconds) * time.Second
	}
	if ttl > 365*24*time.Hour {
		writeError(w, http.StatusBadRequest, "ttl_too_large", "share TTL cannot exceed 365 days", nil)
		return
	}
	if existingToken := strings.TrimSpace(body.ExistingToken); existingToken != "" {
		if existing, resolveErr := s.shareService.Resolve(existingToken); resolveErr == nil && existing.TripID == record.ID {
			shareURL := "/s/" + existingToken
			if baseURL := s.shareBaseURL(r); baseURL != "" {
				shareURL = baseURL + shareURL
			}
			writeJSON(w, http.StatusOK, map[string]any{"id": existing.ID, "trip_id": record.ID, "revision": record.Revision, "expires_at": existing.ExpiresAt, "url": shareURL, "reused": true})
			return
		}
	}
	token, shareRecord, err := s.shareService.Create(record.ID, record.Revision, record.ContentHash, record.Document, ttl)
	if err != nil {
		writeError(w, http.StatusBadRequest, "share_error", err.Error(), nil)
		return
	}
	shareURL := "/s/" + token
	if baseURL := s.shareBaseURL(r); baseURL != "" {
		shareURL = baseURL + shareURL
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": shareRecord.ID, "trip_id": record.ID, "revision": record.Revision, "expires_at": shareRecord.ExpiresAt, "url": shareURL})
}

func (s *Server) shareBaseURL(r *http.Request) string {
	base := strings.TrimRight(strings.TrimSpace(s.publicURL), "/")
	parsed, err := url.Parse(base)
	if base == "" || err != nil || parsed.Hostname() == "0.0.0.0" || (parsed.Hostname() == "localhost" && (parsed.Port() == "" || parsed.Port() == "8080")) {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		return scheme + "://" + r.Host
	}
	return base
}

func (s *Server) revokeShare(w http.ResponseWriter, r *http.Request) {
	if s.shareService == nil {
		writeError(w, http.StatusServiceUnavailable, "sharing_unavailable", "share service is not configured", nil)
		return
	}
	if err := s.shareService.Revoke(r.PathValue("id")); err != nil {
		if errors.Is(err, journeyshare.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "share not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "share_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if strings.HasSuffix(token, ".json") {
		s.publicShareJSONToken(r.Context(), w, strings.TrimSuffix(token, ".json"))
		return
	}
	record, ok := s.resolveShare(r.Context(), w, token)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setSharePageHeaders(w)
	defaultProvider := "baidu"
	if configured, defaultErr := s.defaultMapProviderFor(r.Context()); defaultErr == nil {
		defaultProvider = string(configured)
	}
	_, _ = w.Write(s.renderShareShell(record, s.browserMapKey, s.amapBrowserKey, amapProxyPrefix, defaultProvider))
}

func (s *Server) renderShareShell(record journeyshare.Record, browserKey, amapBrowserKey, amapSecurityProxyPath, defaultMapProvider string) []byte {
	index, err := fs.ReadFile(s.web, "index.html")
	if err != nil {
		return []byte("<!doctype html><html><body>分享页面资源不可用</body></html>")
	}
	data, _ := json.Marshal(map[string]any{"trip": json.RawMessage(record.Content), "browser_key": browserKey, "amap_browser_key": amapBrowserKey, "amap_security_proxy_path": amapSecurityProxyPath, "amap_security_js_code_configured": s.amapSecurityCode != "", "default_map_provider": defaultMapProvider, "revision": record.Revision})
	bootstrap := append([]byte("<script>window.__JOURNEYIN_SHARE__="), data...)
	bootstrap = append(bootstrap, []byte(";</script>")...)
	return bytes.Replace(index, []byte("</head>"), append(bootstrap, []byte("</head>")...), 1)
}

func (s *Server) publicShareJSON(w http.ResponseWriter, r *http.Request) {
	s.publicShareJSONToken(r.Context(), w, strings.TrimSuffix(r.PathValue("token"), ".json"))
}

func (s *Server) publicShareJSONToken(ctx context.Context, w http.ResponseWriter, token string) {
	record, ok := s.resolveShare(ctx, w, token)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=journeyin-share.json")
	setShareHeaders(w)
	_, _ = w.Write(record.Content)
}

func (s *Server) resolveShare(ctx context.Context, w http.ResponseWriter, token string) (journeyshare.Record, bool) {
	if s.shareService == nil {
		writeError(w, http.StatusNotFound, "not_found", "share not found", nil)
		return journeyshare.Record{}, false
	}
	record, err := s.shareService.Resolve(token)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "share not found", nil)
		return journeyshare.Record{}, false
	}
	return record, true
}

func setShareHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'")
}

func setSharePageHeaders(w http.ResponseWriter) {
	setShareHeaders(w)
	// Baidu browser AK domain validation needs an origin/referrer on cross-origin JSAPI and tile requests.
	w.Header().Set("Referrer-Policy", "origin")
	// The HTML shell needs a map-compatible CSP. In particular, BMap may create a base URL,
	// a worker, or a nested browsing context while it resolves tile resources.
	w.Header().Set("Content-Security-Policy", sharePageContentSecurityPolicy)
}
