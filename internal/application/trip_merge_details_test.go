package application

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPreviewMergeEnrichesMainAndChildPlanningPointDetails(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	document := []byte(`{
	  "schema_version":1,"title":"详情增补","status":"draft","timezone":"Asia/Shanghai",
	  "date_range":{"start":"2026-04-18","end":"2026-04-18"},
	  "map":{"preferred_provider":"baidu","enabled_providers":["baidu"]},
	  "days":[{"id":"day-1","date":"2026-04-18","stops":[{
	    "id":"stop-1","sequence":1,"title":"旧主点","address":"旧地址","kind":"poi",
	    "location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.1,"lng":120.1,"crs":"bd09ll"}}},
	    "time_window":{"arrival":"09:00"},"weather":{"source":"fixture"},
	    "children":[{"id":"child-1","sequence":1,"title":"旧子点","address":"旧子地址","kind":"poi","time_window":{"departure":"12:00"},"location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.11,"lng":120.11,"crs":"bd09ll"}}},"weather":{"source":"child-fixture"}}]
	  },{"id":"stop-2","sequence":2,"title":"终点"}],"legs":[{"id":"leg-1","from_stop_id":"stop-1","to_stop_id":"stop-2","mode":"walking","snapshots":[{"provider":"baidu","coordinate_system":"bd09ll","geometry":[[120.1,30.1],[120.2,30.2]]}]}]}]
	}`)
	record, err := service.Create(ctx, document, "test")
	if err != nil {
		t.Fatal(err)
	}
	mainTitle, mainAddress, mainKind, mainArrival, mainDeparture := "新主点", "新主地址", "museum", "10:00", "11:30"
	childTitle, childAddress, childKind, childArrival := "新子点", "新子地址", "viewpoint", "10:15"
	preview, err := service.PreviewMerge(ctx, MergePatch{Days: []MergeDayPatch{{DayID: "day-1", Stops: []MergeStopPatch{
		{StopID: "stop-1", Title: &mainTitle, Address: &mainAddress, Kind: &mainKind, TimeWindow: &MergeTimeWindowPatch{Arrival: &mainArrival, Departure: &mainDeparture}, DescriptionMarkdown: stringPointer("主点详细说明")},
		{StopID: "child-1", ParentStopID: "stop-1", Title: &childTitle, Address: &childAddress, Kind: &childKind, TimeWindow: &MergeTimeWindowPatch{Arrival: &childArrival}, DescriptionMarkdown: stringPointer("子点详细说明")},
	}}}}, record.ID, record.Revision, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Diff) != 10 {
		t.Fatalf("detail merge diff count=%d, want 10: %+v", len(preview.Diff), preview.Diff)
	}
	for _, key := range []string{"route_geometry", "legs", "map", "locations", "weather"} {
		if !preview.Preserved[key] {
			t.Fatalf("merge did not preserve %s: %+v", key, preview.Preserved)
		}
	}
	if _, err := service.CommitSave(ctx, preview.PreviewID, preview.ConfirmationToken, "detail-merge-1", record.Revision, "test"); err != nil {
		t.Fatal(err)
	}
	updated, err := service.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(updated.Document, &root); err != nil {
		t.Fatal(err)
	}
	day := root["days"].([]any)[0].(map[string]any)
	main := day["stops"].([]any)[0].(map[string]any)
	if main["title"] != mainTitle || main["address"] != mainAddress || main["kind"] != mainKind || main["description_markdown"] != "主点详细说明" {
		t.Fatalf("main details=%+v", main)
	}
	mainWindow := main["time_window"].(map[string]any)
	if mainWindow["arrival"] != mainArrival || mainWindow["departure"] != mainDeparture {
		t.Fatalf("main time window=%+v", mainWindow)
	}
	if main["weather"].(map[string]any)["source"] != "fixture" || main["location"] == nil {
		t.Fatalf("main protected data changed=%+v", main)
	}
	child := main["children"].([]any)[0].(map[string]any)
	if child["title"] != childTitle || child["address"] != childAddress || child["kind"] != childKind || child["description_markdown"] != "子点详细说明" {
		t.Fatalf("child details=%+v", child)
	}
	childWindow := child["time_window"].(map[string]any)
	if childWindow["arrival"] != childArrival || childWindow["departure"] != "12:00" {
		t.Fatalf("child time window=%+v", childWindow)
	}
	if child["weather"].(map[string]any)["source"] != "child-fixture" || child["location"] == nil {
		t.Fatalf("child protected data changed=%+v", child)
	}
	if len(day["legs"].([]any)) != 1 {
		t.Fatalf("route legs changed=%+v", day["legs"])
	}
}

func TestPreviewMergeRejectsInvalidPlanningPointTime(t *testing.T) {
	service := testService(t)
	record, err := service.Create(context.Background(), []byte(mergeTripDocument), "test")
	if err != nil {
		t.Fatal(err)
	}
	invalid := "9:30"
	_, err = service.PreviewMerge(context.Background(), MergePatch{Days: []MergeDayPatch{{DayID: "day-1", Stops: []MergeStopPatch{{StopID: "stop-1", TimeWindow: &MergeTimeWindowPatch{Arrival: &invalid}}}}}}, record.ID, record.Revision, "test")
	if err == nil {
		t.Fatal("invalid time window was accepted")
	}
}
