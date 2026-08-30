package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
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
		clearRouteLegsAroundDay(&trip, dayIndex)
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
	releasePlan, err := s.acquirePlan(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	defer releasePlan()
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	if record.Revision != expectedRevision {
		return store.TripRecord{}, store.ErrRevisionConflict
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
	type routePlanSegment struct {
		dayIndex int
		from     domain.Stop
		to       domain.Stop
	}
	segments := make([]routePlanSegment, 0)
	foundDay := input.DayID == ""
	for dayIndex := range trip.Days {
		day := &trip.Days[dayIndex]
		if input.DayID != "" && day.ID != input.DayID {
			continue
		}
		foundDay = true
		day.Legs = nil
		stops := orderedRouteStops(day.Stops)
		// A day's first route point can be the previous day's final point.
		// Store this boundary leg on the destination day so a single-day view
		// still shows the route entering that day.
		if dayIndex > 0 && len(stops) > 0 {
			previousStops := orderedRouteStops(trip.Days[dayIndex-1].Stops)
			if len(previousStops) > 0 {
				segments = append(segments, routePlanSegment{dayIndex: dayIndex, from: previousStops[len(previousStops)-1], to: stops[0]})
			}
		}
		for index := 0; index+1 < len(stops); index++ {
			segments = append(segments, routePlanSegment{dayIndex: dayIndex, from: stops[index], to: stops[index+1]})
		}
	}
	if input.DayID != "" && !foundDay {
		return store.TripRecord{}, fmt.Errorf("day %s not found", input.DayID)
	}
	if len(segments) == 0 {
		return store.TripRecord{}, errors.New("at least two adjacent saved stops are required to plan a route")
	}
	for _, segment := range segments {
		origin, err := savedPoint(segment.from.Location)
		if err != nil {
			return store.TripRecord{}, fmt.Errorf("day %s stop %s: %w", trip.Days[segment.dayIndex].ID, segment.from.ID, err)
		}
		destination, err := savedPoint(segment.to.Location)
		if err != nil {
			return store.TripRecord{}, fmt.Errorf("day %s stop %s: %w", trip.Days[segment.dayIndex].ID, segment.to.ID, err)
		}
		snapshot, err := s.mapService.Route(ctx, providerID, journeymaps.RouteRequest{Origin: origin, Destination: destination, Mode: mode, DepartureAt: input.DepartureAt})
		if err != nil {
			return store.TripRecord{}, fmt.Errorf("route %s -> %s: %w", segment.from.ID, segment.to.ID, err)
		}
		legID, err := domain.NewID("leg")
		if err != nil {
			return store.TripRecord{}, err
		}
		trip.Days[segment.dayIndex].Legs = append(trip.Days[segment.dayIndex].Legs, domain.RouteLeg{ID: legID, FromStopID: segment.from.ID, ToStopID: segment.to.ID, Mode: string(mode), Snapshots: []domain.RouteSnapshot{routeSnapshot(snapshot)}})
	}
	normalized, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, err
	}
	return s.Replace(ctx, tripID, expectedRevision, normalized, source)
}

func orderedRouteStops(stops []domain.Stop) []domain.Stop {
	ordered := append([]domain.Stop(nil), stops...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Sequence < ordered[j].Sequence })
	return ordered
}

