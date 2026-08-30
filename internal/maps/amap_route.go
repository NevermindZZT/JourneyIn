package maps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	amapMaxRequestAttempts = 3
	amapRetryBaseDelay     = 250 * time.Millisecond
	amapRetryMaxDelay      = 5 * time.Second
)

// getWithRetry performs one JSON request while keeping provider credentials out
// of returned network errors. The validator lets callers classify API-level
// errors (for example AMap QPS and quota responses) without duplicating retry
// behavior in every endpoint implementation.
func (p *AMapProvider) getWithRetry(ctx context.Context, path string, params url.Values, target any, validate func() error) error {
	for attempt := 0; attempt < amapMaxRequestAttempts; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, strings.TrimRight(p.config.BaseURL, "/")+path+"?"+params.Encode(), nil)
		if err != nil {
			cancel()
			return fmt.Errorf("amap request construction failed: %v", safeAMapError(err))
		}
		req.Header.Set("Accept", "application/json")
		response, err := p.client.Do(req)
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if amapNetworkRetryable(err) && attempt+1 < amapMaxRequestAttempts {
				if err := waitAMapRetry(ctx, attempt); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("%w: amap request failed: %v", ErrProviderTemporary, safeAMapError(err))
		}

		if response.StatusCode < 200 || response.StatusCode >= 300 {
			status := response.StatusCode
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			cancel()
			err = amapHTTPStatusError(status)
			if amapRetryable(err) && attempt+1 < amapMaxRequestAttempts {
				if err := waitAMapRetry(ctx, attempt); err != nil {
					return err
				}
				continue
			}
			return err
		}

		err = json.NewDecoder(response.Body).Decode(target)
		_ = response.Body.Close()
		cancel()
		if err != nil {
			if attempt+1 < amapMaxRequestAttempts {
				if waitErr := waitAMapRetry(ctx, attempt); waitErr != nil {
					return waitErr
				}
				continue
			}
			return fmt.Errorf("%w: amap response decode failed: %v", ErrProviderTemporary, err)
		}
		if validate == nil {
			return nil
		}
		if err := validate(); err != nil {
			if amapRetryable(err) && attempt+1 < amapMaxRequestAttempts {
				if waitErr := waitAMapRetry(ctx, attempt); waitErr != nil {
					return waitErr
				}
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("%w: amap request attempts exhausted", ErrProviderTemporary)
}

func amapNetworkRetryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) && (networkErr.Timeout() || networkErr.Temporary())
}

func amapHTTPStatusError(status int) error {
	switch {
	case status == http.StatusRequestTimeout:
		return fmt.Errorf("%w: amap HTTP status %d", ErrProviderTemporary, status)
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: amap HTTP status %d", ErrProviderRateLimited, status)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: amap HTTP status %d", ErrProviderUnauthorized, status)
	case status >= 500:
		return fmt.Errorf("%w: amap HTTP status %d", ErrProviderTemporary, status)
	default:
		return fmt.Errorf("amap HTTP status %d", status)
	}
}

func amapRetryable(err error) bool {
	return errors.Is(err, ErrProviderTemporary) || errors.Is(err, ErrProviderRateLimited)
}

