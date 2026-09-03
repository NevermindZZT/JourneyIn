package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"journeyin/internal/store"
)

const mergeTripDocument = `{
  "schema_version": 1,
  "title": "受限合并测试行程",
  "status": "draft",
  "timezone": "Asia/Shanghai",
  "date_range": {"start":"2026-04-18","end":"2026-04-18"},
  "description_markdown": "# 旧总体说明",
  "links": [
    {"id":"link-old","title":"旧来源","url":"https://example.com/old","kind":"reference"}
  ],
  "map": {"preferred_provider":"baidu","enabled_providers":["baidu"],"default_mode":"walking"},
  "days": [{
    "id":"day-1",
    "date":"2026-04-18",
    "title":"第一天",
    "notes_markdown":"旧日程备注",
    "stops":[
      {
        "id":"stop-1",
        "sequence":1,
        "title":"起点",
        "address":"旧地址",
        "location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.1,"lng":120.1}}},
        "time_window":{"start":"09:00"},
        "description_markdown":"旧地点说明",
        "links":[{"id":"stop-link-old","title":"旧地点来源","url":"https://example.com/stop-old","kind":"reference","future_link_field":"preserve"}],
        "weather":{"source":"fixture","condition":"晴"}
      },
      {"id":"stop-2","sequence":2,"title":"终点"}
    ],
    "legs":[{
      "id":"leg-1",
      "from_stop_id":"stop-1",
      "to_stop_id":"stop-2",
      "mode":"walking",
      "snapshots":[{
        "provider":"baidu",
        "coordinate_system":"bd09ll",
        "mode":"walking",
        "strategy":"default",
        "source":"fixture",
        "geometry":[[120.1,30.1],[120.15,30.15],[120.2,30.2]],
        "distance_m":1000,
        "duration_s":900,
        "fetched_at":"2026-04-01T00:00:00Z",
        "expires_at":"2026-04-02T00:00:00Z"
      }]
    }]
  }],
  "metadata":{"source":"test","future_metadata":"preserve"},
  "future_field":{"opaque":[1,2,3]}
}`

func stringPointer(value string) *string { return &value }

func TestPreviewMergePreservesProtectedTripData(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	record, err := service.Create(ctx, []byte(mergeTripDocument), "test")
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), record.Document...)

	preview, err := service.PreviewMerge(ctx, MergePatch{
		DescriptionMarkdown: stringPointer("# 新总体说明"),
		Links: &MergeLinkPatch{
			Add:       []PatchLink{{Title: "新来源", URL: "https://example.com/new", Kind: "reference"}},
			RemoveIDs: []string{"link-old"},
		},
		Days: []MergeDayPatch{{
			DayID:         "day-1",
			NotesMarkdown: stringPointer("## 新日程备注"),
			Stops: []MergeStopPatch{{
				StopID:              "stop-1",
				DescriptionMarkdown: stringPointer("### 新地点说明"),
				Links:               &MergeLinkPatch{Add: []PatchLink{{Title: "新地点来源", URL: "https://example.com/stop-new", Kind: "reference"}}},
			}},
		}},
	}, record.ID, record.Revision, "test")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Operation != "merge" || preview.TargetTripID != record.ID || preview.BaseRevision != record.Revision {
		t.Fatalf("unexpected merge preview metadata: %+v", preview)
	}
	if len(preview.ChangedPaths) != 5 || len(preview.Diff) != 5 {
		t.Fatalf("unexpected merge diff: paths=%v diff=%v", preview.ChangedPaths, preview.Diff)
	}
	for _, key := range []string{"route_geometry", "legs", "map", "locations", "weather"} {
		if !preview.Preserved[key] {
			t.Fatalf("merge preview did not preserve %s: %#v", key, preview.Preserved)
		}
	}
	unchanged, err := service.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Revision != record.Revision {
		t.Fatalf("preview changed revision: got %d want %d", unchanged.Revision, record.Revision)
	}

	committed, err := service.CommitSave(ctx, preview.PreviewID, preview.ConfirmationToken, "merge-idempotency-001", record.Revision, "test")
	if err != nil {
		t.Fatal(err)
	}
	if committed.Status != "updated" || committed.Revision != record.Revision+1 {
		t.Fatalf("unexpected merge commit: %+v", committed)
	}
	updated, err := service.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeProtected, err := protectedMergeProjection(before)
	if err != nil {
		t.Fatal(err)
	}
	afterProtected, err := protectedMergeProjection(updated.Document)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeProtected, afterProtected) {
		t.Fatalf("protected Trip data changed:\nbefore=%s\nafter=%s", beforeProtected, afterProtected)
	}

	var document map[string]any
	if err := json.Unmarshal(updated.Document, &document); err != nil {
		t.Fatal(err)
	}
	if document["description_markdown"] != "# 新总体说明" {
		t.Fatalf("root Markdown = %#v", document["description_markdown"])
	}
	if _, ok := document["future_field"]; !ok {
		t.Fatal("unknown root field was not preserved")
	}
	links, ok := document["links"].([]any)
	if !ok || len(links) != 1 {
		t.Fatalf("root links = %#v", document["links"])
	}
	days := document["days"].([]any)
	day := days[0].(map[string]any)
	if day["notes_markdown"] != "## 新日程备注" {
		t.Fatalf("day Markdown = %#v", day["notes_markdown"])
	}
	stops := day["stops"].([]any)
	stop := stops[0].(map[string]any)
	if stop["description_markdown"] != "### 新地点说明" {
		t.Fatalf("stop Markdown = %#v", stop["description_markdown"])
	}
	stopLinks := stop["links"].([]any)
	if len(stopLinks) != 2 {
		t.Fatalf("stop links = %#v", stop["links"])
	}
	originalStopLink, ok := stopLinks[0].(map[string]any)
	if !ok || originalStopLink["future_link_field"] != "preserve" {
		t.Fatalf("untouched link fields were not preserved: %#v", stopLinks[0])
	}
	replay, err := service.CommitSave(ctx, preview.PreviewID, preview.ConfirmationToken, "merge-idempotency-001", record.Revision, "test")
	if err != nil {
		t.Fatal(err)
	}
	if replay.Status != "already_applied" || replay.TripID != committed.TripID {
		t.Fatalf("unexpected merge replay: first=%+v replay=%+v", committed, replay)
	}
}

