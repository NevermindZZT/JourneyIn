package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"journeyin/internal/domain"
	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

type AddStopInput struct {
	ID                  string
	Sequence            int
	Kind                string
	Title               string
	Address             string
	Location            json.RawMessage
	TimeWindow          json.RawMessage
	DescriptionMarkdown string
	Links               []domain.Link
	Weather             json.RawMessage
}

type PlanInput struct {
	Provider    journeymaps.ProviderID
	Mode        journeymaps.TravelMode
	DayID       string
	DepartureAt *time.Time
}

var ErrPlanningLocationRequired = errors.New("all planned stops must have a saved location")

func (s *TripService) AddStop(ctx context.Context, tripID string, expectedRevision int, dayID string, input AddStopInput, source string) (store.TripRecord, error) {
	if strings.TrimSpace(input.Title) == "" {
		return store.TripRecord{}, errors.New("stop title is required")
	}
	if len(input.Location) == 0 || string(input.Location) == "null" {
		return store.TripRecord{}, ErrPlanningLocationRequired
	}
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, fmt.Errorf("decode trip: %w", err)
	}
	stopID := strings.TrimSpace(input.ID)
	if stopID == "" {
		stopID, err = domain.NewID("stop")
		if err != nil {
			return store.TripRecord{}, err
		}
	}
	stop := domain.Stop{ID: stopID, Sequence: input.Sequence, Kind: input.Kind, Title: strings.TrimSpace(input.Title), Address: strings.TrimSpace(input.Address), Location: append(json.RawMessage(nil), input.Location...), TimeWindow: append(json.RawMessage(nil), input.TimeWindow...), DescriptionMarkdown: input.DescriptionMarkdown, Links: input.Links, Weather: append(json.RawMessage(nil), input.Weather...)}
	found := false
	for dayIndex := range trip.Days {
		if trip.Days[dayIndex].ID != dayID {
			continue
		}
		found = true
		day := &trip.Days[dayIndex]
		position := len(day.Stops)
		if stop.Sequence > 0 && stop.Sequence <= len(day.Stops) {
			position = stop.Sequence - 1
		}
		day.Stops = append(day.Stops, domain.Stop{})
		copy(day.Stops[position+1:], day.Stops[position:])
		day.Stops[position] = stop
		for index := range day.Stops {
			day.Stops[index].Sequence = index + 1
		}
		day.Legs = nil
		break
	}
	if !found {
		return store.TripRecord{}, fmt.Errorf("day %s not found", dayID)
	}
	normalized, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, err
	}
	return s.Replace(ctx, tripID, expectedRevision, normalized, source)
}