func waitAMapRetry(ctx context.Context, attempt int) error {
	delay := amapRetryBaseDelay
	for i := 0; i < attempt; i++ {
		if delay >= amapRetryMaxDelay/2 {
			delay = amapRetryMaxDelay
			break
		}
		delay *= 2
	}
	if delay > amapRetryMaxDelay {
		delay = amapRetryMaxDelay
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func safeAMapError(err error) error {
	for err != nil {
		var urlErr *url.Error
		if !errors.As(err, &urlErr) {
			break
		}
		err = urlErr.Err
	}
	if err == nil {
		return errors.New("unknown network error")
	}
	return err
}

func amapStatusError(infoCode, info string) error {
	infoCode = strings.TrimSpace(infoCode)
	info = strings.TrimSpace(info)
	if info == "" {
		info = "unknown error"
	}
	var category error
	switch infoCode {
	case "10003", "10044", "40000":
		category = ErrProviderQuotaExceeded
	case "10004", "10014", "10019", "10020", "10021":
		category = ErrProviderRateLimited
	case "10015", "10016", "10017":
		category = ErrProviderTemporary
	case "10001", "10002", "10005", "10006", "10007", "10008", "10009", "10012", "10013", "10026", "10041":
		category = ErrProviderUnauthorized
	}
	if category != nil {
		return fmt.Errorf("%w: amap API %s: %s", category, infoCode, info)
	}
	return fmt.Errorf("amap API %s: %s", infoCode, info)
}

type amapScalar string

func (s *amapScalar) UnmarshalJSON(data []byte) error {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 || string(data) == "null" {
		*s = ""
		return nil
	}
	if data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*s = amapScalar(value)
		return nil
	}
	*s = amapScalar(string(data))
	return nil
}

func (s amapScalar) String() string { return strings.TrimSpace(string(s)) }

func (s amapScalar) Int() (int, error) {
	value := s.String()
	if value == "" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func (s amapScalar) Float() (float64, error) {
	value := s.String()
	if value == "" {
		return 0, nil
	}
	return strconv.ParseFloat(value, 64)
}

type amapRouteResponse struct {
	Status   string        `json:"status"`
	Info     string        `json:"info"`
	InfoCode string        `json:"infocode"`
	Route    amapRouteData `json:"route"`
}

type amapRouteData struct {
	Paths    []amapRoutePath   `json:"paths"`
	Transits []json.RawMessage `json:"transits"`
	raw      json.RawMessage
}

func (r *amapRouteData) UnmarshalJSON(data []byte) error {
	type routeDataAlias amapRouteData
	var value routeDataAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*r = amapRouteData(value)
	r.raw = append(r.raw[:0], data...)
	return nil
}

type amapRoutePath struct {
	Distance amapScalar      `json:"distance"`
	Duration amapScalar      `json:"duration"`
	Cost     amapRouteCost   `json:"cost"`
	Polyline string          `json:"polyline"`
	Steps    []amapRouteStep `json:"steps"`
	raw      json.RawMessage
}

type amapRouteCost struct {
	Duration amapScalar `json:"duration"`
}

type amapRouteStep struct {
	Polyline string `json:"polyline"`
}

func (p *amapRoutePath) UnmarshalJSON(data []byte) error {
	type routePathAlias amapRoutePath
	var value routePathAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*p = amapRoutePath(value)
	p.raw = append(p.raw[:0], data...)
	return nil
}

func (p amapRoutePath) durationValue() amapScalar {
	if p.Duration.String() != "" {
		return p.Duration
	}
	return p.Cost.Duration
}

func (p *AMapProvider) Route(ctx context.Context, request RouteRequest) (RouteSnapshot, error) {
	if p.serverKey() == "" {
		return RouteSnapshot{}, unavailable(p.ID())
	}
	if err := validatePoint(request.Origin); err != nil {
		return RouteSnapshot{}, fmt.Errorf("invalid route origin: %w", err)
	}
	if err := validatePoint(request.Destination); err != nil {
		return RouteSnapshot{}, fmt.Errorf("invalid route destination: %w", err)
	}
	if request.Mode != ModeDriving && request.Mode != ModeWalking && request.Mode != ModeCycling && request.Mode != ModeTransit {
		return RouteSnapshot{}, fmt.Errorf("amap does not support travel mode %q", request.Mode)
	}
	origin, err := toAMapPoint(request.Origin)
	if err != nil {
		return RouteSnapshot{}, fmt.Errorf("convert route origin: %w", err)
	}
	destination, err := toAMapPoint(request.Destination)
	if err != nil {
		return RouteSnapshot{}, fmt.Errorf("convert route destination: %w", err)
	}

	path := "/v5/direction/driving"
	showFields := "cost,navi,polyline"
	if request.Mode == ModeDriving {
		showFields = "cost,tmcs,navi,cities,polyline"
	}
	params := url.Values{
		"origin":      {formatAMapPoint(origin)},
		"destination": {formatAMapPoint(destination)},
		"key":         {p.serverKey()},
		"output":      {"json"},
		"show_fields": {showFields},
	}
	switch request.Mode {
	case ModeWalking:
		path = "/v5/direction/walking"
	case ModeCycling:
		path = "/v5/direction/bicycling"
	case ModeTransit:
		path = "/v5/direction/transit/integrated"
		if strings.TrimSpace(request.OriginCityCode) == "" || strings.TrimSpace(request.DestinationCityCode) == "" {
			return RouteSnapshot{}, errors.New("amap transit route requires origin_citycode and destination_citycode")
		}
		params.Set("city1", strings.TrimSpace(request.OriginCityCode))
		params.Set("city2", strings.TrimSpace(request.DestinationCityCode))
		if request.OriginPOIID != "" || request.DestinationPOIID != "" {
			if request.OriginPOIID == "" || request.DestinationPOIID == "" {
				return RouteSnapshot{}, errors.New("amap transit route requires both origin_poi_id and destination_poi_id")
			}
			params.Set("originpoi", strings.TrimSpace(request.OriginPOIID))
			params.Set("destinationpoi", strings.TrimSpace(request.DestinationPOIID))
		}
		if request.DepartureAt != nil {
			params.Set("date", request.DepartureAt.Format("2006-01-02"))
			params.Set("time", strconv.Itoa(request.DepartureAt.Hour())+"-"+strconv.Itoa(request.DepartureAt.Minute()))
		}
	}
	if request.Mode != ModeTransit {
		if request.OriginPOIID != "" {
			params.Set("origin_id", strings.TrimSpace(request.OriginPOIID))
		}
		if request.DestinationPOIID != "" {
			params.Set("destination_id", strings.TrimSpace(request.DestinationPOIID))
		}
	}
	strategy := strings.TrimSpace(request.Strategy)
	if strategy == "" && request.Mode == ModeDriving {
		strategy = "32"
	}
	if strategy != "" {
		params.Set("strategy", strategy)
	}
	if request.AlternativeRoute > 0 {
		if request.Mode == ModeTransit {
			params.Set("AlternativeRoute", strconv.Itoa(request.AlternativeRoute))
		} else {
			params.Set("alternative_route", strconv.Itoa(request.AlternativeRoute))
		}
	}

	var response amapRouteResponse
	if err := p.getWithRetry(ctx, path, params, &response, func() error {
		if response.Status != "1" {
			return amapStatusError(response.InfoCode, response.Info)
		}
		return nil
	}); err != nil {
		return RouteSnapshot{}, err
	}

	var distance, duration int
	var geometry []GeoPoint
	if request.Mode == ModeTransit {
		if len(response.Route.Transits) == 0 {
			return RouteSnapshot{}, errors.New("amap transit route returned no routes")
		}
		var summary struct {
			Distance amapScalar    `json:"distance"`
			Duration amapScalar    `json:"duration"`
			Cost     amapRouteCost `json:"cost"`
		}
		if err := json.Unmarshal(response.Route.Transits[0], &summary); err != nil {
			return RouteSnapshot{}, fmt.Errorf("decode amap transit route: %w", err)
		}
		distance, err = summary.Distance.Int()
		if err != nil {
			return RouteSnapshot{}, fmt.Errorf("invalid amap transit distance: %w", err)
		}
		durationValue := summary.Duration
		if durationValue.String() == "" {
			durationValue = summary.Cost.Duration
		}
		duration, err = durationValue.Int()
		if err != nil {
			return RouteSnapshot{}, fmt.Errorf("invalid amap transit duration: %w", err)
		}
		geometry, err = collectAMapPolylines(response.Route.Transits[0])
		if err != nil {
			return RouteSnapshot{}, err
		}
	} else {
		if len(response.Route.Paths) == 0 {
			return RouteSnapshot{}, fmt.Errorf("amap %s route returned no routes", request.Mode)
		}
		selected := response.Route.Paths[0]
		distance, err = selected.Distance.Int()
		if err != nil {
			return RouteSnapshot{}, fmt.Errorf("invalid amap route distance: %w", err)
		}
		duration, err = selected.durationValue().Int()
		if err != nil {
			return RouteSnapshot{}, fmt.Errorf("invalid amap route duration: %w", err)
		}
		for _, step := range selected.Steps {
			points, parseErr := parseAMapPolyline(step.Polyline)
			if parseErr != nil {
				return RouteSnapshot{}, parseErr
			}
			geometry = appendUniqueGeoPoints(geometry, points)
		}
		if len(geometry) < 2 && selected.Polyline != "" {
			points, parseErr := parseAMapPolyline(selected.Polyline)
			if parseErr != nil {
				return RouteSnapshot{}, parseErr
			}
			geometry = appendUniqueGeoPoints(geometry, points)
		}
		if len(geometry) < 2 && len(selected.raw) > 0 {
			points, collectErr := collectAMapPolylines(selected.raw)
			if collectErr != nil {
				return RouteSnapshot{}, collectErr
			}
			geometry = appendUniqueGeoPoints(geometry, points)
		}
		if len(geometry) < 2 && len(response.Route.raw) > 0 {
			points, collectErr := collectAMapPolylines(response.Route.raw)
			if collectErr != nil {
				return RouteSnapshot{}, collectErr
			}
			geometry = appendUniqueGeoPoints(geometry, points)
		}
	}
	if len(geometry) < 2 {
		return RouteSnapshot{}, fmt.Errorf("amap %s route returned no usable geometry", request.Mode)
	}
	now := time.Now().UTC()
	return RouteSnapshot{
		Provider:         p.ID(),
		CoordinateSystem: CRSGCJ02,
		Mode:             request.Mode,
		Strategy:         strategy,
		Source:           "amap-webservice-route-v5",
		Geometry:         geometry,
		DistanceM:        distance,
		DurationS:        duration,
		FetchedAt:        now,
		ExpiresAt:        now.Add(time.Hour),
	}, nil
}

func toAMapPoint(point GeoPoint) (GeoPoint, error) {
	if err := validatePoint(point); err != nil {
		return GeoPoint{}, err
	}
	switch point.CRS {
	case CRSGCJ02:
		return point, nil
	case CRSBD09LL:
		return bd09llToGCJ02(point), nil
	case CRSWGS84:
		return wgs84ToGCJ02(point), nil
	default:
		return GeoPoint{}, fmt.Errorf("unsupported coordinate system %q", point.CRS)
	}
}

func formatAMapPoint(point GeoPoint) string {
	return fmt.Sprintf("%.6f,%.6f", point.Lng, point.Lat)
}

func parseAMapPolyline(raw string) ([]GeoPoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ";")
	points := make([]GeoPoint, 0, len(parts))
	for _, part := range parts {
		values := strings.Split(strings.TrimSpace(part), ",")
		if len(values) != 2 {
			return nil, fmt.Errorf("invalid amap route point %q", part)
		}
		lng, err1 := strconv.ParseFloat(strings.TrimSpace(values[0]), 64)
		lat, err2 := strconv.ParseFloat(strings.TrimSpace(values[1]), 64)
		if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
			return nil, fmt.Errorf("invalid amap route coordinates %q", part)
		}
		points = appendUniqueGeoPoints(points, []GeoPoint{{Lat: lat, Lng: lng, CRS: CRSGCJ02}})
	}
	return points, nil
}