func TestMergePatchRejectsUnsupportedFields(t *testing.T) {
	cases := []string{
		`{"map":{}}`,
		`{"days":[{"day_id":"day-1","stops":[{"stop_id":"stop-1","location":{}}]}]}`,
		`{"links":{"add":[{"title":"来源","url":"https://example.com","purpose":"unsupported"}]}}`,
		`{"description_markdown":null}`,
	}
	for _, input := range cases {
		var patch MergePatch
		if err := json.Unmarshal([]byte(input), &patch); err == nil {
			t.Errorf("unsupported merge patch was accepted: %s", input)
		}
	}
}

func TestPreviewMergeRequiresCurrentRevisionAndTarget(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	record, err := service.Create(ctx, []byte(mergeTripDocument), "test")
	if err != nil {
		t.Fatal(err)
	}
	patch := MergePatch{DescriptionMarkdown: stringPointer("new")}
	if _, err := service.PreviewMerge(ctx, patch, record.ID, record.Revision+1, "test"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("stale preview error = %v, want revision conflict", err)
	}
	if _, err := service.PreviewMerge(ctx, patch, "", record.Revision, "test"); err == nil {
		t.Fatal("missing merge target was accepted")
	}
}

func TestMergeCommitRejectsConcurrentRevision(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	record, err := service.Create(ctx, []byte(mergeTripDocument), "test")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewMerge(ctx, MergePatch{DescriptionMarkdown: stringPointer("merge text")}, record.ID, record.Revision, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Replace(ctx, record.ID, record.Revision, record.Document, "other-writer"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitSave(ctx, preview.PreviewID, preview.ConfirmationToken, "merge-idempotency-003", record.Revision, "test"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("concurrent merge commit error = %v, want revision conflict", err)
	}
	current, err := service.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != record.Revision+1 {
		t.Fatalf("concurrent commit changed revision unexpectedly: got %d", current.Revision)
	}
	var document map[string]any
	if err := json.Unmarshal(current.Document, &document); err != nil {
		t.Fatal(err)
	}
	if document["description_markdown"] != "# 旧总体说明" {
		t.Fatalf("stale merge was applied: %#v", document["description_markdown"])
	}
}

func TestMergeCommitRequiresExpectedRevision(t *testing.T) {
	ctx := context.Background()
	service := testService(t)
	record, err := service.Create(ctx, []byte(mergeTripDocument), "test")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.PreviewMerge(ctx, MergePatch{DescriptionMarkdown: stringPointer("new")}, record.ID, record.Revision, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitSave(ctx, preview.PreviewID, preview.ConfirmationToken, "merge-idempotency-002", 0, "test"); err == nil {
		t.Fatal("merge commit accepted missing expected revision")
	}
	current, err := service.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != record.Revision {
		t.Fatalf("failed commit changed revision: got %d want %d", current.Revision, record.Revision)
	}
}
