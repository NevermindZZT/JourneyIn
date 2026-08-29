package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNavigationURLGenerationRemainsPublicWithAuth(t *testing.T) {
	auth := NewAuthenticator("admin", "secret", "")
	called := false
	handler := RequireAPIAuthWithAuthenticator(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}), auth)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/maps/navigation", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !called || response.Code != http.StatusNoContent {
		t.Fatalf("navigation endpoint was not public: called=%v status=%d", called, response.Code)
	}
}
