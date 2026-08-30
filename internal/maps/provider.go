package maps

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

type ProviderID string
type CoordinateSystem string
type TravelMode string
type Platform string

const (
	ProviderBaidu   ProviderID       = "baidu"
	ProviderAMap    ProviderID       = "amap"
	CRSWGS84        CoordinateSystem = "wgs84"
	CRSGCJ02        CoordinateSystem = "gcj02"
	CRSBD09LL       CoordinateSystem = "bd09ll"
	ModeDriving     TravelMode       = "driving"
	ModeWalking     TravelMode       = "walking"
	ModeCycling     TravelMode       = "cycling"
	ModeTransit     TravelMode       = "transit"
	PlatformWeb     Platform         = "web"
	PlatformAndroid Platform         = "android"
	PlatformIOS     Platform         = "ios"
)

var (
	ErrProviderUnavailable   = errors.New("map provider unavailable")
	ErrProviderTemporary     = errors.New("map provider temporarily unavailable")
	ErrProviderRateLimited   = errors.New("map provider rate limited")
	ErrProviderUnauthorized  = errors.New("map provider unauthorized")
	ErrProviderQuotaExceeded = errors.New("map provider quota exceeded")
)

type GeoPoint struct {
	Lat float64          `json:"lat"`
	Lng float64          `json:"lng"`
	CRS CoordinateSystem `json:"crs"`
}

type PlaceCandidate struct {
	ID       string     `json:"id,omitempty"`
	Name     string     `json:"name"`
	Address  string     `json:"address,omitempty"`
	Location GeoPoint   `json:"location"`
	Provider ProviderID `json:"provider"`
}

type POISearchResult struct {
	Items    []PlaceCandidate `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

type POISearchProvider interface {
	SearchPOI(context.Context, string, string, int, int) (POISearchResult, error)
}

type TaggedPOISearchProvider interface {
	SearchPOIWithTag(context.Context, string, string, string, int, int) (POISearchResult, error)
}

type RouteRequest struct {
	Origin      GeoPoint   `json:"origin"`
	Destination GeoPoint   `json:"destination"`
	Mode        TravelMode `json:"mode"`
	DepartureAt *time.Time `json:"departure_at,omitempty"`
}

type RouteSnapshot struct {
	Provider         ProviderID       `json:"provider"`
	CoordinateSystem CoordinateSystem `json:"coordinate_system"`
	Mode             TravelMode       `json:"mode"`
	Geometry         []GeoPoint       `json:"geometry,omitempty"`
	DistanceM        int              `json:"distance_m,omitempty"`
	DurationS        int              `json:"duration_s,omitempty"`
	FetchedAt        time.Time        `json:"fetched_at"`
	ExpiresAt        time.Time        `json:"expires_at"`
}

type WeatherRequest struct {
	Location  GeoPoint `json:"location"`
	LocalDate string   `json:"local_date"`
	Timezone  string   `json:"timezone"`
}

type WeatherSnapshot struct {
	Provider     ProviderID `json:"provider"`
	LocalDate    string     `json:"local_date"`
	Condition    string     `json:"condition,omitempty"`
	TemperatureC *float64   `json:"temperature_c,omitempty"`
	FetchedAt    time.Time  `json:"fetched_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	Available    bool       `json:"available"`
}

type NavTarget struct {
	Name     string   `json:"name"`
	Address  string   `json:"address,omitempty"`
	Location GeoPoint `json:"location"`
}

type MapProvider interface {
	ID() ProviderID
	Geocode(context.Context, string, string) ([]PlaceCandidate, error)
	ReverseGeocode(context.Context, GeoPoint) (string, error)
	Route(context.Context, RouteRequest) (RouteSnapshot, error)
	Weather(context.Context, WeatherRequest) (WeatherSnapshot, error)
	NavigationURL(NavTarget, TravelMode, Platform) (string, error)
}

type Registry struct{ providers map[ProviderID]MapProvider }

func NewRegistry(providers ...MapProvider) *Registry {
	registry := &Registry{providers: make(map[ProviderID]MapProvider)}
	for _, provider := range providers {
		registry.providers[provider.ID()] = provider
	}
	return registry
}
func (r *Registry) Get(id ProviderID) (MapProvider, bool) {
	provider, ok := r.providers[id]
	return provider, ok
}
func (r *Registry) IDs() []ProviderID {
	result := make([]ProviderID, 0, len(r.providers))
	for id := range r.providers {
		result = append(result, id)
	}
	return result
}

type UnavailableProvider struct{ provider ProviderID }

func NewUnavailableProvider(id ProviderID) UnavailableProvider {
	return UnavailableProvider{provider: id}
}
func (p UnavailableProvider) ID() ProviderID { return p.provider }
func (p UnavailableProvider) Geocode(context.Context, string, string) ([]PlaceCandidate, error) {
	return nil, unavailable(p.provider)
}
func (p UnavailableProvider) ReverseGeocode(context.Context, GeoPoint) (string, error) {
	return "", unavailable(p.provider)
}
func (p UnavailableProvider) Route(context.Context, RouteRequest) (RouteSnapshot, error) {
	return RouteSnapshot{}, unavailable(p.provider)
}
func (p UnavailableProvider) Weather(context.Context, WeatherRequest) (WeatherSnapshot, error) {
	return WeatherSnapshot{Provider: p.provider, Available: false}, unavailable(p.provider)
}
func (p UnavailableProvider) NavigationURL(target NavTarget, mode TravelMode, platform Platform) (string, error) {
	if target.Name == "" {
		return "", fmt.Errorf("navigation target name is required")
	}
	if target.Location.CRS == "" {
		return "", fmt.Errorf("navigation target CRS is required")
	}
	return "", unavailable(p.provider)
}

func unavailable(id ProviderID) error {
	return fmt.Errorf("%w: %s is not configured", ErrProviderUnavailable, id)
}
func ValidateNavigationURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	switch parsed.Scheme {
	case "http", "https":
		if parsed.Host == "" {
			return fmt.Errorf("navigation URL must have a host")
		}
	case "baidumap", "amapuri", "androidamap", "iosamap":
		if parsed.Host == "" && parsed.Opaque == "" {
			return fmt.Errorf("native navigation URL must have a target")
		}
	default:
		return fmt.Errorf("unsupported navigation URL scheme %q", parsed.Scheme)
	}
	return nil
}
