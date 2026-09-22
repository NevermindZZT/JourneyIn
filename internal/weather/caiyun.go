package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const defaultCaiyunBaseURL = "https://api.caiyunapp.com/v2.6"

type CaiyunProvider struct {
	token   string
	baseURL string
	client  *http.Client
	mu      sync.RWMutex
}

func NewCaiyunProvider(token string) *CaiyunProvider {
	return &CaiyunProvider{
		token:   strings.TrimSpace(token),
		baseURL: defaultCaiyunBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *CaiyunProvider) ID() ProviderID {
	return ProviderCaiyun
}

func (p *CaiyunProvider) SetToken(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.token = strings.TrimSpace(token)
}

func (p *CaiyunProvider) Token() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.token
}

func (p *CaiyunProvider) TokenConfigured() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.token != ""
}

func (p *CaiyunProvider) SetBaseURL(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(url) != "" {
		p.baseURL = strings.TrimRight(strings.TrimSpace(url), "/")
	}
}

type caiyunResponse struct {
	Status     string       `json:"status"`
	Message    string       `json:"message,omitempty"`
	Error      string       `json:"error,omitempty"`
	ServerTime int64        `json:"server_time"`
	Result     caiyunResult `json:"result"`
}

type caiyunResult struct {
	Realtime caiyunRealtime `json:"realtime"`
	Daily    caiyunDaily    `json:"daily"`
}

type caiyunRealtime struct {
	Status      string     `json:"status"`
	Temperature float64    `json:"temperature"`
	Humidity    float64    `json:"humidity"`
	Skycon      string     `json:"skycon"`
	Pressure    float64    `json:"pressure"`
	Wind        caiyunWind `json:"wind"`
}

type caiyunWind struct {
	Speed     float64 `json:"speed"`
	Direction float64 `json:"direction"`
}

type caiyunDaily struct {
	Status      string              `json:"status"`
	Temperature []caiyunDailyTemp   `json:"temperature"`
	Skycon      []caiyunDailySkycon `json:"skycon"`
	Humidity    []caiyunDailyScalar `json:"humidity"`
	Wind        []caiyunDailyWind   `json:"wind"`
	Pressure    []caiyunDailyScalar `json:"pressure"`
}

type caiyunDailyTemp struct {
	Date string  `json:"date"`
	Max  float64 `json:"max"`
	Min  float64 `json:"min"`
	Avg  float64 `json:"avg"`
}

type caiyunDailySkycon struct {
	Date  string `json:"date"`
	Value string `json:"value"`
}

type caiyunDailyScalar struct {
	Date string  `json:"date"`
	Max  float64 `json:"max"`
	Min  float64 `json:"min"`
	Avg  float64 `json:"avg"`
}

type caiyunDailyWind struct {
	Date string     `json:"date"`
	Max  caiyunWind `json:"max"`
	Min  caiyunWind `json:"min"`
	Avg  caiyunWind `json:"avg"`
}

