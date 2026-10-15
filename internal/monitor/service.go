package monitor

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"journeyin/internal/domain"
	"journeyin/internal/weather"
)

type Store interface {
	CreateMonitor(ctx context.Context, m Monitor) error
	GetMonitor(ctx context.Context, id string) (Monitor, error)
	ListMonitors(ctx context.Context, enabledOnly bool) ([]Monitor, error)
	UpdateMonitor(ctx context.Context, m Monitor) error
	DeleteMonitor(ctx context.Context, id string) error
	SaveMonitorEvaluations(ctx context.Context, monitorID string, evals []Evaluation) error
	GetMonitorEvaluations(ctx context.Context, monitorID string, limit int) ([]Evaluation, error)
	GetTopEvaluations(ctx context.Context, limit int) ([]Evaluation, error)
	GetHolidayCalendar(ctx context.Context, year int) (map[string]HolidayRecord, error)
}

type TripCreator interface {
	Create(ctx context.Context, document []byte, source string) (any, error)
}

type Service struct {
	store      Store
	weatherSvc *weather.Service
	evaluator  *Evaluator
	logger     *slog.Logger
	mu         sync.RWMutex
}

func NewService(store Store, weatherSvc *weather.Service, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		store:      store,
		weatherSvc: weatherSvc,
		evaluator:  NewEvaluator(weatherSvc),
		logger:     logger,
	}
}

func (s *Service) CreateMonitor(ctx context.Context, m Monitor) (Monitor, error) {
	if m.ID == "" {
		m.ID = newID("mon")
	}
	if m.Title == "" {
		m.Title = m.PlaceName
	}
	if m.CRS == "" {
		m.CRS = "gcj02"
	}
	now := time.Now().UTC()
	m.CreatedAt = now
	m.UpdatedAt = now

	if err := s.store.CreateMonitor(ctx, m); err != nil {
		return Monitor{}, err
	}

	// Trigger initial evaluation immediately
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = s.EvaluateOne(bgCtx, m.ID)
	}()

	return m, nil
}

func (s *Service) GetMonitor(ctx context.Context, id string) (Monitor, error) {
	return s.store.GetMonitor(ctx, id)
}

func (s *Service) ListMonitors(ctx context.Context, enabledOnly bool) ([]Monitor, error) {
	return s.store.ListMonitors(ctx, enabledOnly)
}

func (s *Service) UpdateMonitor(ctx context.Context, m Monitor) error {
	m.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateMonitor(ctx, m); err != nil {
		return err
	}
	// Re-evaluate asynchronously
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = s.EvaluateOne(bgCtx, m.ID)
	}()
	return nil
}

func (s *Service) DeleteMonitor(ctx context.Context, id string) error {
	return s.store.DeleteMonitor(ctx, id)
}

func (s *Service) GetEvaluations(ctx context.Context, monitorID string, limit int) ([]Evaluation, error) {
	return s.store.GetMonitorEvaluations(ctx, monitorID, limit)
}

func (s *Service) GetRadarSummary(ctx context.Context) (RadarSummary, error) {
	monitors, err := s.store.ListMonitors(ctx, false)
	if err != nil {
		return RadarSummary{}, err
	}

	topEvals, err := s.store.GetTopEvaluations(ctx, 1)
	if err != nil {
		return RadarSummary{}, err
	}

	recommendedCount := 0
	for _, m := range monitors {
		if m.LatestEvaluation != nil && m.LatestEvaluation.Level == LevelStronglyRecommended {
			recommendedCount++
		}
	}

	summary := RadarSummary{
		TotalMonitors:    len(monitors),
		RecommendedCount: recommendedCount,
	}

	if len(topEvals) > 0 {
		top := topEvals[0]
		summary.TopOpportunity = &top
		for _, m := range monitors {
			if m.ID == top.MonitorID {
				summary.TopPlaceName = m.PlaceName
				break
			}
		}
	}
	return summary, nil
}

