package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenMeteoProvider_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openMeteoResponse{
			Current: openMeteoCurrent{
				Temperature2M:    26.5,
				RelativeHumidity: 65.0,
				WeatherCode:      1,
				WindSpeed10M:     12.0,
				WindDirection10M: 180.0,
				SurfacePressure:  1012.0,
			},
			Daily: openMeteoDaily{
				Time:                     []string{"2026-09-20", "2026-09-28", "2026-10-04"},
				WeatherCode:              []int{0, 63, 2},
				Temperature2MMax:         []float64{30.0, 24.0, 22.0},
				Temperature2MMin:         []float64{20.0, 18.0, 15.0},
				WindSpeed10MMax:          []float64{15.0, 25.0, 8.0},
				WindDirection10MDominant: []float64{90.0, 225.0, 0.0},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewOpenMeteoProvider()
	p.SetBaseURL(server.URL)

	snap, err := p.Weather(context.Background(), WeatherRequest{
		Location:  GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: "2026-09-28", // 第 8 天预报
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !snap.Available {
		t.Fatal("expected snap to be available")
	}
	if snap.Condition != "中雨" {
		t.Errorf("expected condition 中雨, got %s", snap.Condition)
	}
	if snap.TempMinC == nil || *snap.TempMinC != 18.0 {
		t.Errorf("expected tempMin 18.0, got %v", snap.TempMinC)
	}
	if snap.TempMaxC == nil || *snap.TempMaxC != 24.0 {
		t.Errorf("expected tempMax 24.0, got %v", snap.TempMaxC)
	}
	if snap.CurrentCondition != "晴间多云" {
		t.Errorf("expected currentCondition 晴间多云, got %s", snap.CurrentCondition)
	}
	if snap.WindDirection != "西南风" {
		t.Errorf("expected windDirection 西南风, got %s", snap.WindDirection)
	}
	if snap.WindPower != "4级" {
		t.Errorf("expected windPower 4级, got %s", snap.WindPower)
	}
}