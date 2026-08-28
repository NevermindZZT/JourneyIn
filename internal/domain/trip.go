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
	if strings.TrimSpace(t.Title) == "" {
		add("title", "required", "title is required", "error")
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
	start, startErr := time.Parse("2006-01-02", t.DateRange.Start)
	end, endErr := time.Parse("2006-01-02", t.DateRange.End)
	if startErr != nil {
		add("date_range.start", "date", "start must use YYYY-MM-DD", "error")
	}
	if endErr != nil {
		add("date_range.end", "date", "end must use YYYY-MM-DD", "error")
	}
	if startErr == nil && endErr == nil && end.Before(start) {
		add("date_range", "order", "end must not be before start", "error")
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
	}
	validateLinks("links", t.Links, add)
	return issues
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