func clearRouteLegsAroundDay(trip *domain.Trip, dayIndex int) {
	if dayIndex < 0 || dayIndex >= len(trip.Days) {
		return
	}
	trip.Days[dayIndex].Legs = nil
	if dayIndex+1 < len(trip.Days) {
		trip.Days[dayIndex+1].Legs = nil
	}
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

// MoveStop changes the order of a main stop or one of its child stops.
// Main-stop moves invalidate this day's legs and the following day's cross-day leg; child-stop moves do not.
func (s *TripService) MoveStop(ctx context.Context, tripID string, expectedRevision int, dayID, stopID, direction, source string) (store.TripRecord, error) {
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction != "up" && direction != "down" {
		return store.TripRecord{}, errors.New("direction must be up or down")
	}
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	if record.Revision != expectedRevision {
		return store.TripRecord{}, store.ErrRevisionConflict
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, fmt.Errorf("decode trip: %w", err)
	}
	found := false
	moved := false
	for dayIndex := range trip.Days {
		if trip.Days[dayIndex].ID != dayID {
			continue
		}
		day := &trip.Days[dayIndex]
		for stopIndex := range day.Stops {
			if day.Stops[stopIndex].ID == stopID {
				found = true
				newIndex := stopIndex
				if direction == "up" && stopIndex > 0 {
					newIndex = stopIndex - 1
				} else if direction == "down" && stopIndex+1 < len(day.Stops) {
					newIndex = stopIndex + 1
				}
				if newIndex != stopIndex {
					day.Stops[stopIndex], day.Stops[newIndex] = day.Stops[newIndex], day.Stops[stopIndex]
					moved = true
					clearRouteLegsAroundDay(&trip, dayIndex)
				}
				for index := range day.Stops {
					day.Stops[index].Sequence = index + 1
				}
				break
			}
			parent := &day.Stops[stopIndex]
			for childIndex := range parent.Children {
				if parent.Children[childIndex].ID != stopID {
					continue
				}
				found = true
				newIndex := childIndex
				if direction == "up" && childIndex > 0 {
					newIndex = childIndex - 1
				} else if direction == "down" && childIndex+1 < len(parent.Children) {
					newIndex = childIndex + 1
				}
				if newIndex != childIndex {
					parent.Children[childIndex], parent.Children[newIndex] = parent.Children[newIndex], parent.Children[childIndex]
					moved = true
				}
				for index := range parent.Children {
					parent.Children[index].Sequence = index + 1
				}
				break
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
	if !moved {
		return record, nil
	}
	normalized, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, err
	}
	return s.Replace(ctx, tripID, expectedRevision, normalized, source)
}

// ReorderStop moves a main stop or child stop to an exact 1-based sequence.
// Main-stop reordering invalidates this day's legs and the following day's cross-day leg.
func (s *TripService) ReorderStop(ctx context.Context, tripID string, expectedRevision int, dayID, stopID string, targetSequence int, source string) (store.TripRecord, error) {
	if targetSequence < 1 {
		return store.TripRecord{}, errors.New("target sequence must be positive")
	}
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	if record.Revision != expectedRevision {
		return store.TripRecord{}, store.ErrRevisionConflict
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, fmt.Errorf("decode trip: %w", err)
	}
	found := false
	moved := false
	for dayIndex := range trip.Days {
		if trip.Days[dayIndex].ID != dayID {
			continue
		}
		day := &trip.Days[dayIndex]
		for stopIndex := range day.Stops {
			if day.Stops[stopIndex].ID == stopID {
				found = true
				if targetSequence > len(day.Stops) {
					return store.TripRecord{}, fmt.Errorf("target sequence must be between 1 and %d", len(day.Stops))
				}
				newIndex := targetSequence - 1
				if newIndex != stopIndex {
					stop := day.Stops[stopIndex]
					if newIndex > stopIndex {
						copy(day.Stops[stopIndex:newIndex], day.Stops[stopIndex+1:newIndex+1])
					} else {
						copy(day.Stops[newIndex+1:stopIndex+1], day.Stops[newIndex:stopIndex])
					}
					day.Stops[newIndex] = stop
					moved = true
					clearRouteLegsAroundDay(&trip, dayIndex)
				}
				for index := range day.Stops {
					day.Stops[index].Sequence = index + 1
				}
				break
			}
			parent := &day.Stops[stopIndex]
			for childIndex := range parent.Children {
				if parent.Children[childIndex].ID != stopID {
					continue
				}
				found = true
				if targetSequence > len(parent.Children) {
					return store.TripRecord{}, fmt.Errorf("target sequence must be between 1 and %d", len(parent.Children))
				}
				newIndex := targetSequence - 1
				if newIndex != childIndex {
					child := parent.Children[childIndex]
					if newIndex > childIndex {
						copy(parent.Children[childIndex:newIndex], parent.Children[childIndex+1:newIndex+1])
					} else {
						copy(parent.Children[newIndex+1:childIndex+1], parent.Children[newIndex:childIndex])
					}
					parent.Children[newIndex] = child
					moved = true
				}
				for index := range parent.Children {
					parent.Children[index].Sequence = index + 1
				}
				break
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
	if !moved {
		return record, nil
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

func (s *TripService) DeleteStop(ctx context.Context, tripID string, expectedRevision int, dayID, stopID, source string) (store.TripRecord, error) {
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, err
	}
	var trip domain.Trip
	if err := json.Unmarshal(record.Document, &trip); err != nil {
		return store.TripRecord{}, fmt.Errorf("decode trip: %w", err)
	}
	found := false
	for dayIndex := range trip.Days {
		if trip.Days[dayIndex].ID != dayID {
			continue
		}
		day := &trip.Days[dayIndex]
		for stopIndex := range day.Stops {
			if day.Stops[stopIndex].ID == stopID {
				day.Stops = append(day.Stops[:stopIndex], day.Stops[stopIndex+1:]...)
				for index := range day.Stops {
					day.Stops[index].Sequence = index + 1
				}
				clearRouteLegsAroundDay(&trip, dayIndex)
				found = true
				break
			}
			parent := &day.Stops[stopIndex]
			for childIndex := range parent.Children {
				if parent.Children[childIndex].ID == stopID {
					parent.Children = append(parent.Children[:childIndex], parent.Children[childIndex+1:]...)
					for index := range parent.Children {
						parent.Children[index].Sequence = index + 1
					}
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
	normalized, err := json.Marshal(trip)
	if err != nil {
		return store.TripRecord{}, err
	}
	return s.Replace(ctx, tripID, expectedRevision, normalized, source)
}
