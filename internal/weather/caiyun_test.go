package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCaiyunProvider_MissingToken(t *testing.T) {
	p := NewCaiyunProvider("")
	snapshot, err := p.Weather(context.Background(), WeatherRequest{
		Location:  GeoPoint{Lat: 30.24, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: "2026-09-25",
	})
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}
	if snapshot.Available {
		t.Error("expected available to be false")
	}
}

func TestCaiyunProvider_InvalidPoint(t *testing.T) {
	p := NewCaiyunProvider("test_token")
	_, err := p.Weather(context.Background(), WeatherRequest{
		Location:  GeoPoint{Lat: 0, Lng: 0, CRS: CRSGCJ02},
		LocalDate: "2026-09-25",
	})
	if err == nil {
		t.Fatal("expected error for 0,0 location, got nil")
	}
}

func TestCaiyunProvider_Forecast15DaysSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "invalid method", http.StatusMethodNotAllowed)
			return
		}
		// 模拟彩云 v2.6 15天预报响应
		resp := caiyunResponse{
			Status: "ok",
			Result: caiyunResult{
				Realtime: caiyunRealtime{
					Status:      "ok",
					Temperature: 28.5,
					Humidity:    0.65,
					Skycon:      "CLEAR_DAY",
					Pressure:    101325.0,
					Wind:        caiyunWind{Speed: 15.0, Direction: 180.0},
				},
				Daily: caiyunDaily{
					Status: "ok",
					Temperature: []caiyunDailyTemp{
						{Date: "2026-09-20T00:00+08:00", Min: 20.0, Max: 30.0, Avg: 25.0},
						{Date: "2026-09-25T00:00+08:00", Min: 18.0, Max: 26.0, Avg: 22.0}, // 第 6 天
						{Date: "2026-10-02T00:00+08:00", Min: 15.0, Max: 23.0, Avg: 19.0}, // 第 13 天
					},
					Skycon: []caiyunDailySkycon{
						{Date: "2026-09-20T00:00+08:00", Value: "CLEAR_DAY"},
						{Date: "2026-09-25T00:00+08:00", Value: "MODERATE_RAIN"},
						{Date: "2026-10-02T00:00+08:00", Value: "PARTLY_CLOUDY_DAY"},
					},
					Humidity: []caiyunDailyScalar{
						{Date: "2026-09-20T00:00+08:00", Avg: 0.60},
						{Date: "2026-09-25T00:00+08:00", Avg: 0.88},
						{Date: "2026-10-02T00:00+08:00", Avg: 0.50},
					},
					Wind: []caiyunDailyWind{
						{Date: "2026-09-20T00:00+08:00", Avg: caiyunWind{Speed: 10, Direction: 90}},
						{Date: "2026-09-25T00:00+08:00", Avg: caiyunWind{Speed: 25, Direction: 220}},
						{Date: "2026-10-02T00:00+08:00", Avg: caiyunWind{Speed: 5, Direction: 0}},
					},
					Pressure: []caiyunDailyScalar{
						{Date: "2026-09-20T00:00+08:00", Avg: 101200.0},
						{Date: "2026-09-25T00:00+08:00", Avg: 100850.0},
						{Date: "2026-10-02T00:00+08:00", Avg: 101500.0},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	p := NewCaiyunProvider("valid_token")
	p.SetBaseURL(server.URL)

	// 查询第 6 天 (2026-09-25)
	snapshot, err := p.Weather(context.Background(), WeatherRequest{
		Location:  GeoPoint{Lat: 30.24, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: "2026-09-25",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !snapshot.Available {
		t.Fatal("expected snapshot to be available")
	}
	if snapshot.Condition != "中雨" {
		t.Errorf("expected Condition 中雨, got %s", snapshot.Condition)
	}
	if snapshot.TemperatureC == nil || *snapshot.TemperatureC != 22.0 {
		t.Errorf("expected TemperatureC 22.0, got %v", snapshot.TemperatureC)
	}
	if snapshot.TempMinC == nil || *snapshot.TempMinC != 18.0 {
		t.Errorf("expected TempMinC 18.0, got %v", snapshot.TempMinC)
	}
	if snapshot.TempMaxC == nil || *snapshot.TempMaxC != 26.0 {
		t.Errorf("expected TempMaxC 26.0, got %v", snapshot.TempMaxC)
	}
	if snapshot.CurrentCondition != "晴" {
		t.Errorf("expected CurrentCondition 晴, got %s", snapshot.CurrentCondition)
	}
	if snapshot.HumidityPercent == nil || *snapshot.HumidityPercent != 88.0 {
		t.Errorf("expected HumidityPercent 88.0, got %v", snapshot.HumidityPercent)
	}
	if snapshot.WindDirection != "西南风" {
		t.Errorf("expected WindDirection 西南风, got %s", snapshot.WindDirection)
	}
	if snapshot.WindPower != "4级" {
		t.Errorf("expected WindPower 4级, got %s", snapshot.WindPower)
	}
	if snapshot.PressureHPa == nil || *snapshot.PressureHPa != 1008.5 {
		t.Errorf("expected PressureHPa 1008.5, got %v", snapshot.PressureHPa)
	}

	// 查询第 13 天 (2026-10-02)
	snap13, err := p.Weather(context.Background(), WeatherRequest{
		Location:  GeoPoint{Lat: 30.24, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: "2026-10-02",
	})
	if err != nil {
		t.Fatalf("unexpected error for day 13: %v", err)
	}
	if !snap13.Available || snap13.Condition != "多云" {
		t.Errorf("expected day 13 available with 多云, got %v, %s", snap13.Available, snap13.Condition)
	}

	// 查询超出范围的日期 (2026-11-01)
	snapOut, err := p.Weather(context.Background(), WeatherRequest{
		Location:  GeoPoint{Lat: 30.24, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: "2026-11-01",
	})
	if err != nil {
		t.Fatalf("unexpected error for out of range date: %v", err)
	}
	if snapOut.Available {
		t.Error("expected available to be false for date not in forecasts")
	}
}