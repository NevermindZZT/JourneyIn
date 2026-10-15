package mcptransport

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"journeyin/internal/application"
	"journeyin/internal/domain"
	"journeyin/internal/monitor"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/security"
	"journeyin/internal/store"
)

type Server struct {
	app            *application.TripService
	monitorService *monitor.Service
	version        string
	schema         fs.FS
	mcp            *mcp.Server
}

func (s *Server) SetMonitorService(svc *monitor.Service) {
	s.monitorService = svc
}

type ValidateArgs struct {
	TripJSON string `json:"trip_json"`
}
type PreviewArgs struct {
	TripJSON         string                 `json:"trip_json,omitempty"`
	Operation        string                 `json:"operation"`
	TargetTripID     string                 `json:"target_trip_id,omitempty"`
	ExpectedRevision int                    `json:"expected_revision,omitempty"`
	Patch            application.MergePatch `json:"patch,omitempty"`
	patchProvided    bool
}

func (p *PreviewArgs) UnmarshalJSON(data []byte) error {
	type plain PreviewArgs
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("preview arguments must be a JSON object")
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = PreviewArgs(decoded)
	_, p.patchProvided = raw["patch"]
	return nil
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
type HistoryListArgs struct {
	TripID string `json:"trip_id"`
	Limit  int    `json:"limit,omitempty"`
}
type HistoryGetArgs struct {
	TripID    string `json:"trip_id"`
	HistoryID string `json:"history_id"`
}
type CreateMonitorArgs struct {
	PlaceName      string   `json:"place_name"`
	Title          string   `json:"title,omitempty"`
	Lat            float64  `json:"lat"`
	Lng            float64  `json:"lng"`
	CRS            string   `json:"crs,omitempty"`
	Months         []int    `json:"months,omitempty"`
	StargazingMode bool     `json:"stargazing_mode,omitempty"`
	MaxLeaveDays   int      `json:"max_leave_days,omitempty"`
	MaxCloudCover  int      `json:"max_cloud_cover,omitempty"`
	MaxRainProb    int      `json:"max_rain_prob,omitempty"`
	MinDurationDays int     `json:"min_duration_days,omitempty"`
	MaxDurationDays int     `json:"max_duration_days,omitempty"`
	OriginCity     string   `json:"origin_city,omitempty"`
	MaxBudget      float64  `json:"max_budget,omitempty"`
}

type ListMonitorsArgs struct {
	EnabledOnly bool `json:"enabled_only,omitempty"`
}

type GetMonitorArgs struct {
	MonitorID string `json:"monitor_id"`
}

type ConvertMonitorTripArgs struct {
	MonitorID   string `json:"monitor_id"`
	WindowStart string `json:"window_start"`
	WindowEnd   string `json:"window_end"`
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
	TripID   string         `json:"trip_id"`
	Revision int            `json:"revision"`
	Document map[string]any `json:"document"`
}
type HistoryListOutput struct {
	Items []map[string]any `json:"items"`
}
type HistoryGetOutput struct {
	TripID         string         `json:"trip_id"`
	HistoryID      string         `json:"history_id"`
	SourceRevision int            `json:"source_revision"`
	Label          string         `json:"label,omitempty"`
	CreatedAt      string         `json:"created_at"`
	Document       map[string]any `json:"document"`
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
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.preview_save_trip", Description: "Create a short-lived preview for creating, replacing, or safely merging a trip. Merge supports Trip description and link operations, Day.title and notes_markdown, and main or child Stop details (title, address, kind, time_window arrival/departure, description_markdown, and links) addressed by stable IDs. Omit links fields when unchanged; a supplied links patch must contain a non-empty add or remove_ids operation. Merge preserves map, locations, weather, legs, route snapshots, and geometry. It does not commit a revision."}, result.previewSaveTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.commit_save_trip", Description: "Commit a user-confirmed JourneyIn trip preview with idempotency protection."}, result.commitSaveTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.plan_trip", Description: "Generate saved routes for adjacent planning points and persist route snapshots."}, result.planTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.refresh_routes", Description: "Recalculate routes for a selected Provider, mode, and optional Day without changing Stop order."}, result.planTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.get_trip", Description: "Read one complete JourneyIn Trip object by ID, including Markdown, days, stops, legs, links, map data, and current revision."}, result.getTrip)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.list_trips", Description: "List JourneyIn trips visible to the current connection."}, result.listTrips)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.list_trip_history", Description: "List user-saved read-only history versions for a JourneyIn trip."}, result.listTripHistory)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.get_trip_history", Description: "Read one complete immutable saved JourneyIn Trip history snapshot, including Markdown, days, stops, legs, links, and map data."}, result.getTripHistory)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.create_attraction_monitor", Description: "Create a new attraction opportunity monitor rule with target place name, coordinates, seasonality months, stargazing mode, and holiday constraints. The system automatically monitors future calendar and weather windows to recommend the optimal trip timing."}, result.createAttractionMonitor)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.list_attraction_monitors", Description: "List all monitored attraction rules and their latest opportunity evaluations."}, result.listAttractionMonitors)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.get_attraction_monitor", Description: "Get details of an attraction monitor rule, including its upcoming 45-day evaluation timeline of candidate travel windows (lunar phase, weather, holidays, and scores)."}, result.getAttractionMonitor)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.get_radar_opportunities", Description: "Get travel radar summary and currently recommended optimal travel windows across all monitored attractions."}, result.getRadarOpportunities)
	mcp.AddTool(server, &mcp.Tool{Name: "journeyin.convert_monitor_to_trip", Description: "Convert a recommended travel window from an attraction monitor directly into a new JourneyIn Trip draft with dates and starting location."}, result.convertMonitorToTrip)
	result.registerResources()
	return result
}

