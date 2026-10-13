package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"journeyin/internal/application"
	"journeyin/internal/domain"
	"journeyin/internal/security"
	journeyshare "journeyin/internal/share"
	"journeyin/internal/store"
)

const maxBundleItems = 50
const maxBundleBytes = 16 << 20

type shareBundleCreateBody struct {
	TripIDs    []string `json:"trip_ids"`
	TTLSeconds *int     `json:"ttl_seconds,omitempty"`
	Permanent  bool     `json:"permanent,omitempty"`
}

type shareImportPreviewBody struct {
	SourceOrigin string `json:"source_origin"`
	Items        []struct {
		TripID      string          `json:"trip_id"`
		ContentHash string          `json:"content_hash"`
		Trip        json.RawMessage `json:"trip"`
	} `json:"items"`
}

type shareImportCommitBody struct {
	PreviewID         string `json:"preview_id"`
	ConfirmationToken string `json:"confirmation_token"`
	IdempotencyKey    string `json:"idempotency_key"`
}

func (s *Server) shareActor(w http.ResponseWriter, r *http.Request) (security.Principal, bool) {
	principal, ok := security.PrincipalFromContext(r.Context())
	if !ok {
		principal = security.SystemPrincipal()
	}
	if !principal.IsAdmin() && principal.AccessMode == security.AccessReadOnly {
		writeError(w, http.StatusForbidden, "forbidden", "trip edit permission is required to create share bundles", nil)
		return security.Principal{}, false
	}
	return principal, true
}

func (s *Server) listShareBundles(w http.ResponseWriter, r *http.Request) {
	if s.bundleService == nil {
		writeError(w, http.StatusServiceUnavailable, "sharing_unavailable", "share bundle service is not configured", nil)
		return
	}
	actor, ok := security.PrincipalFromContext(r.Context())
	if !ok {
		actor = security.SystemPrincipal()
	}
	actorID := actor.UserID
	if actorID == "" {
		actorID = "system"
	}
	items, err := s.bundleService.List(actorID, actor.IsAdmin())
	if errors.Is(err, journeyshare.ErrBundleForbidden) {
		writeError(w, http.StatusForbidden, "forbidden", "share bundle access is denied", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share_error", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createShareBundle(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.shareActor(w, r)
	if !ok {
		return
	}
	if s.bundleService == nil {
		writeError(w, http.StatusServiceUnavailable, "sharing_unavailable", "share bundle service is not configured", nil)
		return
	}
	data, err := readBody(r, 64<<10)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
		return
	}
	var body shareBundleCreateBody
	if err := json.Unmarshal(data, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if len(body.TripIDs) == 0 || len(body.TripIDs) > maxBundleItems {
		writeError(w, http.StatusBadRequest, "invalid_bundle", "select between 1 and 50 trips", nil)
		return
	}
	ttl := 30 * 24 * time.Hour
	if body.Permanent {
		ttl = 0
	} else if body.TTLSeconds != nil {
		if *body.TTLSeconds < 0 || *body.TTLSeconds > int((3650*24*time.Hour)/time.Second) {
			writeError(w, http.StatusBadRequest, "invalid_ttl", "share TTL must be between 1 second and 10 years", nil)
			return
		}
		if *body.TTLSeconds > 0 {
			ttl = time.Duration(*body.TTLSeconds) * time.Second
		}
	}
	seen := make(map[string]bool, len(body.TripIDs))
	items := make([]journeyshare.BundleItem, 0, len(body.TripIDs))
	totalBytes := 0
	for _, tripID := range body.TripIDs {
		tripID = strings.TrimSpace(tripID)
		if tripID == "" || seen[tripID] {
			writeError(w, http.StatusBadRequest, "invalid_bundle", "trip ids must be non-empty and unique", nil)
			return
		}
		seen[tripID] = true
		if !s.canEditTrip(r.Context(), tripID) {
			writeError(w, http.StatusForbidden, "forbidden", "you may only share trips you can edit", map[string]any{"trip_id": tripID})
			return
		}
		record, err := s.trips.Get(r.Context(), tripID)
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "trip not found", map[string]any{"trip_id": tripID})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "store_error", err.Error(), nil)
			return
		}
		_, trip, issues, err := domain.NormalizeTrip(record.Document)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "share_error", "stored trip cannot be serialized for sharing", nil)
			return
		}
		if hasValidationErrors(issues) {
			writeError(w, http.StatusConflict, "invalid_trip", "stored trip does not pass current validation", map[string]any{"trip_id": tripID, "issues": issues})
			return
		}
		encodedTrip, err := json.Marshal(trip)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "share_error", "could not serialize trip snapshot", nil)
			return
		}
		canonical, err := sanitizeShareTrip(encodedTrip)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "share_error", "could not sanitize trip snapshot", nil)
			return
		}
		totalBytes += len(canonical)
		if len(canonical) > 4<<20 || totalBytes > maxBundleBytes {
			writeError(w, http.StatusRequestEntityTooLarge, "bundle_too_large", "share bundle exceeds the 16 MiB size limit", nil)
			return
		}
		items = append(items, journeyshare.BundleItem{TripID: trip.ID, ContentHash: domain.ContentHash(canonical), Trip: canonical})
	}
	bundle, _, err := journeyshare.NewBundleDocument(items, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_bundle", err.Error(), nil)
		return
	}
	shareBase := strings.TrimRight(strings.TrimSpace(s.publicURL), "/")
	baseURL, parseErr := url.Parse(shareBase)
	if parseErr != nil || (baseURL.Scheme != "http" && baseURL.Scheme != "https") || baseURL.Hostname() == "" || baseURL.User != nil || (baseURL.Path != "" && baseURL.Path != "/") || baseURL.RawQuery != "" || baseURL.Fragment != "" || baseURL.Hostname() == "0.0.0.0" || baseURL.Hostname() == "localhost" {
		shareBase = ""
	}
	if shareBase != "" {
		bundle.SourceOrigin = strings.ToLower(baseURL.Scheme) + "://" + strings.ToLower(baseURL.Host)
	}
	snapshot, err := json.Marshal(bundle)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share_error", "could not serialize share bundle", nil)
		return
	}
	actorID := actor.UserID
	if actorID == "" {
		actorID = "system"
	}
	token, record, err := s.bundleService.Create(snapshot, actorID, ttl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share_error", err.Error(), nil)
		return
	}
	shareURL := "/s/" + token
	if shareBase != "" {
		shareURL = shareBase + shareURL
	}
	var expiresAt any
	if !record.ExpiresAt.IsZero() {
		expiresAt = record.ExpiresAt
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": record.ID, "url": shareURL, "expires_at": expiresAt, "permanent": record.ExpiresAt.IsZero(), "item_count": len(items)})
}

