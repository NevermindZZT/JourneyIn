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

const defaultSevenTimerBaseURL = "http://www.7timer.info/bin/api.pl"

type SevenTimerProvider struct {
	baseURL string
	client  *http.Client
	mu      sync.RWMutex
}

func NewSevenTimerProvider() *SevenTimerProvider {
	return &SevenTimerProvider{
		baseURL: defaultSevenTimerBaseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *SevenTimerProvider) ID() ProviderID {
	return Provider7Timer
}

func (p *SevenTimerProvider) SetBaseURL(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if strings.TrimSpace(url) != "" {
		p.baseURL = strings.TrimSpace(url)
	}
}

type sevenTimerResponse struct {
	Product    string            `json:"product"`
	Init       string            `json:"init"`
	Dataseries []sevenTimerPoint `json:"dataseries"`
}

type sevenTimerPoint struct {
	Timepoint    int            `json:"timepoint"`
	Cloudcover   int            `json:"cloudcover"`
	Seeing       int            `json:"seeing"`
	Transparency int            `json:"transparency"`
	LiftedIndex  int            `json:"lifted_index"`
	RH2m         int            `json:"rh2m"`
	Wind10m      sevenTimerWind `json:"wind10m"`
	Temp2m       int            `json:"temp2m"`
	PrecType     string         `json:"prec_type"`
}

type sevenTimerWind struct {
	Direction string `json:"direction"`
	Speed     int    `json:"speed"`
}

func (p *SevenTimerProvider) Weather(ctx context.Context, request WeatherRequest) (WeatherSnapshot, error) {
	if err := ValidatePoint(request.Location); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}

	wgsLat, wgsLng := NormalizeToWGS84(request.Location)
	p.mu.RLock()
	baseURL := p.baseURL
	p.mu.RUnlock()

	params := url.Values{
		"lon":     {fmt.Sprintf("%.4f", wgsLng)},
		"lat":     {fmt.Sprintf("%.4f", wgsLat)},
		"product": {"astro"},
		"output":  {"json"},
	}

	apiURL := baseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	req.Header.Set("User-Agent", "JourneyIn-Weather/0.7.8")

	resp, err := p.client.Do(req)
	if err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, fmt.Errorf("7timer api status %d", resp.StatusCode)
	}

	var payload sevenTimerResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return WeatherSnapshot{Provider: p.ID(), LocalDate: request.LocalDate, Available: false}, err
	}

	now := time.Now().UTC()
	initTime, err := time.Parse("2006010215", payload.Init)
	if err != nil {
		initTime, err = time.Parse("2006010204", payload.Init)
		if err != nil {
			initTime = now
		}
	}

	targetDate := strings.TrimSpace(request.LocalDate)
	loc := time.UTC
	if tz := strings.TrimSpace(request.Timezone); tz != "" {
		if loaded, err := time.LoadLocation(tz); err == nil {
			loc = loaded
		}
	}

	var matchSeeingSum, matchTransSum, matchCloudSum float64
	var matchTempSum float64
	var minTemp, maxTemp float64
	matchCount := 0
	hasRain := false

	for _, pt := range payload.Dataseries {
		ptTime := initTime.Add(time.Duration(pt.Timepoint) * time.Hour).In(loc)
		ptDate := ptTime.Format("2006-01-02")
		if ptDate != targetDate {
			continue
		}

		matchCount++
		matchSeeingSum += float64(pt.Seeing)
		matchTransSum += float64(pt.Transparency)
		matchCloudSum += sevenTimerCloudToPct(pt.Cloudcover)
		tVal := float64(pt.Temp2m)
		matchTempSum += tVal

		if matchCount == 1 || tVal < minTemp {
			minTemp = tVal
		}
		if matchCount == 1 || tVal > maxTemp {
			maxTemp = tVal
		}
		if pt.PrecType != "none" {
			hasRain = true
		}
	}

	if matchCount == 0 {
		return WeatherSnapshot{
			Provider:  p.ID(),
			LocalDate: request.LocalDate,
			FetchedAt: now,
			ExpiresAt: now.Add(time.Hour),
			Available: false,
		}, nil
	}

	avgSeeing := math.Round((matchSeeingSum/float64(matchCount))*10) / 10
	avgTrans := math.Round((matchTransSum/float64(matchCount))*10) / 10
	cloudPct := int(math.Round(matchCloudSum / float64(matchCount)))
	avgTemp := math.Round((matchTempSum/float64(matchCount))*10) / 10

	condition := "晴"
	if hasRain {
		condition = "有降水"
	} else if cloudPct > 60 {
		condition = "阴"
	} else if cloudPct > 20 {
		condition = "多云"
	}

	return WeatherSnapshot{
		Provider:      p.ID(),
		LocalDate:     request.LocalDate,
		Condition:     condition,
		TemperatureC:  &avgTemp,
		TempMinC:      &minTemp,
		TempMaxC:      &maxTemp,
		CloudCoverPct: &cloudPct,
		Seeing:        &avgSeeing,
		Transparency:  &avgTrans,
		FetchedAt:     now,
		ExpiresAt:     now.Add(weatherSnapshotTTL),
		Available:     true,
	}, nil
}

// 7Timer cloudcover 1-9 级转换为百分比 (0-100)
func sevenTimerCloudToPct(cc int) float64 {
	switch cc {
	case 1:
		return 3.0 // 0%-6%
	case 2:
		return 12.0 // 6%-19%
	case 3:
		return 25.0 // 19%-31%
	case 4:
		return 38.0 // 31%-44%
	case 5:
		return 50.0 // 44%-56%
	case 6:
		return 63.0 // 56%-69%
	case 7:
		return 75.0 // 69%-81%
	case 8:
		return 88.0 // 81%-94%
	case 9:
		return 97.0 // 94%-100%
	default:
		return 20.0
	}
}
