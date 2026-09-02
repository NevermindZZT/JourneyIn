package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"journeyin/internal/domain"
	"journeyin/internal/store"
)

const tripDateLayout = "2006-01-02"

var ErrTripDetailsEmpty = errors.New("at least one trip detail field is required")
var ErrIdempotencyKeyRequired = errors.New("idempotency key is required")

type UpdateTripDetailsInput struct {
	Title     *string
	DateRange *domain.DateRange
}

type TripDetailsChangeSummary struct {
	Changed             bool `json:"changed"`
	TitleChanged        bool `json:"title_changed"`
	DateRangeChanged    bool `json:"date_range_changed"`
	AddedDays           int  `json:"added_days"`
	RemovedDays         int  `json:"removed_days"`
	ClearedWeatherStops int  `json:"cleared_weather_stops"`
}

type DateRangeConflictDay struct {
	DayID     string `json:"day_id"`
	Date      string `json:"date"`
	StopCount int    `json:"stop_count"`
}

type DateRangeConflictError struct {
	Days []DateRangeConflictDay
}

func (e *DateRangeConflictError) Error() string {
	if e == nil || len(e.Days) == 0 {
		return "date range would remove planning points"
	}
	return fmt.Sprintf("date range would remove planning points from %d day(s)", len(e.Days))
}

func (s *TripService) UpdateTripDetails(ctx context.Context, tripID string, expectedRevision int, input UpdateTripDetailsInput, source string) (store.TripRecord, TripDetailsChangeSummary, error) {
	if input.Title == nil && input.DateRange == nil {
		return store.TripRecord{}, TripDetailsChangeSummary{}, ErrTripDetailsEmpty
	}

	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, TripDetailsChangeSummary{}, err
	}
	if record.Revision != expectedRevision {
		return store.TripRecord{}, TripDetailsChangeSummary{}, store.ErrRevisionConflict
	}

	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, TripDetailsChangeSummary{}, fmt.Errorf("decode trip: %w", err)
	}

	var changes TripDetailsChangeSummary
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return store.TripRecord{}, changes, errors.New("trip title is required")
		}
		if utf8.RuneCountInString(title) > 120 {
			return store.TripRecord{}, changes, errors.New("trip title must be at most 120 characters")
		}
		if title != trip.Title {
			trip.Title = title
			changes.TitleChanged = true
		}
	}

	if input.DateRange != nil {
		start, _, dayCount, err := parseTripDateRange(*input.DateRange)
		if err != nil {
			return store.TripRecord{}, changes, err
		}
		if input.DateRange.Start != trip.DateRange.Start || input.DateRange.End != trip.DateRange.End {
			added, removed, cleared, err := rebaseTripDays(&trip, start, dayCount)
			if err != nil {
				return store.TripRecord{}, changes, err
			}
			trip.DateRange = domain.DateRange{Start: strings.TrimSpace(input.DateRange.Start), End: strings.TrimSpace(input.DateRange.End)}
			changes.DateRangeChanged = true
			changes.AddedDays = added
			changes.RemovedDays = removed
			changes.ClearedWeatherStops = cleared
		}
	}

	if !changes.TitleChanged && !changes.DateRangeChanged {
		return record, changes, nil
	}

	normalized, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, changes, fmt.Errorf("encode trip: %w", err)
	}
	updated, err := s.Replace(ctx, tripID, expectedRevision, normalized, source)
	if err != nil {
		return store.TripRecord{}, changes, err
	}
	changes.Changed = true
	return updated, changes, nil
}

type tripDetailsIdempotencyPayload struct {
	Record  store.TripRecord
	Changes TripDetailsChangeSummary
}

