package weather

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

type CacheStore interface {
	GetMapCache(ctx context.Context, provider, category, key string) ([]byte, bool, error)
	PutMapCache(ctx context.Context, provider, category, key string, data []byte, expiresAt, now time.Time) error
}

type MapWeatherFetcher func(ctx context.Context, req WeatherRequest) (WeatherSnapshot, error)

type MapWeatherAdapter struct {
	id      ProviderID
	fetcher MapWeatherFetcher
}

func NewMapWeatherAdapter(id ProviderID, fetcher MapWeatherFetcher) *MapWeatherAdapter {
	return &MapWeatherAdapter{id: id, fetcher: fetcher}
}

func (a *MapWeatherAdapter) ID() ProviderID {
	return a.id
}

func (a *MapWeatherAdapter) Weather(ctx context.Context, req WeatherRequest) (WeatherSnapshot, error) {
	if a.fetcher == nil {
		return WeatherSnapshot{Provider: a.id, LocalDate: req.LocalDate, Available: false}, Unavailable(a.id)
	}
	return a.fetcher(ctx, req)
}

type Service struct {
	registry          *Registry
	qweather          *QWeatherProvider
	caiyun            *CaiyunProvider
	openmeteo         *OpenMeteoProvider
	store             CacheStore
	defaultProvider   ProviderID
	defaultProviderMu sync.RWMutex
	cacheMu           sync.Mutex
}

func NewService(registry *Registry, qweather *QWeatherProvider, caiyun *CaiyunProvider, openmeteo *OpenMeteoProvider, store CacheStore, defaultProvider ProviderID) *Service {
	if defaultProvider == "" {
		defaultProvider = ProviderAuto
	}
	return &Service{
		registry:        registry,
		qweather:        qweather,
		caiyun:          caiyun,
		openmeteo:       openmeteo,
		store:           store,
		defaultProvider: defaultProvider,
	}
}

func (s *Service) Registry() *Registry {
	return s.registry
}

func (s *Service) QWeatherProvider() *QWeatherProvider {
	return s.qweather
}

func (s *Service) CaiyunProvider() *CaiyunProvider {
	return s.caiyun
}

func (s *Service) OpenMeteoProvider() *OpenMeteoProvider {
	return s.openmeteo
}

func (s *Service) SetDefaultProvider(id ProviderID) {
	s.defaultProviderMu.Lock()
	defer s.defaultProviderMu.Unlock()
	if id == "" {
		id = ProviderAuto
	}
	s.defaultProvider = id
}

func (s *Service) DefaultProvider() ProviderID {
	s.defaultProviderMu.RLock()
	defer s.defaultProviderMu.RUnlock()
	if s.defaultProvider == "" {
		return ProviderAuto
	}
	return s.defaultProvider
}

func (s *Service) Weather(ctx context.Context, providerID ProviderID, request WeatherRequest) (WeatherSnapshot, error) {
	return s.WeatherWithCache(ctx, providerID, request, true)
}

func (s *Service) WeatherWithCache(ctx context.Context, providerID ProviderID, request WeatherRequest, useCache bool) (WeatherSnapshot, error) {
	resolvedID := providerID
	if resolvedID == "" {
		resolvedID = s.DefaultProvider()
	}

	cacheKey := weatherCacheKey(resolvedID, request)
	fetch := func() ([]byte, error) {
		snapshot, err := s.fetchWeather(ctx, resolvedID, request)
		if err != nil {
			return nil, err
		}
		return json.Marshal(snapshot)
	}

	var data []byte
	var err error
	if useCache && s.store != nil {
		cached, ok, getErr := s.store.GetMapCache(ctx, string(resolvedID), "weather", cacheKey)
		if getErr == nil && ok && len(cached) > 0 {
			data = cached
		}
	}

	if len(data) == 0 {
		data, err = fetch()
		if err != nil {
			return WeatherSnapshot{Provider: resolvedID, LocalDate: request.LocalDate, Available: false}, err
		}
		if s.store != nil {
			s.cacheMu.Lock()
			_ = s.store.PutMapCache(ctx, string(resolvedID), "weather", cacheKey, data, time.Now().UTC().Add(6*time.Hour), time.Now().UTC())
			s.cacheMu.Unlock()
		}
	}

	var result WeatherSnapshot
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) fetchWeather(ctx context.Context, providerID ProviderID, request WeatherRequest) (WeatherSnapshot, error) {
	if providerID == ProviderAuto {
		return s.fetchAutoWeather(ctx, request)
	}
	p, ok := s.registry.Get(providerID)
	if !ok {
		return WeatherSnapshot{Provider: providerID, LocalDate: request.LocalDate, Available: false}, Unavailable(providerID)
	}
	return p.Weather(ctx, request)
}

