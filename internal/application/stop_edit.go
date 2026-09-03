package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"journeyin/internal/store"
)

const (
	maxPlanningPointTitleRunes   = 200
	maxPlanningPointAddressRunes = 500
)

type UpdatePlanningPointInput struct {
	Title       *string
	Address     *string
	Location    json.RawMessage
	LocationSet bool
}

type UpdatePlanningPointChanges struct {
	Changed          bool `json:"changed"`
	TitleChanged     bool `json:"title_changed"`
	AddressChanged   bool `json:"address_changed"`
	LocationChanged  bool `json:"location_changed"`
	RouteInvalidated bool `json:"route_invalidated"`
	WeatherCleared   bool `json:"weather_cleared"`
}

var ErrPlanningPointUpdateEmpty = errors.New("planning point update must include title, address, or location")

// UpdatePlanningPoint updates a main Stop or a nested SubStop by stable ID.
// It edits the stored raw JSON tree so untouched fields and provider-specific
// data survive the revision. A main-stop location change invalidates the
// current day's route and the next day's cross-day route, and clears weather
// tied to the old location.
func (s *TripService) UpdatePlanningPoint(ctx context.Context, tripID string, expectedRevision int, dayID, stopID string, input UpdatePlanningPointInput, source string) (store.TripRecord, UpdatePlanningPointChanges, error) {
	changes := UpdatePlanningPointChanges{}
	if err := validatePlanningPointUpdate(input); err != nil {
		return store.TripRecord{}, changes, err
	}
	record, err := s.store.GetTrip(ctx, tripID)
	if err != nil {
		return store.TripRecord{}, changes, err
	}
	if record.Revision != expectedRevision {
		return store.TripRecord{}, changes, store.ErrRevisionConflict
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(record.Document, &root); err != nil || root == nil {
		if err == nil {
			err = errors.New("trip document must be a JSON object")
		}
		return store.TripRecord{}, changes, fmt.Errorf("decode trip document: %w", err)
	}
	days, err := rawArray(root["days"], "trip days")
	if err != nil {
		return store.TripRecord{}, changes, err
	}

	var target map[string]json.RawMessage
	var parent map[string]json.RawMessage
	var dayObject map[string]json.RawMessage
	var dayIndex, stopIndex, childIndex int
	var targetIsChild bool
	found := false
	for currentDayIndex, dayRaw := range days {
		currentDay, decodeErr := rawObject(dayRaw, "day")
		if decodeErr != nil {
			return store.TripRecord{}, changes, decodeErr
		}
		if rawString(currentDay["id"]) != dayID {
			continue
		}
		stops, decodeErr := rawArray(currentDay["stops"], "day stops")
		if decodeErr != nil {
			return store.TripRecord{}, changes, decodeErr
		}
		for currentStopIndex, stopRaw := range stops {
			stopObject, decodeErr := rawObject(stopRaw, "stop")
			if decodeErr != nil {
				return store.TripRecord{}, changes, decodeErr
			}
			if rawString(stopObject["id"]) == stopID {
				target = stopObject
				dayObject = currentDay
				dayIndex, stopIndex = currentDayIndex, currentStopIndex
				found = true
				break
			}
			children, hasChildren := stopObject["children"]
			if !hasChildren {
				continue
			}
			childObjects, decodeErr := rawArray(children, "stop children")
			if decodeErr != nil {
				return store.TripRecord{}, changes, decodeErr
			}
			for currentChildIndex, childRaw := range childObjects {
				childObject, childErr := rawObject(childRaw, "sub-stop")
				if childErr != nil {
					return store.TripRecord{}, changes, childErr
				}
				if rawString(childObject["id"]) != stopID {
					continue
				}
				target = childObject
				parent = stopObject
				dayObject = currentDay
				dayIndex, stopIndex, childIndex = currentDayIndex, currentStopIndex, currentChildIndex
				targetIsChild = true
				found = true
				break
			}
			if found {
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		return store.TripRecord{}, changes, fmt.Errorf("stop %s not found in day %s", stopID, dayID)
	}

	if input.Title != nil {
		value := strings.TrimSpace(*input.Title)
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return store.TripRecord{}, changes, marshalErr
		}
		if !rawEqual(target["title"], encoded) {
			target["title"] = encoded
			changes.TitleChanged = true
		}
	}
	if input.Address != nil {
		value := strings.TrimSpace(*input.Address)
		if value == "" {
			if _, ok := target["address"]; ok {
				delete(target, "address")
				changes.AddressChanged = true
			}
		} else {
			encoded, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				return store.TripRecord{}, changes, marshalErr
			}
			if !rawEqual(target["address"], encoded) {
				target["address"] = encoded
				changes.AddressChanged = true
			}
		}
	}
	if input.LocationSet && !rawEqual(target["location"], input.Location) {
		target["location"] = append(json.RawMessage(nil), input.Location...)
		changes.LocationChanged = true
		if _, ok := target["weather"]; ok {
			delete(target, "weather")
			changes.WeatherCleared = true
		}
	}
	changes.Changed = changes.TitleChanged || changes.AddressChanged || changes.LocationChanged
	if !changes.Changed {
		return record, changes, nil
	}

	stops, err := rawArray(dayObject["stops"], "day stops")
	if err != nil {
		return store.TripRecord{}, changes, err
	}
	if targetIsChild {
		children, childErr := rawArray(parent["children"], "stop children")
		if childErr != nil {
			return store.TripRecord{}, changes, childErr
		}
		children[childIndex], err = marshalRawObject(target)
		if err != nil {
			return store.TripRecord{}, changes, err
		}
		parent["children"], err = json.Marshal(children)
		if err != nil {
			return store.TripRecord{}, changes, err
		}
		stops[stopIndex], err = marshalRawObject(parent)
	} else {
		stops[stopIndex], err = marshalRawObject(target)
	}
	if err != nil {
		return store.TripRecord{}, changes, err
	}
	dayObject["stops"], err = json.Marshal(stops)
	if err != nil {
		return store.TripRecord{}, changes, err
	}
	days[dayIndex], err = marshalRawObject(dayObject)
	if err != nil {
		return store.TripRecord{}, changes, err
	}

	if changes.LocationChanged && !targetIsChild {
		changes.RouteInvalidated = true
		if err := clearRawRouteLegs(days, dayIndex); err != nil {
			return store.TripRecord{}, changes, err
		}
	}
	root["days"], err = json.Marshal(days)
	if err != nil {
		return store.TripRecord{}, changes, err
	}
	normalized, err := json.Marshal(root)
	if err != nil {
		return store.TripRecord{}, changes, err
	}
	record, err = s.Replace(ctx, tripID, expectedRevision, normalized, source)
	if err != nil {
		return store.TripRecord{}, changes, err
	}
	return record, changes, nil
}

func validatePlanningPointUpdate(input UpdatePlanningPointInput) error {
	if input.Title == nil && input.Address == nil && !input.LocationSet {
		return ErrPlanningPointUpdateEmpty
	}
	if input.Title != nil {
		value := strings.TrimSpace(*input.Title)
		if value == "" {
			return errors.New("planning point title is required")
		}
		if utf8.RuneCountInString(value) > maxPlanningPointTitleRunes {
			return fmt.Errorf("planning point title must be at most %d characters", maxPlanningPointTitleRunes)
		}
	}
	if input.Address != nil && utf8.RuneCountInString(strings.TrimSpace(*input.Address)) > maxPlanningPointAddressRunes {
		return fmt.Errorf("planning point address must be at most %d characters", maxPlanningPointAddressRunes)
	}
	if input.LocationSet {
		return validatePlanningPointLocation(input.Location)
	}
	return nil
}

func validatePlanningPointLocation(raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ErrPlanningLocationRequired
	}
	object, err := rawObject(trimmed, "planning point location")
	if err != nil {
		return err
	}
	var preferred string
	if err := json.Unmarshal(object["preferred"], &preferred); err != nil || normalizeSavedCRS(preferred) == "" {
		return errors.New("planning point location.preferred must identify wgs84, gcj02, or bd09ll")
	}
	coordinates, err := rawObject(object["coordinates"], "planning point coordinates")
	if err != nil || len(coordinates) == 0 {
		if err != nil {
			return err
		}
		return ErrPlanningLocationRequired
	}
	preferredCRS := normalizeSavedCRS(preferred)
	foundPreferred := false
	for rawCRS, rawPoint := range coordinates {
		crs := normalizeSavedCRS(rawCRS)
		if crs == "" {
			return fmt.Errorf("planning point location.coordinates.%s uses an unsupported CRS", rawCRS)
		}
		if crs == preferredCRS {
			foundPreferred = true
		}
		point, pointErr := rawObject(rawPoint, "planning point coordinate")
		if pointErr != nil {
			return pointErr
		}
		var lat, lng *float64
		if value, ok := point["lat"]; ok {
			if err := json.Unmarshal(value, &lat); err != nil {
				return errors.New("planning point coordinate.lat must be a number")
			}
		}
		if value, ok := point["lng"]; ok {
			if err := json.Unmarshal(value, &lng); err != nil {
				return errors.New("planning point coordinate.lng must be a number")
			}
		}
		if lat == nil || lng == nil || !validPlanningCoordinate(*lat, *lng) {
			return fmt.Errorf("planning point coordinate.%s must contain finite coordinates in range", rawCRS)
		}
		if *lat == 0 && *lng == 0 {
			return fmt.Errorf("planning point coordinate.%s must not be 0,0", rawCRS)
		}
		if declared, ok := point["crs"]; ok {
			var declaredCRS string
			if err := json.Unmarshal(declared, &declaredCRS); err != nil {
				return errors.New("planning point coordinate.crs must be a string")
			}
			if normalized := normalizeSavedCRS(declaredCRS); normalized != "" && normalized != crs {
				return fmt.Errorf("planning point coordinate.%s CRS does not match its coordinate key", rawCRS)
			}
		}
	}
	if !foundPreferred {
		return fmt.Errorf("planning point location.preferred %q has no matching coordinate", preferred)
	}
	_, err = parseSavedLocation(trimmed)
	return err
}

func validPlanningCoordinate(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsInf(lat, 0) && !math.IsNaN(lng) && !math.IsInf(lng, 0) && lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180
}

func rawObject(raw json.RawMessage, name string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("%s must be a JSON object: %w", name, err)
	}
	if object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", name)
	}
	return object, nil
}

func rawArray(raw json.RawMessage, name string) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("%s must be a JSON array: %w", name, err)
	}
	if items == nil {
		return nil, fmt.Errorf("%s must be a JSON array", name)
	}
	return items, nil
}

func marshalRawObject(object map[string]json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(object)
}

func rawString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

func rawEqual(left, right json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(left), bytes.TrimSpace(right))
}

func clearRawRouteLegs(days []json.RawMessage, dayIndex int) error {
	for _, index := range []int{dayIndex, dayIndex + 1} {
		if index < 0 || index >= len(days) {
			continue
		}
		day, err := rawObject(days[index], "day")
		if err != nil {
			return err
		}
		delete(day, "legs")
		days[index], err = marshalRawObject(day)
		if err != nil {
			return err
		}
	}
	return nil
}
