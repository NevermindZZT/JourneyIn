package monitor

import (
	"context"
	"strings"
	"testing"
	"time"

	"journeyin/internal/weather"
)

type mockWeatherProvider struct {
	id       weather.ProviderID
	response weather.WeatherSnapshot
	err      error
}

func (m *mockWeatherProvider) ID() weather.ProviderID {
	return m.id
}

func (m *mockWeatherProvider) Weather(ctx context.Context, req weather.WeatherRequest) (weather.WeatherSnapshot, error) {
	if m.err != nil {
		return weather.WeatherSnapshot{Provider: m.id, LocalDate: req.LocalDate, Available: false}, m.err
	}
	res := m.response
	res.Provider = m.id
	res.LocalDate = req.LocalDate
	return res, nil
}

func TestEvaluator_FarHorizonWindowsWithoutWeatherClassifiedAsWaiting(t *testing.T) {
	evaluator := NewEvaluator(nil)

	// Now is 2026-10-18
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	// Monitor set for November (starts ~20 days from now, >15 days)
	m := Monitor{
		ID:        "mon_yuanyang",
		Title:     "元阳梯田景区 · 最佳游玩时机监控",
		PlaceName: "元阳梯田景区",
		Lat:       23.11891,
		Lng:       102.78035,
		CRS:       "gcj02",
		Criteria: MonitorCriteria{
			Months:          []int{11}, // November only (>15 days away)
			MaxLeaveDays:    1,
			MinDurationDays: 2,
			MaxDurationDays: 2,
		},
	}

	evals, err := evaluator.EvaluateMonitor(context.Background(), m, nil, now, 45)
	if err != nil {
		t.Fatalf("EvaluateMonitor failed: %v", err)
	}

	if len(evals) == 0 {
		t.Fatalf("expected at least one November window within 45 days, got 0")
	}

	for _, ev := range evals {
		// All November windows are > 15 days out, so no real weather data exists
		if ev.Level != LevelWaiting {
			t.Errorf("window %s ~ %s expected level 'waiting', got '%s' (score %v)", ev.WindowStart, ev.WindowEnd, ev.Level, ev.Score)
		}
		if ev.Score > 55 {
			t.Errorf("window %s ~ %s score without weather should be <= 55, got %v", ev.WindowStart, ev.WindowEnd, ev.Score)
		}
		if !strings.Contains(ev.Details.WeatherCondition, "超出预报范围") {
			t.Errorf("expected WeatherCondition to state out of forecast range, got '%s'", ev.Details.WeatherCondition)
		}
		if ev.Details.CloudCoverAvg != nil {
			t.Errorf("expected nil CloudCoverAvg without real weather, got %v", *ev.Details.CloudCoverAvg)
		}
		if ev.Details.RainProbAvg != nil {
			t.Errorf("expected nil RainProbAvg without real weather, got %v", *ev.Details.RainProbAvg)
		}
		if ev.Details.Scores.WeatherScore != nil {
			t.Errorf("expected nil WeatherScore without real weather, got %v", *ev.Details.Scores.WeatherScore)
		}
	}
}

func TestEvaluator_NearHorizonWindowWithRealWeather(t *testing.T) {
	tempMin := 16.0
	tempMax := 25.0
	mock := &mockWeatherProvider{
		id: weather.ProviderOpenMeteo,
		response: weather.WeatherSnapshot{
			Condition: "晴",
			Available: true,
			TempMinC:  &tempMin,
			TempMaxC:  &tempMax,
		},
	}
	reg := weather.NewRegistry()
	reg.Register(mock)
	weatherSvc := weather.NewService(reg, nil, nil, nil, nil, weather.ProviderOpenMeteo)
	evaluator := NewEvaluator(weatherSvc)

	// Now is 2026-10-18, next weekend is 2026-10-24 (within 15 days)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	m := Monitor{
		ID:        "mon_near",
		Title:     "西湖周边",
		PlaceName: "西湖",
		Lat:       30.25,
		Lng:       120.15,
		CRS:       "gcj02",
		Criteria: MonitorCriteria{
			Months:          []int{10},
			MaxLeaveDays:    0,
			MinDurationDays: 2,
			MaxDurationDays: 2,
		},
	}

	evals, err := evaluator.EvaluateMonitor(context.Background(), m, nil, now, 15)
	if err != nil {
		t.Fatalf("EvaluateMonitor failed: %v", err)
	}

	if len(evals) == 0 {
		t.Fatalf("expected near-horizon window, got 0")
	}

	top := evals[0]
	if top.Level != LevelStronglyRecommended && top.Level != LevelAcceptable {
		t.Errorf("expected sunny near window to be recommended or acceptable, got '%s'", top.Level)
	}
	if top.Details.CloudCoverAvg == nil || *top.Details.CloudCoverAvg > 30 {
		t.Errorf("expected low cloud cover for sunny weather, got %v", top.Details.CloudCoverAvg)
	}
	if top.Details.Scores.WeatherScore == nil || *top.Details.Scores.WeatherScore < 80 {
		t.Errorf("expected high weather score for sunny weather, got %v", top.Details.Scores.WeatherScore)
	}
	if !strings.Contains(top.Details.WeatherCondition, "晴空少云") {
		t.Errorf("expected weather condition to indicate sunny, got '%s'", top.Details.WeatherCondition)
	}
}

func TestEvaluator_NearHorizonRainyWeatherClassifiedAsWaiting(t *testing.T) {
	mock := &mockWeatherProvider{
		id: weather.ProviderOpenMeteo,
		response: weather.WeatherSnapshot{
			Condition: "暴雨",
			Available: true,
		},
	}
	reg := weather.NewRegistry()
	reg.Register(mock)
	weatherSvc := weather.NewService(reg, nil, nil, nil, nil, weather.ProviderOpenMeteo)
	evaluator := NewEvaluator(weatherSvc)

	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	m := Monitor{
		ID:        "mon_rain",
		Title:     "西湖周边雨天",
		PlaceName: "西湖",
		Lat:       30.25,
		Lng:       120.15,
		CRS:       "gcj02",
		Criteria: MonitorCriteria{
			Months:          []int{10},
			MaxLeaveDays:    0,
			MinDurationDays: 2,
			MaxDurationDays: 2,
		},
	}

	evals, err := evaluator.EvaluateMonitor(context.Background(), m, nil, now, 15)
	if err != nil {
		t.Fatalf("EvaluateMonitor failed: %v", err)
	}

	if len(evals) == 0 {
		t.Fatalf("expected near-horizon window, got 0")
	}

	top := evals[0]
	if top.Level != LevelWaiting {
		t.Errorf("rainy weather must be classified as 'waiting', got '%s'", top.Level)
	}
	if top.Details.Scores.WeatherScore == nil || *top.Details.Scores.WeatherScore > 30 {
		t.Errorf("expected low weather score for rain, got %v", top.Details.Scores.WeatherScore)
	}
}
