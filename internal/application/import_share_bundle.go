package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"journeyin/internal/domain"
	"journeyin/internal/security"
	"journeyin/internal/store"
)

const maxShareImportItems = 50
const maxShareImportBytes = 16 << 20

var ErrShareImportPreviewExpired = errors.New("share import preview not found or expired")
var ErrShareImportConfirmation = errors.New("invalid share import confirmation")

type ShareImportInputItem struct {
	SourceTripID string
	ContentHash  string
	Document     []byte
}

type ShareImportPreviewItem struct {
	SourceTripID    string                   `json:"source_trip_id"`
	ContentHash     string                   `json:"content_hash"`
	Title           string                   `json:"title"`
	Status          string                   `json:"status"`
	StartDate       string                   `json:"start_date"`
	EndDate         string                   `json:"end_date"`
	Days            int                      `json:"days"`
	Stops           int                      `json:"stops"`
	AlreadyImported bool                     `json:"already_imported"`
	Warnings        []domain.ValidationIssue `json:"warnings,omitempty"`
}

type ShareImportPreviewResult struct {
	PreviewID            string                   `json:"preview_id,omitempty"`
	ExpiresAt            string                   `json:"expires_at,omitempty"`
	RequiresConfirmation bool                     `json:"requires_confirmation"`
	ConfirmationToken    string                   `json:"confirmation_token,omitempty"`
	Items                []ShareImportPreviewItem `json:"items"`
}

type ShareImportCommitResult struct {
	Items  []store.ImportedTripResult `json:"items"`
	Status string                     `json:"status"`
}

type shareImportPreview struct {
	ID               string
	ConfirmationHash string
	PayloadHash      string
	PrincipalID      string
	SourceOrigin     string
	Items            []store.ImportedShareTrip
	ExpiresAt        time.Time
}

func (s *TripService) PreviewShareImport(ctx context.Context, sourceOrigin string, inputs []ShareImportInputItem) (ShareImportPreviewResult, error) {
	if err := s.requireCreateTrip(ctx); err != nil {
		return ShareImportPreviewResult{}, err
	}
	origin, err := normalizeShareSourceOrigin(sourceOrigin)
	if err != nil {
		return ShareImportPreviewResult{}, err
	}
	if len(inputs) == 0 || len(inputs) > maxShareImportItems {
		return ShareImportPreviewResult{}, errors.New("select between 1 and 50 shared trips")
	}
	principalID := "system"
	if principal, ok := security.PrincipalFromContext(ctx); ok && principal.UserID != "" {
		principalID = principal.UserID
	}
	items := make([]store.ImportedShareTrip, 0, len(inputs))
	previews := make([]ShareImportPreviewItem, 0, len(inputs))
	seen := make(map[string]bool, len(inputs))
	totalBytes := 0
	hashInput := strings.Builder{}
	hashInput.WriteString(origin)
	for _, input := range inputs {
		totalBytes += len(input.Document)
		if totalBytes > maxShareImportBytes {
			return ShareImportPreviewResult{}, errors.New("share import bundle exceeds 16 MiB")
		}
		if input.SourceTripID == "" || len(input.SourceTripID) > 128 || seen[input.SourceTripID] {
			return ShareImportPreviewResult{}, errors.New("share import contains an empty or duplicate trip id")
		}
		seen[input.SourceTripID] = true
		if len(input.Document) == 0 || len(input.Document) > 4<<20 {
			return ShareImportPreviewResult{}, errors.New("shared trip document is empty or exceeds 4 MiB")
		}
		_, sourceTrip, sourceIssues, err := domain.NormalizeTrip(input.Document)
		if err != nil {
			return ShareImportPreviewResult{}, err
		}
		if hasErrors(sourceIssues) {
			return ShareImportPreviewResult{}, ValidationError{Issues: sourceIssues}
		}
		canonicalSource, err := json.Marshal(sourceTrip)
		if err != nil {
			return ShareImportPreviewResult{}, err
		}
		canonicalHash := domain.ContentHash(canonicalSource)
		if sourceTrip.ID != input.SourceTripID || (input.ContentHash != "" && canonicalHash != input.ContentHash) {
			return ShareImportPreviewResult{}, errors.New("shared trip identity or content hash does not match")
		}
		contentHash := canonicalHash
		exists, err := s.store.HasImportedShareSource(ctx, origin, input.SourceTripID, contentHash)
		if err != nil {
			return ShareImportPreviewResult{}, err
		}
		itemPreview := ShareImportPreviewItem{SourceTripID: input.SourceTripID, ContentHash: contentHash, Title: sourceTrip.Title, Status: sourceTrip.Status, StartDate: sourceTrip.DateRange.Start, EndDate: sourceTrip.DateRange.End, Days: len(sourceTrip.Days), AlreadyImported: exists, Warnings: warnings(sourceIssues)}
		for _, day := range sourceTrip.Days {
			itemPreview.Stops += len(day.Stops)
		}
		previews = append(previews, itemPreview)
		_, _ = fmt.Fprintf(&hashInput, "\n%s:%s", input.SourceTripID, contentHash)
		if exists {
			continue
		}
		copyDocument, err := remapImportedTripIDs(sourceTrip)
		if err != nil {
			return ShareImportPreviewResult{}, err
		}
		items = append(items, store.ImportedShareTrip{Document: copyDocument, SourceOrigin: origin, SourceTripID: input.SourceTripID, SourceContentHash: contentHash})
	}
	result := ShareImportPreviewResult{Items: previews, RequiresConfirmation: len(items) > 0}
	if len(items) == 0 {
		return result, nil
	}
	previewID, err := opaqueID("shareimport")
	if err != nil {
		return ShareImportPreviewResult{}, err
	}
	token, err := opaqueID("confirm")
	if err != nil {
		return ShareImportPreviewResult{}, err
	}
	requestHash := sha256.Sum256([]byte(hashInput.String()))
	p := shareImportPreview{ID: previewID, ConfirmationHash: domain.ContentHash([]byte(token)), PayloadHash: hex.EncodeToString(requestHash[:]), PrincipalID: principalID, SourceOrigin: origin, Items: items, ExpiresAt: time.Now().UTC().Add(15 * time.Minute)}
	s.mu.Lock()
	s.shareImportPreviews[previewID] = p
	s.mu.Unlock()
	result.PreviewID = previewID
	result.ExpiresAt = p.ExpiresAt.Format(time.RFC3339)
	result.ConfirmationToken = token
	return result, nil
}

