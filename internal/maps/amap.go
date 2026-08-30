package maps

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AMapConfig struct {
	ServerKey      string
	JSKey          string
	SecurityJSCode string
	BaseURL        string
	HTTPClient     *http.Client
	RequestTimeout time.Duration
}

type AMapProvider struct {
	UnavailableProvider
	SourceApplication string
	config            AMapConfig
	client            *http.Client
	configMu          sync.RWMutex
}

func NewAMapProvider(source string) *AMapProvider {
	return NewAMapProviderWithConfig(source, AMapConfig{})
}

func NewAMapProviderWithConfig(source string, config AMapConfig) *AMapProvider {
	if source == "" {
		source = "journeyin"
	}
	if config.BaseURL == "" {
		config.BaseURL = "https://restapi.amap.com"
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 15 * time.Second
	}
	client := config.HTTPClient
	if client == nil {
		if transport, ok := http.DefaultTransport.(*http.Transport); ok {
			transport = transport.Clone()
			transport.MaxConnsPerHost = 1
			transport.MaxIdleConnsPerHost = 1
			client = &http.Client{Transport: transport, Timeout: config.RequestTimeout}
		} else {
			client = &http.Client{Timeout: config.RequestTimeout}
		}
	}
	return &AMapProvider{UnavailableProvider: NewUnavailableProvider(ProviderAMap), SourceApplication: source, config: config, client: client}
}

func (p *AMapProvider) SetServerKey(value string) {
	p.configMu.Lock()
	p.config.ServerKey = strings.TrimSpace(value)
	p.configMu.Unlock()
}
func (p *AMapProvider) SetJSKey(value string) {
	p.configMu.Lock()
	p.config.JSKey = strings.TrimSpace(value)
	p.configMu.Unlock()
}
func (p *AMapProvider) SetSecurityJSCode(value string) {
	p.configMu.Lock()
	p.config.SecurityJSCode = strings.TrimSpace(value)
	p.configMu.Unlock()
}
func (p *AMapProvider) serverKey() string {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.config.ServerKey
}
func (p *AMapProvider) ServerKeyConfigured() bool { return p.serverKey() != "" }
func (p *AMapProvider) browserKey() string {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.config.JSKey
}
func (p *AMapProvider) securityJSCode() string {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.config.SecurityJSCode
}
func (p *AMapProvider) BrowserKeyConfigured() bool     { return p.browserKey() != "" }
func (p *AMapProvider) SecurityJSCodeConfigured() bool { return p.securityJSCode() != "" }

