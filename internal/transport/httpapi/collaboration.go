package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"

	"journeyin/internal/collaboration"
	"journeyin/internal/store"
)

type collaborationGrantContextKey struct{}
type collaborationBearerTokenContextKey struct{}

const collaborationSessionCookie = "journeyin_collaboration"

type createCollaborationShareBody struct {
	TTLSeconds *int `json:"ttl_seconds,omitempty"`
}

func collaborationGrantFromRequest(r *http.Request) (collaboration.Grant, bool) {
	grant, ok := r.Context().Value(collaborationGrantContextKey{}).(collaboration.Grant)
	return grant, ok
}

// collaborationBearerTokenFromRequest is used only during the bootstrap exchange.
func collaborationBearerTokenFromRequest(r *http.Request) string {
	value, _ := r.Context().Value(collaborationBearerTokenContextKey{}).(string)
	return value
}

func (s *Server) collaborationCookieSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	publicURL, err := url.Parse(strings.TrimSpace(s.publicURL))
	return err == nil && strings.EqualFold(publicURL.Scheme, "https")
}

func (s *Server) setCollaborationSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{Name: collaborationSessionCookie, Value: token, Path: "/api/v1/collaboration/", HttpOnly: true, Secure: s.collaborationCookieSecure(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) clearCollaborationSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: collaborationSessionCookie, Value: "", Path: "/api/v1/collaboration/", MaxAge: -1, HttpOnly: true, Secure: s.collaborationCookieSecure(r), SameSite: http.SameSiteStrictMode})
}

func collaborationOriginPort(u *url.URL) string {
	port := u.Port()
	if port == "80" && strings.EqualFold(u.Scheme, "http") {
		return ""
	}
	if port == "443" && strings.EqualFold(u.Scheme, "https") {
		return ""
	}
	return port
}

func (s *Server) collaborationOriginMatches(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}
	expected, expectedErr := url.Parse(strings.TrimSpace(s.shareBaseURL(r)))
	supplied, suppliedErr := url.Parse(origin)
	if expectedErr != nil || suppliedErr != nil || expected.Hostname() == "" || supplied.Hostname() == "" {
		return false
	}
	if (supplied.Path != "" && supplied.Path != "/") || supplied.RawQuery != "" || supplied.Fragment != "" || supplied.User != nil {
		return false
	}
	return strings.EqualFold(expected.Scheme, supplied.Scheme) && strings.EqualFold(expected.Hostname(), supplied.Hostname()) && collaborationOriginPort(expected) == collaborationOriginPort(supplied)
}

// withCollaborationAuth is installed only on the explicit collaboration route allowlist.
// It never authorizes owner APIs, even when the same bearer token is presented there.
func (s *Server) withCollaborationAuth(requireTrip bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.collaborationService == nil {
			writeError(w, http.StatusServiceUnavailable, "collaboration_unavailable", "collaboration sharing is not configured", nil)
			return
		}
		token := ""
		bearerToken := false
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = parts[1]
			bearerToken = true
		} else if cookie, cookieErr := r.Cookie(collaborationSessionCookie); cookieErr == nil {
			token = cookie.Value
		}
		if token == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "collaboration_token_required", "a collaboration link token is required", nil)
			return
		}
		grant, err := s.collaborationService.Resolve(token)
		if errors.Is(err, collaboration.ErrExpired) || errors.Is(err, collaboration.ErrRevoked) {
			s.clearCollaborationSessionCookie(w, r)
			writeError(w, http.StatusGone, "collaboration_link_inactive", "this collaboration link has expired or been revoked", nil)
			return
		}
		if errors.Is(err, collaboration.ErrNotFound) {
			s.clearCollaborationSessionCookie(w, r)
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeError(w, http.StatusUnauthorized, "invalid_collaboration_token", "collaboration link is invalid", nil)
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "collaboration_error", "could not validate collaboration link", nil)
			return
		}
		if !bearerToken && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !s.collaborationOriginMatches(r) {
			writeError(w, http.StatusForbidden, "collaboration_origin_rejected", "collaboration write must come from this JourneyIn origin", nil)
			return
		}
		if requireTrip && r.PathValue("id") != grant.TripID {
			writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
			return
		}
		bucket, limit := collaborationRequestBucket(r.URL.Path)
		if !s.collaborationService.AllowRequest(grant.ID+bucket, limit) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "collaboration_rate_limited", "too many requests for this collaboration link; try again shortly", nil)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		ctx := context.WithValue(r.Context(), collaborationGrantContextKey{}, grant)
		if bearerToken {
			ctx = context.WithValue(ctx, collaborationBearerTokenContextKey{}, token)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func collaborationRequestBucket(path string) (string, int) {
	if strings.HasPrefix(path, "/api/v1/collaboration/maps/") || strings.Contains(path, "/weather") || strings.HasSuffix(path, "/plan") || strings.HasSuffix(path, "/routes/refresh") {
		return ":provider", 30
	}
	return ":api", 120
}
func (s *Server) createCollaborationShare(w http.ResponseWriter, r *http.Request) {
	if s.collaborationService == nil {
		writeError(w, http.StatusServiceUnavailable, "collaboration_unavailable", "collaboration sharing is not configured", nil)
		return
	}
	tripID := r.PathValue("id")
	if _, err := s.trips.Get(r.Context(), tripID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	var body createCollaborationShareBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	ttl := collaboration.DefaultTTL
	if body.TTLSeconds != nil {
		seconds := int64(*body.TTLSeconds)
		if seconds <= 0 || seconds > int64(collaboration.MaxTTL/time.Second) {
			writeError(w, http.StatusBadRequest, "invalid_ttl", "collaboration links must expire within 90 days", nil)
			return
		}
		ttl = time.Duration(seconds) * time.Second
	}
	token, grant, err := s.collaborationService.Create(tripID, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "collaboration_error", err.Error(), nil)
		return
	}
	shareURL := strings.TrimRight(s.shareBaseURL(r), "/") + "/c#" + token
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         grant.ID,
		"trip_id":    grant.TripID,
		"permission": "editor",
		"created_at": grant.CreatedAt,
		"expires_at": grant.ExpiresAt,
		"url":        shareURL,
	})
}

