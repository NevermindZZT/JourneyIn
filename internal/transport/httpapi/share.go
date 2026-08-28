package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
)

type createShareBody struct {
	TripID     string `json:"trip_id"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

func (s *Server) createShare(w http.ResponseWriter, r *http.Request) {
	if s.shareService == nil {
		writeError(w, http.StatusServiceUnavailable, "sharing_unavailable", "share service is not configured", nil)
		return
	}
	var body createShareBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	record, err := s.trips.Get(r.Context(), body.TripID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "trip not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
		return
	}
	ttl := 7 * 24 * time.Hour
	if body.TTLSeconds > 0 {
		ttl = time.Duration(body.TTLSeconds) * time.Second
	}
	if ttl > 365*24*time.Hour {
		writeError(w, http.StatusBadRequest, "ttl_too_large", "share TTL cannot exceed 365 days", nil)
		return
	}
	token, shareRecord, err := s.shareService.Create(record.ID, record.Revision, record.ContentHash, record.Document, ttl)
	if err != nil {
		writeError(w, http.StatusBadRequest, "share_error", err.Error(), nil)
		return
	}
	shareURL := "/s/" + token
	if s.publicURL != "" {
		shareURL = s.publicURL + shareURL
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": shareRecord.ID, "trip_id": record.ID, "revision": record.Revision, "expires_at": shareRecord.ExpiresAt, "url": shareURL})
}

func (s *Server) revokeShare(w http.ResponseWriter, r *http.Request) {
	if s.shareService == nil {
		writeError(w, http.StatusServiceUnavailable, "sharing_unavailable", "share service is not configured", nil)
		return
	}
	if err := s.shareService.Revoke(r.PathValue("id")); err != nil {
		if errors.Is(err, journeyshare.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "share not found", nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "share_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) publicShare(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if strings.HasSuffix(token, ".json") {
		s.publicShareJSONToken(w, strings.TrimSuffix(token, ".json"))
		return
	}
	record, ok := s.resolveShare(w, token)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	setShareHeaders(w)
	var document map[string]any
	_ = json.Unmarshal(record.Content, &document)
	title, _ := document["title"].(string)
	if title == "" {
		title = "JourneyIn 旅行规划"
	}
	escaped := html.EscapeString(string(record.Content))
	page := fmt.Sprintf("<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\"><title>%s</title><style>body{margin:0;background:#f5f7f4;color:#152b2d;font-family:system-ui,sans-serif}main{max-width:1000px;margin:0 auto;padding:32px 20px}pre{overflow:auto;padding:20px;border-radius:16px;background:#fff;border:1px solid #d5e1df;white-space:pre-wrap}</style></head><body><main><p>JOURNEYIN · 只读分享</p><h1>%s</h1><p>此页面是不可编辑的行程快照，revision %d。</p><pre>%s</pre></main></body></html>", html.EscapeString(title), html.EscapeString(title), record.Revision, escaped)
	_, _ = w.Write([]byte(page))
}

func (s *Server) publicShareJSON(w http.ResponseWriter, r *http.Request) {
	s.publicShareJSONToken(w, strings.TrimSuffix(r.PathValue("token"), ".json"))
}

func (s *Server) publicShareJSONToken(w http.ResponseWriter, token string) {
	record, ok := s.resolveShare(w, token)
	if !ok {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=journeyin-share.json")
	setShareHeaders(w)
	_, _ = w.Write(record.Content)
}

func (s *Server) resolveShare(w http.ResponseWriter, token string) (journeyshare.Record, bool) {
	if s.shareService == nil {
		writeError(w, http.StatusNotFound, "not_found", "share not found", nil)
		return journeyshare.Record{}, false
	}
	record, err := s.shareService.Resolve(token)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "share not found", nil)
		return journeyshare.Record{}, false
	}
	return record, true
}

func setShareHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'")
}
