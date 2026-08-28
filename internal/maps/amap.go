package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AMapConfig struct {
	ServerKey      string
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
		client = &http.Client{Timeout: config.RequestTimeout}
	}
	return &AMapProvider{UnavailableProvider: NewUnavailableProvider(ProviderAMap), SourceApplication: source, config: config, client: client}
}

func (p *AMapProvider) SetServerKey(value string) {
	p.configMu.Lock()
	p.config.ServerKey = strings.TrimSpace(value)
	p.configMu.Unlock()
}
func (p *AMapProvider) serverKey() string {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.config.ServerKey
}
func (p *AMapProvider) ServerKeyConfigured() bool { return p.serverKey() != "" }

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
		} `json:"geocodes"`
	}
	if err := p.get(ctx, "/v3/geocode/geo", params, &response); err != nil {
		return nil, err
	}
	if response.Status != "1" {
		return nil, fmt.Errorf("amap API %s: %s", response.InfoCode, response.Info)
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
		items = append(items, PlaceCandidate{Name: name, Address: item.FormattedAddress, Location: point, Provider: p.ID()})
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
		} `json:"pois"`
	}
	if err := p.get(ctx, "/v5/place/text", params, &response); err != nil {
		return POISearchResult{}, err
	}
	if response.Status != "1" {
		return POISearchResult{}, fmt.Errorf("amap API %s: %s", response.InfoCode, response.Info)
	}
	items := make([]PlaceCandidate, 0, len(response.Pois))
	for _, item := range response.Pois {
		point, err := parseAMapLocation(item.Location)
		if err != nil || item.Name == "" {
			continue
		}
		items = append(items, PlaceCandidate{ID: item.ID, Name: item.Name, Address: item.Address, Location: point, Provider: p.ID()})
	}
	total, _ := strconv.Atoi(response.Count)
	return POISearchResult{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (p *AMapProvider) get(ctx context.Context, path string, params url.Values, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.config.BaseURL, "/")+path+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	response, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("amap request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("amap HTTP status %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(target)
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

func (p *AMapProvider) NavigationURL(target NavTarget, mode TravelMode, platform Platform) (string, error) {
	if target.Name == "" {
		return "", fmt.Errorf("navigation target name is required")
	}
	if target.Location.CRS != CRSGCJ02 {
		return "", fmt.Errorf("amap navigation requires gcj02 target coordinates")
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
	query.Set("to", fmt.Sprintf("%.8f,%.8f,%s", target.Location.Lng, target.Location.Lat, target.Name))
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
		query.Set("dlat", fmt.Sprintf("%.8f", target.Location.Lat))
		query.Set("dlon", fmt.Sprintf("%.8f", target.Location.Lng))
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
