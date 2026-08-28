package sync

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrConflict            = errors.New("revision conflict")
	ErrIdempotencyConflict = errors.New("idempotency key reused with different change")
	ErrNotFound            = errors.New("change not found")
)

type Change struct {
	Sequence     Cursor    `json:"sequence,omitempty"`
	ChangeID     string    `json:"change_id"`
	AggregateID  string    `json:"aggregate_id"`
	DeviceID     string    `json:"device_id"`
	Operation    string    `json:"operation"`
	Hash         string    `json:"hash"`
	BaseRevision int       `json:"base_revision"`
	NewRevision  int       `json:"new_revision"`
	Payload      []byte    `json:"payload,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}
type Cursor int64
type Store interface {
	Push(Change) (Change, error)
	Pull(string, Cursor, int) ([]Change, Cursor, error)
}
type MemoryStore struct {
	mu        sync.Mutex
	changes   []Change
	byID      map[string]Change
	revisions map[string]int
	seq       Cursor
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{byID: map[string]Change{}, revisions: map[string]int{}}
}
func (s *MemoryStore) Push(c Change) (Change, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.ChangeID == "" || c.AggregateID == "" || c.DeviceID == "" {
		return Change{}, errors.New("missing change identity")
	}
	if old, ok := s.byID[c.ChangeID]; ok {
		if old.Hash != c.Hash || old.AggregateID != c.AggregateID {
			return Change{}, ErrIdempotencyConflict
		}
		return old, nil
	}
	if s.revisions[c.AggregateID] != c.BaseRevision {
		return Change{}, ErrConflict
	}
	if c.NewRevision != c.BaseRevision+1 {
		return Change{}, ErrConflict
	}
	s.seq++
	c.Sequence = s.seq
	c.CreatedAt = time.Now().UTC()
	s.changes = append(s.changes, c)
	s.byID[c.ChangeID] = c
	s.revisions[c.AggregateID] = c.NewRevision
	return c, nil
}
func (s *MemoryStore) Pull(aggregate string, cur Cursor, limit int) ([]Change, Cursor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	out := []Change{}
	next := cur
	for i, c := range s.changes {
		if Cursor(i+1) <= cur || (aggregate != "" && c.AggregateID != aggregate) {
			continue
		}
		if len(out) >= limit {
			break
		}
		out = append(out, c)
		next = Cursor(i + 1)
	}
	return out, next, nil
}

type Service struct{ store Store }

func NewService(st Store) *Service               { return &Service{store: st} }
func (s *Service) Push(c Change) (Change, error) { return s.store.Push(c) }
func (s *Service) Pull(a string, c Cursor, l int) ([]Change, Cursor, error) {
	return s.store.Pull(a, c, l)
}