func (s *TripService) UpdateTripDetailsIdempotent(ctx context.Context, tripID string, expectedRevision int, input UpdateTripDetailsInput, idempotencyKey, source string) (store.TripRecord, TripDetailsChangeSummary, bool, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return store.TripRecord{}, TripDetailsChangeSummary{}, false, ErrIdempotencyKeyRequired
	}
	requestData, err := json.Marshal(struct {
		TripID           string
		ExpectedRevision int
		Input            UpdateTripDetailsInput
	}{TripID: tripID, ExpectedRevision: expectedRevision, Input: input})
	if err != nil {
		return store.TripRecord{}, TripDetailsChangeSummary{}, false, err
	}
	requestHash := domain.ContentHash(requestData)
	scope := "trip:" + tripID + ":details"

	s.mu.Lock()
	defer s.mu.Unlock()
	if replay, ok, err := s.store.Idempotency(ctx, scope, idempotencyKey, requestHash); err != nil {
		return store.TripRecord{}, TripDetailsChangeSummary{}, false, err
	} else if ok {
		var payload tripDetailsIdempotencyPayload
		if err := json.Unmarshal(replay, &payload); err != nil {
			return store.TripRecord{}, TripDetailsChangeSummary{}, false, err
		}
		return payload.Record, payload.Changes, true, nil
	}

	record, changes, err := s.UpdateTripDetails(ctx, tripID, expectedRevision, input, source)
	if err != nil {
		return store.TripRecord{}, changes, false, err
	}
	payload, err := json.Marshal(tripDetailsIdempotencyPayload{Record: record, Changes: changes})
	if err != nil {
		return store.TripRecord{}, changes, false, err
	}
	if err := s.store.SaveIdempotency(ctx, scope, idempotencyKey, requestHash, payload); err != nil {
		return store.TripRecord{}, changes, false, err
	}
	return record, changes, false, nil
}

func parseTripDateRange(value domain.DateRange) (time.Time, time.Time, int, error) {
	start, err := time.Parse(tripDateLayout, strings.TrimSpace(value.Start))
	if err != nil {
		return time.Time{}, time.Time{}, 0, errors.New("date_range.start must use YYYY-MM-DD")
	}
	end, err := time.Parse(tripDateLayout, strings.TrimSpace(value.End))
	if err != nil {
		return time.Time{}, time.Time{}, 0, errors.New("date_range.end must use YYYY-MM-DD")
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, 0, errors.New("date_range.end must not be before start")
	}
	dayCount := int(end.Sub(start)/(24*time.Hour)) + 1
	if dayCount > 60 {
		return time.Time{}, time.Time{}, 0, errors.New("date range cannot exceed 60 days")
	}
	return start, end, dayCount, nil
}

func rebaseTripDays(trip *domain.Trip, start time.Time, newDayCount int) (int, int, int, error) {
	if newDayCount < len(trip.Days) {
		conflicts := make([]DateRangeConflictDay, 0)
		for _, day := range trip.Days[newDayCount:] {
			stopCount := countDayPlanningPoints(day)
			if stopCount > 0 {
				conflicts = append(conflicts, DateRangeConflictDay{DayID: day.ID, Date: day.Date, StopCount: stopCount})
			}
		}
		if len(conflicts) > 0 {
			return 0, 0, 0, &DateRangeConflictError{Days: conflicts}
		}
	}

	kept := len(trip.Days)
	if kept > newDayCount {
		kept = newDayCount
	}
	clearedWeatherStops := 0
	for index := 0; index < kept; index++ {
		newDate := start.AddDate(0, 0, index).Format(tripDateLayout)
		if trip.Days[index].Date != newDate {
			clearedWeatherStops += clearDayWeather(&trip.Days[index])
			trip.Days[index].Date = newDate
		}
	}

	removedDays := 0
	if newDayCount < len(trip.Days) {
		removedDays = len(trip.Days) - newDayCount
		trip.Days = trip.Days[:newDayCount]
	}

	addedDays := 0
	for len(trip.Days) < newDayCount {
		index := len(trip.Days)
		id, err := domain.NewID("day")
		if err != nil {
			return 0, 0, 0, err
		}
		trip.Days = append(trip.Days, domain.Day{
			ID:    id,
			Date:  start.AddDate(0, 0, index).Format(tripDateLayout),
			Title: fmt.Sprintf("第 %d 天", index+1),
			Stops: []domain.Stop{},
			Legs:  []domain.RouteLeg{},
		})
		addedDays++
	}

	return addedDays, removedDays, clearedWeatherStops, nil
}

func countDayPlanningPoints(day domain.Day) int {
	count := 0
	for _, stop := range day.Stops {
		count++
		count += len(stop.Children)
	}
	return count
}

func clearDayWeather(day *domain.Day) int {
	cleared := 0
	for stopIndex := range day.Stops {
		stop := &day.Stops[stopIndex]
		if rawJSONPresent(stop.Weather) {
			stop.Weather = nil
			cleared++
		}
		for childIndex := range stop.Children {
			child := &stop.Children[childIndex]
			if rawJSONPresent(child.Weather) {
				child.Weather = nil
				cleared++
			}
		}
	}
	return cleared
}

func rawJSONPresent(value json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "null"
}
