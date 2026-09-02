package sync

import (
	"errors"
	"testing"
)

func TestPushIdempotencyConflict(t *testing.T) {
	m := NewMemoryStore()
	c := Change{ChangeID: "c", AggregateID: "t", DeviceID: "d", Hash: "h", BaseRevision: 0, NewRevision: 1}
	if _, e := m.Push(c); e != nil {
		t.Fatal(e)
	}
	if _, e := m.Push(c); e != nil {
		t.Fatal(e)
	}
	c.Hash = "x"
	if _, e := m.Push(c); !errors.Is(e, ErrIdempotencyConflict) {
		t.Fatal(e)
	}
}
func TestRevisionConflict(t *testing.T) {
	m := NewMemoryStore()
	_, _ = m.Push(Change{ChangeID: "1", AggregateID: "t", DeviceID: "d", Hash: "h", BaseRevision: 0, NewRevision: 1})
	_, e := m.Push(Change{ChangeID: "2", AggregateID: "t", DeviceID: "d", Hash: "h2", BaseRevision: 0, NewRevision: 1})
	if !errors.Is(e, ErrConflict) {
		t.Fatal(e)
	}
}

func TestHistoryChangeDoesNotAdvanceWorkingRevision(t *testing.T) {
	m := NewMemoryStore()
	if _, err := m.Push(Change{ChangeID: "working-1", AggregateID: "t", DeviceID: "d", Hash: "working", BaseRevision: 0, NewRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Push(Change{ChangeID: "history-1", AggregateID: "t", DeviceID: "d", Operation: OperationHistorySave, Hash: "history", BaseRevision: 1, NewRevision: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Push(Change{ChangeID: "working-2", AggregateID: "t", DeviceID: "d", Hash: "working-2", BaseRevision: 1, NewRevision: 2}); err != nil {
		t.Fatal(err)
	}
}
