package share

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestBundleDocumentHashAndValidation(t *testing.T) {
	now := time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC)
	trip := []byte(`{"schema_version":1,"id":"trip_a","title":"A","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-10-13","end":"2026-10-13"},"days":[{"id":"day_a","date":"2026-10-13","stops":[{"id":"stop_a","sequence":1,"title":"Place"}]}]}`)
	items := []BundleItem{{TripID: "trip_a", ContentHash: BundleContentHash(trip), Trip: trip}}
	doc, encoded, err := NewBundleDocument(items, now)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBundle(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.BundleHash != doc.BundleHash || len(decoded.Items) != 1 {
		t.Fatalf("unexpected decoded bundle: %+v", decoded)
	}
	tamperedDoc := doc
	tamperedDoc.BundleHash = "invalid"
	tampered, err := json.Marshal(tamperedDoc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeBundle(tampered); !errors.Is(err, ErrBundleHashMismatch) {
		t.Fatalf("expected bundle hash mismatch, got %v", err)
	}
	if _, _, err := NewBundleDocument([]BundleItem{items[0], items[0]}, now); err == nil {
		t.Fatal("expected duplicate trip id to be rejected")
	}
}

func TestBundleServiceExpiryAndCreatorRevocation(t *testing.T) {
	memory := NewMemoryBundleStore()
	service := NewBundleService(memory)
	now := time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	trip := []byte(`{"schema_version":1,"id":"trip_a","title":"A","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-10-13","end":"2026-10-13"},"days":[{"id":"day_a","date":"2026-10-13","stops":[{"id":"stop_a","sequence":1,"title":"Place"}]}]}`)
	_, snapshot, err := NewBundleDocument([]BundleItem{{TripID: "trip_a", ContentHash: BundleContentHash(trip), Trip: trip}}, now)
	if err != nil {
		t.Fatal(err)
	}
	token, record, err := service.Create(snapshot, "owner", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(token); err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(record.ID, "other", false); !errors.Is(err, ErrBundleForbidden) {
		t.Fatalf("expected owner check, got %v", err)
	}
	if err := service.Revoke(record.ID, "owner", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(token); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted bundle to be not found, got %v", err)
	}
	expiredToken, _, err := service.Create(snapshot, "owner", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := service.Resolve(expiredToken); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected expired bundle, got %v", err)
	}
}

func TestBundleResolveRejectsCorruptSnapshot(t *testing.T) {
	memory := NewMemoryBundleStore()
	service := NewBundleService(memory)
	trip := []byte(`{"schema_version":1,"id":"trip_a","title":"A","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-10-13","end":"2026-10-13"},"days":[{"id":"day_a","date":"2026-10-13","stops":[{"id":"stop_a","sequence":1,"title":"Place"}]}]}`)
	_, snapshot, err := NewBundleDocument([]BundleItem{{TripID: "trip_a", ContentHash: BundleContentHash(trip), Trip: trip}}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	token, record, err := service.Create(snapshot, "owner", 0)
	if err != nil {
		t.Fatal(err)
	}
	record.Snapshot[0] ^= 1
	if err := memory.PutBundle(record); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(token); !errors.Is(err, ErrBundleCorrupt) {
		t.Fatalf("expected corrupt bundle rejection, got %v", err)
	}
}
