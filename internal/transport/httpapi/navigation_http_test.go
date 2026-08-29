package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNavigationEndpointReturnsNativeAndHTTPSLinks(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	body, err := json.Marshal(map[string]any{
		"provider": "amap",
		"target": map[string]any{
			"name":     "西湖",
			"location": map[string]any{"lat": 30.25, "lng": 120.15, "crs": "bd09ll"},
		},
		"mode":     "walking",
		"platform": "android",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL+"/api/v1/maps/navigation", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("navigation status=%d", response.StatusCode)
	}
	var result struct {
		URL         string `json:"url"`
		FallbackURL string `json:"fallback_url"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	primary, err := url.Parse(result.URL)
	if err != nil {
		t.Fatal(err)
	}
	fallback, err := url.Parse(result.FallbackURL)
	if err != nil {
		t.Fatal(err)
	}
	if primary.Scheme != "amapuri" || fallback.Scheme != "https" || fallback.Host != "uri.amap.com" {
		t.Fatalf("unexpected navigation links: primary=%q fallback=%q", result.URL, result.FallbackURL)
	}
	if primary.Query().Get("dlat") == "30.25000000" || primary.Query().Get("dlon") == "120.15000000" {
		t.Fatalf("native AMap link did not convert BD09LL coordinates: %q", result.URL)
	}
}

func TestNavigationEndpointIsReachableWithoutSession(t *testing.T) {
	server := testHTTPServer(t)
	defer server.Close()
	authenticated := RequireAPIAuthWithAuthenticator(server.Config.Handler, NewAuthenticator("admin", "secret", ""))
	body, err := json.Marshal(map[string]any{
		"provider": "amap",
		"target": map[string]any{
			"name":     "西湖",
			"location": map[string]any{"lat": 30.25, "lng": 120.15, "crs": "bd09ll"},
		},
		"mode":     "walking",
		"platform": "web",
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/maps/navigation", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	authenticated.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("navigation request without session status=%d body=%s", response.Code, strings.TrimSpace(response.Body.String()))
	}
}
