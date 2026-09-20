package share

import (
	"errors"
	"testing"
	"time"
)

func TestCreateResolveRevokeExpire(t *testing.T) {
	m := NewMemoryStore()
	s := NewService(m)
	token, r, e := s.Create("trip", 2, "hash", []byte("x"), time.Hour)
	if e != nil || token == "" || r.TokenHash == [32]byte{} {
		t.Fatal(e)
	}
	if _, e = s.Resolve(token); e != nil {
		t.Fatal(e)
	}
	if e = s.Revoke(r.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Resolve(token); !errors.Is(e, ErrRevoked) {
		t.Fatal(e)
	}
}
func TestExpired(t *testing.T) {
	m := NewMemoryStore()
	s := NewService(m)
	token, _, _ := s.Create("trip", 1, "h", nil, time.Nanosecond)
	time.Sleep(time.Millisecond)
	if _, e := s.Resolve(token); !errors.Is(e, ErrExpired) {
		t.Fatal(e)
	}
}

func TestPermanentShare(t *testing.T) {
	m := NewMemoryStore()
	s := NewService(m)
	token, r, err := s.Create("trip", 1, "hash", []byte("permanent-snapshot"), 0)
	if err != nil {
		t.Fatalf("create permanent share failed: %v", err)
	}
	if !r.ExpiresAt.IsZero() {
		t.Fatalf("expected zero ExpiresAt for permanent share, got %v", r.ExpiresAt)
	}

	// Advance service clock by 100 years
	s.now = func() time.Time {
		return time.Now().Add(100 * 365 * 24 * time.Hour)
	}
	resolved, err := s.Resolve(token)
	if err != nil {
		t.Fatalf("expected permanent share to resolve in far future, got %v", err)
	}
	if string(resolved.Content) != "permanent-snapshot" {
		t.Fatalf("unexpected content: %s", string(resolved.Content))
	}

	// Revocation still works on permanent shares
	if err := s.Revoke(r.ID); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if _, err := s.Resolve(token); !errors.Is(err, ErrRevoked) {
		t.Fatalf("expected ErrRevoked after revoke, got %v", err)
	}
}
