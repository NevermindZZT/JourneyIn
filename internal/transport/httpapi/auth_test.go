package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"journeyin/internal/security"
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
	server := NewServer(nil, nil, nil, "test", nil)
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

func TestPhotoFileEndpointRequiresAuthenticationWhenEnabled(t *testing.T) {
	auth := NewAuthenticator("admin", "secret", "")
	server := NewServer(nil, nil, nil, "test", nil)
	server.SetAuthenticator(auth)
	protected := RequireAPIAuthWithAuthenticator(server.Handler(), auth)

	anonymousRequest := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-id/file", nil)
	anonymousResponse := httptest.NewRecorder()
	protected.ServeHTTP(anonymousResponse, anonymousRequest)
	if anonymousResponse.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous photo file status=%d, want %d", anonymousResponse.Code, http.StatusUnauthorized)
	}

	session, err := auth.Login("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}
	authenticatedRequest := httptest.NewRequest(http.MethodGet, "/api/v1/photos/photo-id/file", nil)
	authenticatedRequest.AddCookie(&http.Cookie{Name: authSessionCookie, Value: session})
	authenticatedResponse := httptest.NewRecorder()
	protected.ServeHTTP(authenticatedResponse, authenticatedRequest)
	// There is deliberately no configured photo service in this unit test, so a
	// 404 proves the request passed auth and reached the endpoint handler.
	if authenticatedResponse.Code != http.StatusNotFound {
		t.Fatalf("authenticated photo file status=%d, want endpoint 404", authenticatedResponse.Code)
	}
}

func TestPrincipalForBearerMapsLegacyAPITokenToSystemAdmin(t *testing.T) {
	auth := NewAuthenticator("", "", "legacy-api-token")
	principal, err := auth.PrincipalForBearer(context.Background(), "legacy-api-token")
	if err != nil || !principal.IsAdmin() || !principal.System {
		t.Fatalf("legacy API token principal=%+v err=%v", principal, err)
	}
	if _, err := auth.PrincipalForBearer(context.Background(), "wrong-token"); err == nil {
		t.Fatal("accepted an invalid bearer token")
	}
	if security.AccessAllTripsEdit != principal.AccessMode {
		t.Fatalf("legacy API token access mode=%s", principal.AccessMode)
	}
}
func TestSessionLifetimeAndEnvOverride(t *testing.T) {
	// Default should be 30 days
	auth := NewAuthenticator("admin", "secret", "")
	if auth.SessionTTL() != 30*24*time.Hour {
		t.Fatalf("expected default 30 days, got %v", auth.SessionTTL())
	}

	// Environment variable override
	t.Setenv("JOURNEYIN_SESSION_DAYS", "7")
	auth7 := NewAuthenticator("admin", "secret", "")
	if auth7.SessionTTL() != 7*24*time.Hour {
		t.Fatalf("expected 7 days, got %v", auth7.SessionTTL())
	}

	// Invalid value falls back to default 30 days
	t.Setenv("JOURNEYIN_SESSION_DAYS", "invalid")
	authInvalid := NewAuthenticator("admin", "secret", "")
	if authInvalid.SessionTTL() != 30*24*time.Hour {
		t.Fatalf("expected fallback 30 days, got %v", authInvalid.SessionTTL())
	}
}

func TestSlidingWindowSessionRenewal(t *testing.T) {
	auth := NewAuthenticator("admin", "secret", "")
	auth.SetSessionTTL(10 * time.Second) // 10s TTL, threshold is 5s

	session, err := auth.Login("admin", "secret")
	if err != nil {
		t.Fatal(err)
	}

	handler := RequireAPIAuthWithAuthenticator(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), auth)

	// 1. Fresh session (remaining ~10s > 5s threshold): should NOT renew (no Set-Cookie header)
	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/photos/status", nil)
	req1.AddCookie(&http.Cookie{Name: authSessionCookie, Value: session})
	res1 := httptest.NewRecorder()
	handler.ServeHTTP(res1, req1)
	if res1.Code != http.StatusOK {
		t.Fatalf("req1 status=%d", res1.Code)
	}
	if len(res1.Result().Cookies()) != 0 {
		t.Fatalf("expected no Set-Cookie for fresh session, got %+v", res1.Result().Cookies())
	}

	// 2. Age the session in memory so remaining is 3s (< 5s threshold)
	auth.mu.Lock()
	auth.sessions[session] = time.Now().Add(3 * time.Second)
	auth.mu.Unlock()

	// 3. Request with aged session: should trigger renewal and emit Set-Cookie with 10s Max-Age
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/photos/status", nil)
	req2.AddCookie(&http.Cookie{Name: authSessionCookie, Value: session})
	res2 := httptest.NewRecorder()
	handler.ServeHTTP(res2, req2)
	if res2.Code != http.StatusOK {
		t.Fatalf("req2 status=%d", res2.Code)
	}
	cookies := res2.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie on eligible sliding renewal, got none")
	}
	renewedCookie := cookies[0]
	if renewedCookie.Name != authSessionCookie || renewedCookie.Value != session || renewedCookie.MaxAge != 10 {
		t.Fatalf("unexpected renewed cookie: %+v", renewedCookie)
	}

	// 4. Verify session expiry in auth was extended to ~10s
	auth.mu.RLock()
	newExpiry := auth.sessions[session]
	auth.mu.RUnlock()
	if time.Until(newExpiry) < 8*time.Second {
		t.Fatalf("expected session expiry extended to ~10s, got remaining %v", time.Until(newExpiry))
	}
}