func (s *Server) revokeShareBundle(w http.ResponseWriter, r *http.Request) {
	if s.bundleService == nil {
		writeError(w, http.StatusServiceUnavailable, "sharing_unavailable", "share bundle service is not configured", nil)
		return
	}
	actor, ok := security.PrincipalFromContext(r.Context())
	if !ok {
		actor = security.SystemPrincipal()
	}
	actorID := actor.UserID
	if actorID == "" {
		actorID = "system"
	}
	err := s.bundleService.Revoke(r.PathValue("id"), actorID, actor.IsAdmin())
	if errors.Is(err, journeyshare.ErrBundleForbidden) {
		writeError(w, http.StatusForbidden, "forbidden", "only the creator or an administrator can revoke this bundle", nil)
		return
	}
	if errors.Is(err, journeyshare.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "share bundle not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "share_error", err.Error(), nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) previewShareImport(w http.ResponseWriter, r *http.Request) {
	data, err := readBody(r, maxBundleBytes+(1<<20))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
		return
	}
	var body shareImportPreviewBody
	if err := json.Unmarshal(data, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	if len(body.Items) == 0 || len(body.Items) > maxBundleItems {
		writeError(w, http.StatusBadRequest, "invalid_bundle", "select between 1 and 50 shared trips", nil)
		return
	}
	items := make([]application.ShareImportInputItem, 0, len(body.Items))
	for _, item := range body.Items {
		items = append(items, application.ShareImportInputItem{SourceTripID: item.TripID, ContentHash: item.ContentHash, Document: item.Trip})
	}
	result, err := s.trips.PreviewShareImport(r.Context(), body.SourceOrigin, items)
	if err != nil {
		writeShareImportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) commitShareImport(w http.ResponseWriter, r *http.Request) {
	data, err := readBody(r, 64<<10)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", err.Error(), nil)
		return
	}
	var body shareImportCommitBody
	if err := json.Unmarshal(data, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error(), nil)
		return
	}
	result, err := s.trips.CommitShareImport(r.Context(), body.PreviewID, body.ConfirmationToken, body.IdempotencyKey)
	if err != nil {
		writeShareImportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeShareImportError(w http.ResponseWriter, err error) {
	if writePermissionError(w, err) {
		return
	}
	if errors.Is(err, application.ErrShareImportPreviewExpired) {
		writeError(w, http.StatusGone, "preview_expired", err.Error(), nil)
		return
	}
	if errors.Is(err, application.ErrShareImportConfirmation) {
		writeError(w, http.StatusBadRequest, "invalid_confirmation", err.Error(), nil)
		return
	}
	if errors.Is(err, store.ErrIdempotencyConflict) || errors.Is(err, store.ErrImportedShareAlreadyExists) {
		writeError(w, http.StatusConflict, "import_conflict", err.Error(), nil)
		return
	}
	var validation application.ValidationError
	if errors.As(err, &validation) {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error(), map[string]any{"issues": validation.Issues})
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_share_bundle", err.Error(), nil)
}

func setShareJSONCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

type bundlePageItem struct {
	Title     string
	StartDate string
	EndDate   string
	Days      int
	Stops     int
}
type bundlePageData struct {
	Items       []bundlePageItem
	DownloadURL string
}

var shareBundlePageTemplate = template.Must(template.New("share-bundle").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="robots" content="noindex,nofollow,noarchive"><meta name="referrer" content="no-referrer"><title>JourneyIn 多行程分享</title><style>body{margin:0;background:#f4f6f5;color:#17221f;font:16px/1.6 system-ui,sans-serif}.card{max-width:760px;margin:8vh auto;padding:32px;background:white;border-radius:20px;box-shadow:0 12px 36px #17221f16}h1{margin:.25em 0}ol{padding-left:1.5em}li{padding:12px 0;border-bottom:1px solid #e6ebe8}.meta{color:#63716b;font-size:14px}.button{display:inline-block;margin-top:20px;padding:12px 18px;border-radius:12px;background:#166b59;color:white;text-decoration:none;font-weight:650}.hint{color:#63716b;font-size:14px}</style></head><body><main class="card"><p>JOURNEYIN · 只读分享包</p><h1>多行程分享</h1><p>此链接包含 {{len .Items}} 条行程快照。导入后会在目标实例创建独立副本。</p><ol>{{range .Items}}<li><strong>{{.Title}}</strong><div class="meta">{{.StartDate}} — {{.EndDate}} · {{.Days}} 天 · {{.Stops}} 个规划点</div></li>{{end}}</ol><a class="button" href="{{.DownloadURL}}">下载分享包 JSON</a><p class="hint">分享内容固定于创建时的快照；撤销链接不会删除已经下载或导入的副本。</p></main></body></html>`))

func (s *Server) renderShareBundlePage(record journeyshare.BundleRecord, token string) ([]byte, error) {
	bundle, err := journeyshare.DecodeBundle(record.Snapshot)
	if err != nil {
		return nil, err
	}
	data := bundlePageData{DownloadURL: "/s/" + token + ".json"}
	for _, item := range bundle.Items {
		var trip domain.Trip
		if err := json.Unmarshal(item.Trip, &trip); err != nil {
			return nil, err
		}
		data.Items = append(data.Items, bundlePageItem{Title: trip.Title, StartDate: trip.DateRange.Start, EndDate: trip.DateRange.End, Days: len(trip.Days), Stops: countTripStops(trip)})
	}
	var output bytes.Buffer
	if err := shareBundlePageTemplate.Execute(&output, data); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func countTripStops(trip domain.Trip) int {
	count := 0
	for _, day := range trip.Days {
		count += len(day.Stops)
	}
	return count
}

func sanitizeShareTrip(document []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(document, &value); err != nil {
		return nil, err
	}
	stripShareCredentials(value)
	sanitized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var trip domain.Trip
	if err := json.Unmarshal(sanitized, &trip); err != nil {
		return nil, err
	}
	return json.Marshal(trip)
}

func stripShareCredentials(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, nested := range current {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			if normalized == "key" || strings.Contains(normalized, "apikey") || strings.Contains(normalized, "token") || strings.Contains(normalized, "password") || strings.Contains(normalized, "passwd") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "credential") || strings.Contains(normalized, "cookie") || strings.Contains(normalized, "authorization") || strings.HasSuffix(normalized, "_key") || strings.HasSuffix(normalized, "_ak") || normalized == "ak" {
				delete(current, key)
				continue
			}
			stripShareCredentials(nested)
		}
	case []any:
		for _, nested := range current {
			stripShareCredentials(nested)
		}
	}
}
