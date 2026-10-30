package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const defaultOpenMeteoBaseURL = "https://api.open-meteo.com/v1/forecast"
const defaultOpenMeteoAirQualityURL = "https://air-quality-api.open-meteo.com/v1/air-quality"

type OpenMeteoProvider struct {
	baseURL       string
	airQualityURL string
	client        *http.Client
	mu            sync.RWMutex
}

func NewOpenMeteoProvider() *OpenMeteoProvider {
	return &OpenMeteoProvider{
		baseURL:       defaultOpenMeteoBaseURL,
		airQualityURL: defaultOpenMeteoAirQualityURL,
		client:        &http.Client{Timeout: 10 * time.Second},
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

func (p *OpenMeteoProvider) SetAirQualityBaseURL(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(url) != "" {
		p.airQualityURL = strings.TrimSpace(url)
	}
}

func (p *OpenMeteoProvider) AirQualityBaseURL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.airQualityURL == "" {
		return defaultOpenMeteoAirQualityURL
	}
	return p.airQualityURL
}

type openMeteoResponse struct {
	Latitude  float64          `json:"latitude"`
	Longitude float64          `json:"longitude"`
	Timezone  string           `json:"timezone"`
	Current   openMeteoCurrent `json:"current"`
	Daily     openMeteoDaily   `json:"daily"`
	Hourly    *openMeteoHourly `json:"hourly,omitempty"`
}

type openMeteoCurrent struct {
	Time                string  `json:"time"`
	Temperature2M       float64 `json:"temperature_2m"`
	RelativeHumidity    float64 `json:"relative_humidity_2m"`
	ApparentTemperature float64 `json:"apparent_temperature"`
	WeatherCode         int     `json:"weather_code"`
	WindSpeed10M        float64 `json:"wind_speed_10m"`
	WindDirection10M    float64 `json:"wind_direction_10m"`
	SurfacePressure     float64 `json:"surface_pressure"`
	Visibility          float64 `json:"visibility"`
	CloudCover          float64 `json:"cloud_cover"`
	UVIndex             float64 `json:"uv_index"`
}

type openMeteoHourly struct {
	Time           []string  `json:"time"`
	Visibility     []float64 `json:"visibility"`
	CloudCover     []float64 `json:"cloud_cover"`
	CloudCoverLow  []float64 `json:"cloud_cover_low"`
	CloudCoverMid  []float64 `json:"cloud_cover_mid"`
	CloudCoverHigh []float64 `json:"cloud_cover_high"`
}

type openMeteoDaily struct {
	Time                        []string  `json:"time"`
	WeatherCode                 []int     `json:"weather_code"`
	Temperature2MMax            []float64 `json:"temperature_2m_max"`
	Temperature2MMin            []float64 `json:"temperature_2m_min"`
	ApparentTemperatureMax      []float64 `json:"apparent_temperature_max"`
	ApparentTemperatureMin      []float64 `json:"apparent_temperature_min"`
	PrecipitationSum            []float64 `json:"precipitation_sum"`
	PrecipitationProbabilityMax []int     `json:"precipitation_probability_max"`
	WindSpeed10MMax             []float64 `json:"wind_speed_10m_max"`
	WindDirection10MDominant    []float64 `json:"wind_direction_10m_dominant"`
	UVIndexMax                  []float64 `json:"uv_index_max"`
}

type openMeteoAirResponse struct {
	Hourly *openMeteoAirHourly `json:"hourly,omitempty"`
}

type openMeteoAirHourly struct {
	Time                []string  `json:"time"`
	PM10                []float64 `json:"pm10"`
	PM25                []float64 `json:"pm2_5"`
	AerosolOpticalDepth []float64 `json:"aerosol_optical_depth"`
	Dust                []float64 `json:"dust"`
	EuropeanAQI         []float64 `json:"european_aqi"`
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
		"latitude":      {fmt.Sprintf("%.4f", wgsLat)},
		"longitude":     {fmt.Sprintf("%.4f", wgsLng)},
		"daily":         {"weather_code,temperature_2m_max,temperature_2m_min,apparent_temperature_max,apparent_temperature_min,precipitation_sum,precipitation_probability_max,wind_speed_10m_max,wind_direction_10m_dominant,uv_index_max"},
		"hourly":        {"visibility,cloud_cover,cloud_cover_low,cloud_cover_mid,cloud_cover_high"},
		"current":       {"temperature_2m,relative_humidity_2m,apparent_temperature,weather_code,wind_speed_10m,wind_direction_10m,surface_pressure,visibility,cloud_cover,uv_index"},
		"timezone":      {tz},
		"forecast_days": {"16"},
	}

	p.mu.RLock()
	baseURL := p.baseURL
	p.mu.RUnlock()

	apiURL := baseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	req.Header.Set("User-Agent", "JourneyIn-Weather/0.7.10")

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

	// 并行/连带请求 Open-Meteo Air Quality 接口 (AOD 气溶胶、PM2.5、PM10、AQI)
	var airPayload *openMeteoAirResponse
	airBaseURL := p.AirQualityBaseURL()
	airParams := url.Values{
		"latitude":      {fmt.Sprintf("%.4f", wgsLat)},
		"longitude":     {fmt.Sprintf("%.4f", wgsLng)},
		"hourly":        {"pm10,pm2_5,aerosol_optical_depth,dust,european_aqi"},
		"timezone":      {tz},
		"forecast_days": {"16"},
	}
	airReq, airErr := http.NewRequestWithContext(ctx, http.MethodGet, airBaseURL+"?"+airParams.Encode(), nil)
	if airErr == nil {
		airReq.Header.Set("User-Agent", "JourneyIn-Weather/0.7.10")
		if airResp, err := p.client.Do(airReq); err == nil {
			if airResp.StatusCode == http.StatusOK {
				var decoded openMeteoAirResponse
				if json.NewDecoder(airResp.Body).Decode(&decoded) == nil {
					airPayload = &decoded
				}
			}
			_ = airResp.Body.Close()
		}
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

		// 聚合目标日期的 hourly 数据 (能见度, 总云量, 低中高分层云量)
		var visKm *float64
		var cloudCoverPct *int
		var cloudLow *int
		var cloudMid *int
		var cloudHigh *int

		if payload.Hourly != nil && len(payload.Hourly.Time) > 0 {
			var visSum, cloudSum, lowSum, midSum, highSum float64
			count := 0
			for hIdx, hTime := range payload.Hourly.Time {
				if strings.HasPrefix(hTime, targetDate) {
					count++
					if hIdx < len(payload.Hourly.Visibility) {
						visSum += payload.Hourly.Visibility[hIdx]
					}
					if hIdx < len(payload.Hourly.CloudCover) {
						cloudSum += payload.Hourly.CloudCover[hIdx]
					}
					if hIdx < len(payload.Hourly.CloudCoverLow) {
						lowSum += payload.Hourly.CloudCoverLow[hIdx]
					}
					if hIdx < len(payload.Hourly.CloudCoverMid) {
						midSum += payload.Hourly.CloudCoverMid[hIdx]
					}
					if hIdx < len(payload.Hourly.CloudCoverHigh) {
						highSum += payload.Hourly.CloudCoverHigh[hIdx]
					}
				}
			}
			if count > 0 {
				v := (visSum / float64(count)) / 1000.0 // 米 -> 千米
				visKm = &v
				c := int(math.Round(cloudSum / float64(count)))
				cloudCoverPct = &c
				l := int(math.Round(lowSum / float64(count)))
				cloudLow = &l
				m := int(math.Round(midSum / float64(count)))
				cloudMid = &m
				h := int(math.Round(highSum / float64(count)))
				cloudHigh = &h
			}
		}

		if visKm == nil && payload.Current.Visibility > 0 {
			v := payload.Current.Visibility / 1000.0
			visKm = &v
		}
		if cloudCoverPct == nil && payload.Current.CloudCover >= 0 {
			c := int(math.Round(payload.Current.CloudCover))
			cloudCoverPct = &c
		}

		var rainProbPct *int
		if i < len(payload.Daily.PrecipitationProbabilityMax) {
			rp := payload.Daily.PrecipitationProbabilityMax[i]
			rainProbPct = &rp
		}

		var apparentTemp *float64
		if i < len(payload.Daily.ApparentTemperatureMax) && i < len(payload.Daily.ApparentTemperatureMin) {
			at := (payload.Daily.ApparentTemperatureMax[i] + payload.Daily.ApparentTemperatureMin[i]) / 2
			apparentTemp = &at
		} else if payload.Current.ApparentTemperature != 0 {
			at := payload.Current.ApparentTemperature
			apparentTemp = &at
		}

		var uvIndex *float64
		if i < len(payload.Daily.UVIndexMax) {
			uv := payload.Daily.UVIndexMax[i]
			uvIndex = &uv
		} else if payload.Current.UVIndex > 0 {
			uv := payload.Current.UVIndex
			uvIndex = &uv
		}

		var aodVal *float64
		var pm25Val *float64
		var pm10Val *float64
		var aqiVal *int
		if airPayload != nil && airPayload.Hourly != nil && len(airPayload.Hourly.Time) > 0 {
			var aodSum, pm25Sum, pm10Sum, aqiSum float64
			airCount := 0
			for aIdx, aTime := range airPayload.Hourly.Time {
				if strings.HasPrefix(aTime, targetDate) {
					airCount++
					if aIdx < len(airPayload.Hourly.AerosolOpticalDepth) {
						aodSum += airPayload.Hourly.AerosolOpticalDepth[aIdx]
					}
					if aIdx < len(airPayload.Hourly.PM25) {
						pm25Sum += airPayload.Hourly.PM25[aIdx]
					}
					if aIdx < len(airPayload.Hourly.PM10) {
						pm10Sum += airPayload.Hourly.PM10[aIdx]
					}
					if aIdx < len(airPayload.Hourly.EuropeanAQI) {
						aqiSum += airPayload.Hourly.EuropeanAQI[aIdx]
					}
				}
			}
			if airCount > 0 {
				aod := math.Round((aodSum/float64(airCount))*100) / 100
				aodVal = &aod
				p25 := math.Round((pm25Sum/float64(airCount))*10) / 10
				pm25Val = &p25
				p10 := math.Round((pm10Sum/float64(airCount))*10) / 10
				pm10Val = &p10
				aqi := int(math.Round(aqiSum / float64(airCount)))
				aqiVal = &aqi
			}
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
			VisibilityKm:     visKm,
			CloudCoverPct:    cloudCoverPct,
			CloudCoverLow:    cloudLow,
			CloudCoverMid:    cloudMid,
			CloudCoverHigh:   cloudHigh,
			RainProbPct:      rainProbPct,
			UVIndex:          uvIndex,
			ApparentTempC:    apparentTemp,
			AQI:              aqiVal,
			PM25:             pm25Val,
			PM10:             pm10Val,
			AOD:              aodVal,
			FetchedAt:        now,
			ExpiresAt:        now.Add(weatherSnapshotTTL),
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
