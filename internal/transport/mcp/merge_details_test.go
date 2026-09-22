package mcptransport

import (
	"encoding/json"
	"testing"
)

func TestPreviewArgsAcceptsRestrictedPlanningPointDetailPatch(t *testing.T) {
	var input PreviewArgs
	data := []byte("{\"operation\":\"merge\",\"target_trip_id\":\"trip-1\",\"expected_revision\":7,\"patch\":{\"days\":[{\"day_id\":\"day-1\",\"stops\":[{\"stop_id\":\"stop-1\",\"title\":\"更新名称\",\"address\":\"更新地址\",\"kind\":\"museum\",\"time_window\":{\"arrival\":\"09:30\",\"departure\":\"11:00\"}},{\"stop_id\":\"child-1\",\"parent_stop_id\":\"stop-1\",\"description_markdown\":\"子点说明\"}] }]}}")
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if !input.patchProvided || len(input.Patch.Days) != 1 || len(input.Patch.Days[0].Stops) != 2 {
		t.Fatalf("detail patch was not decoded: %+v", input)
	}
	main := input.Patch.Days[0].Stops[0]
	if main.Title == nil || *main.Title != "更新名称" || main.TimeWindow == nil || main.TimeWindow.Arrival == nil || *main.TimeWindow.Arrival != "09:30" {
		t.Fatalf("main detail fields missing: %+v", main)
	}
	child := input.Patch.Days[0].Stops[1]
	if child.ParentStopID != "stop-1" || child.DescriptionMarkdown == nil {
		t.Fatalf("child targeting fields missing: %+v", child)
	}
}

func TestPreviewArgsRejectsUnsupportedPlanningPointDetailField(t *testing.T) {
	var input PreviewArgs
	data := []byte("{\"operation\":\"merge\",\"patch\":{\"days\":[{\"day_id\":\"day-1\",\"stops\":[{\"stop_id\":\"stop-1\",\"location\":{}}]}]} }")
	if err := json.Unmarshal(data, &input); err == nil {
		t.Fatal("location patch was accepted")
	}
}
