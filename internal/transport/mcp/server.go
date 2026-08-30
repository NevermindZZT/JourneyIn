package mcptransport

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"journeyin/internal/application"
	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

type Server struct {
	app     *application.TripService
	version string
	schema  fs.FS
	mcp     *mcp.Server
}

type ValidateArgs struct {
	TripJSON string `json:"trip_json"`
}
type PreviewArgs struct {
	TripJSON         string `json:"trip_json"`
	Operation        string `json:"operation"`
	TargetTripID     string `json:"target_trip_id,omitempty"`
	ExpectedRevision int    `json:"expected_revision,omitempty"`
}
type CommitArgs struct {
	PreviewID         string `json:"preview_id"`
	ConfirmationToken string `json:"confirmation_token"`
	IdempotencyKey    string `json:"idempotency_key"`
	ExpectedRevision  int    `json:"expected_revision,omitempty"`
}
type GetArgs struct {
	TripID string `json:"trip_id"`
}
type PlanArgs struct {
	TripID           string                 `json:"trip_id"`
	ExpectedRevision int                    `json:"expected_revision"`
	Provider         journeymaps.ProviderID `json:"provider,omitempty"`
	Mode             journeymaps.TravelMode `json:"mode,omitempty"`
	DayID            string                 `json:"day_id,omitempty"`
	Strategy         string                 `json:"strategy,omitempty"`
	AlternativeRoute int                    `json:"alternative_route,omitempty"`
}
type ListArgs struct {
	Limit int `json:"limit,omitempty"`
}

type ValidateOutput struct {
	Valid    bool                     `json:"valid"`
	TripJSON string                   `json:"trip_json,omitempty"`
	Summary  map[string]any           `json:"summary,omitempty"`
	Issues   []domain.ValidationIssue `json:"issues,omitempty"`
}

type ListOutput struct {
	Items []map[string]any `json:"items"`
}

type GetOutput struct {
	TripID   string          `json:"trip_id"`
	Revision int             `json:"revision"`
	Document json.RawMessage `json:"document"`
}
type PlanOutput struct {
	TripID          string `json:"trip_id"`
	Revision        int    `json:"revision"`
	PlannedSegments int    `json:"planned_segments"`
}

func NewServer(app *application.TripService, version string, schema fs.FS) *Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "journeyin", Version: version}, nil)
	result := &Server{app: app, version: version, schema: schema, mcp: server}
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.get_capabilities", Description: "Return JourneyIn capabilities and supported schema versions."}, result.getCapabilities)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.validate_trip", Description: "Validate a JourneyIn Trip JSON document without writing it."}, result.validateTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.preview_save_trip", Description: "Create a short-lived preview for creating or replacing a trip; does not commit a revision."}, result.previewSaveTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.commit_save_trip", Description: "Commit a user-confirmed JourneyIn trip preview with idempotency protection."}, result.commitSaveTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.plan_trip", Description: "Generate saved routes for adjacent planning points and persist route snapshots."}, result.planTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.refresh_routes", Description: "Recalculate routes for a selected Provider, mode, and optional Day without changing Stop order."}, result.planTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.get_trip", Description: "Read one JourneyIn trip by ID."}, result.getTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.list_trips", Description: "List JourneyIn trips visible to the current connection."}, result.listTrips)
	result.registerResources()
	return result
}

func (s *Server) HTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server { return s.mcp }, nil)
}
func (s *Server) RunStdio(ctx context.Context) error { return s.mcp.Run(ctx, &mcp.StdioTransport{}) }

func (s *Server) getCapabilities(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, map[string]any, error) {
	return nil, map[string]any{"version": s.version, "schema_versions": []int{1}, "tools": []string{"journeyin.get_capabilities", "journeyin.validate_trip", "journeyin.preview_save_trip", "journeyin.commit_save_trip", "journeyin.plan_trip", "journeyin.refresh_routes", "journeyin.get_trip", "journeyin.list_trips"}, "resources": []string{"journeyin://schema/trip/v1", "journeyin://trips/{trip_id}"}, "map_providers": []string{"baidu", "amap"}}, nil
}

func (s *Server) validateTrip(ctx context.Context, req *mcp.CallToolRequest, input ValidateArgs) (*mcp.CallToolResult, ValidateOutput, error) {
	normalized, trip, issues, err := s.app.Validate([]byte(input.TripJSON))
	if err != nil {
		return nil, ValidateOutput{}, err
	}
	output := ValidateOutput{Valid: !hasErrors(issues), TripJSON: string(normalized), Summary: trip.Summary(), Issues: issues}
	return nil, output, nil
}

func (s *Server) previewSaveTrip(ctx context.Context, req *mcp.CallToolRequest, input PreviewArgs) (*mcp.CallToolResult, application.PreviewResult, error) {
	result, err := s.app.PreviewSave(ctx, []byte(input.TripJSON), input.Operation, input.TargetTripID, input.ExpectedRevision, "mcp")
	return nil, result, err
}

func (s *Server) commitSaveTrip(ctx context.Context, req *mcp.CallToolRequest, input CommitArgs) (*mcp.CallToolResult, application.CommitResult, error) {
	result, err := s.app.CommitSave(ctx, input.PreviewID, input.ConfirmationToken, input.IdempotencyKey, input.ExpectedRevision, "mcp")
	return nil, result, err
}

func (s *Server) planTrip(ctx context.Context, req *mcp.CallToolRequest, input PlanArgs) (*mcp.CallToolResult, PlanOutput, error) {
	record, err := s.app.PlanTrip(ctx, input.TripID, input.ExpectedRevision, application.PlanInput{Provider: input.Provider, Mode: input.Mode, DayID: input.DayID, Strategy: input.Strategy, AlternativeRoute: input.AlternativeRoute}, "mcp:plan_trip")
	if err != nil {
		return nil, PlanOutput{}, err
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return nil, PlanOutput{}, err
	}
	segments := 0
	for _, day := range trip.Days {
		segments += len(day.Legs)
	}
	return nil, PlanOutput{TripID: record.ID, Revision: record.Revision, PlannedSegments: segments}, nil
}

func (s *Server) getTrip(ctx context.Context, req *mcp.CallToolRequest, input GetArgs) (*mcp.CallToolResult, GetOutput, error) {
	record, err := s.app.Get(ctx, input.TripID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, GetOutput{}, errors.New("trip not found")
	}
	if err != nil {
		return nil, GetOutput{}, err
	}
	return nil, GetOutput{TripID: record.ID, Revision: record.Revision, Document: json.RawMessage(record.Document)}, nil
}

func (s *Server) listTrips(ctx context.Context, req *mcp.CallToolRequest, input ListArgs) (*mcp.CallToolResult, ListOutput, error) {
	records, err := s.app.List(ctx, input.Limit)
	if err != nil {
		return nil, ListOutput{}, err
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, map[string]any{"id": record.ID, "title": record.Title, "status": record.Status, "start_date": record.StartDate, "end_date": record.EndDate, "revision": record.Revision})
	}
	return nil, ListOutput{Items: items}, nil
}

func RequireBearer(next http.Handler, token string) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimSpace(r.Header.Get("Authorization"))
		if value != "Bearer "+token {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func hasErrors(issues []domain.ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}
