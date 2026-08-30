package application

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

func TestPlanTripSerializesDuplicateRequests(t *testing.T) {
	service := testService(t)
	provider := &fakePlanningProvider{}
	service.SetMapService(NewMapService(service.store, journeymaps.NewRegistry(provider), 8, 0))
	tripJSON := []byte("{\"schema_version\":1,\"title\":\"重复规划测试\",\"status\":\"draft\",\"timezone\":\"Asia/Shanghai\",\"date_range\":{\"start\":\"2026-04-18\",\"end\":\"2026-04-18\"},\"days\":[{\"id\":\"day-1\",\"date\":\"2026-04-18\",\"stops\":[]}]} ")
	record, err := service.Create(context.Background(), tripJSON, "test")
	if err != nil {
		t.Fatal(err)
	}
	for index, location := range []json.RawMessage{
		json.RawMessage("{\"preferred\":\"bd09ll\",\"coordinates\":{\"bd09ll\":{\"lat\":30.200000,\"lng\":120.100000,\"crs\":\"bd09ll\"}},\"source\":\"test\"}"),
		json.RawMessage("{\"preferred\":\"bd09ll\",\"coordinates\":{\"bd09ll\":{\"lat\":30.210000,\"lng\":120.110000,\"crs\":\"bd09ll\"}},\"source\":\"test\"}"),
	} {
		record, err = service.AddStop(context.Background(), record.ID, record.Revision, "day-1", AddStopInput{Title: string(rune('A' + index)), Location: location}, "test")
		if err != nil {
			t.Fatal(err)
		}
	}
	initialRevision := record.Revision
	results := make(chan error, 2)
	var group sync.WaitGroup
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, planErr := service.PlanTrip(context.Background(), record.ID, initialRevision, PlanInput{Provider: provider.ID(), Mode: journeymaps.ModeWalking}, "test")
			results <- planErr
		}()
	}
	group.Wait()
	close(results)

	successes := 0
	conflicts := 0
	for planErr := range results {
		switch {
		case planErr == nil:
			successes++
		case errors.Is(planErr, store.ErrRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected planning error: %v", planErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("planning results successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}
	if got := provider.routeCalls.Load(); got != 1 {
		t.Fatalf("provider route calls=%d, want 1", got)
	}
	current, err := service.Get(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != initialRevision+1 {
		t.Fatalf("final revision=%d, want %d", current.Revision, initialRevision+1)
	}
}
