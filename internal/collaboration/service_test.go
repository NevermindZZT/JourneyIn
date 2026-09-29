package collaboration

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"
)

func TestCreateResolveRevokeAndExpire(t *testing.T) {
	store := NewMemoryStore()
	service := NewService(store)
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	token, grant, err := service.Create("trip-1", DefaultTTL)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 || grant.TripID != "trip-1" || grant.TokenHash == [32]byte{} {
		t.Fatalf("unexpected collaboration grant: %+v", grant)
	}
	if grant.TokenHash != sha256.Sum256([]byte(token)) {
		t.Fatal("stored token hash does not match bearer token")
	}
	if got := grant.ExpiresAt.Sub(now); got != DefaultTTL {
		t.Fatalf("expiry duration=%s, want %s", got, DefaultTTL)
	}
	resolved, err := service.Resolve(token)
	if err != nil || resolved.ID != grant.ID {
		t.Fatalf("resolve=%+v err=%v", resolved, err)
	}
	if lenMustList(t, service, "trip-2") != 0 || lenMustList(t, service, "trip-1") != 1 {
		t.Fatal("collaboration grants must be isolated by trip")
	}
	if err := service.Revoke("trip-2", grant.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-trip revoke error=%v, want not found", err)
	}
	if err := service.Revoke("trip-1", grant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(token); !errors.Is(err, ErrRevoked) {
		t.Fatalf("resolve after revoke=%v, want revoked", err)
	}

	expiredToken, _, err := service.Create("trip-1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err := service.Resolve(expiredToken); !errors.Is(err, ErrExpired) {
		t.Fatalf("resolve after expiry=%v, want expired", err)
	}
}

func lenMustList(t *testing.T, service *Service, tripID string) int {
	t.Helper()
	grants, err := service.List(tripID)
	if err != nil {
		t.Fatal(err)
	}
	return len(grants)
}

func TestTTLBounds(t *testing.T) {
	service := NewService(NewMemoryStore())
	if _, _, err := service.Create("trip-1", 0); err == nil {
		t.Fatal("expected permanent collaboration tokens to be rejected")
	}
	if _, _, err := service.Create("trip-1", MaxTTL+time.Second); err == nil {
		t.Fatal("expected TTL above maximum to be rejected")
	}
}

func TestAllowRequestRateLimitIsScopedByGrantAndWindow(t *testing.T) {
	service := NewService(NewMemoryStore())
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	if !service.AllowRequest("grant-a:api", 2) || !service.AllowRequest("grant-a:api", 2) {
		t.Fatal("first requests should be allowed")
	}
	if service.AllowRequest("grant-a:api", 2) {
		t.Fatal("request over the grant limit should be rejected")
	}
	if !service.AllowRequest("grant-b:api", 2) {
		t.Fatal("a separate collaboration link should have an independent limit")
	}
	now = now.Add(time.Minute)
	if !service.AllowRequest("grant-a:api", 2) {
		t.Fatal("request window should reset after one minute")
	}
}
