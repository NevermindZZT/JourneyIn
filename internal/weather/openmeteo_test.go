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
				Temperature2M:       26.5,
				RelativeHumidity:    65.0,
				ApparentTemperature: 28.0,
				WeatherCode:         1,
				WindSpeed10M:        12.0,
				WindDirection10M:    180.0,
				SurfacePressure:     1012.0,
				Visibility:          10000.0,
				CloudCover:          40.0,
				UVIndex:             5.0,
			},
			Daily: openMeteoDaily{
				Time:                        []string{"2026-09-20", "2026-09-28", "2026-10-04"},
				WeatherCode:                 []int{0, 63, 2},
				Temperature2MMax:            []float64{30.0, 24.0, 22.0},
				Temperature2MMin:            []float64{20.0, 18.0, 15.0},
				ApparentTemperatureMax:      []float64{32.0, 25.0, 23.0},
				ApparentTemperatureMin:      []float64{21.0, 19.0, 16.0},
				PrecipitationProbabilityMax: []int{10, 85, 20},
				WindSpeed10MMax:             []float64{15.0, 25.0, 8.0},
				WindDirection10MDominant:    []float64{90.0, 225.0, 0.0},
				UVIndexMax:                  []float64{6.5, 4.0, 5.0},
			},
			Hourly: &openMeteoHourly{
				Time:           []string{"2026-09-28T00:00", "2026-09-28T12:00"},
				Visibility:     []float64{12000.0, 14000.0},
				CloudCover:     []float64{60.0, 70.0},
				CloudCoverLow:  []float64{10.0, 20.0},
				CloudCoverMid:  []float64{40.0, 50.0},
				CloudCoverHigh: []float64{70.0, 80.0},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	airServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		airResp := openMeteoAirResponse{
			Hourly: &openMeteoAirHourly{
				Time:                []string{"2026-09-28T00:00", "2026-09-28T12:00"},
				PM10:                []float64{25.0, 35.0},
				PM25:                []float64{12.0, 18.0},
				AerosolOpticalDepth: []float64{0.20, 0.30},
				Dust:                []float64{5.0, 7.0},
				EuropeanAQI:         []float64{30.0, 40.0},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(airResp)
	}))
	defer airServer.Close()

	p := NewOpenMeteoProvider()
	p.SetBaseURL(server.URL)
	p.SetAirQualityBaseURL(airServer.URL)

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
	if got := snap.ExpiresAt.Sub(snap.FetchedAt); got != weatherSnapshotTTL {
		t.Fatalf("weather snapshot TTL=%s, want %s", got, weatherSnapshotTTL)
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
	if snap.VisibilityKm == nil || *snap.VisibilityKm != 13.0 {
		t.Errorf("expected VisibilityKm 13.0, got %v", snap.VisibilityKm)
	}
	if snap.CloudCoverPct == nil || *snap.CloudCoverPct != 65 {
		t.Errorf("expected CloudCoverPct 65, got %v", snap.CloudCoverPct)
	}
	if snap.CloudCoverLow == nil || *snap.CloudCoverLow != 15 {
		t.Errorf("expected CloudCoverLow 15, got %v", snap.CloudCoverLow)
	}
	if snap.CloudCoverMid == nil || *snap.CloudCoverMid != 45 {
		t.Errorf("expected CloudCoverMid 45, got %v", snap.CloudCoverMid)
	}
	if snap.CloudCoverHigh == nil || *snap.CloudCoverHigh != 75 {
		t.Errorf("expected CloudCoverHigh 75, got %v", snap.CloudCoverHigh)
	}
	if snap.RainProbPct == nil || *snap.RainProbPct != 85 {
		t.Errorf("expected RainProbPct 85, got %v", snap.RainProbPct)
	}
	if snap.ApparentTempC == nil || *snap.ApparentTempC != 22.0 {
		t.Errorf("expected ApparentTempC 22.0, got %v", snap.ApparentTempC)
	}
	if snap.UVIndex == nil || *snap.UVIndex != 4.0 {
		t.Errorf("expected UVIndex 4.0, got %v", snap.UVIndex)
	}
	if snap.AOD == nil || *snap.AOD != 0.25 {
		t.Errorf("expected AOD 0.25, got %v", snap.AOD)
	}
	if snap.PM25 == nil || *snap.PM25 != 15.0 {
		t.Errorf("expected PM25 15.0, got %v", snap.PM25)
	}
	if snap.PM10 == nil || *snap.PM10 != 30.0 {
		t.Errorf("expected PM10 30.0, got %v", snap.PM10)
	}
	if snap.AQI == nil || *snap.AQI != 35 {
		t.Errorf("expected AQI 35, got %v", snap.AQI)
	}
}
