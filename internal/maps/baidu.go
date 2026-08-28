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

type BaiduConfig struct {
	ServerAK       string
	BaseURL        string
	HTTPClient     *http.Client
	RequestTimeout time.Duration
}

type BaiduProvider struct {
	config   BaiduConfig
	client   *http.Client
	configMu sync.RWMutex
}

func NewBaiduProvider(config BaiduConfig) *BaiduProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.map.baidu.com"
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = 15 * time.Second
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: config.RequestTimeout}
	}
	return &BaiduProvider{config: config, client: client}
}
func (p *BaiduProvider) ID() ProviderID { return ProviderBaidu }
func (p *BaiduProvider) SetServerAK(value string) {
	p.configMu.Lock()
	p.config.ServerAK = strings.TrimSpace(value)
	p.configMu.Unlock()
}
func (p *BaiduProvider) serverAK() string {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.config.ServerAK
}
func (p *BaiduProvider) ServerAKConfigured() bool { return p.serverAK() != "" }

func (p *BaiduProvider) Geocode(ctx context.Context, address, city string) ([]PlaceCandidate, error) {
	if p.serverAK() == "" {
		return nil, unavailable(p.ID())
	}
	params := url.Values{"address": {address}, "output": {"json"}, "ret_coordtype": {"bd09ll"}, "ak": {p.serverAK()}}
	if city != "" {
		params.Set("city", city)
	}
	var response struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Result  struct {
			Location      GeoPoint `json:"location"`
			Precise       int      `json:"precise"`
			Confidence    int      `json:"confidence"`
			Comprehension int      `json:"comprehension"`
		} `json:"result"`
	}
	if err := p.get(ctx, "/geocoding/v3/", params, &response); err != nil {
		return nil, err
	}
	if response.Status != 0 {
		return nil, baiduStatusError(response.Status, response.Message)
	}
	if response.Result.Location.Lat == 0 && response.Result.Location.Lng == 0 {
		return nil, fmt.Errorf("baidu geocode returned no location")
	}
	response.Result.Location.CRS = CRSBD09LL
	return []PlaceCandidate{{Name: address, Address: address, Location: response.Result.Location, Provider: p.ID()}}, nil
}

func (p *BaiduProvider) SearchPOI(ctx context.Context, query, region string, page, pageSize int) (POISearchResult, error) {
	return p.SearchPOIWithTag(ctx, query, region, "", page, pageSize)
}

func (p *BaiduProvider) SearchPOIWithTag(ctx context.Context, query, region, tag string, page, pageSize int) (POISearchResult, error) {
	query = strings.TrimSpace(query)
	region = strings.TrimSpace(region)
	tag = strings.TrimSpace(tag)
	if query == "" {
		return POISearchResult{}, fmt.Errorf("poi search query is required")
	}
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
	ak := p.serverAK()
	if ak == "" {
		return POISearchResult{}, unavailable(p.ID())
	}
	params := url.Values{"query": {query}, "region": {region}, "output": {"json"}, "scope": {"2"}, "page_num": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}, "ret_coordtype": {"bd09ll"}, "ak": {ak}}
	if tag != "" {
		params.Set("tag", tag)
	}
	var response struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Total   int    `json:"total"`
		Results []struct {
			UID      string   `json:"uid"`
			Name     string   `json:"name"`
			Address  string   `json:"address"`
			Location GeoPoint `json:"location"`
		} `json:"results"`
	}
	if err := p.get(ctx, "/place/v2/search", params, &response); err != nil {
		return POISearchResult{}, err
	}
	if response.Status != 0 {
		return POISearchResult{}, baiduStatusError(response.Status, response.Message)
	}
	items := make([]PlaceCandidate, 0, len(response.Results))
	for _, result := range response.Results {
		if result.Name == "" || (result.Location.Lat == 0 && result.Location.Lng == 0) {
			continue
		}
		result.Location.CRS = CRSBD09LL
		items = append(items, PlaceCandidate{ID: result.UID, Name: result.Name, Address: result.Address, Location: result.Location, Provider: p.ID()})
	}
	return POISearchResult{Items: items, Total: response.Total, Page: page, PageSize: pageSize}, nil
}

