package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"journeyin/internal/monitor"
)

var ErrMonitorNotFound = errors.New("attraction monitor not found")

func (s *Store) CreateMonitor(ctx context.Context, m monitor.Monitor) error {
	criteriaJSON, err := m.Criteria.ToJSON()
	if err != nil {
		return fmt.Errorf("encode criteria: %w", err)
	}

	enabledInt := 0
	if m.Enabled {
		enabledInt = 1
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := `INSERT INTO attraction_monitors (id, title, place_name, lat, lng, crs, criteria_json, origin_city, max_budget, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, query,
		m.ID, m.Title, m.PlaceName, m.Lat, m.Lng, m.CRS, criteriaJSON,
		m.OriginCity, m.MaxBudget, enabledInt, now, now,
	)
	return err
}

func (s *Store) GetMonitor(ctx context.Context, id string) (monitor.Monitor, error) {
	query := `SELECT id, title, place_name, lat, lng, crs, criteria_json, origin_city, max_budget, enabled, created_at, updated_at
		FROM attraction_monitors WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	var m monitor.Monitor
	var criteriaRaw, createdText, updatedText string
	var enabledInt int

	if err := row.Scan(&m.ID, &m.Title, &m.PlaceName, &m.Lat, &m.Lng, &m.CRS, &criteriaRaw, &m.OriginCity, &m.MaxBudget, &enabledInt, &createdText, &updatedText); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return m, ErrMonitorNotFound
		}
		return m, err
	}

	m.Enabled = enabledInt == 1
	m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdText)
	m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedText)
	m.Criteria, _ = monitor.CriteriaFromJSON(criteriaRaw)

	// Fetch latest evaluation
	evalQuery := `SELECT id, monitor_id, window_start, window_end, score, level, details_json, evaluated_at
		FROM monitor_evaluations WHERE monitor_id = ?
		ORDER BY CASE level WHEN 'strongly_recommended' THEN 3 WHEN 'acceptable' THEN 2 WHEN 'waiting' THEN 1 ELSE 0 END DESC, score DESC, window_start ASC LIMIT 1`
	evalRow := s.db.QueryRowContext(ctx, evalQuery, id)
	var ev monitor.Evaluation
	var detailsRaw, evalDateText string
	if err := evalRow.Scan(&ev.ID, &ev.MonitorID, &ev.WindowStart, &ev.WindowEnd, &ev.Score, &ev.Level, &detailsRaw, &evalDateText); err == nil {
		ev.EvaluatedAt, _ = time.Parse(time.RFC3339Nano, evalDateText)
		_ = json.Unmarshal([]byte(detailsRaw), &ev.Details)
		m.LatestEvaluation = &ev
	}

	return m, nil
}

func (s *Store) ListMonitors(ctx context.Context, enabledOnly bool) ([]monitor.Monitor, error) {
	query := `SELECT id, title, place_name, lat, lng, crs, criteria_json, origin_city, max_budget, enabled, created_at, updated_at
		FROM attraction_monitors`
	if enabledOnly {
		query += " WHERE enabled = 1"
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []monitor.Monitor
	for rows.Next() {
		var m monitor.Monitor
		var criteriaRaw, createdText, updatedText string
		var enabledInt int

		if err := rows.Scan(&m.ID, &m.Title, &m.PlaceName, &m.Lat, &m.Lng, &m.CRS, &criteriaRaw, &m.OriginCity, &m.MaxBudget, &enabledInt, &createdText, &updatedText); err != nil {
			continue
		}
		m.Enabled = enabledInt == 1
		m.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdText)
		m.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedText)
		m.Criteria, _ = monitor.CriteriaFromJSON(criteriaRaw)

		// Fetch top recommendation snapshot for card
		evalQuery := `SELECT id, monitor_id, window_start, window_end, score, level, details_json, evaluated_at
			FROM monitor_evaluations WHERE monitor_id = ?
			ORDER BY CASE level WHEN 'strongly_recommended' THEN 3 WHEN 'acceptable' THEN 2 WHEN 'waiting' THEN 1 ELSE 0 END DESC, score DESC, window_start ASC LIMIT 1`
		evalRow := s.db.QueryRowContext(ctx, evalQuery, m.ID)
		var ev monitor.Evaluation
		var detailsRaw, evalDateText string
		if err := evalRow.Scan(&ev.ID, &ev.MonitorID, &ev.WindowStart, &ev.WindowEnd, &ev.Score, &ev.Level, &detailsRaw, &evalDateText); err == nil {
			ev.EvaluatedAt, _ = time.Parse(time.RFC3339Nano, evalDateText)
			_ = json.Unmarshal([]byte(detailsRaw), &ev.Details)
			m.LatestEvaluation = &ev
		}

		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) UpdateMonitor(ctx context.Context, m monitor.Monitor) error {
	criteriaJSON, err := m.Criteria.ToJSON()
	if err != nil {
		return fmt.Errorf("encode criteria: %w", err)
	}

	enabledInt := 0
	if m.Enabled {
		enabledInt = 1
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	query := `UPDATE attraction_monitors SET title=?, place_name=?, lat=?, lng=?, crs=?, criteria_json=?, origin_city=?, max_budget=?, enabled=?, updated_at=?
		WHERE id = ?`
	res, err := s.db.ExecContext(ctx, query,
		m.Title, m.PlaceName, m.Lat, m.Lng, m.CRS, criteriaJSON,
		m.OriginCity, m.MaxBudget, enabledInt, now, m.ID,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrMonitorNotFound
	}
	return nil
}

func (s *Store) DeleteMonitor(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, "DELETE FROM attraction_monitors WHERE id = ?", id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrMonitorNotFound
	}
	return nil
}

