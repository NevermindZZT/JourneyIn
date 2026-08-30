package httpapi

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	amapProxyPrefix  = "/_AMapService"
	amapProxyMaxBody = 4 << 20
)

// amapServiceProxy implements the official JS API serviceHost shape without
// proxying map tiles or arbitrary hosts. The browser key remains public; the
// JS security code is appended only on this server-side hop.
func (s *Server) amapServiceProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "AMap service proxy only supports GET and POST", nil)
		return
	}
	securityCode := strings.TrimSpace(s.amapSecurityCode)
	if securityCode == "" {
		writeError(w, http.StatusServiceUnavailable, "amap_security_not_configured", "AMap JS security code is not configured", nil)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, amapProxyPrefix)
	if path == "" || strings.Contains(path, "..") || (!strings.HasPrefix(path, "/v3/") && !strings.HasPrefix(path, "/v4/") && !strings.HasPrefix(path, "/v5/")) || strings.HasPrefix(path, "/v4/map/styles") {
		writeError(w, http.StatusNotFound, "amap_service_not_allowed", "AMap service path is not allowed", nil)
		return
	}

	query := r.URL.Query()
	if browserKey := strings.TrimSpace(s.amapBrowserKey); browserKey != "" && query.Get("key") != "" && query.Get("key") != browserKey {
		writeError(w, http.StatusForbidden, "amap_key_mismatch", "AMap service key does not match this application", nil)
		return
	}
	query.Del("jscode")
	query.Set("jscode", securityCode)
	target := &url.URL{Scheme: "https", Host: "restapi.amap.com", Path: path, RawQuery: query.Encode()}
	var body io.Reader
	if r.Method == http.MethodPost {
		data, err := io.ReadAll(io.LimitReader(r.Body, amapProxyMaxBody+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "amap_proxy_body_error", err.Error(), nil)
			return
		}
		if len(data) > amapProxyMaxBody {
			writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "AMap service request body is too large", nil)
			return
		}
		body = strings.NewReader(string(data))
	}
	request, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), body)
	if err != nil {
		writeError(w, http.StatusBadGateway, "amap_proxy_request_error", "could not create AMap proxy request", nil)
		return
	}
	if value := r.Header.Get("Accept"); value != "" {
		request.Header.Set("Accept", value)
	}
	if value := r.Header.Get("Content-Type"); value != "" {
		request.Header.Set("Content-Type", value)
	}
	client := &http.Client{Timeout: 15 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}
		writeError(w, http.StatusBadGateway, "amap_proxy_unavailable", "AMap service proxy is unavailable", nil)
		return
	}
	defer response.Body.Close()
	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if cacheControl := response.Header.Get("Cache-Control"); cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, io.LimitReader(response.Body, amapProxyMaxBody))
}
