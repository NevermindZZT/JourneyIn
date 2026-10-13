package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"journeyin/internal/application"
	"journeyin/internal/security"
)

const (
	authSessionCookie = "journeyin_session"
	authSessionTTL    = 24 * time.Hour
)

var ErrAuthNotConfigured = errors.New("username/password authentication is not configured")
var ErrInvalidCredentials = errors.New("invalid username or password")

type Authenticator struct {
	username string
	password string
	apiToken string

	mu              sync.RWMutex
	sessions        map[string]time.Time
	userService     *application.UserService
	userModeEnabled bool
}

func NewAuthenticator(username, password, apiToken string) *Authenticator {
	return &Authenticator{
		username: strings.TrimSpace(username),
		password: password,
		apiToken: strings.TrimSpace(apiToken),
		sessions: make(map[string]time.Time),
	}
}

func (a *Authenticator) SetUserService(users *application.UserService) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.userService = users
	a.mu.Unlock()
	a.RefreshUserModeEnabled()
}

func (a *Authenticator) RefreshUserModeEnabled() {
	if a == nil {
		return
	}
	a.mu.RLock()
	users := a.userService
	a.mu.RUnlock()
	if users == nil {
		return
	}
	enabled, err := users.HasUsers(context.Background())
	if err != nil {
		return
	}
	a.mu.Lock()
	a.userModeEnabled = enabled
	a.mu.Unlock()
}

func (a *Authenticator) userAuth() (*application.UserService, bool) {
	if a == nil {
		return nil, false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.userService, a.userModeEnabled
}

func (a *Authenticator) Enabled() bool {
	return a != nil && (a.apiToken != "" || a.UsernamePasswordEnabled())
}

func (a *Authenticator) UsernamePasswordEnabled() bool {
	return a != nil && a.username != "" && strings.TrimSpace(a.password) != ""
}

func (a *Authenticator) Login(username, password string) (string, error) {
	users, userMode := a.userAuth()
	if userMode && users != nil {
		if !a.Enabled() {
			return "", ErrAuthNotConfigured
		}
		principal, err := users.Authenticate(context.Background(), username, password)
		if err != nil {
			return "", ErrInvalidCredentials
		}
		token, _, err := users.CreateSession(context.Background(), principal.UserID)
		return token, err
	}
	if !a.UsernamePasswordEnabled() {
		return "", ErrAuthNotConfigured
	}
	usernameMatch := subtle.ConstantTimeCompare([]byte(strings.TrimSpace(username)), []byte(a.username))
	passwordMatch := subtle.ConstantTimeCompare([]byte(password), []byte(a.password))
	if usernameMatch != 1 || passwordMatch != 1 {
		return "", ErrInvalidCredentials
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	a.mu.Lock()
	a.sessions[token] = time.Now().Add(authSessionTTL)
	a.mu.Unlock()
	return token, nil
}

func systemPrincipal() security.Principal {
	return security.Principal{UserID: "system", Username: "System", Role: security.RoleAdmin, AccessMode: security.AccessAllTripsEdit, System: true}
}

func (a *Authenticator) PrincipalForRequest(r *http.Request) (security.Principal, bool) {
	if a == nil {
		return systemPrincipal(), true
	}
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(authorization, "Bearer ") {
		raw := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if raw == "" {
			return security.Principal{}, false
		}
		if a.apiToken != "" && subtle.ConstantTimeCompare([]byte(raw), []byte(a.apiToken)) == 1 {
			return systemPrincipal(), true
		}
		users, userMode := a.userAuth()
		if userMode && users != nil {
			if principal, err := users.PrincipalForAPIToken(r.Context(), raw); err == nil {
				return principal, true
			}
		}
		return security.Principal{}, false
	}
	if !a.Enabled() {
		return systemPrincipal(), true
	}
	users, userMode := a.userAuth()
	cookie, err := r.Cookie(authSessionCookie)
	if err != nil || cookie.Value == "" {
		return security.Principal{}, false
	}
	if userMode && users != nil {
		principal, err := users.PrincipalForSession(r.Context(), cookie.Value)
		return principal, err == nil
	}
	a.mu.RLock()
	expiresAt, ok := a.sessions[cookie.Value]
	username := a.username
	a.mu.RUnlock()
	if !ok {
		return security.Principal{}, false
	}
	if time.Now().After(expiresAt) {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
		return security.Principal{}, false
	}
	return security.Principal{UserID: "legacy-admin", Username: username, Role: security.RoleAdmin, AccessMode: security.AccessAllTripsEdit}, true
}

func (a *Authenticator) PrincipalForBearer(ctx context.Context, raw string) (security.Principal, error) {
	if a == nil || raw == "" {
		return security.Principal{}, ErrInvalidCredentials
	}
	if a.apiToken != "" && subtle.ConstantTimeCompare([]byte(raw), []byte(a.apiToken)) == 1 {
		return systemPrincipal(), nil
	}
	users, userMode := a.userAuth()
	if userMode && users != nil {
		return users.PrincipalForAPIToken(ctx, raw)
	}
	return security.Principal{}, ErrInvalidCredentials
}

func (a *Authenticator) Authorized(r *http.Request) bool {
	_, ok := a.PrincipalForRequest(r)
	return ok
}

func (a *Authenticator) SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(authSessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *Authenticator) ClearSession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(authSessionCookie); err == nil && cookie.Value != "" {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
		users, userMode := a.userAuth()
		if userMode && users != nil {
			_ = users.RevokeSession(r.Context(), cookie.Value)
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:     authSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func RequireAPIAuth(next http.Handler, token string) http.Handler {
	return RequireAPIAuthWithAuthenticator(next, NewAuthenticator("", "", token))
}

func RequireAPIAuthWithAuthenticator(next http.Handler, authenticator *Authenticator) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := security.PrincipalFromContext(r.Context()); ok {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/maps/navigation" || strings.HasPrefix(r.URL.Path, "/api/v1/collaboration/") {
			next.ServeHTTP(w, r)
			return
		}
		principal, ok := authenticator.PrincipalForRequest(r)
		if !ok {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(security.WithPrincipal(r.Context(), principal))
		next.ServeHTTP(w, r)
	})
}
