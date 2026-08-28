package domain

import (
	"encoding/json"
	"testing"
)

func TestNormalizeTripGeneratesIDAndPreservesValidity(t *testing.T) {
	input := []byte(`{
      "schema_version": 1,
      "title": "杭州周末行",
      "status": "draft",
      "timezone": "Asia/Shanghai",
      "date_range": {"start":"2026-04-18","end":"2026-04-19"},
      "days": [{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"西湖断桥"}]}]
    }`)
	normalized, trip, issues, err := NormalizeTrip(input)
	if err != nil {
		t.Fatal(err)
	}
	if trip.ID == "" {
		t.Fatal("expected generated trip id")
	}
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	var decoded map[string]any
	if err := json.Unmarshal(normalized, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["id"] == nil {
		t.Fatal("normalized JSON did not include generated id")
	}
}

func TestTripValidationRejectsInvalidLinksAndDates(t *testing.T) {
	input := []byte(`{
      "schema_version": 1,
      "title": "测试",
      "status": "draft",
      "timezone": "Asia/Shanghai",
      "date_range": {"start":"2026-04-20","end":"2026-04-19"},
      "links": [{"title":"坏链接","url":"javascript:alert(1)"}],
      "days": [{"id":"day-1","date":"2026-04-20","stops":[]}]
    }`)
	_, _, issues, err := NormalizeTrip(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) < 2 {
		t.Fatalf("expected validation issues, got %+v", issues)
	}
	foundOrder, foundURL := false, false
	for _, issue := range issues {
		if issue.Code == "order" {
			foundOrder = true
		}
		if issue.Code == "url" {
			foundURL = true
		}
	}
	if !foundOrder || !foundURL {
		t.Fatalf("expected date order and URL errors, got %+v", issues)
	}
}