func (p *CaiyunProvider) Weather(ctx context.Context, request WeatherRequest) (WeatherSnapshot, error) {
	token := p.Token()
	if token == "" {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, Unavailable(p.ID())
	}
	if err := ValidatePoint(request.Location); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}

	// 彩云原生推荐经纬度 (gcj02 或 wgs84)；经度在前，纬度在后
	gcjLat, gcjLng := NormalizeToGCJ02(request.Location)
	apiURL := fmt.Sprintf("%s/%s/%.6f,%.6f/weather?dailysteps=15&alert=false", p.baseURL, token, gcjLng, gcjLat)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	req.Header.Set("User-Agent", "JourneyIn-Weather/0.5.9")

	resp, err := p.client.Do(req)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, fmt.Errorf("caiyun api status %d", resp.StatusCode)
	}

	var payload caiyunResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	if payload.Status != "ok" {
		msg := payload.Message
		if msg == "" {
			msg = payload.Error
		}
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, fmt.Errorf("caiyun error: %s", msg)
	}

	now := time.Now().UTC()
	currentCondition := FormatSkycon(payload.Result.Realtime.Skycon)
	currentTemp := payload.Result.Realtime.Temperature
	currentHumidity := payload.Result.Realtime.Humidity * 100
	currentWindDir := FormatWindDirection(payload.Result.Realtime.Wind.Direction)
	currentWindPower := FormatWindPower(payload.Result.Realtime.Wind.Speed)

	// 寻找匹配 local_date 的 daily 预报
	targetDate := strings.TrimSpace(request.LocalDate)
	for i, tempCast := range payload.Result.Daily.Temperature {
		castDate := tempCast.Date
		if len(castDate) >= 10 {
			castDate = castDate[:10]
		}
		if castDate != targetDate {
			continue
		}

		minTemp := tempCast.Min
		maxTemp := tempCast.Max
		avgTemp := tempCast.Avg
		if avgTemp == 0 && (minTemp != 0 || maxTemp != 0) {
			avgTemp = (minTemp + maxTemp) / 2
		}

		condition := ""
		if i < len(payload.Result.Daily.Skycon) {
			condition = FormatSkycon(payload.Result.Daily.Skycon[i].Value)
		}
		if condition == "" {
			condition = currentCondition
		}

		var humidity *float64
		if i < len(payload.Result.Daily.Humidity) {
			h := payload.Result.Daily.Humidity[i].Avg * 100
			humidity = &h
		} else {
			humidity = &currentHumidity
		}

		windDir := currentWindDir
		windPower := currentWindPower
		if i < len(payload.Result.Daily.Wind) {
			w := payload.Result.Daily.Wind[i]
			windDir = FormatWindDirection(w.Avg.Direction)
			windPower = FormatWindPower(w.Avg.Speed)
		}

		var pressure *float64
		if i < len(payload.Result.Daily.Pressure) && payload.Result.Daily.Pressure[i].Avg > 0 {
			p := payload.Result.Daily.Pressure[i].Avg / 100 // Pa -> hPa
			pressure = &p
		} else if payload.Result.Realtime.Pressure > 0 {
			p := payload.Result.Realtime.Pressure / 100
			pressure = &p
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
			HumidityPercent:  humidity,
			WindDirection:    windDir,
			WindPower:        windPower,
			PressureHPa:      pressure,
			FetchedAt:        now,
			ExpiresAt:        now.Add(6 * time.Hour),
			Available:        true,
		}, nil
	}

	// 未找到指定日期的未来预报 (例如已过期的历史日期或超过15天的远期)
	return WeatherSnapshot{
		Provider:  p.ID(),
		LocalDate: request.LocalDate,
		FetchedAt: now,
		ExpiresAt: now.Add(time.Hour),
		Available: false,
	}, nil
}

var skyconMap = map[string]string{
	"CLEAR_DAY":           "晴",
	"CLEAR_NIGHT":         "晴",
	"PARTLY_CLOUDY_DAY":   "多云",
	"PARTLY_CLOUDY_NIGHT": "多云",
	"CLOUDY":              "阴",
	"LIGHT_HAZE":          "轻度雾霾",
	"MODERATE_HAZE":       "中度雾霾",
	"HEAVY_HAZE":          "重度雾霾",
	"LIGHT_RAIN":          "小雨",
	"MODERATE_RAIN":       "中雨",
	"HEAVY_RAIN":          "大雨",
	"STORM_RAIN":          "暴雨",
	"FOG":                 "雾",
	"LIGHT_SNOW":          "小雪",
	"MODERATE_SNOW":       "中雪",
	"HEAVY_SNOW":          "大雪",
	"STORM_SNOW":          "暴雪",
	"DUST":                "浮尘",
	"SAND":                "沙尘",
	"WIND":                "大风",
}

func FormatSkycon(code string) string {
	upper := strings.ToUpper(strings.TrimSpace(code))
	if name, ok := skyconMap[upper]; ok {
		return name
	}
	if upper == "" {
		return "晴"
	}
	return upper
}

func FormatWindDirection(degrees float64) string {
	for degrees < 0 {
		degrees += 360
	}
	for degrees >= 360 {
		degrees -= 360
	}
	switch {
	case degrees >= 337.5 || degrees < 22.5:
		return "北风"
	case degrees >= 22.5 && degrees < 67.5:
		return "东北风"
	case degrees >= 67.5 && degrees < 112.5:
		return "东风"
	case degrees >= 112.5 && degrees < 157.5:
		return "东南风"
	case degrees >= 157.5 && degrees < 202.5:
		return "南风"
	case degrees >= 202.5 && degrees < 247.5:
		return "西南风"
	case degrees >= 247.5 && degrees < 292.5:
		return "西风"
	case degrees >= 292.5 && degrees < 337.5:
		return "西北风"
	default:
		return "无持续风向"
	}
}

// FormatWindPower 将风速 (km/h) 转换为蒲福风级描述
func FormatWindPower(speedKmH float64) string {
	switch {
	case speedKmH < 1:
		return "<3级"
	case speedKmH <= 5:
		return "1级"
	case speedKmH <= 11:
		return "2级"
	case speedKmH <= 19:
		return "3级"
	case speedKmH <= 28:
		return "4级"
	case speedKmH <= 38:
		return "5级"
	case speedKmH <= 49:
		return "6级"
	case speedKmH <= 61:
		return "7级"
	case speedKmH <= 74:
		return "8级"
	case speedKmH <= 88:
		return "9级"
	default:
		return "10级以上"
	}
}
