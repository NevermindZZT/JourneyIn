package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"journeyin/internal/domain"
	"journeyin/internal/store"
)

type TripService struct {
	store      *store.Store
	mapService *MapService
	mu         sync.Mutex
	previews   map[string]preview
	planMu     sync.Mutex
	planLocks  map[string]chan struct{}
}

type preview struct {
	ID               string
	ConfirmationHash string
	PayloadHash      string
	Document         []byte
	Operation        string
	TargetTripID     string
	ExpectedRevision int
	ExpiresAt        time.Time
	CreatedBy        string
}

type PreviewResult struct {
	PreviewID            string                   `json:"preview_id"`
	ExpiresAt            string                   `json:"expires_at"`
	RequiresConfirmation bool                     `json:"requires_confirmation"`
	ConfirmationToken    string                   `json:"confirmation_token,omitempty"`
	Summary              map[string]any           `json:"summary"`
	Warnings             []domain.ValidationIssue `json:"warnings,omitempty"`
}

type CommitResult struct {
	TripID   string `json:"trip_id"`
	Revision int    `json:"revision"`
	Status   string `json:"status"`
	ViewURL  string `json:"view_url"`
}

func NewTripService(s *store.Store) *TripService {
	return &TripService{store: s, previews: make(map[string]preview), planLocks: make(map[string]chan struct{})}
}
func (s *TripService) SetMapService(service *MapService) { s.mapService = service }

