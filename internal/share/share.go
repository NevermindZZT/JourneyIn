package share

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("share not found")
	ErrExpired  = errors.New("share expired")
	ErrRevoked  = errors.New("share revoked")
)

type Record struct {
	ID, TripID  string
	Revision    int
	ContentHash string
	TokenHash   [32]byte
	ExpiresAt   time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	Content     []byte
}
type Snapshot struct {
	TripID      string
	Revision    int
	ContentHash string
	Content     []byte
}
type Store interface {
	Put(Record) error
	Get([32]byte) (Record, error)
	Revoke(string, time.Time) error
}
type MemoryStore struct {
	mu     sync.RWMutex
	byHash map[[32]byte]Record
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{byHash: make(map[[32]byte]Record)} }
func (s *MemoryStore) Put(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byHash[r.TokenHash] = r
	return nil
}
func (s *MemoryStore) Get(h [32]byte) (Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.byHash[h]
	if !ok {
		return Record{}, ErrNotFound
	}
	return r, nil
}
func (s *MemoryStore) Revoke(id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, r := range s.byHash {
		if r.ID == id {
			r.RevokedAt = &at
			s.byHash[h] = r
			return nil
		}
	}
	return ErrNotFound
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }
func (s *Service) Create(tripID string, revision int, contentHash string, content []byte, ttl time.Duration) (string, Record, error) {
	if tripID == "" || revision < 1 || contentHash == "" {
		return "", Record{}, errors.New("invalid share snapshot")
	}
	if ttl <= 0 {
		return "", Record{}, errors.New("ttl must be positive")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Record{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	h := sha256.Sum256([]byte(token))
	now := s.now().UTC()
	r := Record{ID: base64.RawURLEncoding.EncodeToString(raw[:12]), TripID: tripID, Revision: revision, ContentHash: contentHash, TokenHash: h, ExpiresAt: now.Add(ttl), CreatedAt: now, Content: append([]byte(nil), content...)}
	if err := s.store.Put(r); err != nil {
		return "", Record{}, err
	}
	return token, r, nil
}
func (s *Service) Resolve(token string) (Record, error) {
	h := sha256.Sum256([]byte(token))
	r, err := s.store.Get(h)
	if err != nil {
		return Record{}, err
	}
	now := s.now()
	if r.RevokedAt != nil {
		return Record{}, ErrRevoked
	}
	if !now.Before(r.ExpiresAt) {
		return Record{}, ErrExpired
	}
	return r, nil
}
func (s *Service) Revoke(id string) error { return s.store.Revoke(id, s.now().UTC()) }
func (s *Service) Expire(id string) error {
	r, err := s.find(id)
	if err != nil {
		return err
	}
	t := s.now().UTC().Add(-time.Nanosecond)
	r.ExpiresAt = t
	return s.store.Put(r)
}
func (s *Service) find(id string) (Record, error) {
	if id == "" {
		return Record{}, ErrNotFound
	}
	for _, x := range []string{} {
		_ = x
	}
	if m, ok := s.store.(*MemoryStore); ok {
		m.mu.RLock()
		defer m.mu.RUnlock()
		for _, r := range m.byHash {
			if r.ID == id {
				return r, nil
			}
		}
	}
	return Record{}, ErrNotFound
}
func TokenHash(token string) [32]byte { return sha256.Sum256([]byte(token)) }
func EqualHash(a, b [32]byte) bool    { return subtle.ConstantTimeCompare(a[:], b[:]) == 1 }