func (s *Server) HTTPHandler() http.Handler {
	return mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server { return s.mcp }, nil)
}
func (s *Server) RunStdio(ctx context.Context) error { return s.mcp.Run(ctx, &mcp.StdioTransport{}) }

func (s *Server) getCapabilities(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, map[string]any, error) {
	return nil, map[string]any{"version": s.version, "schema_versions": []int{1}, "features": map[string]any{"preview_merge": true, "merge_patch_version": 3}, "tools": []string{"journeyin.get_capabilities", "journeyin.validate_trip", "journeyin.preview_save_trip", "journeyin.commit_save_trip", "journeyin.plan_trip", "journeyin.refresh_routes", "journeyin.get_trip", "journeyin.list_trips", "journeyin.list_trip_history", "journeyin.get_trip_history", "journeyin.create_attraction_monitor", "journeyin.list_attraction_monitors", "journeyin.get_attraction_monitor", "journeyin.get_radar_opportunities", "journeyin.convert_monitor_to_trip"}, "resources": []string{"journeyin://schema/trip/v1", "journeyin://trips/{trip_id}", "journeyin://trips/{trip_id}/history/{history_id}"}, "map_providers": []string{"baidu", "amap"}}, nil
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
	if input.Operation == "merge" {
		if !input.patchProvided && input.Patch.IsEmpty() {
			return nil, application.PreviewResult{}, errors.New("patch is required for merge")
		}
		if strings.TrimSpace(input.TripJSON) != "" {
			return nil, application.PreviewResult{}, errors.New("trip_json must not be provided with operation merge")
		}
		result, err := s.app.PreviewMerge(ctx, input.Patch, input.TargetTripID, input.ExpectedRevision, "mcp")
		return nil, result, err
	}
	if input.patchProvided || !input.Patch.IsEmpty() {
		return nil, application.PreviewResult{}, errors.New("patch is only supported with operation merge")
	}
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

func (s *Server) listTripHistory(ctx context.Context, req *mcp.CallToolRequest, input HistoryListArgs) (*mcp.CallToolResult, HistoryListOutput, error) {
	records, err := s.app.ListTripVersions(ctx, input.TripID, input.Limit)
	if err != nil {
		return nil, HistoryListOutput{}, err
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, map[string]any{"history_id": record.ID, "trip_id": record.TripID, "source_revision": record.SourceRevision, "title": record.Title, "start_date": record.StartDate, "end_date": record.EndDate, "label": record.Label, "content_hash": record.ContentHash, "created_at": record.CreatedAt})
	}
	return nil, HistoryListOutput{Items: items}, nil
}

func (s *Server) getTripHistory(ctx context.Context, req *mcp.CallToolRequest, input HistoryGetArgs) (*mcp.CallToolResult, HistoryGetOutput, error) {
	record, err := s.app.GetTripVersion(ctx, input.TripID, input.HistoryID)
	if err != nil {
		return nil, HistoryGetOutput{}, err
	}
	document, err := decodeTripDocument(record.Document)
	if err != nil {
		return nil, HistoryGetOutput{}, err
	}
	return nil, HistoryGetOutput{TripID: record.TripID, HistoryID: record.ID, SourceRevision: record.SourceRevision, Label: record.Label, CreatedAt: record.CreatedAt.Format(time.RFC3339Nano), Document: document}, nil
}

