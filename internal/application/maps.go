package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	journeymaps "journeyin/internal/maps"
	"journeyin/internal/store"
)

type MapService struct {
	registry  *journeymaps.Registry
	store     *store.Store
	semaphore chan struct{}
	// Every provider gets its own conservative single-flight gate. This prevents
	// route, POI, geocode, and weather calls from exceeding the provider's
	// concurrent-request allowance while still allowing different providers to run independently.
	providerMu         sync.Mutex
	providerSemaphores map[journeymaps.ProviderID]chan struct{}
	dailyLimit         int
	mu                 sync.Mutex
	flights            map[string]*mapFlight
}

type mapFlight struct {
	done chan struct{}
	data []byte
	err  error
}

type placeDirectoryLocation struct {
	Point    journeymaps.GeoPoint `json:"point"`
	CityCode string               `json:"citycode,omitempty"`
	AdCode   string               `json:"adcode,omitempty"`
	TypeCode string               `json:"typecode,omitempty"`
}

func NewMapService(database *store.Store, registry *journeymaps.Registry, maxConcurrency, dailyLimit int) *MapService {
	if maxConcurrency < 1 {
		maxConcurrency = 2
	}
	return &MapService{registry: registry, store: database, semaphore: make(chan struct{}, maxConcurrency), providerSemaphores: make(map[journeymaps.ProviderID]chan struct{}), dailyLimit: dailyLimit, flights: make(map[string]*mapFlight)}
}

func (s *MapService) provider(id journeymaps.ProviderID) (journeymaps.MapProvider, error) {
	if s.registry == nil {
		return nil, fmt.Errorf("map registry is not configured")
	}
	provider, ok := s.registry.Get(id)
	if !ok {
		return nil, fmt.Errorf("map provider %s is not registered", id)
	}
	return provider, nil
}