func (s *Service) EvaluateOne(ctx context.Context, monitorID string) error {
	m, err := s.store.GetMonitor(ctx, monitorID)
	if err != nil {
		return err
	}

	holidays, _ := s.store.GetHolidayCalendar(ctx, time.Now().Year())
	evals, err := s.evaluator.EvaluateMonitor(ctx, m, holidays, time.Now(), 45)
	if err != nil {
		return err
	}

	return s.store.SaveMonitorEvaluations(ctx, monitorID, evals)
}

func (s *Service) EvaluateAll(ctx context.Context) error {
	monitors, err := s.store.ListMonitors(ctx, true)
	if err != nil {
		return err
	}

	holidays, _ := s.store.GetHolidayCalendar(ctx, time.Now().Year())
	now := time.Now()

	for _, m := range monitors {
		evals, err := s.evaluator.EvaluateMonitor(ctx, m, holidays, now, 45)
		if err != nil {
			s.logger.Error("evaluate monitor failed", "id", m.ID, "error", err)
			continue
		}
		if err := s.store.SaveMonitorEvaluations(ctx, m.ID, evals); err != nil {
			s.logger.Error("save evaluations failed", "id", m.ID, "error", err)
		}
	}
	return nil
}

// ConvertToTrip builds a canonical JourneyIn Trip draft document from an evaluation window.
func (s *Service) ConvertToTrip(
	ctx context.Context,
	monitorID string,
	windowStart string,
	windowEnd string,
	creator func(ctx context.Context, document []byte, source string) (string, error),
) (string, error) {
	m, err := s.store.GetMonitor(ctx, monitorID)
	if err != nil {
		return "", err
	}

	startT, err := time.Parse("2006-01-02", windowStart)
	if err != nil {
		return "", errors.New("invalid window_start format")
	}
	endT, err := time.Parse("2006-01-02", windowEnd)
	if err != nil {
		return "", errors.New("invalid window_end format")
	}
	if endT.Before(startT) {
		return "", errors.New("window_end must not be before window_start")
	}

	daysCount := int(endT.Sub(startT).Hours()/24) + 1
	if daysCount > 30 {
		return "", errors.New("opportunity window cannot exceed 30 days")
	}

	// Build Day list
	days := make([]domain.Day, 0, daysCount)
	for i := 0; i < daysCount; i++ {
		curDate := startT.AddDate(0, 0, i).Format("2006-01-02")
		dayID := newID("day")
		d := domain.Day{
			ID:    dayID,
			Date:  curDate,
			Title: fmt.Sprintf("Day %d", i+1),
		}

		// Add monitored POI as the main stop of Day 1
		if i == 0 {
			stopID := newID("stop")
			locJSON, _ := json.Marshal(map[string]any{
				"preferred": m.CRS,
				"coordinates": map[string]any{
					m.CRS: map[string]any{
						"lat": m.Lat,
						"lng": m.Lng,
						"crs": m.CRS,
					},
				},
				"source": "journeyin:monitor",
			})

			d.Stops = []domain.Stop{
				{
					ID:       stopID,
					Sequence: 1,
					Kind:     "poi",
					Title:    m.PlaceName,
					Location: locJSON,
				},
			}
		}
		days = append(days, d)
	}

	tripID := newID("trip")
	tripTitle := fmt.Sprintf("%s · 推荐时机游玩", m.PlaceName)
	tripDoc := domain.Trip{
		SchemaVersion: 1,
		ID:            tripID,
		Title:         tripTitle,
		Status:        "draft",
		Timezone:      "Asia/Shanghai",
		DateRange: domain.DateRange{
			Start: windowStart,
			End:   windowEnd,
		},
		DescriptionMarkdown: fmt.Sprintf("本行程由【旅行雷达】智能推荐生成：\n- 目标景点：%s\n- 推荐出行窗口：%s 至 %s", m.PlaceName, windowStart, windowEnd),
		Map: domain.MapConfig{
			PreferredProvider: "amap",
			DefaultMode:       "driving",
		},
		Days: days,
	}

	encoded, err := json.Marshal(tripDoc)
	if err != nil {
		return "", fmt.Errorf("encode trip: %w", err)
	}

	createdTripID, err := creator(ctx, encoded, "monitor:convert")
	if err != nil {
		return "", fmt.Errorf("create trip from monitor: %w", err)
	}

	return createdTripID, nil
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(b))
}
