package collaboration

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sort"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("collaboration link not found")
	ErrExpired  = errors.New("collaboration link expired")
	ErrRevoked  = errors.New("collaboration link revoked")
)

const (
	DefaultTTL = 7 * 24 * time.Hour
	MaxTTL     = 90 * 24 * time.Hour
)

type Grant struct {
	ID        string
	TripID    string
	TokenHash [32]byte
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

type Store interface {
	Put(Grant) error
	Get([32]byte) (Grant, error)
	List(string) ([]Grant, error)
	Revoke(string, string, time.Time) error
}

type requestWindow struct {
	started time.Time
	count   int
}

type Service struct {
	store  Store
	now    func() time.Time
	rateMu sync.Mutex
	rates  map[string]requestWindow
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now, rates: make(map[string]requestWindow)}
}

func (s *Service) AllowRequest(grantID string, limit int) bool {
	if grantID == "" || limit < 1 {
		return false
	}
	now := s.now().UTC()
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	if s.rates == nil {
		s.rates = make(map[string]requestWindow)
	}
	for id, window := range s.rates {
		if now.Sub(window.started) > time.Minute {
			delete(s.rates, id)
		}
	}
	window := s.rates[grantID]
	if window.started.IsZero() || now.Sub(window.started) >= time.Minute {
		window = requestWindow{started: now}
	}
	if window.count >= limit {
		return false
	}
	window.count++
	s.rates[grantID] = window
	return true
}

func (s *Service) Create(tripID string, ttl time.Duration) (string, Grant, error) {
	if tripID == "" {
		return "", Grant{}, errors.New("trip id is required")
	}
	if ttl <= 0 || ttl > MaxTTL {
		return "", Grant{}, errors.New("collaboration link TTL must be between 1 second and 90 days")
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", Grant{}, err
	}
	idBytes := make([]byte, 12)
	if _, err := rand.Read(idBytes); err != nil {
		return "", Grant{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	now := s.now().UTC()
	grant := Grant{
		ID:        base64.RawURLEncoding.EncodeToString(idBytes),
		TripID:    tripID,
		TokenHash: sha256.Sum256([]byte(token)),
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}
	if err := s.store.Put(grant); err != nil {
		return "", Grant{}, err
	}
	return token, grant, nil
}

func (s *Service) Resolve(token string) (Grant, error) {
	if len(token) != 43 {
		return Grant{}, ErrNotFound
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(decoded) != 32 {
		return Grant{}, ErrNotFound
	}
	hash := sha256.Sum256([]byte(token))
	grant, err := s.store.Get(hash)
	if err != nil {
		return Grant{}, err
	}
	if grant.RevokedAt != nil {
		return Grant{}, ErrRevoked
	}
	if !s.now().UTC().Before(grant.ExpiresAt) {
		return Grant{}, ErrExpired
	}
	return grant, nil
}

func (s *Service) List(tripID string) ([]Grant, error) {
	if tripID == "" {
		return nil, errors.New("trip id is required")
	}
	grants, err := s.store.List(tripID)
	if err != nil {
		return nil, err
	}
	sort.Slice(grants, func(i, j int) bool {
		if grants[i].CreatedAt.Equal(grants[j].CreatedAt) {
			return grants[i].ID > grants[j].ID
		}
		return grants[i].CreatedAt.After(grants[j].CreatedAt)
	})
	return grants, nil
}

func (s *Service) Revoke(tripID, id string) error {
	if tripID == "" || id == "" {
		return ErrNotFound
	}
	return s.store.Revoke(tripID, id, s.now().UTC())
}

type MemoryStore struct {
	mu     sync.RWMutex
	byHash map[[32]byte]Grant
	byID   map[string][32]byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byHash: make(map[[32]byte]Grant), byID: make(map[string][32]byte)}
}

func (s *MemoryStore) Put(grant Grant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.byHash[grant.TokenHash]; exists {
		return errors.New("duplicate collaboration token hash")
	}
	s.byHash[grant.TokenHash] = grant
	s.byID[grant.ID] = grant.TokenHash
	return nil
}

func (s *MemoryStore) Get(hash [32]byte) (Grant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	grant, ok := s.byHash[hash]
	if !ok {
		return Grant{}, ErrNotFound
	}
	return grant, nil
}

func (s *MemoryStore) List(tripID string) ([]Grant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	grants := make([]Grant, 0)
	for _, grant := range s.byHash {
		if grant.TripID == tripID {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func (s *MemoryStore) Revoke(tripID, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash, ok := s.byID[id]
	if !ok {
		return ErrNotFound
	}
	grant := s.byHash[hash]
	if grant.TripID != tripID || grant.RevokedAt != nil {
		return ErrNotFound
	}
	grant.RevokedAt = &at
	s.byHash[hash] = grant
	return nil
}
