package maps

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAMapProviderRouteWalking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/direction/walking" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("show_fields") != "cost,navi,polyline" {
			t.Fatalf("show_fields=%q", r.URL.Query().Get("show_fields"))
		}
		if r.URL.Query().Get("origin") != "120.150000,30.250000" || r.URL.Query().Get("destination") != "120.160000,30.260000" {
			t.Fatalf("coordinates=%s -> %s", r.URL.Query().Get("origin"), r.URL.Query().Get("destination"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"1","info":"OK","infocode":"10000","route":{"paths":[{"distance":"1200","duration":"720","steps":[{"polyline":"120.150000,30.250000;120.155000,30.255000"},{"polyline":"120.155000,30.255000;120.160000,30.260000"}]}]}}`))
	}))
	defer server.Close()
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: server.URL})
	result, err := provider.Route(context.Background(), RouteRequest{Origin: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}, Destination: GeoPoint{Lat: 30.26, Lng: 120.16, CRS: CRSGCJ02}, Mode: ModeWalking})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != ProviderAMap || result.CoordinateSystem != CRSGCJ02 || result.Mode != ModeWalking || result.Source == "" {
		t.Fatalf("metadata=%+v", result)
	}
	if result.DistanceM != 1200 || result.DurationS != 720 || len(result.Geometry) != 3 {
		t.Fatalf("route=%+v", result)
	}
}

func TestAMapProviderRouteTransitUsesCityAndDeparture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if r.URL.Path != "/v5/direction/transit/integrated" || query.Get("city1") != "010" || query.Get("city2") != "021" || query.Get("date") != "2026-04-18" || query.Get("time") != "9-30" {
			t.Fatalf("request=%s?%s", r.URL.Path, r.URL.RawQuery)
		}
		if query.Get("originpoi") != "A1" || query.Get("destinationpoi") != "A2" {
			t.Fatalf("poi ids=%s/%s", query.Get("originpoi"), query.Get("destinationpoi"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"1","info":"OK","infocode":"10000","route":{"transits":[{"distance":"5000","duration":"1800","segments":[{"walking":{"steps":[{"polyline":"120.150000,30.250000;120.151000,30.251000"}]}},{"bus":{"buslines":[{"polyline":"120.151000,30.251000;120.160000,30.260000"}]}}]}]}}`))
	}))
	defer server.Close()
	departure := time.Date(2026, 4, 18, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: server.URL})
	result, err := provider.Route(context.Background(), RouteRequest{Origin: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}, Destination: GeoPoint{Lat: 30.26, Lng: 120.16, CRS: CRSGCJ02}, Mode: ModeTransit, DepartureAt: &departure, OriginPOIID: "A1", DestinationPOIID: "A2", OriginCityCode: "010", DestinationCityCode: "021"})
	if err != nil {
		t.Fatal(err)
	}
	if result.DistanceM != 5000 || result.DurationS != 1800 || len(result.Geometry) != 3 || result.CoordinateSystem != CRSGCJ02 {
		t.Fatalf("route=%+v", result)
	}
}

func TestAMapProviderTransitRequiresCityCodes(t *testing.T) {
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: "http://127.0.0.1:1"})
	_, err := provider.Route(context.Background(), RouteRequest{Origin: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}, Destination: GeoPoint{Lat: 30.26, Lng: 120.16, CRS: CRSGCJ02}, Mode: ModeTransit})
	if err == nil || !strings.Contains(err.Error(), "origin_citycode") {
		t.Fatalf("err=%v", err)
	}
}

