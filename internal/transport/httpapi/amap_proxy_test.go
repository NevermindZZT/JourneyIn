package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAMapServiceProxyRequiresSecurityCodeAndRestrictsPaths(t *testing.T) {
	server := NewServer(nil, nil, nil, "test", nil)
	request := httptest.NewRequest(http.MethodGet, "/_AMapService/v3/geocode/geo?key=public-key", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "public-key") {
		t.Fatalf("unconfigured proxy response=%d body=%s", response.Code, response.Body.String())
	}

	server.SetAMapBrowserKey("public-key")
	server.SetAMapSecurityJSCode("secret-code")
	request = httptest.NewRequest(http.MethodGet, "/_AMapService/v3/geocode/geo?key=other-key", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || strings.Contains(response.Body.String(), "secret-code") {
		t.Fatalf("key mismatch response=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/_AMapService/v4/map/styles", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("styles proxy status=%d", response.Code)
	}
	request = httptest.NewRequest(http.MethodDelete, "/_AMapService/v3/geocode/geo", nil)
	response = httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method status=%d", response.Code)
	}
}
