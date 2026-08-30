package domain

import "testing"

func TestValidateMultiProviderRouteSnapshots(t *testing.T) {
	trip := Trip{
		SchemaVersion: 1,
		Title:         "multi",
		Status:        "draft",
		Timezone:      "Asia/Shanghai",
		DateRange:     DateRange{Start: "2026-04-18", End: "2026-04-18"},
		Map:           MapConfig{PreferredProvider: "amap", EnabledProviders: []string{"baidu", "amap"}, DefaultMode: "walking"},
		Days: []Day{{
			ID:   "day-1",
			Date: "2026-04-18",
			Stops: []Stop{
				{ID: "a", Sequence: 1, Title: "A"},
				{ID: "b", Sequence: 2, Title: "B"},
			},
			Legs: []RouteLeg{{
				ID: "leg-1", FromStopID: "a", ToStopID: "b", Mode: "walking",
				Snapshots: []RouteSnapshot{
					{Provider: "baidu", CoordinateSystem: "bd09ll", Mode: "walking", Geometry: [][]float64{{120, 30}, {120.1, 30.1}}},
					{Provider: "amap", CoordinateSystem: "gcj02", Mode: "walking", Geometry: [][]float64{{120, 30}, {120.1, 30.1}}},
				},
			}},
		}},
	}
	for _, issue := range trip.Validate() {
		if issue.Level == "error" {
			t.Fatalf("unexpected validation error: %+v", issue)
		}
	}
}

func TestValidateRejectsInvalidRouteGeometry(t *testing.T) {
	trip := Trip{
		SchemaVersion: 1,
		Title:         "invalid",
		Status:        "draft",
		Timezone:      "Asia/Shanghai",
		DateRange:     DateRange{Start: "2026-04-18", End: "2026-04-18"},
		Days: []Day{{
			ID: "day-1", Date: "2026-04-18",
			Stops: []Stop{{ID: "a", Sequence: 1, Title: "A"}, {ID: "b", Sequence: 2, Title: "B"}},
			Legs: []RouteLeg{{
				ID: "leg-1", FromStopID: "a", ToStopID: "b",
				Snapshots: []RouteSnapshot{{Provider: "amap", CoordinateSystem: "gcj02", Geometry: [][]float64{{120}}}},
			}},
		}},
	}
	issues := trip.Validate()
	found := false
	for _, issue := range issues {
		if issue.Code == "shape" && issue.Level == "error" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected invalid geometry issue, got %+v", issues)
	}
}
