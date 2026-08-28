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
