package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	Provider         journeymaps.ProviderID
	Mode             journeymaps.TravelMode
	DayID            string
	DepartureAt      *time.Time
	Strategy         string
	AlternativeRoute int
}

type routePlanSegment struct {
	dayIndex int
	from     domain.Stop
	to       domain.Stop
}

type plannedRoute struct {
	dayIndex int
	fromID   string
	toID     string
	snapshot journeymaps.RouteSnapshot
}

type savedLocationData struct {
	Point        journeymaps.GeoPoint
	Coordinates  map[string]journeymaps.GeoPoint
	ProviderRefs map[string]string
	CityCode     string
	AdCode       string
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
		providerID = s.DefaultMapProvider()
	}
	if len(trip.Map.EnabledProviders) > 0 {
		enabled := false
		for _, candidate := range trip.Map.EnabledProviders {
			if candidate == string(providerID) {
				enabled = true
				break
			}
		}
		if !enabled {
			return store.TripRecord{}, fmt.Errorf("map provider %s is disabled for this trip", providerID)
		}
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
	// A successful planning operation establishes the Provider and mode that
	// should be restored when this Trip is opened again. Route snapshots remain
	// provider-specific, so this does not replace snapshots from other Providers.
	if providerID == journeymaps.ProviderAMap || providerID == journeymaps.ProviderBaidu {
		trip.Map.PreferredProvider = string(providerID)
	}
	trip.Map.DefaultMode = string(mode)
	segments := make([]routePlanSegment, 0)
	targetDays := make(map[int]bool)
	foundDay := input.DayID == ""
	for dayIndex := range trip.Days {
		day := &trip.Days[dayIndex]
		if input.DayID != "" && day.ID != input.DayID {
			continue
		}
		foundDay = true
		targetDays[dayIndex] = true
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
	planned := make([]plannedRoute, 0, len(segments))
	for _, segment := range segments {
		originData, err := parseSavedLocation(segment.from.Location)
		if err != nil {
			return store.TripRecord{}, fmt.Errorf("day %s stop %s: %w", trip.Days[segment.dayIndex].ID, segment.from.ID, err)
		}
		destinationData, err := parseSavedLocation(segment.to.Location)
		if err != nil {
			return store.TripRecord{}, fmt.Errorf("day %s stop %s: %w", trip.Days[segment.dayIndex].ID, segment.to.ID, err)
		}
		originPoint := savedPointForProvider(originData, providerID)
		destinationPoint := savedPointForProvider(destinationData, providerID)
		strategy := strings.TrimSpace(input.Strategy)
		if strategy == "" && mode == journeymaps.ModeDriving && providerID == journeymaps.ProviderAMap {
			strategy = "32"
		}
		var snapshot journeymaps.RouteSnapshot
		if sameRouteLocation(originData, destinationData, providerID, originPoint, destinationPoint) {
			now := time.Now().UTC()
			snapshot = journeymaps.RouteSnapshot{Provider: providerID, CoordinateSystem: routeCoordinateSystem(providerID), Mode: mode, Strategy: strategy, Source: "journeyin-same-location", DistanceM: 0, DurationS: 0, FetchedAt: now, ExpiresAt: now.Add(time.Hour)}
		} else {
			request := journeymaps.RouteRequest{
				Origin:              originPoint,
				Destination:         destinationPoint,
				Mode:                mode,
				DepartureAt:         input.DepartureAt,
				OriginPOIID:         providerPOIID(originData.ProviderRefs, providerID),
				DestinationPOIID:    providerPOIID(destinationData.ProviderRefs, providerID),
				OriginCityCode:      originData.CityCode,
				DestinationCityCode: destinationData.CityCode,
				Strategy:            strings.TrimSpace(input.Strategy),
				AlternativeRoute:    input.AlternativeRoute,
			}
			snapshot, err = s.mapService.Route(ctx, providerID, request)
			if err != nil {
				return store.TripRecord{}, fmt.Errorf("route %s -> %s: %w", segment.from.ID, segment.to.ID, err)
			}
		}
		if err != nil {
			return store.TripRecord{}, fmt.Errorf("route %s -> %s: %w", segment.from.ID, segment.to.ID, err)
		}
		planned = append(planned, plannedRoute{dayIndex: segment.dayIndex, fromID: segment.from.ID, toID: segment.to.ID, snapshot: snapshot})
	}
	for dayIndex := range targetDays {
		dayPlans := make([]plannedRoute, 0)
		for _, item := range planned {
			if item.dayIndex == dayIndex {
				dayPlans = append(dayPlans, item)
			}
		}
		if len(dayPlans) == 0 {
			trip.Days[dayIndex].Legs = nil
			continue
		}
		merged, err := mergeRouteLegs(trip.Days[dayIndex].Legs, dayPlans, providerID, mode)
		if err != nil {
			return store.TripRecord{}, err
		}
		trip.Days[dayIndex].Legs = merged
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

func parseSavedLocation(raw json.RawMessage) (savedLocationData, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return savedLocationData{}, ErrPlanningLocationRequired
	}
	var location struct {
		Preferred    string                          `json:"preferred"`
		Coordinates  map[string]journeymaps.GeoPoint `json:"coordinates"`
		ProviderRefs map[string]string               `json:"provider_refs"`
		CityCode     string                          `json:"citycode"`
		AdCode       string                          `json:"adcode"`
		Lat          *float64                        `json:"lat"`
		Lng          *float64                        `json:"lng"`
		CRS          string                          `json:"crs"`
	}
	if err := json.Unmarshal(raw, &location); err != nil {
		return savedLocationData{}, fmt.Errorf("invalid saved location: %w", err)
	}
	normalizedCoordinates := make(map[string]journeymaps.GeoPoint, len(location.Coordinates)+1)
	for rawCRS, point := range location.Coordinates {
		crs := normalizeSavedCRS(rawCRS)
		if crs == "" {
			crs = normalizeSavedCRS(string(point.CRS))
		}
		if crs == "" {
			continue
		}
		point.CRS = journeymaps.CoordinateSystem(crs)
		normalizedCoordinates[crs] = point
	}
	preferred := normalizeSavedCRS(location.Preferred)
	if len(normalizedCoordinates) == 0 && location.Lat != nil && location.Lng != nil {
		crs := normalizeSavedCRS(location.CRS)
		if crs == "" {
			crs = preferred
		}
		if crs != "" {
			normalizedCoordinates[crs] = journeymaps.GeoPoint{Lat: *location.Lat, Lng: *location.Lng, CRS: journeymaps.CoordinateSystem(crs)}
		}
	}
	keys := []string{preferred, string(journeymaps.CRSBD09LL), string(journeymaps.CRSGCJ02), string(journeymaps.CRSWGS84)}
	for _, key := range keys {
		if key == "" {
			continue
		}
		if point, ok := normalizedCoordinates[key]; ok {
			return savedLocationData{Point: point, Coordinates: normalizedCoordinates, ProviderRefs: location.ProviderRefs, CityCode: strings.TrimSpace(location.CityCode), AdCode: strings.TrimSpace(location.AdCode)}, nil
		}
	}
	return savedLocationData{}, ErrPlanningLocationRequired
}

func normalizeSavedCRS(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.NewReplacer("-", "", "_", "").Replace(value)
	switch value {
	case "bd09", "bd09ll", "baidu":
		return string(journeymaps.CRSBD09LL)
	case "gcj02", "gcj02ll", "amap", "autonavi":
		return string(journeymaps.CRSGCJ02)
	case "wgs84", "wgs84ll", "gps":
		return string(journeymaps.CRSWGS84)
	default:
		return ""
	}
}

func savedPoint(raw json.RawMessage) (journeymaps.GeoPoint, error) {
	location, err := parseSavedLocation(raw)
	if err != nil {
		return journeymaps.GeoPoint{}, err
	}
	return location.Point, nil
}

func savedPointForProvider(location savedLocationData, provider journeymaps.ProviderID) journeymaps.GeoPoint {
	preferred := string(journeymaps.CRSBD09LL)
	if provider == journeymaps.ProviderAMap {
		preferred = string(journeymaps.CRSGCJ02)
	}
	if point, ok := location.Coordinates[preferred]; ok {
		if point.CRS == "" {
			point.CRS = journeymaps.CoordinateSystem(preferred)
		}
		return point
	}
	return location.Point
}

func sameRoutePoint(a, b journeymaps.GeoPoint) bool {
	return a.CRS == b.CRS && math.Abs(a.Lat-b.Lat) <= 1e-7 && math.Abs(a.Lng-b.Lng) <= 1e-7
}

func sameRouteLocation(a, b savedLocationData, provider journeymaps.ProviderID, aPoint, bPoint journeymaps.GeoPoint) bool {
	aID := providerPOIID(a.ProviderRefs, provider)
	bID := providerPOIID(b.ProviderRefs, provider)
	return (aID != "" && aID == bID) || sameRoutePoint(aPoint, bPoint)
}

func routeCoordinateSystem(provider journeymaps.ProviderID) journeymaps.CoordinateSystem {
	if provider == journeymaps.ProviderAMap {
		return journeymaps.CRSGCJ02
	}
	return journeymaps.CRSBD09LL
}

func providerPOIID(refs map[string]string, provider journeymaps.ProviderID) string {
	for _, key := range []string{string(provider) + "_uid", string(provider) + "_poi_id", string(provider) + "_id"} {
		if value := strings.TrimSpace(refs[key]); value != "" {
			return value
		}
	}
	return ""
}

func routePairKey(fromID, toID string) string { return fromID + "\x00" + toID }

func mergeRouteLegs(existing []domain.RouteLeg, planned []plannedRoute, providerID journeymaps.ProviderID, mode journeymaps.TravelMode) ([]domain.RouteLeg, error) {
	validPairs := make(map[string]bool, len(planned))
	for _, item := range planned {
		validPairs[routePairKey(item.fromID, item.toID)] = true
	}
	existingByPair := make(map[string]domain.RouteLeg, len(existing))
	for _, leg := range existing {
		key := routePairKey(leg.FromStopID, leg.ToStopID)
		if !validPairs[key] {
			continue
		}
		filtered := make([]domain.RouteSnapshot, 0, len(leg.Snapshots))
		for _, snapshot := range leg.Snapshots {
			if snapshot.Provider == string(providerID) && (snapshot.Mode == "" || snapshot.Mode == string(mode)) {
				continue
			}
			filtered = append(filtered, snapshot)
		}
		leg.Snapshots = filtered
		if len(filtered) > 0 {
			existingByPair[key] = leg
		}
	}
	merged := make([]domain.RouteLeg, 0, len(planned))
	for _, item := range planned {
		key := routePairKey(item.fromID, item.toID)
		leg, ok := existingByPair[key]
		if !ok {
			id, err := domain.NewID("leg")
			if err != nil {
				return nil, err
			}
			leg.ID = id
		}
		leg.FromStopID = item.fromID
		leg.ToStopID = item.toID
		leg.Mode = string(mode)
		snapshot := routeSnapshot(item.snapshot)
		if snapshot.Provider == "" {
			snapshot.Provider = string(providerID)
		}
		if snapshot.Mode == "" {
			snapshot.Mode = string(mode)
		}
		leg.Snapshots = append(leg.Snapshots, snapshot)
		merged = append(merged, leg)
	}
	return merged, nil
}

func routeSnapshot(snapshot journeymaps.RouteSnapshot) domain.RouteSnapshot {
	geometry := make([][]float64, 0, len(snapshot.Geometry))
	for _, point := range snapshot.Geometry {
		geometry = append(geometry, []float64{point.Lng, point.Lat})
	}
	return domain.RouteSnapshot{Provider: string(snapshot.Provider), CoordinateSystem: string(snapshot.CoordinateSystem), Mode: string(snapshot.Mode), Strategy: snapshot.Strategy, Source: snapshot.Source, Geometry: geometry, DistanceM: snapshot.DistanceM, DurationS: snapshot.DurationS, FetchedAt: snapshot.FetchedAt.UTC().Format(time.RFC3339Nano), ExpiresAt: snapshot.ExpiresAt.UTC().Format(time.RFC3339Nano)}
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
func (s *TripService) MoveStopToDay(ctx context.Context, tripID string, expectedRevision int, sourceDayID, stopID, targetDayID string, targetSequence int, source string) (store.TripRecord, error) {
	sourceDayID = strings.TrimSpace(sourceDayID)
	targetDayID = strings.TrimSpace(targetDayID)
	stopID = strings.TrimSpace(stopID)
	if sourceDayID == "" || targetDayID == "" {
		return store.TripRecord{}, errors.New("source and target day are required")
	}
	if sourceDayID == targetDayID {
		return store.TripRecord{}, errors.New("source and target day must differ")
	}
	if targetSequence < 0 {
		return store.TripRecord{}, errors.New("target sequence cannot be negative")
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

	sourceDayIndex, targetDayIndex := -1, -1
	sourceStopIndex := -1
	var moving domain.Stop
	childFound := false
	for dayIndex := range trip.Days {
		if trip.Days[dayIndex].ID == sourceDayID {
			sourceDayIndex = dayIndex
			for stopIndex := range trip.Days[dayIndex].Stops {
				stop := trip.Days[dayIndex].Stops[stopIndex]
				if stop.ID == stopID {
					sourceStopIndex = stopIndex
					moving = stop
					break
				}
				for _, child := range stop.Children {
					if child.ID == stopID {
						childFound = true
						break
					}
				}
				if childFound {
					break
				}
			}
		}
		if trip.Days[dayIndex].ID == targetDayID {
			targetDayIndex = dayIndex
		}
	}
	if sourceDayIndex < 0 {
		return store.TripRecord{}, fmt.Errorf("source day %s not found", sourceDayID)
	}
	if targetDayIndex < 0 {
		return store.TripRecord{}, fmt.Errorf("target day %s not found", targetDayID)
	}
	if sourceStopIndex < 0 {
		if childFound {
			return store.TripRecord{}, errors.New("changing a child planning point's day is not supported; move its parent instead")
		}
		return store.TripRecord{}, fmt.Errorf("stop %s not found in source day %s", stopID, sourceDayID)
	}

	clearRouteLegsAroundDay(&trip, sourceDayIndex)
	clearRouteLegsAroundDay(&trip, targetDayIndex)
	sourceDay := &trip.Days[sourceDayIndex]
	sourceDay.Stops = append(sourceDay.Stops[:sourceStopIndex], sourceDay.Stops[sourceStopIndex+1:]...)
	for index := range sourceDay.Stops {
		sourceDay.Stops[index].Sequence = index + 1
	}
	targetDay := &trip.Days[targetDayIndex]
	position := len(targetDay.Stops)
	if targetSequence > 0 {
		if targetSequence > len(targetDay.Stops)+1 {
			return store.TripRecord{}, fmt.Errorf("target sequence must be between 1 and %d", len(targetDay.Stops)+1)
		}
		position = targetSequence - 1
	}
	targetDay.Stops = append(targetDay.Stops, domain.Stop{})
	copy(targetDay.Stops[position+1:], targetDay.Stops[position:])
	targetDay.Stops[position] = moving
	for index := range targetDay.Stops {
		targetDay.Stops[index].Sequence = index + 1
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
		provider = s.DefaultMapProvider()
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
	locationData, err := parseSavedLocation(targetLocation)
	if err != nil {
		return store.TripRecord{}, err
	}
	localDate := input.LocalDate
	if localDate == "" {
		localDate = dayDate
	}
	snapshot, err := s.mapService.Weather(ctx, provider, journeymaps.WeatherRequest{Location: locationData.Point, LocalDate: localDate, Timezone: trip.Timezone, CityCode: locationData.CityCode, AdCode: locationData.AdCode})
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
