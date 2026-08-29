package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
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

	mu       sync.RWMutex
	sessions map[string]time.Time
}

func NewAuthenticator(username, password, apiToken string) *Authenticator {
	return &Authenticator{
		username: strings.TrimSpace(username),
		password: password,
		apiToken: strings.TrimSpace(apiToken),
		sessions: make(map[string]time.Time),
	}
}

func (a *Authenticator) Enabled() bool {
	return a != nil && (a.apiToken != "" || a.UsernamePasswordEnabled())
}

func (a *Authenticator) UsernamePasswordEnabled() bool {
	return a != nil && a.username != "" && a.password != ""
}

func (a *Authenticator) Login(username, password string) (string, error) {
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

func (a *Authenticator) Authorized(r *http.Request) bool {
	if a == nil || !a.Enabled() {
		return true
	}
	if a.apiToken != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+a.apiToken)) == 1 {
		return true
	}
	cookie, err := r.Cookie(authSessionCookie)
	if err != nil || cookie.Value == "" {
		return false
	}
	a.mu.RLock()
	expiresAt, ok := a.sessions[cookie.Value]
	a.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(expiresAt) {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
		return false
	}
	return true
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
	if authenticator == nil || !authenticator.Enabled() {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Navigation URL generation is stateless and carries no trip or credential data,
		// so shared read-only pages can use it without an owner session.
		if !strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api/v1/health" || r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/maps/navigation" {
			next.ServeHTTP(w, r)
			return
		}
		if !authenticator.Authorized(r) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