func (s *MapService) SearchPOIByPriority(ctx context.Context, preferred journeymaps.ProviderID, query, region, tag string, page, pageSize int) (journeymaps.ProviderID, journeymaps.POISearchResult, error) {
	query = strings.TrimSpace(query)
	region = strings.TrimSpace(region)
	tag = strings.TrimSpace(tag)
	if s.store != nil {
		_ = s.store.PurgeExpiredPlaceDirectory(ctx, time.Now().UTC())
		records, err := s.store.FindPlaceDirectory(ctx, query, region, tag, 20)
		if err != nil {
			return "", journeymaps.POISearchResult{}, err
		}
		if len(records) > 0 {
			items := make([]journeymaps.PlaceCandidate, 0, len(records))
			for _, record := range records {
				var point journeymaps.GeoPoint
				var metadata placeDirectoryLocation
				if json.Unmarshal(record.LocationJSON, &metadata) == nil && metadata.Point.CRS != "" {
					point = metadata.Point
				} else if json.Unmarshal(record.LocationJSON, &point) != nil {
					continue
				}
				items = append(items, journeymaps.PlaceCandidate{ID: record.ProviderID, Name: record.Name, Address: record.Address, Location: point, Provider: journeymaps.ProviderID(record.Provider), CityCode: metadata.CityCode, AdCode: metadata.AdCode, TypeCode: metadata.TypeCode})
			}
			if len(items) > 0 {
				return journeymaps.ProviderID(records[0].Provider), journeymaps.POISearchResult{Items: items, Total: len(items), Page: page, PageSize: pageSize}, nil
			}
		}
	}
	providers := []journeymaps.ProviderID{journeymaps.ProviderAMap, journeymaps.ProviderBaidu}
	if preferred == journeymaps.ProviderBaidu {
		providers = []journeymaps.ProviderID{journeymaps.ProviderBaidu, journeymaps.ProviderAMap}
	}
	var firstErr error
	var emptyProvider journeymaps.ProviderID
	var emptyResult *journeymaps.POISearchResult
	for _, providerID := range providers {
		result, err := s.SearchPOIWithTag(ctx, providerID, query, region, tag, page, pageSize)
		if err == nil {
			if len(result.Items) > 0 {
				return providerID, result, nil
			}
			if emptyResult == nil {
				copyResult := result
				emptyResult = &copyResult
				emptyProvider = providerID
			}
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if emptyResult != nil {
		return emptyProvider, *emptyResult, nil
	}
	if firstErr == nil {
		firstErr = journeymaps.ErrProviderUnavailable
	}
	return "", journeymaps.POISearchResult{}, firstErr
}

func (s *MapService) SearchPOI(ctx context.Context, providerID journeymaps.ProviderID, query, region string, page, pageSize int) (journeymaps.POISearchResult, error) {
	return s.SearchPOIWithTag(ctx, providerID, query, region, "", page, pageSize)
}

func (s *MapService) SearchPOIWithTag(ctx context.Context, providerID journeymaps.ProviderID, query, region, tag string, page, pageSize int) (journeymaps.POISearchResult, error) {
	query = strings.TrimSpace(query)
	region = strings.TrimSpace(region)
	tag = strings.TrimSpace(tag)
	if region == "" {
		region = "全国"
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 20 {
		pageSize = 20
	}
	provider, err := s.provider(providerID)
	if err != nil {
		return journeymaps.POISearchResult{}, err
	}
	searcher, ok := provider.(journeymaps.POISearchProvider)
	if !ok {
		return journeymaps.POISearchResult{}, fmt.Errorf("map provider %s does not support POI search", providerID)
	}
	cacheKey := mapCacheKey(struct {
		Query, Region, Tag string
		Page, PageSize     int
	}{query, region, tag, page, pageSize})
	data, err := s.cached(ctx, providerID, "poi_search", cacheKey, 15*time.Minute, func() ([]byte, error) {
		value, err := s.call(ctx, providerID, func() (any, error) {
			if tagged, supported := provider.(journeymaps.TaggedPOISearchProvider); supported && tag != "" {
				return tagged.SearchPOIWithTag(ctx, query, region, tag, page, pageSize)
			}
			return searcher.SearchPOI(ctx, query, region, page, pageSize)
		})
		if err != nil {
			return nil, err
		}
		result, ok := value.(journeymaps.POISearchResult)
		if ok && len(result.Items) == 0 && tag == "" {
			fallback, fallbackErr := s.Geocode(ctx, providerID, query, region)
			if fallbackErr == nil && len(fallback) > 0 {
				result.Items = fallback
				result.Total = len(fallback)
			}
		}
		return json.Marshal(result)
	})
	if err != nil {
		return journeymaps.POISearchResult{}, err
	}
	var result journeymaps.POISearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	if s.store != nil {
		s.persistPlaceDirectory(ctx, result, region, tag)
	}
	return result, nil
}

func (s *MapService) persistPlaceDirectory(ctx context.Context, result journeymaps.POISearchResult, region, category string) {
	now := time.Now().UTC()
	for _, item := range result.Items {
		if item.Name == "" || item.Location.CRS == "" {
			continue
		}
		location, err := json.Marshal(placeDirectoryLocation{Point: item.Location, CityCode: item.CityCode, AdCode: item.AdCode, TypeCode: item.TypeCode})
		if err != nil {
			continue
		}
		providerID := item.ID
		if providerID == "" {
			providerID = mapCacheKey(struct {
				Name, Address string
				Location      journeymaps.GeoPoint
			}{item.Name, item.Address, item.Location})
		}
		_ = s.store.UpsertPlaceDirectory(ctx, store.PlaceDirectoryRecord{Provider: string(item.Provider), ProviderID: providerID, Name: item.Name, Address: item.Address, Region: region, Category: category, LocationJSON: location, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour)})
	}
}

func (s *MapService) Geocode(ctx context.Context, providerID journeymaps.ProviderID, address, city string) ([]journeymaps.PlaceCandidate, error) {
	provider, err := s.provider(providerID)
	if err != nil {
		return nil, err
	}
	cacheKey := mapCacheKey(struct{ Address, City string }{strings.TrimSpace(address), strings.TrimSpace(city)})
	data, err := s.cached(ctx, providerID, "geocode", cacheKey, 24*time.Hour, func() ([]byte, error) {
		result, err := s.call(ctx, providerID, func() (any, error) { return provider.Geocode(ctx, address, city) })
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	if err != nil {
		return nil, err
	}
	var result []journeymaps.PlaceCandidate
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *MapService) ReverseGeocode(ctx context.Context, providerID journeymaps.ProviderID, point journeymaps.GeoPoint) (string, error) {
	provider, err := s.provider(providerID)
	if err != nil {
		return "", err
	}
	cacheKey := mapCacheKey(point)
	data, err := s.cached(ctx, providerID, "reverse_geocode", cacheKey, 24*time.Hour, func() ([]byte, error) {
		result, err := s.call(ctx, providerID, func() (any, error) { return provider.ReverseGeocode(ctx, point) })
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	if err != nil {
		return "", err
	}
	var result string
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	return result, nil
}

func (s *MapService) Route(ctx context.Context, providerID journeymaps.ProviderID, request journeymaps.RouteRequest) (journeymaps.RouteSnapshot, error) {
	provider, err := s.provider(providerID)
	if err != nil {
		return journeymaps.RouteSnapshot{}, err
	}
	cacheRequest := request
	if cacheRequest.DepartureAt != nil {
		value := cacheRequest.DepartureAt.In(cacheRequest.DepartureAt.Location())
		bucket := time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), (value.Minute()/15)*15, 0, 0, value.Location())
		cacheRequest.DepartureAt = &bucket
	}
	cacheKey := mapCacheKey(cacheRequest)
	data, err := s.cached(ctx, providerID, "route", cacheKey, 30*time.Minute, func() ([]byte, error) {
		// Use the same quarter-hour-bucketed departure that forms the cache key.
		// Otherwise two requests in one bucket could share a snapshot fetched for a different time.
		result, err := s.call(ctx, providerID, func() (any, error) { return provider.Route(ctx, cacheRequest) })
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	if err != nil {
		return journeymaps.RouteSnapshot{}, err
	}
	var result journeymaps.RouteSnapshot
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *MapService) Weather(ctx context.Context, providerID journeymaps.ProviderID, request journeymaps.WeatherRequest) (journeymaps.WeatherSnapshot, error) {
	provider, err := s.provider(providerID)
	if err != nil {
		return journeymaps.WeatherSnapshot{}, err
	}
	cacheKey := mapCacheKey(request)
	data, err := s.cached(ctx, providerID, "weather", cacheKey, 6*time.Hour, func() ([]byte, error) {
		result, err := s.call(ctx, providerID, func() (any, error) { return provider.Weather(ctx, request) })
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	})
	if err != nil {
		return journeymaps.WeatherSnapshot{}, err
	}
	var result journeymaps.WeatherSnapshot
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *MapService) acquireProvider(ctx context.Context, providerID journeymaps.ProviderID) (func(), error) {
	s.providerMu.Lock()
	gate := s.providerSemaphores[providerID]
	if gate == nil {
		gate = make(chan struct{}, 1)
		s.providerSemaphores[providerID] = gate
	}
	s.providerMu.Unlock()
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *MapService) call(ctx context.Context, providerID journeymaps.ProviderID, fn func() (any, error)) (any, error) {
	releaseProvider, err := s.acquireProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	defer releaseProvider()
	select {
	case s.semaphore <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-s.semaphore }()
	if s.store != nil && s.dailyLimit > 0 {
		if err := s.store.ReserveMapRequest(ctx, string(providerID), time.Now().UTC().Format("2006-01-02"), s.dailyLimit); err != nil {
			return nil, err
		}
	}
	return fn()
}

func (s *MapService) cached(ctx context.Context, providerID journeymaps.ProviderID, kind, cacheKey string, ttl time.Duration, fetch func() ([]byte, error)) ([]byte, error) {
	provider := string(providerID)
	if s.store != nil {
		if entry, ok, err := s.store.GetMapCache(ctx, provider, kind, cacheKey); err != nil {
			return nil, err
		} else if ok {
			return entry.ResponseJSON, nil
		}
	}
	flightKey := provider + "|" + kind + "|" + cacheKey
	s.mu.Lock()
	if flight, ok := s.flights[flightKey]; ok {
		done := flight.done
		s.mu.Unlock()
		select {
		case <-done:
			return flight.data, flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	flight := &mapFlight{done: make(chan struct{})}
	s.flights[flightKey] = flight
	s.mu.Unlock()
	data, err := fetch()
	if err == nil && s.store != nil {
		err = s.store.PutMapCache(ctx, provider, kind, cacheKey, data, time.Now().UTC().Add(ttl), time.Now().UTC())
	}
	s.mu.Lock()
	flight.data = data
	flight.err = err
	close(flight.done)
	delete(s.flights, flightKey)
	s.mu.Unlock()
	return data, err
}

func mapCacheKey(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}
