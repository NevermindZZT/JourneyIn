package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"journeyin/internal/store"
)

func TestUpdatePlanningPointPreservesRawFieldsAndInvalidatesMainRoute(t *testing.T) {
	service := testService(t)
	document := []byte(`{
    "schema_version":1,
    "title":"点编辑测试",
    "status":"draft",
    "timezone":"Asia/Shanghai",
    "date_range":{"start":"2026-04-18","end":"2026-04-20"},
    "days":[
      {"id":"day-1","date":"2026-04-18","day_extension":{"keep":true},"stops":[
        {"id":"stop-1","sequence":1,"title":"旧地点","address":"旧地址","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.1,"lng":120.1,"crs":"bd09ll"}}},"weather":{"source":"old-weather"},"links":[{"id":"link-1","title":"来源","url":"https://example.com/source","future_link_field":"keep"}],"stop_extension":{"keep":true},"children":[{"id":"child-1","sequence":1,"title":"子地点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.11,"lng":120.11,"crs":"bd09ll"}}},"weather":{"source":"child-weather"},"child_extension":{"keep":true}}]},
        {"id":"stop-2","sequence":2,"title":"终点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.2,"crs":"bd09ll"}}}}
      ],"legs":[{"id":"leg-1","from_stop_id":"stop-1","to_stop_id":"stop-2","legs_extension":{"keep":true}}]},
      {"id":"day-2","date":"2026-04-19","stops":[{"id":"stop-3","sequence":1,"title":"第二天","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.3,"lng":120.3,"crs":"bd09ll"}}}}],"legs":[{"id":"leg-2","from_stop_id":"stop-3","to_stop_id":"stop-3"}]},
      {"id":"day-3","date":"2026-04-20","stops":[],"legs":[{"id":"leg-3","from_stop_id":"x","to_stop_id":"y"}]}
    ]
  }`)
	record, err := service.Create(context.Background(), document, "test")
	if err != nil {
		t.Fatal(err)
	}
	title := "新地点"
	address := "新地址"
	location := json.RawMessage(`{"preferred":"gcj02","coordinates":{"gcj02":{"lat":30.25,"lng":120.25,"crs":"gcj02"}},"source":"amap-place-search","provider_refs":{"amap_poi_id":"poi-new"}}`)
	record, changes, err := service.UpdatePlanningPoint(context.Background(), record.ID, record.Revision, "day-1", "stop-1", UpdatePlanningPointInput{Title: &title, Address: &address, Location: location, LocationSet: true}, "test:update")
	if err != nil {
		t.Fatal(err)
	}
	if record.Revision != 2 || !changes.Changed || !changes.TitleChanged || !changes.AddressChanged || !changes.LocationChanged || !changes.RouteInvalidated || !changes.WeatherCleared {
		t.Fatalf("unexpected update result: revision=%d changes=%+v", record.Revision, changes)
	}

	var root map[string]any
	if err := json.Unmarshal(record.Document, &root); err != nil {
		t.Fatal(err)
	}
	days := root["days"].([]any)
	day1 := days[0].(map[string]any)
	if _, ok := day1["legs"]; ok {
		t.Fatal("current day route legs were not invalidated")
	}
	if day1["day_extension"].(map[string]any)["keep"] != true {
		t.Fatal("day extension was lost")
	}
	stop := day1["stops"].([]any)[0].(map[string]any)
	if stop["title"] != "新地点" || stop["address"] != "新地址" {
		t.Fatalf("point fields were not updated: %+v", stop)
	}
	if _, ok := stop["weather"]; ok {
		t.Fatal("weather tied to the old location was not cleared")
	}
	if stop["stop_extension"].(map[string]any)["keep"] != true {
		t.Fatal("stop extension was lost")
	}
	link := stop["links"].([]any)[0].(map[string]any)
	if link["future_link_field"] != "keep" {
		t.Fatal("untouched Link extension was lost")
	}
	child := stop["children"].([]any)[0].(map[string]any)
	if child["child_extension"].(map[string]any)["keep"] != true {
		t.Fatal("child extension was lost")
	}
	if _, ok := child["weather"]; !ok {
		t.Fatal("child weather should not be changed by a main stop update")
	}
	day2 := days[1].(map[string]any)
	if _, ok := day2["legs"]; ok {
		t.Fatal("next day's cross-day route legs were not invalidated")
	}
	day3 := days[2].(map[string]any)
	if _, ok := day3["legs"]; !ok {
		t.Fatal("unaffected day's route legs were changed")
	}
}

