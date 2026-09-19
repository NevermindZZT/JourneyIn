package weather

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

type ProviderID string

const (
	ProviderAuto      ProviderID = "auto"
	ProviderOpenMeteo ProviderID = "openmeteo"
	ProviderQWeather  ProviderID = "qweather"
	ProviderCaiyun    ProviderID = "caiyun"
	ProviderAMap      ProviderID = "amap"
	ProviderBaidu     ProviderID = "baidu"
)

type CRS string

const (
	CRSWGS84  CRS = "wgs84"
	CRSGCJ02  CRS = "gcj02"
	CRSBD09LL CRS = "bd09ll"
)

type GeoPoint struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
	CRS CRS     `json:"crs"`
}

type WeatherRequest struct {
	Location  GeoPoint `json:"location"`
	LocalDate string   `json:"local_date"`
	Timezone  string   `json:"timezone"`
	CityCode  string   `json:"citycode,omitempty"`
	AdCode    string   `json:"adcode,omitempty"`
}

type WeatherSnapshot struct {
	Provider         ProviderID `json:"provider"`
	LocalDate        string     `json:"local_date"`
	Condition        string     `json:"condition,omitempty"`
	TemperatureC     *float64   `json:"temperature_c,omitempty"`
	TempMinC         *float64   `json:"temp_min_c,omitempty"`
	TempMaxC         *float64   `json:"temp_max_c,omitempty"`
	CurrentCondition string     `json:"current_condition,omitempty"`
	CurrentTempC     *float64   `json:"current_temp_c,omitempty"`
	HumidityPercent  *float64   `json:"humidity_percent,omitempty"`
	WindDirection    string     `json:"wind_direction,omitempty"`
	WindPower        string     `json:"wind_power,omitempty"`
	PressureHPa      *float64   `json:"pressure_hpa,omitempty"`
	FetchedAt        time.Time  `json:"fetched_at"`
	ExpiresAt        time.Time  `json:"expires_at"`
	Available        bool       `json:"available"`
}

type Provider interface {
	ID() ProviderID
	Weather(ctx context.Context, request WeatherRequest) (WeatherSnapshot, error)
}

type Registry struct {
	mu        sync.RWMutex
	providers map[ProviderID]Provider
}

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[ProviderID]Provider),
	}
}

func (r *Registry) Register(p Provider) {
	if p == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[p.ID()] = p
}

func (r *Registry) Get(id ProviderID) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[id]
	return p, ok
}

func (r *Registry) Providers() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	list := make([]Provider, 0, len(r.providers))
	for _, p := range r.providers {
		list = append(list, p)
	}
	return list
}

func ValidatePoint(point GeoPoint) error {
	if point.Lat < -90 || point.Lat > 90 || point.Lng < -180 || point.Lng > 180 {
		return errors.New("location coordinates out of range")
	}
	if point.Lat == 0 && point.Lng == 0 {
		return errors.New("location coordinates cannot be 0,0")
	}
	return nil
}

func Unavailable(provider ProviderID) error {
	return errors.New(string(provider) + "_unavailable")
}

func ParseProviderID(raw string) ProviderID {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	switch ProviderID(trimmed) {
	case ProviderOpenMeteo:
		return ProviderOpenMeteo
	case ProviderQWeather:
		return ProviderQWeather
	case ProviderCaiyun:
		return ProviderCaiyun
	case ProviderAMap:
		return ProviderAMap
	case ProviderBaidu:
		return ProviderBaidu
	case ProviderAuto:
		return ProviderAuto
	default:
		return ProviderAuto
	}
}
