package maps

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestBaiduProviderUsesStandardEndpointsAndNormalizesSnapshots(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ak") != "test-ak" {
			t.Errorf("missing AK")
		}
		w.Header().Set("Content-Type", "application/json")
		var response any
		switch r.URL.Path {
		case "/geocoding/v3/":
			response = map[string]any{"status": 0, "result": map[string]any{"location": map[string]any{"lat": 30.25, "lng": 120.15}}}
		case "/direction/v2/walking":
			response = map[string]any{"status": 0, "result": map[string]any{"routes": []any{map[string]any{"distance": 100, "duration": 200, "steps": []any{map[string]any{"path": "120.150000,30.250000;120.151000,30.251000"}}}}}}
		case "/weather/v1/":
			response = map[string]any{"status": 0, "result": map[string]any{"forecasts": []any{map[string]any{"date": "2026-04-18", "high": 25, "low": 15, "text_day": "晴"}}}}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()
	provider := NewBaiduProvider(BaiduConfig{ServerAK: "test-ak", BaseURL: server.URL})

	places, err := provider.Geocode(t.Context(), "杭州市西湖区", "杭州市")
	if err != nil {
		t.Fatal(err)
	}
	if len(places) != 1 || places[0].Location.CRS != CRSBD09LL {
		t.Fatalf("unexpected geocode: %+v", places)
	}
	route, err := provider.Route(t.Context(), RouteRequest{Origin: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}, Destination: GeoPoint{Lat: 30.26, Lng: 120.16, CRS: CRSGCJ02}, Mode: ModeWalking})
	if err != nil {
		t.Fatal(err)
	}
	if route.DistanceM != 100 || route.DurationS != 200 || len(route.Geometry) != 2 || route.CoordinateSystem != CRSBD09LL {
		t.Fatalf("unexpected route: %+v", route)
	}
	weather, err := provider.Weather(t.Context(), WeatherRequest{Location: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}, LocalDate: "2026-04-18", Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatal(err)
	}
	if !weather.Available || weather.Condition != "晴" || weather.TemperatureC == nil || *weather.TemperatureC != 20 {
		t.Fatalf("unexpected weather: %+v", weather)
	}
}

func TestAMapNavigationURLIsSafeAndProviderSpecific(t *testing.T) {
	provider := NewAMapProvider("journeyin-test")
	raw, err := provider.NavigationURL(NavTarget{Name: "西湖", Location: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}}, ModeWalking, PlatformWeb)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "uri.amap.com" || parsed.Query().Get("mode") != "walk" || parsed.Query().Get("coordinate") != "gaode" {
		t.Fatalf("unexpected URL: %s", raw)
	}
	if err := ValidateNavigationURL(raw); err != nil {
		t.Fatal(err)
	}
}

func TestNativeNavigationURLsUseAllowedSchemes(t *testing.T) {
	baidu, err := NewBaiduProvider(BaiduConfig{}).NavigationURL(NavTarget{Name: "西湖", Location: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSBD09LL}}, ModeWalking, PlatformAndroid)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(baidu, "baidumap://") {
		t.Fatalf("unexpected Baidu scheme: %s", baidu)
	}
	if err := ValidateNavigationURL(baidu); err != nil {
		t.Fatal(err)
	}
	amap, err := NewAMapProvider("test").NavigationURL(NavTarget{Name: "西湖", Location: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}}, ModeWalking, PlatformIOS)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(amap, "iosamap://") {
		t.Fatalf("unexpected AMap scheme: %s", amap)
	}
	if err := ValidateNavigationURL(amap); err != nil {
		t.Fatal(err)
	}
}

func TestBaiduProviderSearchPOI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/place/v2/search" || r.URL.Query().Get("query") != "西湖" || r.URL.Query().Get("region") != "杭州市" || r.URL.Query().Get("ret_coordtype") != "bd09ll" { t.Fatalf("unexpected search request: %s", r.URL.String()) }
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":0,"total":2,"results":[{"uid":"uid-1","name":"西湖","address":"浙江省杭州市","location":{"lat":30.25,"lng":120.15}},{"uid":"uid-2","name":"西湖文化广场","address":"杭州市拱墅区","location":{"lat":30.286,"lng":120.172}}]}`))
	}))
	defer server.Close()
	provider := NewBaiduProvider(BaiduConfig{ServerAK:"test-ak", BaseURL:server.URL})
	result, err := provider.SearchPOI(t.Context(), "西湖", "杭州市", 1, 10)
	if err != nil { t.Fatal(err) }
	if result.Total != 2 || len(result.Items) != 2 || result.Items[0].ID != "uid-1" || result.Items[0].Location.CRS != CRSBD09LL { t.Fatalf("unexpected POI result: %+v", result) }
}
