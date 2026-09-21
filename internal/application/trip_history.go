package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"journeyin/internal/domain"
	"journeyin/internal/store"
)

const savedTripVersionLabelMax = 120

var ErrSavedTripVersionLabelTooLong = errors.New("history label must be at most 120 characters")

type SaveTripVersionResult struct {
	Version      store.SavedTripVersion `json:"version"`
	AlreadySaved bool                   `json:"already_saved"`
}

type RestoreTripVersionResult struct {
	Record    store.TripRecord `json:"record"`
	HistoryID string           `json:"history_id"`
}

func normalizeSavedTripVersionLabel(label string) (string, error) {
	label = strings.TrimSpace(label)
	if utf8.RuneCountInString(label) > savedTripVersionLabelMax {
		return "", ErrSavedTripVersionLabelTooLong
	}
	return label, nil
}

func (s *TripService) SaveTripVersion(ctx context.Context, tripID string, expectedRevision int, label string) (store.SavedTripVersion, bool, error) {
	label, err := normalizeSavedTripVersionLabel(label)
	if err != nil {
		return store.SavedTripVersion{}, false, err
	}
	return s.store.CreateSavedTripVersion(ctx, tripID, expectedRevision, label)
}

func (s *TripService) SaveTripVersionIdempotent(ctx context.Context, tripID string, expectedRevision int, label, idempotencyKey string) (store.SavedTripVersion, bool, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return store.SavedTripVersion{}, false, false, ErrIdempotencyKeyRequired
	}
	label, err := normalizeSavedTripVersionLabel(label)
	if err != nil {
		return store.SavedTripVersion{}, false, false, err
	}
	requestData, err := json.Marshal(struct {
		TripID           string
		ExpectedRevision int
		Label            string
	}{TripID: tripID, ExpectedRevision: expectedRevision, Label: label})
	if err != nil {
		return store.SavedTripVersion{}, false, false, err
	}
	requestHash := domain.ContentHash(requestData)
	scope := "trip:" + tripID + ":history"

	s.mu.Lock()
	defer s.mu.Unlock()
	if replay, ok, err := s.store.Idempotency(ctx, scope, idempotencyKey, requestHash); err != nil {
		return store.SavedTripVersion{}, false, false, err
	} else if ok {
		var payload SaveTripVersionResult
		if err := json.Unmarshal(replay, &payload); err != nil {
			return store.SavedTripVersion{}, false, false, err
		}
		return payload.Version, payload.AlreadySaved, true, nil
	}

	version, alreadySaved, err := s.SaveTripVersion(ctx, tripID, expectedRevision, label)
	if err != nil {
		return store.SavedTripVersion{}, false, false, err
	}
	payload, err := json.Marshal(SaveTripVersionResult{Version: version, AlreadySaved: alreadySaved})
	if err != nil {
		return store.SavedTripVersion{}, false, false, err
	}
	if err := s.store.SaveIdempotency(ctx, scope, idempotencyKey, requestHash, payload); err != nil {
		return store.SavedTripVersion{}, false, false, err
	}
	return version, alreadySaved, false, nil
}

func (s *TripService) ListTripVersions(ctx context.Context, tripID string, limit int) ([]store.SavedTripVersion, error) {
	return s.store.ListSavedTripVersions(ctx, tripID, limit)
}

func (s *TripService) GetTripVersion(ctx context.Context, tripID, versionID string) (store.SavedTripVersion, error) {
	return s.store.GetSavedTripVersion(ctx, tripID, versionID)
}

// RestoreTripVersionIdempotent replaces the current working Trip with one of
// its immutable, user-saved history snapshots. The target Trip ID is retained
// and the normal optimistic revision check protects newer edits.
func (s *TripService) RestoreTripVersionIdempotent(ctx context.Context, tripID, versionID string, expectedRevision int, idempotencyKey string) (store.TripRecord, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return store.TripRecord{}, false, ErrIdempotencyKeyRequired
	}
	requestData, err := json.Marshal(struct {
		TripID           string
		VersionID        string
		ExpectedRevision int
	}{TripID: tripID, VersionID: versionID, ExpectedRevision: expectedRevision})
	if err != nil {
		return store.TripRecord{}, false, err
	}
	requestHash := domain.ContentHash(requestData)
	scope := "trip:" + tripID + ":history-restore"

	s.mu.Lock()
	defer s.mu.Unlock()
	if replay, ok, err := s.store.Idempotency(ctx, scope, idempotencyKey, requestHash); err != nil {
		return store.TripRecord{}, false, err
	} else if ok {
		var payload RestoreTripVersionResult
		if err := json.Unmarshal(replay, &payload); err != nil {
			return store.TripRecord{}, false, err
		}
		return payload.Record, true, nil
	}

	version, err := s.GetTripVersion(ctx, tripID, versionID)
	if err != nil {
		return store.TripRecord{}, false, err
	}
	record, err := s.Replace(ctx, tripID, expectedRevision, version.Document, "history:restore")
	if err != nil {
		return store.TripRecord{}, false, err
	}
	payload, err := json.Marshal(RestoreTripVersionResult{Record: record, HistoryID: version.ID})
	if err != nil {
		return store.TripRecord{}, false, err
	}
	if err := s.store.SaveIdempotency(ctx, scope, idempotencyKey, requestHash, payload); err != nil {
		return store.TripRecord{}, false, err
	}
	return record, false, nil
}

func (s *TripService) DeleteTripVersion(ctx context.Context, tripID, versionID string) error {
	return s.store.DeleteSavedTripVersion(ctx, tripID, versionID)
}

func (s *TripService) DeleteTripVersionIdempotent(ctx context.Context, tripID, versionID, idempotencyKey string) (bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return false, ErrIdempotencyKeyRequired
	}
	requestData, err := json.Marshal(struct {
		TripID    string
		VersionID string
	}{TripID: tripID, VersionID: versionID})
	if err != nil {
		return false, err
	}
	requestHash := domain.ContentHash(requestData)
	scope := "trip:" + tripID + ":history-delete"

	s.mu.Lock()
	defer s.mu.Unlock()
	if replay, ok, err := s.store.Idempotency(ctx, scope, idempotencyKey, requestHash); err != nil {
		return false, err
	} else if ok {
		var payload struct {
			Deleted bool `json:"deleted"`
		}
		if err := json.Unmarshal(replay, &payload); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := s.DeleteTripVersion(ctx, tripID, versionID); err != nil {
		return false, err
	}
	payload, err := json.Marshal(map[string]bool{"deleted": true})
	if err != nil {
		return false, err
	}
	if err := s.store.SaveIdempotency(ctx, scope, idempotencyKey, requestHash, payload); err != nil {
		return false, err
	}
	return false, nil
}
