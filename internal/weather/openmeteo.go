package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultOpenMeteoBaseURL = "https://api.open-meteo.com/v1/forecast"

type OpenMeteoProvider struct {
	baseURL string
	client  *http.Client
	mu      sync.RWMutex
}

func NewOpenMeteoProvider() *OpenMeteoProvider {
	return &OpenMeteoProvider{
		baseURL: defaultOpenMeteoBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *OpenMeteoProvider) ID() ProviderID {
	return ProviderOpenMeteo
}

func (p *OpenMeteoProvider) SetBaseURL(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(url) != "" {
		p.baseURL = strings.TrimSpace(url)
	}
}

type openMeteoResponse struct {
	Latitude  float64              `json:"latitude"`
	Longitude float64              `json:"longitude"`
	Timezone  string               `json:"timezone"`
	Current   openMeteoCurrent     `json:"current"`
	Daily     openMeteoDaily       `json:"daily"`
}

type openMeteoCurrent struct {
	Time             string  `json:"time"`
	Temperature2M    float64 `json:"temperature_2m"`
	RelativeHumidity float64 `json:"relative_humidity_2m"`
	WeatherCode      int     `json:"weather_code"`
	WindSpeed10M     float64 `json:"wind_speed_10m"`
	WindDirection10M float64 `json:"wind_direction_10m"`
	SurfacePressure  float64 `json:"surface_pressure"`
}

type openMeteoDaily struct {
	Time                        []string  `json:"time"`
	WeatherCode                 []int     `json:"weather_code"`
	Temperature2MMax            []float64 `json:"temperature_2m_max"`
	Temperature2MMin            []float64 `json:"temperature_2m_min"`
	PrecipitationSum            []float64 `json:"precipitation_sum"`
	WindSpeed10MMax             []float64 `json:"wind_speed_10m_max"`
	WindDirection10MDominant    []float64 `json:"wind_direction_10m_dominant"`
}

func (p *OpenMeteoProvider) Weather(ctx context.Context, request WeatherRequest) (WeatherSnapshot, error) {
	if err := ValidatePoint(request.Location); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}

	// Open-Meteo 使用标准 WGS-84 坐标系
	wgsLat, wgsLng := NormalizeToWGS84(request.Location)

	tz := strings.TrimSpace(request.Timezone)
	if tz == "" {
		tz = "auto"
	}

	params := url.Values{
		"latitude":         {fmt.Sprintf("%.4f", wgsLat)},
		"longitude":        {fmt.Sprintf("%.4f", wgsLng)},
		"daily":            {"weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum,wind_speed_10m_max,wind_direction_10m_dominant"},
		"current":          {"temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m,wind_direction_10m,surface_pressure"},
		"timezone":         {tz},
		"forecast_days":    {"16"},
	}

	p.mu.RLock()
	baseURL := p.baseURL
	p.mu.RUnlock()

	apiURL := baseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	req.Header.Set("User-Agent", "JourneyIn-Weather/0.5.8")

	resp, err := p.client.Do(req)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, fmt.Errorf("open-meteo api status %d", resp.StatusCode)
	}

	var payload openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}

	now := time.Now().UTC()
	currentCondition := FormatWMOCode(payload.Current.WeatherCode)
	currentTemp := payload.Current.Temperature2M
	currentHumidity := payload.Current.RelativeHumidity
	currentWindDir := FormatWindDirection(payload.Current.WindDirection10M)
	currentWindPower := FormatWindPower(payload.Current.WindSpeed10M)

	targetDate := strings.TrimSpace(request.LocalDate)
	for i, castDate := range payload.Daily.Time {
		if len(castDate) >= 10 {
			castDate = castDate[:10]
		}
		if castDate != targetDate {
			continue
		}

		minTemp := 0.0
		maxTemp := 0.0
		if i < len(payload.Daily.Temperature2MMin) {
			minTemp = payload.Daily.Temperature2MMin[i]
		}
		if i < len(payload.Daily.Temperature2MMax) {
			maxTemp = payload.Daily.Temperature2MMax[i]
		}
		avgTemp := (minTemp + maxTemp) / 2

		condition := currentCondition
		if i < len(payload.Daily.WeatherCode) {
			condition = FormatWMOCode(payload.Daily.WeatherCode[i])
		}

		windDir := currentWindDir
		windPower := currentWindPower
		if i < len(payload.Daily.WindDirection10MDominant) {
			windDir = FormatWindDirection(payload.Daily.WindDirection10MDominant[i])
		}
		if i < len(payload.Daily.WindSpeed10MMax) {
			windPower = FormatWindPower(payload.Daily.WindSpeed10MMax[i])
		}

		var pressure *float64
		if payload.Current.SurfacePressure > 0 {
			pVal := payload.Current.SurfacePressure
			pressure = &pVal
		}

		return WeatherSnapshot{
			Provider:         p.ID(),
			LocalDate:        request.LocalDate,
			Condition:        condition,
			TemperatureC:     &avgTemp,
			TempMinC:         &minTemp,
			TempMaxC:         &maxTemp,
			CurrentCondition: currentCondition,
			CurrentTempC:     &currentTemp,
			HumidityPercent:  &currentHumidity,
			WindDirection:    windDir,
			WindPower:        windPower,
			PressureHPa:      pressure,
			FetchedAt:        now,
			ExpiresAt:        now.Add(6 * time.Hour),
			Available:        true,
		}, nil
	}

	return WeatherSnapshot{
		Provider:  p.ID(),
		LocalDate: request.LocalDate,
		FetchedAt: now,
		ExpiresAt: now.Add(time.Hour),
		Available: false,
	}, nil
}

// FormatWMOCode 将 WMO 标准天气编码转换为通俗中文天气描述
func FormatWMOCode(code int) string {
	switch code {
	case 0:
		return "晴"
	case 1:
		return "晴间多云"
	case 2:
		return "多云"
	case 3:
		return "阴"
	case 45, 48:
		return "雾"
	case 51:
		return "细雨"
	case 53:
		return "毛毛雨"
	case 55:
		return "密毛毛雨"
	case 56, 57:
		return "冻毛毛雨"
	case 61:
		return "小雨"
	case 63:
		return "中雨"
	case 65:
		return "大雨"
	case 66, 67:
		return "冻雨"
	case 71:
		return "小雪"
	case 73:
		return "中雪"
	case 75:
		return "大雪"
	case 77:
		return "雪粒"
	case 80:
		return "小阵雨"
	case 81:
		return "中阵雨"
	case 82:
		return "强阵雨"
	case 85:
		return "阵雪"
	case 86:
		return "强阵雪"
	case 95:
		return "雷阵雨"
	case 96, 99:
		return "雷暴伴有冰雹"
	default:
		return "晴"
	}
}