func (s *Server) getTrip(ctx context.Context, req *mcp.CallToolRequest, input GetArgs) (*mcp.CallToolResult, GetOutput, error) {
	record, err := s.app.Get(ctx, input.TripID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, GetOutput{}, errors.New("trip not found")
	}
	if err != nil {
		return nil, GetOutput{}, err
	}
	document, err := decodeTripDocument(record.Document)
	if err != nil {
		return nil, GetOutput{}, err
	}
	return nil, GetOutput{TripID: record.ID, Revision: record.Revision, Document: document}, nil
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

type BearerPrincipalResolver func(context.Context, string) (security.Principal, error)

func RequireBearer(next http.Handler, token string) http.Handler {
	return RequireBearerWithResolver(next, token, nil)
}

func RequireBearerWithResolver(next http.Handler, token string, resolver BearerPrincipalResolver) http.Handler {
	return requireBearerWithResolver(next, token, resolver, false)
}

// RequireOptionalBearerWithResolver accepts anonymous local calls when no token is supplied,
// while still resolving explicit global or user-scoped Bearer tokens.
func RequireOptionalBearerWithResolver(next http.Handler, token string, resolver BearerPrincipalResolver) http.Handler {
	return requireBearerWithResolver(next, token, resolver, true)
}

func requireBearerWithResolver(next http.Handler, token string, resolver BearerPrincipalResolver, allowAnonymous bool) http.Handler {
	if !allowAnonymous && strings.TrimSpace(token) == "" && resolver == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimSpace(r.Header.Get("Authorization"))
		if value == "" && allowAnonymous {
			r = r.WithContext(security.WithPrincipal(r.Context(), security.SystemPrincipal()))
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(value, "Bearer ") {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		raw := strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
		var principal security.Principal
		if raw != "" && token != "" && subtle.ConstantTimeCompare([]byte(raw), []byte(token)) == 1 {
			principal = security.SystemPrincipal()
		} else if raw != "" && resolver != nil {
			resolved, err := resolver(r.Context(), raw)
			if err == nil {
				principal = resolved
			} else {
				w.Header().Set("WWW-Authenticate", "Bearer")
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		} else {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(security.WithPrincipal(r.Context(), principal))
		next.ServeHTTP(w, r)
	})
}

func decodeTripDocument(data []byte) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document == nil {
		return nil, errors.New("trip document must be a JSON object")
	}
	return document, nil
}

func hasErrors(issues []domain.ValidationIssue) bool {
	for _, issue := range issues {
		if issue.Level == "error" {
			return true
		}
	}
	return false
}

func (s *Server) createAttractionMonitor(ctx context.Context, req *mcp.CallToolRequest, input CreateMonitorArgs) (*mcp.CallToolResult, map[string]any, error) {
	if s.monitorService == nil {
		return nil, nil, errors.New("monitor service is not configured")
	}
	place := strings.TrimSpace(input.PlaceName)
	if place == "" {
		return nil, nil, errors.New("place_name is required")
	}
	if input.Lat == 0 && input.Lng == 0 {
		return nil, nil, errors.New("valid coordinates (lat, lng) are required")
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = place
	}
	crs := strings.TrimSpace(input.CRS)
	if crs == "" {
		crs = "gcj02"
	}

	m, err := s.monitorService.CreateMonitor(ctx, monitor.Monitor{
		Title:     title,
		PlaceName: place,
		Lat:       input.Lat,
		Lng:       input.Lng,
		CRS:       crs,
		Criteria: monitor.MonitorCriteria{
			Months:          input.Months,
			StargazingMode:  input.StargazingMode,
			MaxLeaveDays:    input.MaxLeaveDays,
			MaxCloudCover:   input.MaxCloudCover,
			MaxRainProb:     input.MaxRainProb,
			MinDurationDays: input.MinDurationDays,
			MaxDurationDays: input.MaxDurationDays,
		},
		OriginCity: strings.TrimSpace(input.OriginCity),
		MaxBudget:  input.MaxBudget,
		Enabled:    true,
	})
	if err != nil {
		return nil, nil, err
	}

	return nil, map[string]any{
		"status":      "created",
		"monitor_id":  m.ID,
		"title":       m.Title,
		"place_name":  m.PlaceName,
		"lat":         m.Lat,
		"lng":         m.Lng,
		"crs":         m.CRS,
		"criteria":    m.Criteria,
		"message":     "Attraction monitor created successfully. System will automatically evaluate future calendar and weather windows.",
	}, nil
}

func (s *Server) listAttractionMonitors(ctx context.Context, req *mcp.CallToolRequest, input ListMonitorsArgs) (*mcp.CallToolResult, map[string]any, error) {
	if s.monitorService == nil {
		return nil, nil, errors.New("monitor service is not configured")
	}
	monitors, err := s.monitorService.ListMonitors(ctx, input.EnabledOnly)
	if err != nil {
		return nil, nil, err
	}
	items := make([]map[string]any, 0, len(monitors))
	for _, m := range monitors {
		item := map[string]any{
			"id":          m.ID,
			"title":       m.Title,
			"place_name":  m.PlaceName,
			"lat":         m.Lat,
			"lng":         m.Lng,
			"crs":         m.CRS,
			"criteria":    m.Criteria,
			"enabled":     m.Enabled,
			"origin_city": m.OriginCity,
			"max_budget":  m.MaxBudget,
		}
		if m.LatestEvaluation != nil {
			item["latest_opportunity"] = map[string]any{
				"window_start": m.LatestEvaluation.WindowStart,
				"window_end":   m.LatestEvaluation.WindowEnd,
				"score":        m.LatestEvaluation.Score,
				"level":        m.LatestEvaluation.Level,
				"summary":      m.LatestEvaluation.Details.Summary,
			}
		}
		items = append(items, item)
	}
	return nil, map[string]any{
		"total":    len(items),
		"monitors": items,
	}, nil
}

func (s *Server) getAttractionMonitor(ctx context.Context, req *mcp.CallToolRequest, input GetMonitorArgs) (*mcp.CallToolResult, map[string]any, error) {
	if s.monitorService == nil {
		return nil, nil, errors.New("monitor service is not configured")
	}
	id := strings.TrimSpace(input.MonitorID)
	if id == "" {
		return nil, nil, errors.New("monitor_id is required")
	}
	m, err := s.monitorService.GetMonitor(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	evals, err := s.monitorService.GetEvaluations(ctx, id, 30)
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{
		"monitor":     m,
		"evaluations": evals,
	}, nil
}

func (s *Server) getRadarOpportunities(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, map[string]any, error) {
	if s.monitorService == nil {
		return nil, nil, errors.New("monitor service is not configured")
	}
	summary, err := s.monitorService.GetRadarSummary(ctx)
	if err != nil {
		return nil, nil, err
	}
	monitors, err := s.monitorService.ListMonitors(ctx, true)
	if err != nil {
		return nil, nil, err
	}
	var recommended []map[string]any
	for _, m := range monitors {
		if m.LatestEvaluation != nil && m.LatestEvaluation.Level == monitor.LevelStronglyRecommended {
			recommended = append(recommended, map[string]any{
				"monitor_id":   m.ID,
				"place_name":   m.PlaceName,
				"title":        m.Title,
				"window_start": m.LatestEvaluation.WindowStart,
				"window_end":   m.LatestEvaluation.WindowEnd,
				"score":        m.LatestEvaluation.Score,
				"summary":      m.LatestEvaluation.Details.Summary,
				"moon_phase":   m.LatestEvaluation.Details.MoonPhaseName,
				"moon_illum":   m.LatestEvaluation.Details.MoonIllumination,
				"galactic_h":   m.LatestEvaluation.Details.GalacticHours,
				"weather":      m.LatestEvaluation.Details.WeatherCondition,
			})
		}
	}
	return nil, map[string]any{
		"total_monitors":       summary.TotalMonitors,
		"recommended_count":    summary.RecommendedCount,
		"top_place_name":       summary.TopPlaceName,
		"top_opportunity":      summary.TopOpportunity,
		"recommended_monitors": recommended,
	}, nil
}

func (s *Server) convertMonitorToTrip(ctx context.Context, req *mcp.CallToolRequest, input ConvertMonitorTripArgs) (*mcp.CallToolResult, map[string]any, error) {
	if s.monitorService == nil || s.app == nil {
		return nil, nil, errors.New("monitor service or trip service is not configured")
	}
	id := strings.TrimSpace(input.MonitorID)
	if id == "" {
		return nil, nil, errors.New("monitor_id is required")
	}
	if input.WindowStart == "" || input.WindowEnd == "" {
		return nil, nil, errors.New("window_start and window_end are required (YYYY-MM-DD)")
	}

	tripID, err := s.monitorService.ConvertToTrip(ctx, id, input.WindowStart, input.WindowEnd, func(ctx context.Context, doc []byte, src string) (string, error) {
		record, err := s.app.Create(ctx, doc, "mcp:convert_monitor")
		if err != nil {
			return "", err
		}
		return record.ID, nil
	})
	if err != nil {
		return nil, nil, err
	}

	return nil, map[string]any{
		"status":       "created",
		"trip_id":      tripID,
		"monitor_id":   id,
		"window_start": input.WindowStart,
		"window_end":   input.WindowEnd,
		"message":      "Successfully converted recommended opportunity window into a new JourneyIn Trip draft.",
	}, nil
}