func (s *TripService) PlanTrip(ctx context.Context, tripID string, expectedRevision int, input PlanInput, source string) (store.TripRecord, error) {
	if s.mapService == nil {
		return store.TripRecord{}, errors.New("map service is not configured")
	}
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, fmt.Errorf("decode trip: %w", err)
	}
	providerID := input.Provider
	if providerID == "" {
		providerID = journeymaps.ProviderID(trip.Map.PreferredProvider)
	}
	if providerID == "" {
		providerID = journeymaps.ProviderBaidu
	}
	mode := input.Mode
	if mode == "" {
		mode = journeymaps.TravelMode(trip.Map.DefaultMode)
	}
	if mode == "" {
		mode = journeymaps.ModeWalking
	}
	if !validTravelMode(mode) {
		return store.TripRecord{}, fmt.Errorf("unsupported planning mode %q", mode)
	}
	planned := 0
	for dayIndex := range trip.Days {
		day := &trip.Days[dayIndex]
		if input.DayID != "" && day.ID != input.DayID {
			continue
		}
		day.Legs = nil
		if len(day.Stops) < 2 {
			continue
		}
		legs := make([]domain.RouteLeg, 0, len(day.Stops)-1)
		for index := 0; index+1 < len(day.Stops); index++ {
			origin, err := savedPoint(day.Stops[index].Location)
			if err != nil {
				return store.TripRecord{}, fmt.Errorf("day %s stop %s: %w", day.ID, day.Stops[index].ID, err)
			}
			destination, err := savedPoint(day.Stops[index+1].Location)
			if err != nil {
				return store.TripRecord{}, fmt.Errorf("day %s stop %s: %w", day.ID, day.Stops[index+1].ID, err)
			}
			snapshot, err := s.mapService.Route(ctx, providerID, journeymaps.RouteRequest{Origin: origin, Destination: destination, Mode: mode, DepartureAt: input.DepartureAt})
			if err != nil {
				return store.TripRecord{}, err
			}
			legID, err := domain.NewID("leg")
			if err != nil {
				return store.TripRecord{}, err
			}
			legs = append(legs, domain.RouteLeg{ID: legID, FromStopID: day.Stops[index].ID, ToStopID: day.Stops[index+1].ID, Mode: string(mode), Snapshots: []domain.RouteSnapshot{routeSnapshot(snapshot)}})
			planned++
		}
		day.Legs = legs
	}
	if input.DayID != "" {
		found := false
		for _, day := range trip.Days {
			if day.ID == input.DayID {
				found = true
				break
			}
		}
		if !found {
			return store.TripRecord{}, fmt.Errorf("day %s not found", input.DayID)
		}
	}
	if planned == 0 {
		return store.TripRecord{}, errors.New("at least two saved stops are required to plan a route")
	}
	normalized, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, err
	}
	return s.Replace(ctx, tripID, expectedRevision, normalized, source)
}

