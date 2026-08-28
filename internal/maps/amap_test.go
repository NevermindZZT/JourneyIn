package maps

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAMapProviderSearchPOI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/place/text" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("types") != "110000" || r.URL.Query().Get("city_limit") != "true" {
			t.Fatalf("query=%s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"1","infocode":"10000","count":"1","pois":[{"id":"B001","name":"白石崖","address":"甘肃省","location":"102.123,35.456"}]}`))
	}))
	defer server.Close()
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: server.URL})
	result, err := provider.SearchPOIWithTag(t.Context(), "白石崖", "甘肃省", "旅游景点", 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].ID != "B001" || result.Items[0].Location.CRS != CRSGCJ02 {
		t.Fatalf("result=%+v", result)
	}
}

func TestAMapProviderGeocode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/geocode/geo" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"1","infocode":"10000","geocodes":[{"formatted_address":"甘加","location":"102.525136,35.405856"}]}`))
	}))
	defer server.Close()
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: server.URL})
	items, err := provider.Geocode(t.Context(), "甘加", "青海省")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Location.CRS != CRSGCJ02 || items[0].Location.Lat != 35.405856 {
		t.Fatalf("items=%+v", items)
	}
}