func (p *BaiduProvider) ReverseGeocode(ctx context.Context, point GeoPoint) (string, error) {
	if p.serverAK() == "" {
		return "", unavailable(p.ID())
	}
	if err := validatePoint(point); err != nil {
		return "", err
	}
	params := url.Values{"location": {fmt.Sprintf("%.8f,%.8f", point.Lat, point.Lng)}, "coordtype": {string(reverseCoordType(point.CRS))}, "ret_coordtype": {"bd09ll"}, "output": {"json"}, "ak": {p.serverAK()}}
	var response struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
		Result  struct {
			FormattedAddress string `json:"formatted_address"`
			AddressComponent struct {
				Province     string `json:"province"`
				City         string `json:"city"`
				District     string `json:"district"`
				Street       string `json:"street"`
				StreetNumber string `json:"street_number"`
			} `json:"addressComponent"`
		} `json:"result"`
	}
	if err := p.get(ctx, "/reverse_geocoding/v3/", params, &response); err != nil {
		return "", err
	}
	if response.Status != 0 {
		return "", baiduStatusError(response.Status, response.Message)
	}
	if response.Result.FormattedAddress != "" {
		return response.Result.FormattedAddress, nil
	}
	c := response.Result.AddressComponent
	return strings.TrimSpace(strings.Join([]string{c.Province, c.City, c.District, c.Street, c.StreetNumber}, "")), nil
}

func (p *BaiduProvider) Route(ctx context.Context, request RouteRequest) (RouteSnapshot, error) {
	if p.serverAK() == "" {
		return RouteSnapshot{}, unavailable(p.ID())
	}
	if err := validatePoint(request.Origin); err != nil {
		return RouteSnapshot{}, fmt.Errorf("invalid route origin: %w", err)
	}
	if err := validatePoint(request.Destination); err != nil {
		return RouteSnapshot{}, fmt.Errorf("invalid route destination: %w", err)
	}
	path := "/direction/v2/" + string(request.Mode)
	if request.Mode == ModeDriving {
		path = "/direction/v2/driving"
	}
	if request.Mode == ModeWalking {
		path = "/direction/v2/walking"
	}
	if request.Mode == ModeCycling {
		path = "/direction/v2/riding"
	}
	if request.Mode == ModeTransit {
		path = "/direction/v2/transit"
	}
	params := url.Values{"origin": {fmt.Sprintf("%.8f,%.8f", request.Origin.Lat, request.Origin.Lng)}, "destination": {fmt.Sprintf("%.8f,%.8f", request.Destination.Lat, request.Destination.Lng)}, "coord_type": {string(coordType(request.Origin.CRS))}, "ret_coordtype": {"bd09ll"}, "output": {"json"}, "ak": {p.serverAK()}}
	if request.DepartureAt != nil {
		params.Set("departure_time", strconv.FormatInt(request.DepartureAt.Unix(), 10))
	}
	var response baiduRouteResponse
	if err := p.get(ctx, path, params, &response); err != nil {
		return RouteSnapshot{}, err
	}
	if response.Status != 0 {
		return RouteSnapshot{}, baiduStatusError(response.Status, response.Message)
	}
	if len(response.Result.Routes) == 0 {
		return RouteSnapshot{}, fmt.Errorf("baidu route returned no routes")
	}
	route := response.Result.Routes[0]
	var geometry []GeoPoint
	for _, step := range route.Steps {
		points, err := parseBaiduPath(step.Path, CRSBD09LL)
		if err != nil {
			return RouteSnapshot{}, err
		}
		geometry = append(geometry, points...)
	}
	now := time.Now().UTC()
	return RouteSnapshot{Provider: p.ID(), CoordinateSystem: CRSBD09LL, Mode: request.Mode, Geometry: geometry, DistanceM: route.Distance, DurationS: route.Duration, FetchedAt: now, ExpiresAt: now.Add(time.Hour)}, nil
}

func (p *BaiduProvider) Weather(ctx context.Context, request WeatherRequest) (WeatherSnapshot, error) {
	if p.serverAK() == "" {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, unavailable(p.ID())
	}
	if err := validatePoint(request.Location); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	params := url.Values{"location": {fmt.Sprintf("%.8f,%.8f", request.Location.Lng, request.Location.Lat)}, "coordtype": {string(coordType(request.Location.CRS))}, "data_type": {"all"}, "ak": {p.serverAK()}}
	var response baiduWeatherResponse
	if err := p.get(ctx, "/weather/v1/", params, &response); err != nil {
		return WeatherSnapshot{}, err
	}
	if response.Status != 0 {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, baiduStatusError(response.Status, response.Message)
	}
	now := time.Now().UTC()
	for _, forecast := range response.Result.Forecasts {
		if forecast.Date == request.LocalDate {
			average := (forecast.High + forecast.Low) / 2
			return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Condition: forecast.TextDay, TemperatureC: &average, FetchedAt: now, ExpiresAt: now.Add(6 * time.Hour), Available: true}, nil
		}
	}
	return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, FetchedAt: now, ExpiresAt: now.Add(time.Hour), Available: false}, nil
}

