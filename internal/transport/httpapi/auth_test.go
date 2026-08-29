package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthenticatorLoginAndSession(t *testing.T) {
	auth := NewAuthenticator("admin", "correct horse", "")
	if !auth.Enabled() || !auth.UsernamePasswordEnabled() {
		t.Fatal("expected username/password authentication to be enabled")
	}
	if _, err := auth.Login("admin", "wrong"); err != ErrInvalidCredentials {
		t.Fatalf("expected invalid credentials, got %v", err)
	}
	session, err := auth.Login("admin", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/trips", nil)
	unauthorized := httptest.NewRecorder()
	protected := RequireAPIAuthWithAuthenticator(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), auth)
	protected.ServeHTTP(unauthorized, request)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status=%d", unauthorized.Code)
	}
	request.AddCookie(&http.Cookie{Name: authSessionCookie, Value: session})
	authorized := httptest.NewRecorder()
	protected.ServeHTTP(authorized, request)
	if authorized.Code != http.StatusNoContent {
		t.Fatalf("authorized status=%d", authorized.Code)
	}
}

func TestLoginEndpointSetsHttpOnlySessionCookie(t *testing.T) {
	auth := NewAuthenticator("admin", "secret", "")
	server := NewServer(nil, nil, nil, "0.2.3", nil)
	server.SetAuthenticator(auth)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("{\"username\":\"admin\",\"password\":\"secret\"}"))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.login(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	cookie := response.Result().Cookies()[0]
	if cookie.Name != authSessionCookie || !cookie.HttpOnly || cookie.MaxAge <= 0 {
		t.Fatalf("unexpected session cookie: %+v", cookie)
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["authenticated"] != true || payload["username"] != "admin" {
		t.Fatalf("unexpected login payload: %+v", payload)
	}
}