func (p *AMapProvider) Geocode(ctx context.Context, address, city string) ([]PlaceCandidate, error) {
	if p.serverKey() == "" {
		return nil, unavailable(p.ID())
	}
	params := url.Values{"address": {strings.TrimSpace(address)}, "output": {"json"}, "key": {p.serverKey()}}
	if strings.TrimSpace(city) != "" {
		params.Set("city", strings.TrimSpace(city))
	}
	var response struct {
		Status   string `json:"status"`
		Info     string `json:"info"`
		InfoCode string `json:"infocode"`
		Geocodes []struct {
			FormattedAddress string `json:"formatted_address"`
			Location         string `json:"location"`
			CityCode         string `json:"citycode"`
			AdCode           string `json:"adcode"`
		} `json:"geocodes"`
	}
	if err := p.getWithRetry(ctx, "/v3/geocode/geo", params, &response, func() error {
		if response.Status != "1" {
			return amapStatusError(response.InfoCode, response.Info)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	items := make([]PlaceCandidate, 0, len(response.Geocodes))
	for _, item := range response.Geocodes {
		point, err := parseAMapLocation(item.Location)
		if err != nil {
			continue
		}
		name := strings.TrimSpace(address)
		if name == "" {
			name = item.FormattedAddress
		}
		items = append(items, PlaceCandidate{Name: name, Address: item.FormattedAddress, Location: point, Provider: p.ID(), CityCode: item.CityCode, AdCode: item.AdCode})
	}
	return items, nil
}

func (p *AMapProvider) SearchPOI(ctx context.Context, query, region string, page, pageSize int) (POISearchResult, error) {
	return p.SearchPOIWithTag(ctx, query, region, "", page, pageSize)
}
func (p *AMapProvider) SearchPOIWithTag(ctx context.Context, query, region, tag string, page, pageSize int) (POISearchResult, error) {
	if p.serverKey() == "" {
		return POISearchResult{}, unavailable(p.ID())
	}
	query = strings.TrimSpace(query)
	region = strings.TrimSpace(region)
	if query == "" {
		return POISearchResult{}, fmt.Errorf("poi search query is required")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 25 {
		pageSize = 25
	}
	params := url.Values{"keywords": {query}, "output": {"json"}, "page_num": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}, "key": {p.serverKey()}}
	if region != "" {
		params.Set("region", region)
		params.Set("city_limit", "true")
	}
	if normalized := normalizeAMapTypes(tag); normalized != "" {
		params.Set("types", normalized)
	}
	var response struct {
		Status   string `json:"status"`
		Info     string `json:"info"`
		InfoCode string `json:"infocode"`
		Count    string `json:"count"`
		Pois     []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Address  string `json:"address"`
			Location string `json:"location"`
			CityCode string `json:"citycode"`
			AdCode   string `json:"adcode"`
			TypeCode string `json:"typecode"`
		} `json:"pois"`
	}
	if err := p.getWithRetry(ctx, "/v5/place/text", params, &response, func() error {
		if response.Status != "1" {
			return amapStatusError(response.InfoCode, response.Info)
		}
		return nil
	}); err != nil {
		return POISearchResult{}, err
	}
	items := make([]PlaceCandidate, 0, len(response.Pois))
	for _, item := range response.Pois {
		point, err := parseAMapLocation(item.Location)
		if err != nil || item.Name == "" {
			continue
		}
		items = append(items, PlaceCandidate{ID: item.ID, Name: item.Name, Address: item.Address, Location: point, Provider: p.ID(), CityCode: item.CityCode, AdCode: item.AdCode, TypeCode: item.TypeCode})
	}
	total, _ := strconv.Atoi(response.Count)
	return POISearchResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (p *AMapProvider) get(ctx context.Context, path string, params url.Values, target any) error {
	return p.getWithRetry(ctx, path, params, target, nil)
}
func parseAMapLocation(raw string) (GeoPoint, error) {
	values := strings.Split(strings.TrimSpace(raw), ",")
	if len(values) != 2 {
		return GeoPoint{}, fmt.Errorf("invalid amap location %q", raw)
	}
	lng, err1 := strconv.ParseFloat(strings.TrimSpace(values[0]), 64)
	lat, err2 := strconv.ParseFloat(strings.TrimSpace(values[1]), 64)
	if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return GeoPoint{}, fmt.Errorf("invalid amap coordinates %q", raw)
	}
	return GeoPoint{Lat: lat, Lng: lng, CRS: CRSGCJ02}, nil
}
func normalizeAMapTypes(tag string) string {
	switch strings.TrimSpace(tag) {
	case "旅游景点", "景点":
		return "110000"
	case "酒店":
		return "100000"
	case "餐饮":
		return "050000"
	default:
		return strings.TrimSpace(tag)
	}
}

func bd09llToGCJ02(point GeoPoint) GeoPoint {
	const xPi = math.Pi * 3000.0 / 180.0
	x := point.Lng - 0.0065
	y := point.Lat - 0.006
	z := math.Sqrt(x*x+y*y) - 0.00002*math.Sin(y*xPi)
	theta := math.Atan2(y, x) - 0.000003*math.Cos(x*xPi)
	return GeoPoint{Lat: z * math.Sin(theta), Lng: z * math.Cos(theta), CRS: CRSGCJ02}
}

func wgs84ToGCJ02(point GeoPoint) GeoPoint {
	if point.Lng < 72.004 || point.Lng > 137.8347 || point.Lat < 0.8293 || point.Lat > 55.8271 {
		return GeoPoint{Lat: point.Lat, Lng: point.Lng, CRS: CRSGCJ02}
	}
	const a = 6378245.0
	const ee = 0.00669342162296594323
	dLat := transformLatitude(point.Lng-105.0, point.Lat-35.0)
	dLng := transformLongitude(point.Lng-105.0, point.Lat-35.0)
	radLat := point.Lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - ee*magic*magic
	sqrtMagic := math.Sqrt(magic)
	dLat = dLat * 180.0 / ((a * (1 - ee)) / (magic * sqrtMagic) * math.Pi)
	dLng = dLng * 180.0 / (a / sqrtMagic * math.Cos(radLat) * math.Pi)
	return GeoPoint{Lat: point.Lat + dLat, Lng: point.Lng + dLng, CRS: CRSGCJ02}
}

func transformLatitude(x, y float64) float64 {
	ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(y*math.Pi) + 40.0*math.Sin(y/3.0*math.Pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(y/12.0*math.Pi) + 320.0*math.Sin(y*math.Pi/30.0)) * 2.0 / 3.0
	return ret
}

func transformLongitude(x, y float64) float64 {
	ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(x*math.Pi) + 40.0*math.Sin(x/3.0*math.Pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(x/12.0*math.Pi) + 300.0*math.Sin(x/30.0*math.Pi)) * 2.0 / 3.0
	return ret
}

func (p *AMapProvider) NavigationURL(target NavTarget, mode TravelMode, platform Platform) (string, error) {
	if target.Name == "" {
		return "", fmt.Errorf("navigation target name is required")
	}
	location := target.Location
	switch location.CRS {
	case CRSBD09LL:
		location = bd09llToGCJ02(location)
	case CRSWGS84:
		location = wgs84ToGCJ02(location)
	}
	if location.CRS != CRSGCJ02 {
		return "", fmt.Errorf("amap navigation requires gcj02, bd09ll, or wgs84 target coordinates")
	}
	modeValue := "car"
	switch mode {
	case ModeWalking:
		modeValue = "walk"
	case ModeCycling:
		modeValue = "ride"
	case ModeTransit:
		modeValue = "bus"
	}
	query := url.Values{}
	query.Set("to", fmt.Sprintf("%.8f,%.8f,%s", location.Lng, location.Lat, target.Name))
	query.Set("mode", modeValue)
	query.Set("coordinate", "gaode")
	query.Set("callnative", "0")
	query.Set("src", p.SourceApplication)
	if platform == PlatformAndroid || platform == PlatformIOS {
		query.Del("to")
		query.Del("mode")
		query.Del("coordinate")
		query.Del("callnative")
		query.Set("sourceApplication", p.SourceApplication)
		query.Set("dlat", fmt.Sprintf("%.8f", location.Lat))
		query.Set("dlon", fmt.Sprintf("%.8f", location.Lng))
		query.Set("dname", target.Name)
		query.Set("dev", "0")
		tValue := "0"
		switch mode {
		case ModeTransit:
			tValue = "1"
		case ModeWalking:
			tValue = "2"
		case ModeCycling:
			tValue = "3"
		}
		query.Set("t", tValue)
		prefix := "amapuri://route/plan/?"
		if platform == PlatformIOS {
			prefix = "iosamap://path?"
		}
		return prefix + query.Encode(), nil
	}
	return "https://uri.amap.com/navigation?" + query.Encode(), nil
}