func (s *Store) SaveMonitorEvaluations(ctx context.Context, monitorID string, evals []monitor.Evaluation) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Clear previous evaluations for this monitor
	if _, err := tx.ExecContext(ctx, "DELETE FROM monitor_evaluations WHERE monitor_id = ?", monitorID); err != nil {
		return err
	}

	insertQuery := `INSERT INTO monitor_evaluations (id, monitor_id, window_start, window_end, score, level, details_json, evaluated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	stmt, err := tx.PrepareContext(ctx, insertQuery)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, ev := range evals {
		detailsJSON, _ := json.Marshal(ev.Details)
		if _, err := stmt.ExecContext(ctx, ev.ID, monitorID, ev.WindowStart, ev.WindowEnd, ev.Score, string(ev.Level), string(detailsJSON), now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) GetMonitorEvaluations(ctx context.Context, monitorID string, limit int) ([]monitor.Evaluation, error) {
	if limit <= 0 {
		limit = 30
	}
	query := `SELECT id, monitor_id, window_start, window_end, score, level, details_json, evaluated_at
		FROM monitor_evaluations WHERE monitor_id = ? ORDER BY window_start ASC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []monitor.Evaluation
	for rows.Next() {
		var ev monitor.Evaluation
		var detailsRaw, evalDateText string
		if err := rows.Scan(&ev.ID, &ev.MonitorID, &ev.WindowStart, &ev.WindowEnd, &ev.Score, &ev.Level, &detailsRaw, &evalDateText); err != nil {
			continue
		}
		ev.EvaluatedAt, _ = time.Parse(time.RFC3339Nano, evalDateText)
		_ = json.Unmarshal([]byte(detailsRaw), &ev.Details)
		result = append(result, ev)
	}
	return result, rows.Err()
}

func (s *Store) GetTopEvaluations(ctx context.Context, limit int) ([]monitor.Evaluation, error) {
	if limit <= 0 {
		limit = 5
	}
	query := `SELECT id, monitor_id, window_start, window_end, score, level, details_json, evaluated_at
		FROM monitor_evaluations WHERE level = 'strongly_recommended' ORDER BY score DESC, window_start ASC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []monitor.Evaluation
	for rows.Next() {
		var ev monitor.Evaluation
		var detailsRaw, evalDateText string
		if err := rows.Scan(&ev.ID, &ev.MonitorID, &ev.WindowStart, &ev.WindowEnd, &ev.Score, &ev.Level, &detailsRaw, &evalDateText); err != nil {
			continue
		}
		ev.EvaluatedAt, _ = time.Parse(time.RFC3339Nano, evalDateText)
		_ = json.Unmarshal([]byte(detailsRaw), &ev.Details)
		result = append(result, ev)
	}
	return result, rows.Err()
}

func (s *Store) GetHolidayCalendar(ctx context.Context, year int) (map[string]monitor.HolidayRecord, error) {
	query := `SELECT calendar_date, holiday_name, is_holiday, is_workday, year FROM holiday_calendar WHERE year = ?`
	rows, err := s.db.QueryContext(ctx, query, year)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]monitor.HolidayRecord)
	for rows.Next() {
		var h monitor.HolidayRecord
		var isH, isW int
		if err := rows.Scan(&h.CalendarDate, &h.HolidayName, &isH, &isW, &h.Year); err != nil {
			continue
		}
		h.IsHoliday = isH == 1
		h.IsWorkday = isW == 1
		result[h.CalendarDate] = h
	}
	return result, rows.Err()
}
