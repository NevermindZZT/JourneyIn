package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type inMemoryCacheStore struct {
	cache map[string][]byte
}

func (s *inMemoryCacheStore) GetMapCache(ctx context.Context, provider, category, key string) ([]byte, bool, error) {
	k := provider + ":" + category + ":" + key
	v, ok := s.cache[k]
	return v, ok, nil
}

func (s *inMemoryCacheStore) PutMapCache(ctx context.Context, provider, category, key string, data []byte, expiresAt, now time.Time) error {
	k := provider + ":" + category + ":" + key
	s.cache[k] = data
	return nil
}

type mockWeatherProvider struct {
	id       ProviderID
	response WeatherSnapshot
	err      error
}

func (m *mockWeatherProvider) ID() ProviderID {
	return m.id
}

func (m *mockWeatherProvider) Weather(ctx context.Context, req WeatherRequest) (WeatherSnapshot, error) {
	if m.err != nil {
		return WeatherSnapshot{Provider: m.id, LocalDate: req.LocalDate, Available: false}, m.err
	}
	res := m.response
	res.Provider = m.id
	res.LocalDate = req.LocalDate
	return res, nil
}

func TestWeatherService_AutoModeFallback(t *testing.T) {
	reg := NewRegistry()
	store := &inMemoryCacheStore{cache: make(map[string][]byte)}
	caiyun := NewCaiyunProvider("") // 未配置 token
	reg.Register(caiyun)

	// 注册高德适配器 (3天内生效)
	amapSnap := WeatherSnapshot{Condition: "多云", Available: true}
	reg.Register(&mockWeatherProvider{id: ProviderAMap, response: amapSnap})

	svc := NewService(reg, nil, caiyun, nil, store, ProviderAuto)

	// 查询今天 (属于 3 天内)
	today := time.Now().UTC().Format("2006-01-02")
	snap, err := svc.Weather(context.Background(), ProviderAuto, WeatherRequest{
		Location:  GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: today,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !snap.Available || snap.Provider != ProviderAMap {
		t.Errorf("expected amap fallback for today, got provider=%s, available=%v", snap.Provider, snap.Available)
	}

	// 现在为彩云配置 Token，测试彩云优先
	caiyunMock := &mockWeatherProvider{
		id:       ProviderCaiyun,
		response: WeatherSnapshot{Condition: "彩云晴天", Available: true},
	}
	reg.Register(caiyunMock)
	caiyun.SetToken("configured_token")

	svcWithCaiyun := NewService(reg, nil, caiyun, nil, store, ProviderAuto)
	// 使用不同的日期避免命中上一条缓存
	tomorrow := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	// 为了测 mock，让 caiyunMock 直接走 registry
	snapCaiyun, err := svcWithCaiyun.Weather(context.Background(), ProviderCaiyun, WeatherRequest{
		Location:  GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: tomorrow,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if snapCaiyun.Condition != "彩云晴天" {
		t.Errorf("expected condition 彩云晴天, got %s", snapCaiyun.Condition)
	}
}

func TestWeatherService_CacheHit(t *testing.T) {
	reg := NewRegistry()
	store := &inMemoryCacheStore{cache: make(map[string][]byte)}
	callCount := 0
	fetcher := func(ctx context.Context, req WeatherRequest) (WeatherSnapshot, error) {
		callCount++
		return WeatherSnapshot{Provider: ProviderAMap, LocalDate: req.LocalDate, Condition: "雷阵雨", Available: true}, nil
	}
	reg.Register(NewMapWeatherAdapter(ProviderAMap, fetcher))
	svc := NewService(reg, nil, nil, nil, store, ProviderAMap)

	req := WeatherRequest{Location: GeoPoint{Lat: 31.23, Lng: 121.47, CRS: CRSGCJ02}, LocalDate: "2026-05-01"}

	// 第 1 次调用：穿透 fetch
	snap1, err := svc.Weather(context.Background(), ProviderAMap, req)
	if err != nil || snap1.Condition != "雷阵雨" {
		t.Fatalf("expected snap1 success, got err=%v", err)
	}
	if callCount != 1 {
		t.Errorf("expected callCount 1, got %d", callCount)
	}

	// 第 2 次调用：应命中缓存
	snap2, err := svc.Weather(context.Background(), ProviderAMap, req)
	if err != nil || snap2.Condition != "雷阵雨" {
		t.Fatalf("expected snap2 success, got err=%v", err)
	}
	if callCount != 1 {
		t.Errorf("expected callCount to remain 1 due to cache, got %d", callCount)
	}

	// 第 3 次调用：显式刷新 useCache=false，应穿透缓存
	snap3, err := svc.WeatherWithCache(context.Background(), ProviderAMap, req, false)
	if err != nil || snap3.Condition != "雷阵雨" {
		t.Fatalf("expected snap3 success, got err=%v", err)
	}
	if callCount != 2 {
		t.Errorf("expected callCount to increment to 2, got %d", callCount)
	}
}

func TestWeatherService_QWeatherAndOpenMeteoFallback(t *testing.T) {
	reg := NewRegistry()
	store := &inMemoryCacheStore{cache: make(map[string][]byte)}
	qw := NewQWeatherProvider("", "") // 未配置 key
	reg.Register(qw)
	om := NewOpenMeteoProvider()
	reg.Register(om)

	omMock := &mockWeatherProvider{
		id:       ProviderOpenMeteo,
		response: WeatherSnapshot{Condition: "OpenMeteo大晴天", Available: true},
	}
	reg.Register(omMock)

	svc := NewService(reg, qw, nil, nil, store, ProviderAuto)

	// 查询第 12 天 (超出 3 天高德和 7 天百度)
	day12 := time.Now().UTC().AddDate(0, 0, 12).Format("2006-01-02")
	snap, err := svc.Weather(context.Background(), ProviderAuto, WeatherRequest{
		Location:  GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: day12,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !snap.Available || snap.Condition != "OpenMeteo大晴天" {
		t.Errorf("expected open-meteo fallback for day 12, got provider=%s, cond=%s", snap.Provider, snap.Condition)
	}

	// 现在为和风天气配置 Key，测试和风天气优先于 Open-Meteo
	qwMock := &mockWeatherProvider{
		id:       ProviderQWeather,
		response: WeatherSnapshot{Condition: "和风晴空万里", Available: true},
	}
	reg.Register(qwMock)
	qw.SetKey("configured_key")

	svcWithQW := NewService(reg, qw, nil, nil, store, ProviderAuto)
	day13 := time.Now().UTC().AddDate(0, 0, 13).Format("2006-01-02")
	snapQW, err := svcWithQW.Weather(context.Background(), ProviderAuto, WeatherRequest{
		Location:  GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSGCJ02},
		LocalDate: day13,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !snapQW.Available || snapQW.Condition != "和风晴空万里" {
		t.Errorf("expected qweather priority, got provider=%s, cond=%s", snapQW.Provider, snapQW.Condition)
	}
}

func TestWeatherService_EnrichWithSevenTimerSupplementary(t *testing.T) {
	reg := NewRegistry()
	store := &inMemoryCacheStore{cache: make(map[string][]byte)}

	// 主数据源：只有基础天气 (无 Seeing 和 Transparency)
	temp := 22.0
	mainMock := &mockWeatherProvider{
		id: ProviderAMap,
		response: WeatherSnapshot{
			Condition:    "晴",
			TemperatureC: &temp,
			Available:    true,
		},
	}
	reg.Register(mainMock)

	// 启动一个 mock 7Timer server
	sevenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := sevenTimerResponse{
			Product: "astro",
			Init:    time.Now().UTC().Format("2006010215"),
			Dataseries: []sevenTimerPoint{
				{
					Timepoint:    0,
					Cloudcover:   2,
					Seeing:       4, // 3.5 四舍五入对应
					Transparency: 2,
					Temp2m:       20,
					PrecType:     "none",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer sevenServer.Close()

	sevenProvider := NewSevenTimerProvider()
	sevenProvider.SetBaseURL(sevenServer.URL)

	svc := NewService(reg, nil, nil, nil, store, ProviderAMap)
	svc.SetSevenTimerProvider(sevenProvider)

	// 查询今天 (<= 3 天)
	today := time.Now().UTC().Format("2006-01-02")
	snap, err := svc.Weather(context.Background(), ProviderAMap, WeatherRequest{
		Location:  GeoPoint{Lat: 30.25, Lng: 120.15, CRS: CRSWGS84},
		LocalDate: today,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !snap.Available {
		t.Fatal("expected snapshot to be available")
	}
	// 验证主数据源保持不变
	if snap.Provider != ProviderAMap || snap.Condition != "晴" {
		t.Errorf("expected primary provider amap with 晴, got %s, %s", snap.Provider, snap.Condition)
	}
	// 验证补充数据成功注入！
	if snap.Seeing == nil || *snap.Seeing != 4.0 {
		t.Errorf("expected supplementary Seeing 4.0, got %v", snap.Seeing)
	}
	if snap.Transparency == nil || *snap.Transparency != 2.0 {
		t.Errorf("expected supplementary Transparency 2.0, got %v", snap.Transparency)
	}
}