func savedPoint(raw json.RawMessage) (journeymaps.GeoPoint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return journeymaps.GeoPoint{}, ErrPlanningLocationRequired
	}
	var location struct {
		Preferred   string                          `json:"preferred"`
		Coordinates map[string]journeymaps.GeoPoint `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &location); err != nil {
		return journeymaps.GeoPoint{}, fmt.Errorf("invalid saved location: %w", err)
	}
	keys := []string{location.Preferred, string(journeymaps.CRSBD09LL), string(journeymaps.CRSGCJ02), string(journeymaps.CRSWGS84)}
	for _, key := range keys {
		if point, ok := location.Coordinates[key]; ok && point.CRS != "" {
			return point, nil
		} else if ok {
			point.CRS = journeymaps.CoordinateSystem(key)
			return point, nil
		}
	}
	return journeymaps.GeoPoint{}, ErrPlanningLocationRequired
}

func routeSnapshot(snapshot journeymaps.RouteSnapshot) domain.RouteSnapshot {
	geometry := make([][]float64, 0, len(snapshot.Geometry))
	for _, point := range snapshot.Geometry {
		geometry = append(geometry, []float64{point.Lng, point.Lat})
	}
	return domain.RouteSnapshot{Provider: string(snapshot.Provider), CoordinateSystem: string(snapshot.CoordinateSystem), Geometry: geometry, DistanceM: snapshot.DistanceM, DurationS: snapshot.DurationS, FetchedAt: snapshot.FetchedAt.UTC().Format(time.RFC3339Nano), ExpiresAt: snapshot.ExpiresAt.UTC().Format(time.RFC3339Nano)}
}
func validTravelMode(mode journeymaps.TravelMode) bool {
	return mode == journeymaps.ModeDriving || mode == journeymaps.ModeWalking || mode == journeymaps.ModeCycling || mode == journeymaps.ModeTransit
}

type WeatherInput struct {
	Provider  journeymaps.ProviderID
	LocalDate string
}

func (s *TripService) AddSubStop(ctx context.Context, tripID string, expectedRevision int, dayID, parentStopID string, input AddStopInput, source string) (store.TripRecord, error) {
	if strings.TrimSpace(input.Title) == "" {
		return store.TripRecord{}, errors.New("sub-stop title is required")
	}
	if len(input.Location) == 0 || string(input.Location) == "null" {
		return store.TripRecord{}, ErrPlanningLocationRequired
	}
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, fmt.Errorf("decode trip: %w", err)
	}
	stopID, err := domain.NewID("substop")
	if err != nil {
		return store.TripRecord{}, err
	}
	if strings.TrimSpace(input.ID) != "" {
		stopID = strings.TrimSpace(input.ID)
	}
	child := domain.SubStop{ID: stopID, Sequence: input.Sequence, Kind: input.Kind, Title: strings.TrimSpace(input.Title), Address: strings.TrimSpace(input.Address), Location: append(json.RawMessage(nil), input.Location...), TimeWindow: append(json.RawMessage(nil), input.TimeWindow...), DescriptionMarkdown: input.DescriptionMarkdown, Links: input.Links, Weather: append(json.RawMessage(nil), input.Weather...)}
	found := false
	for dayIndex := range trip.Days {
		if trip.Days[dayIndex].ID != dayID {
			continue
		}
		for stopIndex := range trip.Days[dayIndex].Stops {
			if trip.Days[dayIndex].Stops[stopIndex].ID != parentStopID {
				continue
			}
			found = true
			parent := &trip.Days[dayIndex].Stops[stopIndex]
			position := len(parent.Children)
			if child.Sequence > 0 && child.Sequence <= len(parent.Children) {
				position = child.Sequence - 1
			}
			parent.Children = append(parent.Children, domain.SubStop{})
			copy(parent.Children[position+1:], parent.Children[position:])
			parent.Children[position] = child
			for index := range parent.Children {
				parent.Children[index].Sequence = index + 1
			}
			break
		}
		break
	}
	if !found {
		return store.TripRecord{}, fmt.Errorf("parent stop %s not found", parentStopID)
	}
	normalized, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, err
	}
	return s.Replace(ctx, tripID, expectedRevision, normalized, source)
}

func (s *TripService) RefreshWeather(ctx context.Context, tripID string, expectedRevision int, dayID, stopID string, input WeatherInput, source string) (store.TripRecord, error) {
	if s.mapService == nil {
		return store.TripRecord{}, errors.New("map service is not configured")
	}
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, err
	}
	provider := input.Provider
	if provider == "" {
		provider = journeymaps.ProviderBaidu
	}
	var targetLocation json.RawMessage
	var dayDate string
	found := false
	for dayIndex := range trip.Days {
		if trip.Days[dayIndex].ID != dayID {
			continue
		}
		dayDate = trip.Days[dayIndex].Date
		for stopIndex := range trip.Days[dayIndex].Stops {
			stop := &trip.Days[dayIndex].Stops[stopIndex]
			if stop.ID == stopID {
				targetLocation = stop.Location
				found = true
				break
			}
			for childIndex := range stop.Children {
				child := &stop.Children[childIndex]
				if child.ID == stopID {
					targetLocation = child.Location
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		break
	}
	if !found {
		return store.TripRecord{}, fmt.Errorf("stop %s not found", stopID)
	}
	point, err := savedPoint(targetLocation)
	if err != nil {
		return store.TripRecord{}, err
	}
	localDate := input.LocalDate
	if localDate == "" {
		localDate = dayDate
	}
	snapshot, err := s.mapService.Weather(ctx, provider, journeymaps.WeatherRequest{Location: point, LocalDate: localDate, Timezone: trip.Timezone})
	if err != nil {
		return store.TripRecord{}, err
	}
	weather, err := json.Marshal(snapshot)
	if err != nil {
		return store.TripRecord{}, err
	}
	for dayIndex := range trip.Days {
		if trip.Days[dayIndex].ID != dayID {
			continue
		}
		for stopIndex := range trip.Days[dayIndex].Stops {
			stop := &trip.Days[dayIndex].Stops[stopIndex]
			if stop.ID == stopID {
				stop.Weather = weather
				found = true
				break
			}
			for childIndex := range stop.Children {
				if stop.Children[childIndex].ID == stopID {
					stop.Children[childIndex].Weather = weather
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		break
	}
	normalized, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, err
	}
	return s.Replace(ctx, tripID, expectedRevision, normalized, source)
}
