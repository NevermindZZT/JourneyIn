package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
	"journeyin/internal/weather"
)

// WeatherBatchSize bounds provider calls per HTTP request so large trips can
// advance in several requests rather than timing out behind one long response.
const WeatherBatchSize = 5

type WeatherBatchItem struct {
	DayID  string `json:"day_id"`
	Date   string `json:"date"`
	StopID string `json:"stop_id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type WeatherBatchResult struct {
	Total      int                `json:"total"`
	Offset     int                `json:"offset"`
	NextOffset int                `json:"next_offset"`
	Updated    int                `json:"updated"`
	Skipped    int                `json:"skipped"`
	Failed     int                `json:"failed"`
	Items      []WeatherBatchItem `json:"items"`
}

type weatherBatchTarget struct {
	dayIndex   int
	stopIndex  int
	childIndex int // -1 means main stop
}

// RefreshTripWeatherBatch refreshes up to WeatherBatchSize planning points,
// including child points, with one optimistic-revision write. Failures are
// reported per point and never replace an existing usable snapshot.
func (s *TripService) RefreshTripWeatherBatch(ctx context.Context, tripID string, expectedRevision, offset int, input WeatherInput, source string) (store.TripRecord, WeatherBatchResult, error) {
	result := WeatherBatchResult{Offset: offset, Items: make([]WeatherBatchItem, 0, WeatherBatchSize)}
	if offset < 0 {
		return store.TripRecord{}, result, errors.New("weather offset must be nonnegative")
	}
	if s.weatherService == nil && s.mapService == nil {
		return store.TripRecord{}, result, errors.New("weather service is not configured")
	}
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, result, err
	}
	if record.Revision != expectedRevision {
		return store.TripRecord{}, result, store.ErrRevisionConflict
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, result, err
	}
	targets := make([]weatherBatchTarget, 0)
	for di := range trip.Days {
		for si := range trip.Days[di].Stops {
			targets = append(targets, weatherBatchTarget{di, si, -1})
			for ci := range trip.Days[di].Stops[si].Children {
				targets = append(targets, weatherBatchTarget{di, si, ci})
			}
		}
	}
	result.Total = len(targets)
	if offset > len(targets) {
		return store.TripRecord{}, result, errors.New("weather offset exceeds planning point count")
	}
	end := offset + WeatherBatchSize
	if end > len(targets) {
		end = len(targets)
	}
	result.NextOffset = end
	for _, target := range targets[offset:end] {
		if err := ctx.Err(); err != nil {
			return store.TripRecord{}, result, err
		}
		day := &trip.Days[target.dayIndex]
		var id, title string
		var location json.RawMessage
		var snapshot *json.RawMessage
		if target.childIndex < 0 {
			point := &day.Stops[target.stopIndex]
			id, title, location, snapshot = point.ID, point.Title, point.Location, &point.Weather
		} else {
			point := &day.Stops[target.stopIndex].Children[target.childIndex]
			id, title, location, snapshot = point.ID, point.Title, point.Location, &point.Weather
		}
		item := WeatherBatchItem{DayID: day.ID, Date: day.Date, StopID: id, Title: title}
		position, err := parseSavedLocation(location)
		if err != nil {
			item.Status, item.Reason = "skipped", "缺少可靠坐标"
			result.Skipped++
			result.Items = append(result.Items, item)
			continue
		}
		var weatherJSON []byte
		if s.weatherService != nil {
			requested := weather.ParseProviderID(string(input.Provider))
			forecast, weatherErr := s.weatherService.WeatherWithCache(ctx, requested, weather.WeatherRequest{Location: weather.GeoPoint{Lat: position.Point.Lat, Lng: position.Point.Lng, CRS: weather.CRS(position.Point.CRS)}, LocalDate: day.Date, Timezone: trip.Timezone, CityCode: position.CityCode, AdCode: position.AdCode}, false)
			if weatherErr == nil && forecast.Available {
				weatherJSON, err = json.Marshal(forecast)
			} else {
				err = weatherErr
				if err == nil {
					err = ErrWeatherUnavailable
				}
			}
		} else {
			provider := input.Provider
			if provider == "" {
				provider = s.DefaultMapProvider()
			}
			forecast, weatherErr := s.mapService.WeatherWithCache(ctx, provider, journeymaps.WeatherRequest{Location: position.Point, LocalDate: day.Date, Timezone: trip.Timezone, CityCode: position.CityCode, AdCode: position.AdCode}, false)
			if weatherErr == nil && forecast.Available {
				weatherJSON, err = json.Marshal(forecast)
			} else {
				err = weatherErr
				if err == nil {
					err = ErrWeatherUnavailable
				}
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return store.TripRecord{}, result, ctx.Err()
			}
			item.Status = "failed"
			item.Reason = "暂无可用预报或天气服务暂不可用"
			result.Failed++
		} else {
			*snapshot = weatherJSON
			item.Status = "updated"
			result.Updated++
		}
		result.Items = append(result.Items, item)
	}
	if result.Updated == 0 {
		return record, result, nil
	}
	document, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, result, err
	}
	updated, err := s.Replace(ctx, tripID, expectedRevision, document, source)
	if err != nil {
		return store.TripRecord{}, result, fmt.Errorf("save refreshed weather: %w", err)
	}
	return updated, result, nil
}
