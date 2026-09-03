package mcptransport

import (
	"context"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	journeyinroot "journeyin"
	"journeyin/internal/application"
	"journeyin/internal/store"
)

func TestReadTripToolsDeclareDocumentAsObject(t *testing.T) {
	ctx := context.Background()
	server := NewServer(nil, "test", nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.mcp.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	for _, name := range []string{"journeyin.get_trip", "journeyin.get_trip_history"} {
		t.Run(name, func(t *testing.T) {
			var tool *mcp.Tool
			for _, candidate := range result.Tools {
				if candidate.Name == name {
					tool = candidate
					break
				}
			}
			if tool == nil {
				t.Fatalf("tool %q not found", name)
			}

			schema, ok := tool.OutputSchema.(map[string]any)
			if !ok {
				t.Fatalf("output schema has type %T, want object schema", tool.OutputSchema)
			}
			if got, _ := schema["type"].(string); got != "object" {
				t.Fatalf("output schema type = %q, want object", got)
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("output schema properties has type %T, want object", schema["properties"])
			}
			document, ok := properties["document"].(map[string]any)
			if !ok {
				t.Fatalf("document schema has type %T, want object schema", properties["document"])
			}
			if got, _ := document["type"].(string); got != "object" {
				t.Fatalf("document schema type = %q, want object", got)
			}
		})
	}
}

func TestDecodeTripDocumentPreservesCompleteObject(t *testing.T) {
	document, err := decodeTripDocument([]byte(`{"schema_version":1,"description_markdown":"# Overview","days":[{"id":"day-1","stops":[{"id":"stop-1","description_markdown":"## Details"}]}]}`))
	if err != nil {
		t.Fatalf("decode document: %v", err)
	}
	if got := document["description_markdown"]; got != "# Overview" {
		t.Fatalf("description_markdown = %#v, want complete Markdown", got)
	}
	days, ok := document["days"].([]any)
	if !ok || len(days) != 1 {
		t.Fatalf("days = %#v, want one day", document["days"])
	}
	day, ok := days[0].(map[string]any)
	if !ok {
		t.Fatalf("day has type %T, want object", days[0])
	}
	stops, ok := day["stops"].([]any)
	if !ok || len(stops) != 1 {
		t.Fatalf("stops = %#v, want one stop", day["stops"])
	}
	stop, ok := stops[0].(map[string]any)
	if !ok || stop["description_markdown"] != "## Details" {
		t.Fatalf("stop = %#v, want nested Markdown", stops[0])
	}
}

func TestDecodeTripDocumentRejectsNonObject(t *testing.T) {
	if _, err := decodeTripDocument([]byte(`[]`)); err == nil {
		t.Fatal("decodeTripDocument accepted an array")
	}
	if _, err := decodeTripDocument([]byte(`null`)); err == nil {
		t.Fatal("decodeTripDocument accepted null")
	}
}

func TestReadTripToolsReturnCompleteDocument(t *testing.T) {
	ctx := context.Background()
	migrations, err := fs.Sub(journeyinroot.MigrationFS, "migrations")
	if err != nil {
		t.Fatalf("sub migrations: %v", err)
	}
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "journeyin.db"), migrations)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	app := application.NewTripService(db)
	record, err := app.Create(ctx, []byte(`{"schema_version":1,"title":"MCP完整行程","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"description_markdown":"# 总体说明\n\n## 来源\n[攻略](https://example.com/source)","days":[{"id":"day-1","date":"2026-04-18","title":"第1天","notes_markdown":"## 行程\n\n- 上午：城市漫步","stops":[{"id":"stop-1","sequence":1,"kind":"poi","title":"示例地点","description_markdown":"### 详情\n\n完整说明和来源：[来源](https://example.com/source)"}]}]}`), "test")
	if err != nil {
		t.Fatalf("create trip: %v", err)
	}
	history, _, err := app.SaveTripVersion(ctx, record.ID, record.Revision, "initial")
	if err != nil {
		t.Fatalf("save history: %v", err)
	}

	server := NewServer(app, "test", nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.mcp.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer clientSession.Close()

	assertDocument := func(t *testing.T, result *mcp.CallToolResult) {
		t.Helper()
		if result.IsError {
			t.Fatalf("MCP tool returned error: %+v", result.Content)
		}
		payload, ok := result.StructuredContent.(map[string]any)
		if !ok {
			t.Fatalf("structured content has type %T, want object", result.StructuredContent)
		}
		document, ok := payload["document"].(map[string]any)
		if !ok {
			t.Fatalf("document has type %T, want complete object", payload["document"])
		}
		if document["description_markdown"] != "# 总体说明\n\n## 来源\n[攻略](https://example.com/source)" {
			t.Fatalf("description_markdown was not preserved: %#v", document["description_markdown"])
		}
		days, ok := document["days"].([]any)
		if !ok || len(days) != 1 {
			t.Fatalf("days = %#v, want complete day array", document["days"])
		}
	}

	tripResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "journeyin.get_trip", Arguments: map[string]any{"trip_id": record.ID}})
	if err != nil {
		t.Fatalf("get trip: %v", err)
	}
	assertDocument(t, tripResult)

	historyResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "journeyin.get_trip_history", Arguments: map[string]any{"trip_id": record.ID, "history_id": history.ID}})
	if err != nil {
		t.Fatalf("get trip history: %v", err)
	}
	assertDocument(t, historyResult)
}