func (s *Service) fetchAutoWeather(ctx context.Context, request WeatherRequest) (WeatherSnapshot, error) {
	// 1. 若配置了和风天气 Key，优先使用和风天气 (10~30天预报，高质量本土化数据)
	if qw, ok := s.registry.Get(ProviderQWeather); ok {
		if p, isConcrete := qw.(*QWeatherProvider); !isConcrete || p.KeyConfigured() {
			if snap, err := qw.Weather(ctx, request); err == nil && snap.Available {
				return snap, nil
			}
		}
	}

	// 2. 若配置了彩云天气 Token，尝试调用彩云天气
	if cy, ok := s.registry.Get(ProviderCaiyun); ok {
		if p, isConcrete := cy.(*CaiyunProvider); !isConcrete || p.TokenConfigured() {
			if snap, err := cy.Weather(ctx, request); err == nil && snap.Available {
				return snap, nil
			}
		}
	}

	// 3. 检查请求日期与当前日期的天数差距
	daysDiff := calcDayDiff(request.LocalDate)

	// 若在 3 天内且配置了高德地图，尝试使用高德
	if daysDiff >= 0 && daysDiff <= 3 {
		if amap, ok := s.registry.Get(ProviderAMap); ok {
			if snap, err := amap.Weather(ctx, request); err == nil && snap.Available {
				return snap, nil
			}
		}
	}

	// 若在 7 天内且配置了百度地图，尝试使用百度
	if daysDiff >= 0 && daysDiff <= 7 {
		if baidu, ok := s.registry.Get(ProviderBaidu); ok {
			if snap, err := baidu.Weather(ctx, request); err == nil && snap.Available {
				return snap, nil
			}
		}
	}

	// 4. 无需 Key 的免配置全球 16 天预报：Open-Meteo
	if om, ok := s.registry.Get(ProviderOpenMeteo); ok {
		if snap, err := om.Weather(ctx, request); err == nil && snap.Available {
			return snap, nil
		}
	}

	// 5. 遍历其他任意已注册提供商
	for _, p := range s.registry.Providers() {
		if p.ID() == ProviderAuto || p.ID() == ProviderQWeather || p.ID() == ProviderCaiyun || p.ID() == ProviderAMap || p.ID() == ProviderBaidu || p.ID() == ProviderOpenMeteo {
			continue
		}
		if snap, err := p.Weather(ctx, request); err == nil && snap.Available {
			return snap, nil
		}
	}

	now := time.Now().UTC()
	return WeatherSnapshot{Provider: ProviderAuto, LocalDate: request.LocalDate, FetchedAt: now, ExpiresAt: now.Add(time.Hour), Available: false}, nil
}

func calcDayDiff(localDate string) int {
	localDate = strings.TrimSpace(localDate)
	if len(localDate) < 10 {
		return 0
	}
	target, err := time.Parse("2006-01-02", localDate[:10])
	if err != nil {
		return 0
	}
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	diff := target.Sub(today)
	return int(math.Round(diff.Hours() / 24))
}

func weatherCacheKey(provider ProviderID, request WeatherRequest) string {
	// 对经纬度保留两位小数 (~1.1km 网格)，合并相邻近距离查询的缓存命中率
	latGrid := math.Round(request.Location.Lat*100) / 100
	lngGrid := math.Round(request.Location.Lng*100) / 100
	raw := fmt.Sprintf("%s|%.2f,%.2f|%s|%s|%s", provider, latGrid, lngGrid, request.LocalDate, request.CityCode, request.AdCode)
	hash := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(hash[:16])
}