func (s *Server) listCollaborationShares(w http.ResponseWriter, r *http.Request) {
	if s.collaborationService == nil {
		writeError(w, http.StatusServiceUnavailable, "collaboration_unavailable", "collaboration sharing is not configured", nil)
		return
	}
	tripID := r.PathValue("id")
	if _, err := s.trips.Get(r.Context(), tripID); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	grants, err := s.collaborationService.List(tripID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "collaboration_error", err.Error(), nil)
		return
	}
	now := time.Now().UTC()
	items := make([]map[string]any, 0, len(grants))
	for _, grant := range grants {
		status := "active"
		if grant.RevokedAt != nil {
			status = "revoked"
		} else if !now.Before(grant.ExpiresAt) {
			status = "expired"
		}
		item := map[string]any{
			"id":         grant.ID,
			"trip_id":    grant.TripID,
			"permission": "editor",
			"status":     status,
			"created_at": grant.CreatedAt,
			"expires_at": grant.ExpiresAt,
		}
		if grant.RevokedAt != nil {
			item["revoked_at"] = grant.RevokedAt
		}
		items = append(items, item)
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) revokeCollaborationShare(w http.ResponseWriter, r *http.Request) {
	if s.collaborationService == nil {
		writeError(w, http.StatusServiceUnavailable, "collaboration_unavailable", "collaboration sharing is not configured", nil)
		return
	}
	tripID := r.PathValue("id")
	if err := s.collaborationService.Revoke(tripID, r.PathValue("shareID")); errors.Is(err, collaboration.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "collaboration link not found", nil)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "collaboration_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) collaborationPage(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(s.web, "index.html")
	if err != nil {
		http.Error(w, "collaboration page unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setSharePageHeaders(w)
	_, _ = w.Write(index)
}

func (s *Server) collaborationCurrent(w http.ResponseWriter, r *http.Request) {
	grant, ok := collaborationGrantFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_collaboration_token", "collaboration link is invalid", nil)
		return
	}
	record, err := s.trips.Get(r.Context(), grant.TripID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	defaultProvider := "baidu"
	if configured, providerErr := s.defaultMapProviderFor(r.Context()); providerErr == nil {
		defaultProvider = string(configured)
	}
	if bearerToken := collaborationBearerTokenFromRequest(r); bearerToken != "" {
		s.setCollaborationSessionCookie(w, r, bearerToken)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"trip":                             json.RawMessage(record.Document),
		"trip_id":                          record.ID,
		"revision":                         record.Revision,
		"browser_key":                      s.browserMapKey,
		"amap_browser_key":                 s.amapBrowserKey,
		"amap_security_proxy_path":         amapProxyPrefix,
		"amap_security_js_code_configured": s.amapSecurityCode != "",
		"default_map_provider":             defaultProvider,
	})
}