func (s *TripService) acquirePlan(ctx context.Context, tripID string) (func(), error) {
	s.planMu.Lock()
	lock := s.planLocks[tripID]
	if lock == nil {
		lock = make(chan struct{}, 1)
		s.planLocks[tripID] = lock
	}
	s.planMu.Unlock()
	select {
	case lock <- struct{}{}:
		return func() { <-lock }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *TripService) Validate(document []byte) ([]byte, domain.Trip, []domain.ValidationIssue, error) {
	return domain.NormalizeTrip(document)
}
func (s *TripService) Create(ctx context.Context, document []byte, source string) (store.TripRecord, error) {
	return s.store.CreateTrip(ctx, document, source)
}

// Import creates a new Trip copy and intentionally drops the exported root ID.
// This makes export -> import round-trips safe even when the original Trip remains stored.
func (s *TripService) Import(ctx context.Context, document []byte, source string) (store.TripRecord, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(document, &raw); err != nil {
		return store.TripRecord{}, err
	}
	if raw == nil {
		return store.TripRecord{}, errors.New("trip JSON must be an object")
	}
	delete(raw, "id")
	withoutID, err := json.Marshal(raw)
	if err != nil {
		return store.TripRecord{}, err
	}
	return s.store.CreateTrip(ctx, withoutID, source)
}
func (s *TripService) Get(ctx context.Context, id string) (store.TripRecord, error) {
	return s.store.GetTrip(ctx, id)
}
func (s *TripService) List(ctx context.Context, limit int) ([]store.TripRecord, error) {
	return s.store.ListTrips(ctx, limit)
}
func (s *TripService) Replace(ctx context.Context, id string, expectedRevision int, document []byte, source string) (store.TripRecord, error) {
	return s.store.ReplaceTrip(ctx, id, expectedRevision, document, source)
}
func (s *TripService) Delete(ctx context.Context, id string, expectedRevision int) error {
	return s.store.DeleteTrip(ctx, id, expectedRevision)
}

func (s *TripService) PreviewSave(ctx context.Context, document []byte, operation, targetID string, expectedRevision int, createdBy string) (PreviewResult, error) {
	var normalized []byte
	var trip domain.Trip
	var issues []domain.ValidationIssue
	var err error
	if operation == "replace" {
		normalized, trip, issues, err = domain.NormalizeTripForID(document, targetID)
	} else {
		normalized, trip, issues, err = s.Validate(document)
	}
	if err != nil {
		return PreviewResult{}, err
	}
	if hasErrors(issues) {
		return PreviewResult{}, ValidationError{Issues: issues}
	}
	if operation != "create" && operation != "replace" {
		return PreviewResult{}, errors.New("operation must be create or replace")
	}
	if operation == "replace" && targetID == "" {
		return PreviewResult{}, errors.New("target_trip_id is required for replace")
	}
	if operation == "replace" && expectedRevision < 1 {
		return PreviewResult{}, errors.New("expected_revision is required for replace")
	}
	previewID, err := opaqueID("preview")
	if err != nil {
		return PreviewResult{}, err
	}
	token, err := opaqueID("confirm")
	if err != nil {
		return PreviewResult{}, err
	}
	now := time.Now().UTC()
	p := preview{ID: previewID, ConfirmationHash: domain.ContentHash([]byte(token)), PayloadHash: domain.ContentHash(normalized), Document: normalized, Operation: operation, TargetTripID: targetID, ExpectedRevision: expectedRevision, ExpiresAt: now.Add(15 * time.Minute), CreatedBy: createdBy}
	s.mu.Lock()
	s.previews[previewID] = p
	s.mu.Unlock()
	return PreviewResult{PreviewID: previewID, ExpiresAt: p.ExpiresAt.Format(time.RFC3339), RequiresConfirmation: true, ConfirmationToken: token, Summary: trip.Summary(), Warnings: warnings(issues)}, nil
}

func (s *TripService) CommitSave(ctx context.Context, previewID, confirmationToken, idempotencyKey string, expectedRevision int, source string) (CommitResult, error) {
	if idempotencyKey == "" {
		return CommitResult{}, errors.New("idempotency key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.previews[previewID]
	if ok && time.Now().UTC().After(p.ExpiresAt) {
		delete(s.previews, previewID)
		ok = false
	}
	if !ok {
		return CommitResult{}, errors.New("preview not found or expired")
	}
	if domain.ContentHash([]byte(confirmationToken)) != p.ConfirmationHash {
		return CommitResult{}, errors.New("invalid confirmation token")
	}
	if expectedRevision != 0 && p.Operation == "replace" && expectedRevision != p.ExpectedRevision {
		return CommitResult{}, errors.New("expected revision does not match preview")
	}
	requestHash := domain.ContentHash(append(p.Document, []byte("|"+p.Operation+"|"+p.TargetTripID)...))
	scope := "trip:" + p.TargetTripID
	if p.Operation == "create" {
		scope = "trip:create"
	}
	if replay, ok, err := s.store.Idempotency(ctx, scope, idempotencyKey, requestHash); err != nil {
		return CommitResult{}, err
	} else if ok {
		var result CommitResult
		if err := json.Unmarshal(replay, &result); err != nil {
			return CommitResult{}, err
		}
		result.Status = "already_applied"
		return result, nil
	}
	var record store.TripRecord
	var err error
	if p.Operation == "create" {
		record, err = s.store.CreateTrip(ctx, p.Document, source)
	} else {
		record, err = s.store.ReplaceTrip(ctx, p.TargetTripID, p.ExpectedRevision, p.Document, source)
	}
	if err != nil {
		return CommitResult{}, err
	}
	result := CommitResult{TripID: record.ID, Revision: record.Revision, Status: "created", ViewURL: "/trips/" + record.ID}
	if p.Operation == "replace" {
		result.Status = "updated"
	}
	encoded, _ := json.Marshal(result)
	if err := s.store.SaveIdempotency(ctx, scope, idempotencyKey, requestHash, encoded); err != nil {
		return CommitResult{}, err
	}
	return result, nil
}

type ValidationError struct{ Issues []domain.ValidationIssue }

func (e ValidationError) Error() string { return "trip validation failed" }
func hasErrors(issues []domain.ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}
func warnings(issues []domain.ValidationIssue) []domain.ValidationIssue {
	var result []domain.ValidationIssue
	for _, issue := range issues {
		if issue.Level == "warning" {
			result = append(result, issue)
		}
	}
	return result
}
func opaqueID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(raw), nil
}
