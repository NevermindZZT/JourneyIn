package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

type Trip struct {
	SchemaVersion       int       `json:"schema_version"`
	ID                  string    `json:"id,omitempty"`
	Title               string    `json:"title"`
	Status              string    `json:"status"`
	Locale              string    `json:"locale,omitempty"`
	Timezone            string    `json:"timezone"`
	DateRange           DateRange `json:"date_range"`
	DescriptionMarkdown string    `json:"description_markdown,omitempty"`
	Links               []Link    `json:"links,omitempty"`
	Map                 MapConfig `json:"map,omitempty"`
	Days                []Day     `json:"days"`
	Metadata            Metadata  `json:"metadata,omitempty"`
}

type DateRange struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type Link struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title"`
	URL   string `json:"url"`
	Kind  string `json:"kind,omitempty"`
}

type MapConfig struct {
	PreferredProvider string   `json:"preferred_provider,omitempty"`
	EnabledProviders  []string `json:"enabled_providers,omitempty"`
	DefaultMode       string   `json:"default_mode,omitempty"`
}

type Day struct {
	ID            string     `json:"id"`
	Date          string     `json:"date"`
	Title         string     `json:"title,omitempty"`
	NotesMarkdown string     `json:"notes_markdown,omitempty"`
	Stops         []Stop     `json:"stops"`
	Legs          []RouteLeg `json:"legs,omitempty"`
}

type Stop struct {
	ID                  string          `json:"id"`
	Sequence            int             `json:"sequence"`
	Kind                string          `json:"kind,omitempty"`
	Title               string          `json:"title"`
	Address             string          `json:"address,omitempty"`
	Location            json.RawMessage `json:"location,omitempty"`
	TimeWindow          json.RawMessage `json:"time_window,omitempty"`
	DescriptionMarkdown string          `json:"description_markdown,omitempty"`
	Links               []Link          `json:"links,omitempty"`
	Weather             json.RawMessage `json:"weather,omitempty"`
	Children            []SubStop       `json:"children,omitempty"`
}

type SubStop struct {
	ID                  string          `json:"id"`
	Sequence            int             `json:"sequence"`
	Kind                string          `json:"kind,omitempty"`
	Title               string          `json:"title"`
	Address             string          `json:"address,omitempty"`
	Location            json.RawMessage `json:"location,omitempty"`
	TimeWindow          json.RawMessage `json:"time_window,omitempty"`
	DescriptionMarkdown string          `json:"description_markdown,omitempty"`
	Links               []Link          `json:"links,omitempty"`
	Weather             json.RawMessage `json:"weather,omitempty"`
}

type RouteLeg struct {
	ID         string          `json:"id"`
	FromStopID string          `json:"from_stop_id"`
	ToStopID   string          `json:"to_stop_id"`
	Mode       string          `json:"mode,omitempty"`
	Snapshots  []RouteSnapshot `json:"snapshots,omitempty"`
}

type RouteSnapshot struct {
	Provider         string      `json:"provider"`
	CoordinateSystem string      `json:"coordinate_system"`
	Mode             string      `json:"mode,omitempty"`
	Strategy         string      `json:"strategy,omitempty"`
	Source           string      `json:"source,omitempty"`
	Geometry         [][]float64 `json:"geometry,omitempty"`
	DistanceM        int         `json:"distance_m,omitempty"`
	DurationS        int         `json:"duration_s,omitempty"`
	FetchedAt        string      `json:"fetched_at,omitempty"`
	ExpiresAt        string      `json:"expires_at,omitempty"`
}

