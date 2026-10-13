package mcptransport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"journeyin/internal/security"
)

func TestRequireBearerWithResolverAttachesScopedPrincipal(t *testing.T) {
	userPrincipal := security.Principal{UserID: "usr_1", Username: "editor", Role: security.RoleUser, AccessMode: security.AccessSelectedTripEdit}
	resolver := func(_ context.Context, raw string) (security.Principal, error) {
		if raw != "user-token" {
			return security.Principal{}, errors.New("invalid token")
		}
		return userPrincipal, nil
	}
	var got security.Principal
	handler := RequireBearerWithResolver(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		got, ok = security.PrincipalFromContext(r.Context())
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}), "legacy-admin-token", resolver)

	request := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer user-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("user token status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if got.UserID != userPrincipal.UserID || got.AccessMode != security.AccessSelectedTripEdit {
		t.Fatalf("wrong user principal: %+v", got)
	}

	request = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer legacy-admin-token")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("legacy admin token status=%d", recorder.Code)
	}
	if !got.IsAdmin() || !got.System {
		t.Fatalf("legacy token should be system admin: %+v", got)
	}

	request = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token status=%d", recorder.Code)
	}
}

func TestRequireOptionalBearerAllowsAnonymousButScopesExplicitUserTokens(t *testing.T) {
	userPrincipal := security.Principal{UserID: "usr_optional", Username: "viewer", Role: security.RoleUser, AccessMode: security.AccessReadOnly}
	resolver := func(_ context.Context, raw string) (security.Principal, error) {
		if raw != "viewer-token" {
			return security.Principal{}, errors.New("invalid token")
		}
		return userPrincipal, nil
	}
	var got security.Principal
	handler := RequireOptionalBearerWithResolver(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = security.PrincipalFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}), "", resolver)

	request := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || !got.IsAdmin() || !got.System {
		t.Fatalf("anonymous local principal=%+v status=%d", got, recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer viewer-token")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent || got.UserID != userPrincipal.UserID || got.AccessMode != security.AccessReadOnly {
		t.Fatalf("scoped token principal=%+v status=%d", got, recorder.Code)
	}

	request = httptest.NewRequest(http.MethodGet, "/mcp", nil)
	request.Header.Set("Authorization", "Bearer invalid")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("invalid explicit bearer should not fall back to anonymous local mode: status=%d", recorder.Code)
	}
}
