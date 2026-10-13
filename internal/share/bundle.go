package share

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const BundleFormat = "journeyin.share-bundle"
const BundleSchemaVersion = 1

var ErrBundleHashMismatch = errors.New("share bundle hash mismatch")
var ErrBundleCorrupt = errors.New("share bundle snapshot integrity check failed")
var ErrBundleForbidden = errors.New("share bundle permission denied")

type BundleItem struct {
	TripID      string          `json:"trip_id"`
	ContentHash string          `json:"content_hash"`
	Trip        json.RawMessage `json:"trip"`
}

type BundleDocument struct {
	Format        string       `json:"format"`
	SchemaVersion int          `json:"bundle_schema_version"`
	CreatedAt     string       `json:"created_at"`
	SourceOrigin  string       `json:"source_origin,omitempty"`
	Items         []BundleItem `json:"items"`
	BundleHash    string       `json:"bundle_hash"`
}

func NewBundleDocument(items []BundleItem, now time.Time) (BundleDocument, []byte, error) {
	if len(items) == 0 || len(items) > 50 {
		return BundleDocument{}, nil, errors.New("share bundle must contain 1 to 50 trips")
	}
	doc := BundleDocument{Format: BundleFormat, SchemaVersion: BundleSchemaVersion, CreatedAt: now.UTC().Format(time.RFC3339), Items: append([]BundleItem(nil), items...)}
	if err := doc.SetHash(); err != nil {
		return BundleDocument{}, nil, err
	}
	encoded, err := json.Marshal(doc)
	return doc, encoded, err
}

func (d *BundleDocument) SetHash() error {
	if len(d.Items) == 0 || len(d.Items) > 50 {
		return errors.New("share bundle must contain 1 to 50 trips")
	}
	hash := sha256.New()
	seen := make(map[string]bool, len(d.Items))
	for _, item := range d.Items {
		if item.TripID == "" || item.ContentHash == "" || len(item.Trip) == 0 {
			return errors.New("share bundle item is incomplete")
		}
		if BundleContentHash(item.Trip) != item.ContentHash {
			return errors.New("share bundle trip content hash does not match")
		}
		if seen[item.TripID] {
			return errors.New("share bundle contains duplicate trip ids")
		}
		seen[item.TripID] = true
		_, _ = fmt.Fprintf(hash, "%s\n%s\n", item.TripID, item.ContentHash)
	}
	d.BundleHash = hex.EncodeToString(hash.Sum(nil))
	return nil
}

func DecodeBundle(data []byte) (BundleDocument, error) {
	var doc BundleDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return BundleDocument{}, fmt.Errorf("invalid share bundle: %w", err)
	}
	if doc.Format != BundleFormat || doc.SchemaVersion != BundleSchemaVersion {
		return BundleDocument{}, errors.New("unsupported share bundle format or version")
	}
	provided := doc.BundleHash
	doc.BundleHash = ""
	if err := doc.SetHash(); err != nil {
		return BundleDocument{}, err
	}
	if provided == "" || provided != doc.BundleHash {
		return BundleDocument{}, ErrBundleHashMismatch
	}
	doc.BundleHash = provided
	return doc, nil
}

func BundleContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type BundleRecord struct {
	ID          string
	TokenHash   [32]byte
	ContentHash string
	ItemCount   int
	TripTitles  []string
	Snapshot    []byte
	CreatedBy   string
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}

type BundleSummary struct {
	ID         string   `json:"id"`
	Status     string   `json:"status"`
	CreatedAt  string   `json:"created_at"`
	ExpiresAt  string   `json:"expires_at,omitempty"`
	ItemCount  int      `json:"item_count"`
	TripTitles []string `json:"trip_titles"`
}

type BundleStore interface {
	PutBundle(BundleRecord) error
	GetBundleByTokenHash([32]byte) (BundleRecord, error)
	ListBundles(string, bool) ([]BundleRecord, error)
	RevokeBundle(string, string, bool, time.Time) error
}

type BundleService struct {
	store BundleStore
	now   func() time.Time
}

func NewBundleService(store BundleStore) *BundleService {
	return &BundleService{store: store, now: time.Now}
}