func TestUpdatePlanningPointChildAndTitleOnlyKeepRoutes(t *testing.T) {
	service := testService(t)
	document := []byte(`{"schema_version":1,"title":"子点编辑测试","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"主点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.1,"lng":120.1,"crs":"bd09ll"}}},"weather":{"source":"main-weather"},"children":[{"id":"child-1","sequence":1,"title":"子点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.11,"lng":120.11,"crs":"bd09ll"}}},"weather":{"source":"child-weather"}}]},{"id":"stop-2","sequence":2,"title":"终点","location":{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.2,"lng":120.2,"crs":"bd09ll"}}}}],"legs":[{"id":"leg-1","from_stop_id":"stop-1","to_stop_id":"stop-2"}]}]}`)
	record, err := service.Create(context.Background(), document, "test")
	if err != nil {
		t.Fatal(err)
	}
	childLocation := json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":30.12,"lng":120.12,"crs":"bd09ll"}},"source":"baidu-map-click"}`)
	record, changes, err := service.UpdatePlanningPoint(context.Background(), record.ID, record.Revision, "day-1", "child-1", UpdatePlanningPointInput{Location: childLocation, LocationSet: true}, "test:child")
	if err != nil {
		t.Fatal(err)
	}
	if !changes.LocationChanged || changes.RouteInvalidated || !changes.WeatherCleared {
		t.Fatalf("unexpected child changes: %+v", changes)
	}
	var root map[string]any
	if err := json.Unmarshal(record.Document, &root); err != nil {
		t.Fatal(err)
	}
	day := root["days"].([]any)[0].(map[string]any)
	if _, ok := day["legs"]; !ok {
		t.Fatal("child location update must preserve main stop routes")
	}
	stop := day["stops"].([]any)[0].(map[string]any)
	if _, ok := stop["weather"]; !ok {
		t.Fatal("main stop weather must be preserved")
	}
	child := stop["children"].([]any)[0].(map[string]any)
	if _, ok := child["weather"]; ok {
		t.Fatal("child weather was not cleared")
	}

	title := "主点改名"
	record, changes, err = service.UpdatePlanningPoint(context.Background(), record.ID, record.Revision, "day-1", "stop-1", UpdatePlanningPointInput{Title: &title}, "test:title")
	if err != nil {
		t.Fatal(err)
	}
	if !changes.TitleChanged || changes.LocationChanged || changes.RouteInvalidated || changes.WeatherCleared {
		t.Fatalf("unexpected title-only changes: %+v", changes)
	}
	if err := json.Unmarshal(record.Document, &root); err != nil {
		t.Fatal(err)
	}
	day = root["days"].([]any)[0].(map[string]any)
	if _, ok := day["legs"]; !ok {
		t.Fatal("title-only update must preserve routes")
	}
	stop = day["stops"].([]any)[0].(map[string]any)
	if stop["title"] != "主点改名" {
		t.Fatal("title-only update was not persisted")
	}
	if _, ok := stop["weather"]; !ok {
		t.Fatal("title-only update must preserve weather")
	}
}

func TestUpdatePlanningPointRejectsInvalidLocationAndStaleRevision(t *testing.T) {
	service := testService(t)
	record, err := service.Create(context.Background(), []byte(`{"schema_version":1,"title":"校验","status":"draft","timezone":"Asia/Shanghai","date_range":{"start":"2026-04-18","end":"2026-04-18"},"days":[{"id":"day-1","date":"2026-04-18","stops":[{"id":"stop-1","sequence":1,"title":"点"}]}]}`), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.UpdatePlanningPoint(context.Background(), record.ID, record.Revision, "day-1", "stop-1", UpdatePlanningPointInput{LocationSet: true}, "test"); !errors.Is(err, ErrPlanningLocationRequired) {
		t.Fatalf("missing location error = %v", err)
	}
	invalid := json.RawMessage(`{"preferred":"bd09ll","coordinates":{"bd09ll":{"lat":91,"lng":120,"crs":"bd09ll"}}}`)
	if _, _, err := service.UpdatePlanningPoint(context.Background(), record.ID, record.Revision, "day-1", "stop-1", UpdatePlanningPointInput{Location: invalid, LocationSet: true}, "test"); err == nil {
		t.Fatal("expected invalid location error")
	}
	title := "新标题"
	if _, _, err := service.UpdatePlanningPoint(context.Background(), record.ID, record.Revision-1, "day-1", "stop-1", UpdatePlanningPointInput{Title: &title}, "test"); !errors.Is(err, store.ErrRevisionConflict) {
		t.Fatalf("stale revision error = %v", err)
	}
}