type Metadata struct {
	Source    string `json:"source,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type ValidationIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Level   string `json:"level"`
}

func NewID(prefix string) (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + "_" + hex.EncodeToString(raw), nil
}

func ContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func NormalizeTrip(data []byte) ([]byte, Trip, []ValidationIssue, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, Trip{}, nil, fmt.Errorf("invalid trip JSON: %w", err)
	}
	if raw == nil {
		return nil, Trip{}, nil, errors.New("trip JSON must be an object")
	}
	if _, ok := raw["id"]; !ok {
		id, err := NewID("trip")
		if err != nil {
			return nil, Trip{}, nil, err
		}
		encoded, _ := json.Marshal(id)
		raw["id"] = encoded
		data, err = json.Marshal(raw)
		if err != nil {
			return nil, Trip{}, nil, err
		}
	}
	var trip Trip
	if err := json.Unmarshal(data, &trip); err != nil {
		return nil, Trip{}, nil, fmt.Errorf("decode trip: %w", err)
	}
	issues := trip.Validate()
	return data, trip, issues, nil
}

func NormalizeTripForID(data []byte, id string) ([]byte, Trip, []ValidationIssue, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, Trip{}, nil, fmt.Errorf("invalid trip JSON: %w", err)
	}
	if raw == nil {
		return nil, Trip{}, nil, errors.New("trip JSON must be an object")
	}
	encoded, err := json.Marshal(id)
	if err != nil {
		return nil, Trip{}, nil, err
	}
	raw["id"] = encoded
	normalized, err := json.Marshal(raw)
	if err != nil {
		return nil, Trip{}, nil, err
	}
	return NormalizeTrip(normalized)
}

func (t Trip) Validate() []ValidationIssue {
	var issues []ValidationIssue
	add := func(path, code, message, level string) {
		issues = append(issues, ValidationIssue{Path: path, Code: code, Message: message, Level: level})
	}
	if t.SchemaVersion != 1 {
		add("schema_version", "unsupported", "schema_version must be 1", "error")
	}
	title := strings.TrimSpace(t.Title)
	if title == "" {
		add("title", "required", "title is required", "error")
	} else if utf8.RuneCountInString(title) > 120 {
		add("title", "limit", "title must be at most 120 characters", "error")
	}
	if t.Status == "" {
		add("status", "required", "status is required", "error")
	} else if t.Status != "draft" && t.Status != "published" && t.Status != "archived" {
		add("status", "enum", "status must be draft, published, or archived", "error")
	}
	if t.Timezone == "" {
		add("timezone", "required", "timezone is required", "error")
	} else if _, err := time.LoadLocation(t.Timezone); err != nil {
		add("timezone", "invalid", "timezone must be a valid IANA timezone", "error")
	}
	validateMapConfig(t.Map, add)
	start, startErr := time.Parse("2006-01-02", t.DateRange.Start)
	end, endErr := time.Parse("2006-01-02", t.DateRange.End)
	if startErr != nil {
		add("date_range.start", "date", "start must use YYYY-MM-DD", "error")
	}
	if endErr != nil {
		add("date_range.end", "date", "end must use YYYY-MM-DD", "error")
	}
	if startErr == nil && endErr == nil {
		if end.Before(start) {
			add("date_range", "order", "end must not be before start", "error")
		} else if int(end.Sub(start)/(24*time.Hour))+1 > 60 {
			add("date_range", "limit", "date range cannot exceed 60 days", "error")
		}
	}
	if len(t.Days) == 0 {
		add("days", "required", "at least one day is required", "error")
	}
	if len(t.Days) > 60 {
		add("days", "limit", "at most 60 days are supported", "error")
	}
	for i, day := range t.Days {
		path := fmt.Sprintf("days[%d]", i)
		if day.ID == "" {
			add(path+".id", "required", "day id is required", "error")
		}
		dayDate, err := time.Parse("2006-01-02", day.Date)
		if err != nil {
			add(path+".date", "date", "day date must use YYYY-MM-DD", "error")
		} else if startErr == nil && endErr == nil && (dayDate.Before(start) || dayDate.After(end)) {
			add(path+".date", "range", "day date must be inside date_range", "error")
		}
		if len(day.Stops) > 100 {
			add(path+".stops", "limit", "at most 100 stops are supported per day", "error")
		}
		seen := map[string]bool{}
		for j, stop := range day.Stops {
			stopPath := fmt.Sprintf("%s.stops[%d]", path, j)
			if stop.ID == "" {
				add(stopPath+".id", "required", "stop id is required", "error")
			} else if seen[stop.ID] {
				add(stopPath+".id", "duplicate", "stop id must be unique within a day", "error")
			}
			seen[stop.ID] = true
			if strings.TrimSpace(stop.Title) == "" {
				add(stopPath+".title", "required", "stop title is required", "error")
			}
			if stop.Sequence < 1 {
				add(stopPath+".sequence", "range", "sequence must be positive", "error")
			}
			validateLinks(stopPath+".links", stop.Links, add)
		}
		validateRouteLegs(path+".legs", day.Legs, add)
	}
	validateLinks("links", t.Links, add)
	return issues
}

func validateMapConfig(config MapConfig, add func(string, string, string, string)) {
	if config.PreferredProvider != "" && config.PreferredProvider != "baidu" && config.PreferredProvider != "amap" {
		add("map.preferred_provider", "enum", "preferred_provider must be baidu or amap", "error")
	}
	seen := map[string]bool{}
	for index, provider := range config.EnabledProviders {
		path := fmt.Sprintf("map.enabled_providers[%d]", index)
		if provider != "baidu" && provider != "amap" {
			add(path, "enum", "enabled provider must be baidu or amap", "error")
		}
		if seen[provider] {
			add(path, "duplicate", "enabled providers must be unique", "error")
		}
		seen[provider] = true
	}
	if config.PreferredProvider != "" && len(config.EnabledProviders) > 0 && !seen[config.PreferredProvider] {
		add("map.preferred_provider", "disabled", "preferred provider is not enabled", "warning")
	}
	if config.DefaultMode != "" && !validRouteMode(config.DefaultMode) {
		add("map.default_mode", "enum", "default_mode must be driving, walking, cycling, or transit", "error")
	}
}

func validateRouteLegs(path string, legs []RouteLeg, add func(string, string, string, string)) {
	if len(legs) > 100 {
		add(path, "limit", "at most 100 route legs are supported per day", "error")
	}
	seenPairs := map[string]bool{}
	for index, leg := range legs {
		legPath := fmt.Sprintf("%s[%d]", path, index)
		if strings.TrimSpace(leg.ID) == "" {
			add(legPath+".id", "required", "route leg id is required", "error")
		}
		if strings.TrimSpace(leg.FromStopID) == "" {
			add(legPath+".from_stop_id", "required", "route leg origin is required", "error")
		}
		if strings.TrimSpace(leg.ToStopID) == "" {
			add(legPath+".to_stop_id", "required", "route leg destination is required", "error")
		}
		pair := leg.FromStopID + "\x00" + leg.ToStopID
		if seenPairs[pair] {
			add(legPath, "duplicate", "route leg endpoints must be unique within a day", "error")
		}
		seenPairs[pair] = true
		if leg.Mode != "" && !validRouteMode(leg.Mode) {
			add(legPath+".mode", "enum", "route leg mode is unsupported", "error")
		}
		if len(leg.Snapshots) > 8 {
			add(legPath+".snapshots", "limit", "at most 8 route snapshots are supported per leg", "error")
		}
		for snapshotIndex, snapshot := range leg.Snapshots {
			validateRouteSnapshot(fmt.Sprintf("%s.snapshots[%d]", legPath, snapshotIndex), snapshot, add)
		}
	}
}

func validateRouteSnapshot(path string, snapshot RouteSnapshot, add func(string, string, string, string)) {
	if strings.TrimSpace(snapshot.Provider) == "" {
		add(path+".provider", "required", "route snapshot provider is required", "error")
	}
	if snapshot.CoordinateSystem != "wgs84" && snapshot.CoordinateSystem != "gcj02" && snapshot.CoordinateSystem != "bd09ll" {
		add(path+".coordinate_system", "enum", "route snapshot coordinate_system is unsupported", "error")
	}
	if snapshot.Mode != "" && !validRouteMode(snapshot.Mode) {
		add(path+".mode", "enum", "route snapshot mode is unsupported", "error")
	}
	if snapshot.DistanceM < 0 {
		add(path+".distance_m", "range", "route snapshot distance_m must not be negative", "error")
	}
	if snapshot.DurationS < 0 {
		add(path+".duration_s", "range", "route snapshot duration_s must not be negative", "error")
	}
	if len(snapshot.Geometry) > 20000 {
		add(path+".geometry", "limit", "route snapshot geometry is too large", "error")
	}
	for pointIndex, point := range snapshot.Geometry {
		pointPath := fmt.Sprintf("%s.geometry[%d]", path, pointIndex)
		if len(point) != 2 {
			add(pointPath, "shape", "route geometry points must be [lng, lat]", "error")
			continue
		}
		if point[0] < -180 || point[0] > 180 || point[1] < -90 || point[1] > 90 {
			add(pointPath, "range", "route geometry coordinates are out of range", "error")
		}
	}
	var fetched, expires time.Time
	var fetchedErr, expiresErr error
	if snapshot.FetchedAt != "" {
		fetched, fetchedErr = time.Parse(time.RFC3339Nano, snapshot.FetchedAt)
		if fetchedErr != nil {
			add(path+".fetched_at", "datetime", "fetched_at must be RFC3339", "error")
		}
	}
	if snapshot.ExpiresAt != "" {
		expires, expiresErr = time.Parse(time.RFC3339Nano, snapshot.ExpiresAt)
		if expiresErr != nil {
			add(path+".expires_at", "datetime", "expires_at must be RFC3339", "error")
		}
	}
	if fetchedErr == nil && expiresErr == nil && !fetched.IsZero() && !expires.IsZero() && expires.Before(fetched) {
		add(path, "order", "route snapshot expires_at must not be before fetched_at", "warning")
	}
}

func validRouteMode(mode string) bool {
	switch mode {
	case "driving", "walking", "cycling", "transit":
		return true
	default:
		return false
	}
}

func validateLinks(path string, links []Link, add func(string, string, string, string)) {
	if len(links) > 100 {
		add(path, "limit", "at most 100 links are supported", "error")
	}
	for i, link := range links {
		linkPath := fmt.Sprintf("%s[%d]", path, i)
		parsed, err := url.Parse(link.URL)
		if strings.TrimSpace(link.Title) == "" {
			add(linkPath+".title", "required", "link title is required", "error")
		}
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			add(linkPath+".url", "url", "link URL must be an absolute http/https URL", "error")
		}
	}
}

func (t Trip) Summary() map[string]any {
	stops := 0
	legs := 0
	for _, day := range t.Days {
		stops += len(day.Stops)
		legs += len(day.Legs)
	}
	return map[string]any{
		"trip_id":    t.ID,
		"title":      t.Title,
		"status":     t.Status,
		"start_date": t.DateRange.Start,
		"end_date":   t.DateRange.End,
		"timezone":   t.Timezone,
		"days":       len(t.Days),
		"stops":      stops,
		"legs":       legs,
	}
}