func TestAMapProviderRetriesRateLimitAndMapsQuota(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"status":"0","info":"ACCESS_TOO_FREQUENT","infocode":"10004"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"1","info":"OK","infocode":"10000","route":{"paths":[{"distance":"1","duration":"1","steps":[{"polyline":"120,30;120.001,30.001"}]}]}}`))
	}))
	defer server.Close()
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: server.URL})
	_, err := provider.Route(context.Background(), RouteRequest{Origin: GeoPoint{Lat: 30, Lng: 120, CRS: CRSGCJ02}, Destination: GeoPoint{Lat: 30.001, Lng: 120.001, CRS: CRSGCJ02}, Mode: ModeDriving})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}

	calls.Store(0)
	quota := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"status":"0","info":"DAILY_QUERY_OVER_LIMIT","infocode":"10003"}`))
	}))
	defer quota.Close()
	provider = NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: quota.URL})
	_, err = provider.Route(context.Background(), RouteRequest{Origin: GeoPoint{Lat: 30, Lng: 120, CRS: CRSGCJ02}, Destination: GeoPoint{Lat: 30.001, Lng: 120.001, CRS: CRSGCJ02}, Mode: ModeDriving})
	if !errors.Is(err, ErrProviderQuotaExceeded) || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
}

func TestAMapProviderNetworkErrorDoesNotLeakKey(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: "https://restapi.amap.com/v5/direction/driving?key=secret-ak", Err: errors.New("dial tcp: temporary failure")}
	})}
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "secret-ak", HTTPClient: client, BaseURL: "https://restapi.amap.com", RequestTimeout: 20 * time.Millisecond})
	_, err := provider.Route(context.Background(), RouteRequest{Origin: GeoPoint{Lat: 30, Lng: 120, CRS: CRSGCJ02}, Destination: GeoPoint{Lat: 30.001, Lng: 120.001, CRS: CRSGCJ02}, Mode: ModeDriving})
	if err == nil || strings.Contains(err.Error(), "secret-ak") || !errors.Is(err, ErrProviderTemporary) {
		t.Fatalf("err=%v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestAMapProviderRouteUsesNestedCostAndPathPolyline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("show_fields") != "cost,tmcs,navi,cities,polyline" {
			t.Fatalf("show_fields=%q", r.URL.Query().Get("show_fields"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"1","info":"OK","infocode":"10000","route":{"paths":[{"distance":"1200","cost":{"duration":"720"},"polyline":"120.150000,30.250000;120.160000,30.260000"}]}}`))
	}))
	defer server.Close()
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: server.URL})
	result, err := provider.Route(context.Background(), RouteRequest{Origin: GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02}, Destination: GeoPoint{Lat: 30.26, Lng: 120.16, CRS: CRSGCJ02}, Mode: ModeDriving})
	if err != nil {
		t.Fatal(err)
	}
	if result.DistanceM != 1200 || result.DurationS != 720 || len(result.Geometry) != 2 || result.Strategy != "32" {
		t.Fatalf("route=%+v", result)
	}
}

func TestAMapProviderWeatherAndReverseGeocode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v3/weather/weatherInfo":
			if r.URL.Query().Get("city") != "110108" || r.URL.Query().Get("extensions") != "all" {
				t.Fatalf("weather query=%s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"status":"1","info":"OK","infocode":"10000","forecasts":[{"casts":[{"date":"2026-04-18","dayweather":"晴","nightweather":"多云","daytemp":"25","nighttemp":"15"}]}]}`))
		case "/v3/geocode/regeo":
			if r.URL.Query().Get("location") != "120.150000,30.250000" {
				t.Fatalf("reverse query=%s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"status":"1","info":"OK","infocode":"10000","regeocode":{"formatted_address":"北京市海淀区"}}`))
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer server.Close()
	provider := NewAMapProviderWithConfig("test", AMapConfig{ServerKey: "test-key", BaseURL: server.URL})
	weather, err := provider.Weather(context.Background(), WeatherRequest{LocalDate: "2026-04-18", AdCode: "110108"})
	if err != nil || !weather.Available || weather.Condition != "晴" || weather.TemperatureC == nil || *weather.TemperatureC != 20 {
		t.Fatalf("weather=%+v err=%v", weather, err)
	}
	address, err := provider.ReverseGeocode(context.Background(), GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02})
	if err != nil || address != "北京市海淀区" {
		t.Fatalf("address=%q err=%v", address, err)
	}
}
