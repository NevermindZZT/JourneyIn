package weather

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

const defaultQWeatherHost = "api.qweather.com"

type QWeatherProvider struct {
	key    string
	host   string
	client *http.Client
	mu     sync.RWMutex
}

func NewQWeatherProvider(key, host string) *QWeatherProvider {
	if strings.TrimSpace(host) == "" {
		host = defaultQWeatherHost
	}
	return &QWeatherProvider{
		key:    strings.TrimSpace(key),
		host:   strings.TrimSpace(host),
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *QWeatherProvider) ID() ProviderID {
	return ProviderQWeather
}

func (p *QWeatherProvider) SetKey(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.key = strings.TrimSpace(key)
}

func (p *QWeatherProvider) Key() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.key
}

func (p *QWeatherProvider) KeyConfigured() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.key != ""
}

func (p *QWeatherProvider) SetHost(host string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(host) != "" {
		p.host = strings.TrimSpace(host)
	} else {
		p.host = defaultQWeatherHost
	}
}

func (p *QWeatherProvider) Host() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.host == "" {
		return defaultQWeatherHost
	}
	return p.host
}

type qweatherDailyResponse struct {
	Code       string          `json:"code"`
	UpdateTime string          `json:"updateTime"`
	Daily      []qweatherDaily `json:"daily"`
}

type qweatherDaily struct {
	FxDate       string `json:"fxDate"`
	TempMax      string `json:"tempMax"`
	TempMin      string `json:"tempMin"`
	TextDay      string `json:"textDay"`
	TextNight    string `json:"textNight"`
	WindDirDay   string `json:"windDirDay"`
	WindScaleDay string `json:"windScaleDay"`
	Humidity     string `json:"humidity"`
	Precip       string `json:"precip"`
	Pressure     string `json:"pressure"`
}

type qweatherNowResponse struct {
	Code string      `json:"code"`
	Now  qweatherNow `json:"now"`
}

type qweatherNow struct {
	Temp      string `json:"temp"`
	Text      string `json:"text"`
	WindDir   string `json:"windDir"`
	WindScale string `json:"windScale"`
	Humidity  string `json:"humidity"`
	Pressure  string `json:"pressure"`
}

func (p *QWeatherProvider) Weather(ctx context.Context, request WeatherRequest) (WeatherSnapshot, error) {
	p.mu.RLock()
	key := p.key
	host := p.host
	p.mu.RUnlock()

	if key == "" {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, Unavailable(p.ID())
	}
	if err := ValidatePoint(request.Location); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}

	// 和风天气支持经纬度查询 (经度在前，纬度在后，保留两位小数)
	gcjLat, gcjLng := NormalizeToGCJ02(request.Location)
	locationParam := fmt.Sprintf("%.2f,%.2f", gcjLng, gcjLat)

	baseURL := strings.TrimRight(host, "/")
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}

	// 1. 查询未来最多 10-30 天天气预报 (使用 /v7/weather/10d)
	forecastURL := fmt.Sprintf("%s/v7/weather/10d?location=%s", baseURL, url.QueryEscape(locationParam))
	fReq, err := http.NewRequestWithContext(ctx, http.MethodGet, forecastURL, nil)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	fReq.Header.Set("X-QW-Api-Key", key)
	fReq.Header.Set("User-Agent", "JourneyIn-Weather/0.5.7")

	fResp, err := p.client.Do(fReq)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	defer fResp.Body.Close()

	if fResp.StatusCode != http.StatusOK {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, fmt.Errorf("qweather api status %d", fResp.StatusCode)
	}

	var forecastPayload qweatherDailyResponse
	if err := json.NewDecoder(fResp.Body).Decode(&forecastPayload); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	if forecastPayload.Code != "200" {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, fmt.Errorf("qweather error code %s", forecastPayload.Code)
	}

	// 2. 尝试获取实时天气（可选）
	var currentCondition string
	var currentTemp *float64
	var currentHumidity *float64
	var currentWindDir string
	var currentWindPower string

	nowURL := fmt.Sprintf("%s/v7/weather/now?location=%s", baseURL, url.QueryEscape(locationParam))
	if nReq, nErr := http.NewRequestWithContext(ctx, http.MethodGet, nowURL, nil); nErr == nil {
		nReq.Header.Set("X-QW-Api-Key", key)
		if nResp, err := p.client.Do(nReq); err == nil {
			var nowPayload qweatherNowResponse
			if json.NewDecoder(nResp.Body).Decode(&nowPayload) == nil && nowPayload.Code == "200" {
				currentCondition = strings.TrimSpace(nowPayload.Now.Text)
				if tVal, err := strconv.ParseFloat(nowPayload.Now.Temp, 64); err == nil {
					currentTemp = &tVal
				}
				if hVal, err := strconv.ParseFloat(nowPayload.Now.Humidity, 64); err == nil {
					currentHumidity = &hVal
				}
				currentWindDir = strings.TrimSpace(nowPayload.Now.WindDir)
				scale := strings.TrimSpace(nowPayload.Now.WindScale)
				if scale != "" && !strings.HasSuffix(scale, "级") {
					scale = scale + "级"
				}
				currentWindPower = scale
			}
			_ = nResp.Body.Close()
		}
	}

	now := time.Now().UTC()
	targetDate := strings.TrimSpace(request.LocalDate)
	for _, cast := range forecastPayload.Daily {
		castDate := cast.FxDate
		if len(castDate) >= 10 {
			castDate = castDate[:10]
		}
		if castDate != targetDate {
			continue
		}

		maxTemp, _ := strconv.ParseFloat(cast.TempMax, 64)
		minTemp, _ := strconv.ParseFloat(cast.TempMin, 64)
		avgTemp := (maxTemp + minTemp) / 2

		dayText := strings.TrimSpace(cast.TextDay)
		nightText := strings.TrimSpace(cast.TextNight)
		condition := dayText
		if dayText != "" && nightText != "" && dayText != nightText {
			condition = dayText + "转" + nightText
		} else if condition == "" {
			condition = nightText
		}

		var humidity *float64
		if hVal, err := strconv.ParseFloat(cast.Humidity, 64); err == nil {
			humidity = &hVal
		} else {
			humidity = currentHumidity
		}

		windDir := strings.TrimSpace(cast.WindDirDay)
		if windDir == "" {
			windDir = currentWindDir
		}
		windPower := strings.TrimSpace(cast.WindScaleDay)
		if windPower != "" && !strings.HasSuffix(windPower, "级") {
			windPower = windPower + "级"
		}
		if windPower == "" {
			windPower = currentWindPower
		}

		var pressure *float64
		if pVal, err := strconv.ParseFloat(cast.Pressure, 64); err == nil {
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
			CurrentTempC:     currentTemp,
			HumidityPercent:  humidity,
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
