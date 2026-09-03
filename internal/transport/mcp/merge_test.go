package mcptransport

import (
	"context"
	"encoding/json"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	journeyinroot "journeyin"
	"journeyin/internal/application"
	"journeyin/internal/store"
)

const mcpMergeTripDocument = `{
  "schema_version": 1,
  "title": "MCP 合并测试",
  "status": "draft",
  "timezone": "Asia/Shanghai",
  "date_range": {"start":"2026-04-18","end":"2026-04-18"},
  "description_markdown": "旧总体说明",
  "map": {"preferred_provider":"baidu","enabled_providers":["baidu"],"default_mode":"walking"},
  "days": [{
    "id":"day-1",
    "date":"2026-04-18",
    "notes_markdown":"旧日备注",
    "stops":[
      {"id":"stop-1","sequence":1,"title":"起点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.1,"lng":120.1}}},"description_markdown":"旧地点说明","weather":{"source":"fixture"}},
      {"id":"stop-2","sequence":2,"title":"终点"}
    ],
    "legs":[{"id":"leg-1","from_stop_id":"stop-1","to_stop_id":"stop-2","mode":"walking","snapshots":[{"provider":"baidu","coordinate_system":"bd09ll","mode":"walking","geometry":[[120.1,30.1],[120.2,30.2]],"distance_m":1000,"duration_s":900,"fetched_at":"2026-04-01T00:00:00Z","expires_at":"2026-04-02T00:00:00Z"}]}]
  }],
  "future_field":{"keep":true}
}`

func TestPreviewMergeInputDeclaresPatchObject(t *testing.T) {
	ctx := context.Background()
	server := NewServer(nil, "test", nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.mcp.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var tool *mcp.Tool
	for _, candidate := range toolsResult.Tools {
		if candidate.Name == "journeyin.preview_save_trip" {
			tool = candidate
			break
		}
	}
	if tool == nil {
		t.Fatal("preview_save_trip tool not found")
	}
	inputSchema, ok := tool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("input schema has type %T, want object schema", tool.InputSchema)
	}
	properties, ok := inputSchema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("input schema properties has type %T", inputSchema["properties"])
	}
	patchSchema, ok := properties["patch"].(map[string]any)
	if !ok {
		t.Fatalf("patch schema has type %T, want object schema", properties["patch"])
	}
	if got, _ := patchSchema["type"].(string); got != "object" {
		t.Fatalf("patch schema type = %q, want object", got)
	}
}

func TestPreviewMergeRejectsFullDocument(t *testing.T) {
	var input PreviewArgs
	if err := json.Unmarshal([]byte(`{"operation":"merge","trip_json":"{}","patch":{"description_markdown":"new"}}`), &input); err != nil {
		t.Fatal(err)
	}
	server := NewServer(nil, "test", nil)
	if _, _, err := server.previewSaveTrip(context.Background(), nil, input); err == nil {
		t.Fatal("merge preview accepted trip_json")
	}
}

func TestPreviewMergeMCPFlowPreservesRouteData(t *testing.T) {
	ctx := context.Background()
	migrations, err := fs.Sub(journeyinroot.MigrationFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "journeyin.db"), migrations)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := application.NewTripService(db)
	record, err := app.Create(ctx, []byte(mcpMergeTripDocument), "test")
	if err != nil {
		t.Fatal(err)
	}

	server := NewServer(app, "test", nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.mcp.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	previewResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "journeyin.preview_save_trip",
		Arguments: map[string]any{
			"operation":         "merge",
			"target_trip_id":    record.ID,
			"expected_revision": record.Revision,
			"patch": map[string]any{
				"description_markdown": "新总体说明",
				"days": []any{map[string]any{
					"day_id":         "day-1",
					"notes_markdown": "新日备注",
					"stops": []any{map[string]any{
						"stop_id":              "stop-1",
						"description_markdown": "新地点说明",
					}},
				}},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if previewResult.IsError {
		t.Fatalf("merge preview returned error: %+v", previewResult.Content)
	}
	payload, ok := previewResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("preview structured content has type %T", previewResult.StructuredContent)
	}
	if payload["operation"] != "merge" || payload["base_revision"] != float64(record.Revision) {
		t.Fatalf("unexpected preview payload: %#v", payload)
	}
	preserved, ok := payload["preserved"].(map[string]any)
	if !ok {
		t.Fatalf("preserved payload has type %T", payload["preserved"])
	}
	for _, key := range []string{"route_geometry", "legs", "map", "locations", "weather"} {
		if preserved[key] != true {
			t.Fatalf("preserved[%s] = %#v", key, preserved[key])
		}
	}
	token, ok := payload["confirmation_token"].(string)
	if !ok || token == "" {
		t.Fatalf("missing confirmation token: %#v", payload["confirmation_token"])
	}

	commitResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "journeyin.commit_save_trip",
		Arguments: map[string]any{
			"preview_id":         payload["preview_id"],
			"confirmation_token": token,
			"idempotency_key":    "mcp-merge-idempotency-001",
			"expected_revision":  record.Revision,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if commitResult.IsError {
		t.Fatalf("merge commit returned error: %+v", commitResult.Content)
	}
	commitPayload, ok := commitResult.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("commit structured content has type %T", commitResult.StructuredContent)
	}
	if commitPayload["status"] != "updated" {
		t.Fatalf("commit status = %#v", commitPayload["status"])
	}

	updated, err := app.Get(ctx, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(updated.Document, &document); err != nil {
		t.Fatal(err)
	}
	if document["description_markdown"] != "新总体说明" {
		t.Fatalf("root Markdown = %#v", document["description_markdown"])
	}
	if _, ok := document["future_field"]; !ok {
		t.Fatal("unknown root field was not preserved")
	}
	days := document["days"].([]any)
	day := days[0].(map[string]any)
	if day["notes_markdown"] != "新日备注" {
		t.Fatalf("day Markdown = %#v", day["notes_markdown"])
	}
	stop := day["stops"].([]any)[0].(map[string]any)
	if stop["description_markdown"] != "新地点说明" {
		t.Fatalf("stop Markdown = %#v", stop["description_markdown"])
	}
	legs := day["legs"].([]any)
	leg := legs[0].(map[string]any)
	snapshot := leg["snapshots"].([]any)[0].(map[string]any)
	geometry := snapshot["geometry"].([]any)
	if len(geometry) != 2 {
		t.Fatalf("route geometry = %#v", snapshot["geometry"])
	}
}
