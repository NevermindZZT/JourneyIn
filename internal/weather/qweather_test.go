package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQWeatherProvider_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-QW-Api-Key") != "valid_key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v7/weather/now" {
			resp := qweatherNowResponse{
				Code: "200",
				Now: qweatherNow{
					Temp:      "24",
					Text:      "晴",
					WindDir:   "东风",
					WindScale: "2",
					Humidity:  "60",
					Pressure:  "1012",
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/v7/weather/10d" {
			resp := qweatherDailyResponse{
				Code: "200",
				Daily: []qweatherDaily{
					{
						FxDate:       "2026-09-20",
						TempMax:      "29",
						TempMin:      "20",
						TextDay:      "多云",
						TextNight:    "阴",
						WindDirDay:   "东南风",
						WindScaleDay: "3",
						Humidity:     "70",
						Pressure:     "1010",
					},
					{
						FxDate:       "2026-09-27", // 第 8 天预报
						TempMax:      "26",
						TempMin:      "18",
						TextDay:      "中雨",
						TextNight:    "小雨",
						WindDirDay:   "北风",
						WindScaleDay: "4",
						Humidity:     "85",
						Pressure:     "1008",
					},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := NewQWeatherProvider("valid_key", server.URL)
	snap, err := p.Weather(context.Background(), WeatherRequest{
		Location:  GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: "2026-09-27",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !snap.Available {
		t.Fatal("expected snap to be available")
	}
	if snap.Condition != "中雨转小雨" {
		t.Errorf("expected condition 中雨转小雨, got %s", snap.Condition)
	}
	if snap.TempMinC == nil || *snap.TempMinC != 18.0 {
		t.Errorf("expected tempMin 18.0, got %v", snap.TempMinC)
	}
	if snap.TempMaxC == nil || *snap.TempMaxC != 26.0 {
		t.Errorf("expected tempMax 26.0, got %v", snap.TempMaxC)
	}
	if snap.CurrentCondition != "晴" {
		t.Errorf("expected currentCondition 晴, got %s", snap.CurrentCondition)
	}
	if snap.WindDirection != "北风" {
		t.Errorf("expected windDirection 北风, got %s", snap.WindDirection)
	}
	if snap.WindPower != "4级" {
		t.Errorf("expected windPower 4级, got %s", snap.WindPower)
	}
}