func appendUniqueGeoPoints(dst, src []GeoPoint) []GeoPoint {
	for _, point := range src {
		if len(dst) > 0 {
			last := dst[len(dst)-1]
			if last.Lat == point.Lat && last.Lng == point.Lng && last.CRS == point.CRS {
				continue
			}
		}
		dst = append(dst, point)
	}
	return dst
}

func collectAMapPolylines(raw json.RawMessage) ([]GeoPoint, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode amap route geometry: %w", err)
	}
	var geometry []GeoPoint
	var walk func(any) error
	walk = func(current any) error {
		switch item := current.(type) {
		case []any:
			for _, child := range item {
				if err := walk(child); err != nil {
					return err
				}
			}
		case map[string]any:
			for key, child := range item {
				if strings.EqualFold(key, "polyline") {
					if text, ok := child.(string); ok {
						points, err := parseAMapPolyline(text)
						if err != nil {
							return err
						}
						geometry = appendUniqueGeoPoints(geometry, points)
						continue
					}
				}
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(value); err != nil {
		return nil, err
	}
	return geometry, nil
}

type amapReverseGeocodeResponse struct {
	Status    string `json:"status"`
	Info      string `json:"info"`
	InfoCode  string `json:"infocode"`
	Regeocode struct {
		FormattedAddress string `json:"formatted_address"`
		AddressComponent struct {
			Province string `json:"province"`
			City     string `json:"city"`
			District string `json:"district"`
			Township string `json:"township"`
			Street   string `json:"street"`
			Number   string `json:"number"`
		} `json:"addressComponent"`
	} `json:"regeocode"`
}

func (p *AMapProvider) ReverseGeocode(ctx context.Context, point GeoPoint) (string, error) {
	if p.serverKey() == "" {
		return "", unavailable(p.ID())
	}
	converted, err := toAMapPoint(point)
	if err != nil {
		return "", err
	}
	params := url.Values{
		"location":   {formatAMapPoint(converted)},
		"extensions": {"base"},
		"output":     {"json"},
		"key":        {p.serverKey()},
	}
	var response amapReverseGeocodeResponse
	if err := p.getWithRetry(ctx, "/v3/geocode/regeo", params, &response, func() error {
		if response.Status != "1" {
			return amapStatusError(response.InfoCode, response.Info)
		}
		return nil
	}); err != nil {
		return "", err
	}
	if address := strings.TrimSpace(response.Regeocode.FormattedAddress); address != "" {
		return address, nil
	}
	component := response.Regeocode.AddressComponent
	address := strings.TrimSpace(strings.Join([]string{component.Province, component.City, component.District, component.Township, component.Street, component.Number}, ""))
	if address == "" {
		return "", errors.New("amap reverse geocode returned no address")
	}
	return address, nil
}

type amapWeatherResponse struct {
	Status    string `json:"status"`
	Info      string `json:"info"`
	InfoCode  string `json:"infocode"`
	Forecasts []struct {
		Casts []struct {
			Date         string     `json:"date"`
			DayWeather   string     `json:"dayweather"`
			NightWeather string     `json:"nightweather"`
			DayTemp      amapScalar `json:"daytemp"`
			NightTemp    amapScalar `json:"nighttemp"`
		} `json:"casts"`
	} `json:"forecasts"`
}

func (p *AMapProvider) Weather(ctx context.Context, request WeatherRequest) (WeatherSnapshot, error) {
	if p.serverKey() == "" {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, unavailable(p.ID())
	}
	adcode := strings.TrimSpace(request.AdCode)
	if adcode == "" {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, errors.New("amap weather requires an adcode")
	}
	params := url.Values{
		"city":       {adcode},
		"extensions": {"all"},
		"output":     {"json"},
		"key":        {p.serverKey()},
	}
	var response amapWeatherResponse
	if err := p.getWithRetry(ctx, "/v3/weather/weatherInfo", params, &response, func() error {
		if response.Status != "1" {
			return amapStatusError(response.InfoCode, response.Info)
		}
		return nil
	}); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	now := time.Now().UTC()
	for _, forecast := range response.Forecasts {
		for _, cast := range forecast.Casts {
			if strings.TrimSpace(cast.Date) != strings.TrimSpace(request.LocalDate) {
				continue
			}
			var temperature *float64
			dayTemp, dayErr := cast.DayTemp.Float()
			nightTemp, nightErr := cast.NightTemp.Float()
			switch {
			case dayErr == nil && nightErr == nil && (cast.DayTemp.String() != "" || cast.NightTemp.String() != ""):
				average := (dayTemp + nightTemp) / 2
				temperature = &average
			case dayErr == nil && cast.DayTemp.String() != "":
				temperature = &dayTemp
			case nightErr == nil && cast.NightTemp.String() != "":
				temperature = &nightTemp
			}
			condition := strings.TrimSpace(cast.DayWeather)
			if condition == "" {
				condition = strings.TrimSpace(cast.NightWeather)
			}
			return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Condition: condition, TemperatureC: temperature, FetchedAt: now, ExpiresAt: now.Add(6 * time.Hour), Available: true}, nil
		}
	}
	return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, FetchedAt: now, ExpiresAt: now.Add(time.Hour), Available: false}, nil
}