func (s *BundleService) Create(snapshot []byte, createdBy string, ttl time.Duration) (string, BundleRecord, error) {
	if len(snapshot) == 0 || len(snapshot) > 16<<20 || createdBy == "" || ttl < 0 {
		return "", BundleRecord{}, errors.New("invalid share bundle")
	}
	bundle, err := DecodeBundle(snapshot)
	if err != nil {
		return "", BundleRecord{}, err
	}
	titles := make([]string, 0, len(bundle.Items))
	for _, item := range bundle.Items {
		var trip struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(item.Trip, &trip); err != nil {
			return "", BundleRecord{}, err
		}
		titles = append(titles, trip.Title)
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", BundleRecord{}, err
	}
	rawID := make([]byte, 16)
	if _, err := rand.Read(rawID); err != nil {
		return "", BundleRecord{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	now := s.now().UTC()
	record := BundleRecord{ID: base64.RawURLEncoding.EncodeToString(rawID), TokenHash: sha256.Sum256([]byte(token)), ContentHash: BundleContentHash(snapshot), ItemCount: len(bundle.Items), TripTitles: titles, Snapshot: append([]byte(nil), snapshot...), CreatedBy: createdBy, CreatedAt: now}
	if ttl > 0 {
		record.ExpiresAt = now.Add(ttl)
	}
	if err := s.store.PutBundle(record); err != nil {
		return "", BundleRecord{}, err
	}
	return token, record, nil
}

func (s *BundleService) Resolve(token string) (BundleRecord, error) {
	if token == "" || len(token) > 128 {
		return BundleRecord{}, ErrNotFound
	}
	record, err := s.store.GetBundleByTokenHash(sha256.Sum256([]byte(token)))
	if err != nil {
		return BundleRecord{}, err
	}
	if record.RevokedAt != nil {
		return BundleRecord{}, ErrRevoked
	}
	if !record.ExpiresAt.IsZero() && !s.now().UTC().Before(record.ExpiresAt) {
		return BundleRecord{}, ErrExpired
	}
	if BundleContentHash(record.Snapshot) != record.ContentHash {
		return BundleRecord{}, ErrBundleCorrupt
	}
	if _, err := DecodeBundle(record.Snapshot); err != nil {
		return BundleRecord{}, ErrBundleCorrupt
	}
	return record, nil
}

func (s *BundleService) List(actorID string, admin bool) ([]BundleSummary, error) {
	if !admin && actorID == "" {
		return nil, ErrBundleForbidden
	}
	records, err := s.store.ListBundles(actorID, admin)
	if err != nil {
		return nil, err
	}
	result := make([]BundleSummary, 0, len(records))
	now := s.now().UTC()
	for _, record := range records {
		if record.RevokedAt != nil {
			continue
		}
		status := "active"
		if !record.ExpiresAt.IsZero() && !now.Before(record.ExpiresAt) {
			status = "expired"
		}
		summary := BundleSummary{ID: record.ID, Status: status, CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339), ItemCount: record.ItemCount, TripTitles: append([]string(nil), record.TripTitles...)}
		if !record.ExpiresAt.IsZero() {
			summary.ExpiresAt = record.ExpiresAt.UTC().Format(time.RFC3339)
		}
		result = append(result, summary)
	}
	return result, nil
}

func (s *BundleService) Revoke(id, actorID string, admin bool) error {
	return s.store.RevokeBundle(id, actorID, admin, s.now().UTC())
}

type MemoryBundleStore struct {
	mu     sync.RWMutex
	byHash map[[32]byte]BundleRecord
}

func NewMemoryBundleStore() *MemoryBundleStore {
	return &MemoryBundleStore{byHash: make(map[[32]byte]BundleRecord)}
}
func (s *MemoryBundleStore) PutBundle(record BundleRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byHash[record.TokenHash] = record
	return nil
}
func (s *MemoryBundleStore) GetBundleByTokenHash(hash [32]byte) (BundleRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.byHash[hash]
	if !ok {
		return BundleRecord{}, ErrNotFound
	}
	return record, nil
}
func (s *MemoryBundleStore) ListBundles(actorID string, admin bool) ([]BundleRecord, error) {
	if !admin && actorID == "" {
		return nil, ErrBundleForbidden
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]BundleRecord, 0)
	for _, record := range s.byHash {
		if !admin && record.CreatedBy != actorID {
			continue
		}
		record.Snapshot = append([]byte(nil), record.Snapshot...)
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result, nil
}

func (s *MemoryBundleStore) RevokeBundle(id, actorID string, admin bool, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, record := range s.byHash {
		if record.ID != id {
			continue
		}
		if !admin && (actorID == "" || actorID != record.CreatedBy) {
			return ErrBundleForbidden
		}
		delete(s.byHash, hash)
		return nil
	}
	return ErrNotFound
}