func (s *TripService) CommitShareImport(ctx context.Context, previewID, confirmationToken, idempotencyKey string) (ShareImportCommitResult, error) {
	if strings.TrimSpace(idempotencyKey) == "" {
		return ShareImportCommitResult{}, errors.New("idempotency key is required")
	}
	s.mu.Lock()
	p, ok := s.shareImportPreviews[previewID]
	if ok && time.Now().UTC().After(p.ExpiresAt) {
		delete(s.shareImportPreviews, previewID)
		ok = false
	}
	s.mu.Unlock()
	if !ok {
		return ShareImportCommitResult{}, ErrShareImportPreviewExpired
	}
	if domain.ContentHash([]byte(confirmationToken)) != p.ConfirmationHash {
		return ShareImportCommitResult{}, ErrShareImportConfirmation
	}
	principalID := "system"
	if principal, ok := security.PrincipalFromContext(ctx); ok && principal.UserID != "" {
		principalID = principal.UserID
	}
	if p.PrincipalID != principalID {
		return ShareImportCommitResult{}, security.ErrForbidden
	}
	if err := s.requireCreateTrip(ctx); err != nil {
		return ShareImportCommitResult{}, err
	}
	result, replay, err := s.store.CreateShareImportedTrips(ctx, p.Items, "rest:share-import", "share-import:user:"+principalID, idempotencyKey, p.PayloadHash)
	if err != nil {
		return ShareImportCommitResult{}, err
	}
	status := "created"
	if replay {
		status = "already_applied"
	}
	return ShareImportCommitResult{Items: result, Status: status}, nil
}

func normalizeShareSourceOrigin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 512 {
		return "", errors.New("source_origin is too long")
	}
	if raw == "local-file://import" {
		return raw, nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("source_origin must be an http/https origin without credentials or path")
	}
	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host), nil
}

func remapImportedTripIDs(trip domain.Trip) ([]byte, error) {
	var err error
	if trip.ID, err = domain.NewID("trip"); err != nil {
		return nil, err
	}
	for linkIndex := range trip.Links {
		if trip.Links[linkIndex].ID, err = domain.NewID("link"); err != nil {
			return nil, err
		}
	}
	stopIDs := make(map[string]string)
	for dayIndex := range trip.Days {
		day := &trip.Days[dayIndex]
		if day.ID, err = domain.NewID("day"); err != nil {
			return nil, err
		}
		for stopIndex := range day.Stops {
			stop := &day.Stops[stopIndex]
			oldID := stop.ID
			if stop.ID, err = domain.NewID("stop"); err != nil {
				return nil, err
			}
			stopIDs[oldID] = stop.ID
			for childIndex := range stop.Children {
				child := &stop.Children[childIndex]
				oldChildID := child.ID
				if child.ID, err = domain.NewID("stop"); err != nil {
					return nil, err
				}
				stopIDs[oldChildID] = child.ID
				for linkIndex := range child.Links {
					if child.Links[linkIndex].ID, err = domain.NewID("link"); err != nil {
						return nil, err
					}
				}
			}
			for linkIndex := range stop.Links {
				if stop.Links[linkIndex].ID, err = domain.NewID("link"); err != nil {
					return nil, err
				}
			}
		}
	}
	for dayIndex := range trip.Days {
		day := &trip.Days[dayIndex]
		validLegs := make([]domain.RouteLeg, 0, len(day.Legs))
		for legIndex := range day.Legs {
			leg := day.Legs[legIndex]
			fromID, fromOK := stopIDs[leg.FromStopID]
			toID, toOK := stopIDs[leg.ToStopID]
			if !fromOK || !toOK {
				continue
			}
			if leg.ID, err = domain.NewID("leg"); err != nil {
				return nil, err
			}
			leg.FromStopID, leg.ToStopID = fromID, toID
			validLegs = append(validLegs, leg)
		}
		day.Legs = validLegs
	}
	return json.Marshal(trip)
}