func (p *BaiduProvider) NavigationURL(target NavTarget, mode TravelMode, platform Platform) (string, error) {
	if target.Name == "" {
		return "", fmt.Errorf("navigation target name is required")
	}
	if err := validatePoint(target.Location); err != nil {
		return "", err
	}
	modeValue := "driving"
	switch mode {
	case ModeWalking:
		modeValue = "walking"
	case ModeCycling:
		modeValue = "riding"
	case ModeTransit:
		modeValue = "transit"
	}
	query := url.Values{}
	query.Set("destination", fmt.Sprintf("latlng:%.8f,%.8f|name:%s", target.Location.Lat, target.Location.Lng, target.Name))
	query.Set("mode", modeValue)
	query.Set("coord_type", string(coordType(target.Location.CRS)))
	query.Set("output", "html")
	query.Set("src", "journeyin")
	prefix := "https://api.map.baidu.com/direction?"
	if platform == PlatformAndroid || platform == PlatformIOS {
		prefix = "baidumap://map/direction?"
	}
	return prefix + query.Encode(), nil
}

func (p *BaiduProvider) get(ctx context.Context, path string, params url.Values, output any) error {
	requestCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()
	endpoint := strings.TrimRight(p.config.BaseURL, "/") + path + "?" + params.Encode()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return fmt.Errorf("baidu request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("baidu HTTP status %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode baidu response: %w", err)
	}
	return nil
}

type baiduRouteResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Result  struct {
		Routes []struct {
			Distance int `json:"distance"`
			Duration int `json:"duration"`
			Steps    []struct {
				Path string `json:"path"`
			} `json:"steps"`
		} `json:"routes"`
	} `json:"result"`
}
type baiduWeatherResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Result  struct {
		Forecasts []struct {
			Date    string  `json:"date"`
			High    float64 `json:"high"`
			Low     float64 `json:"low"`
			TextDay string  `json:"text_day"`
		} `json:"forecasts"`
	} `json:"result"`
}

func reverseCoordType(crs CoordinateSystem) CoordinateSystem {
	switch crs {
	case CRSWGS84:
		return "wgs84ll"
	case CRSGCJ02:
		return "gcj02ll"
	default:
		return "bd09ll"
	}
}

func coordType(crs CoordinateSystem) CoordinateSystem {
	switch crs {
	case CRSWGS84:
		return "wgs84"
	case CRSGCJ02:
		return "gcj02"
	default:
		return "bd09ll"
	}
}
func validatePoint(point GeoPoint) error {
	if point.CRS != CRSWGS84 && point.CRS != CRSGCJ02 && point.CRS != CRSBD09LL {
		return fmt.Errorf("unsupported coordinate system %q", point.CRS)
	}
	if point.Lat < -90 || point.Lat > 90 || point.Lng < -180 || point.Lng > 180 {
		return fmt.Errorf("coordinate is out of range")
	}
	return nil
}

func parseBaiduPath(path string, crs CoordinateSystem) ([]GeoPoint, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	parts := strings.Split(path, ";")
	points := make([]GeoPoint, 0, len(parts))
	for _, part := range parts {
		values := strings.Split(strings.TrimSpace(part), ",")
		if len(values) != 2 {
			return nil, fmt.Errorf("invalid baidu route point %q", part)
		}
		lng, err1 := strconv.ParseFloat(values[0], 64)
		lat, err2 := strconv.ParseFloat(values[1], 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("invalid baidu route coordinates %q", part)
		}
		points = append(points, GeoPoint{Lat: lat, Lng: lng, CRS: crs})
	}
	return points, nil
}
func baiduStatusError(status int, message string) error {
	if message == "" {
		message = "unknown error"
	}
	return fmt.Errorf("baidu API status %d: %s", status, message)
}
