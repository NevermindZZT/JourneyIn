package monitor

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"journeyin/internal/weather"
)

type Evaluator struct {
	weatherSvc *weather.Service
}

func NewEvaluator(weatherSvc *weather.Service) *Evaluator {
	return &Evaluator{weatherSvc: weatherSvc}
}

// EvaluateMonitor evaluates a single monitor against future candidate windows up to horizonDays.
func (e *Evaluator) EvaluateMonitor(
	ctx context.Context,
	m Monitor,
	customHolidays map[string]HolidayRecord,
	now time.Time,
	horizonDays int,
) ([]Evaluation, error) {
	if horizonDays <= 0 {
		horizonDays = 45
	}
	startDate := now.AddDate(0, 0, 1) // Start from tomorrow
	endDate := now.AddDate(0, 0, horizonDays)

	// 1. Find candidate calendar travel windows
	windows := FindCandidateWindows(
		startDate, endDate,
		m.Criteria.MaxLeaveDays,
		m.Criteria.MinDurationDays,
		m.Criteria.MaxDurationDays,
		customHolidays,
	)

	var evaluations []Evaluation

	for _, win := range windows {
		// 2. Month filtering: check if all dates fall into criteria.Months
		if len(m.Criteria.Months) > 0 {
			monthValid := true
			for step := 0; step < win.DurationDays; step++ {
				d := win.StartDate.AddDate(0, 0, step)
				mInt := int(d.Month())
				hasMonth := false
				for _, allowed := range m.Criteria.Months {
					if allowed == mInt {
						hasMonth = true
						break
					}
				}
				if !hasMonth {
					monthValid = false
					break
				}
			}
			if !monthValid {
				continue
			}
		}

		// 3. Astronomical evaluation (Moon Phase & Galactic Core)
		var totalIllum float64
		var totalGalacticHours float64
		bestPhaseName := ""

		for step := 0; step < win.DurationDays; step++ {
			d := win.StartDate.AddDate(0, 0, step)
			_, illum, phaseName := MoonPhase(d)
			coreHours, _ := GalacticCoreHours(d, m.Lat)
			totalIllum += illum
			totalGalacticHours += coreHours
			if bestPhaseName == "" || illum < totalIllum/float64(step+1) {
				bestPhaseName = phaseName
			}
		}

		avgIllum := totalIllum / float64(win.DurationDays)
		avgGalacticHours := totalGalacticHours / float64(win.DurationDays)

		var astroScore float64
		if m.Criteria.StargazingMode {
			switch {
			case avgIllum < 0.15:
				astroScore = 100
			case avgIllum < 0.35:
				astroScore = 80
			case avgIllum < 0.60:
				astroScore = 50
			case avgIllum < 0.80:
				astroScore = 25
			default:
				astroScore = 10 // Full moon: heavy light pollution
			}
			if avgGalacticHours >= 4.0 {
				astroScore = math.Min(100, astroScore+10)
			}
		} else {
			astroScore = 80 // Neutral favorable
		}

		// 4. Calendar & Work evaluation
		var calendarScore float64
		switch win.LeaveDays {
		case 0:
			calendarScore = 100 // Zero leave required
		case 1:
			calendarScore = 85
		case 2:
			calendarScore = 70
		default:
			calendarScore = 55
		}
		if win.HolidayName != "" {
			calendarScore = math.Min(100, calendarScore+5)
		}

		// 5. Weather evaluation (within 15-day forecast horizon)
		var weatherScore float64 = 75 // Default neutral for long-range
		weatherSummary := "远期窗口 (按季节气候推算)"
		var cloudAvg float64 = 25
		var rainAvg float64 = 10

		daysFromNow := int(win.StartDate.Sub(now).Hours() / 24)
		if daysFromNow <= 15 && e.weatherSvc != nil {
			// Query weather for the first night/day of window
			wReq := weather.WeatherRequest{
				Location: weather.GeoPoint{
					Lat: m.Lat,
					Lng: m.Lng,
					CRS: weather.CRS(m.CRS),
				},
				LocalDate: win.StartDate.Format("2006-01-02"),
				Timezone:  "Asia/Shanghai",
			}
			snap, err := e.weatherSvc.Weather(ctx, weather.ProviderAuto, wReq)
			if err == nil && snap.Available {
				cond := strings.ToLower(snap.Condition)
				if strings.Contains(cond, "晴") || strings.Contains(cond, "clear") || strings.Contains(cond, "sunny") {
					weatherScore = 95
					cloudAvg = 10
					rainAvg = 5
					weatherSummary = fmt.Sprintf("晴空少云 (%s)", snap.Condition)
				} else if strings.Contains(cond, "多云") || strings.Contains(cond, "cloud") {
					weatherScore = 75
					cloudAvg = 40
					rainAvg = 15
					weatherSummary = fmt.Sprintf("多云适宜 (%s)", snap.Condition)
				} else if strings.Contains(cond, "雨") || strings.Contains(cond, "rain") || strings.Contains(cond, "雪") || strings.Contains(cond, "snow") {
					weatherScore = 20
					cloudAvg = 90
					rainAvg = 75
					weatherSummary = fmt.Sprintf("有降水 (%s)", snap.Condition)
				} else {
					weatherScore = 70
					weatherSummary = snap.Condition
				}
			}
		}

		budgetScore := 80.0

		// 6. Multi-factor weighted composite scoring
		var finalScore float64
		if m.Criteria.StargazingMode {
			finalScore = 0.40*astroScore + 0.35*weatherScore + 0.25*calendarScore
		} else {
			finalScore = 0.40*calendarScore + 0.35*weatherScore + 0.15*astroScore + 0.10*budgetScore
		}
		finalScore = math.Round(math.Max(0, math.Min(100, finalScore))*10) / 10

		// 7. Level determination
		var level EvaluationLevel
		switch {
		case finalScore >= 80:
			level = LevelStronglyRecommended
		case finalScore >= 60:
			level = LevelAcceptable
		default:
			level = LevelWaiting
		}

		// 8. Build human-readable summary
		var summaryParts []string
		if win.HolidayName != "" {
			summaryParts = append(summaryParts, win.HolidayName)
		} else if win.LeaveDays == 0 {
			summaryParts = append(summaryParts, "周末连休")
		} else {
			summaryParts = append(summaryParts, fmt.Sprintf("请%d休%d", win.LeaveDays, win.DurationDays))
		}

		if m.Criteria.StargazingMode {
			if avgIllum < 0.15 {
				summaryParts = append(summaryParts, "新月纯黑夜")
			} else if avgIllum < 0.35 {
				summaryParts = append(summaryParts, "暗夜月相优")
			}
			if avgGalacticHours >= 3.5 {
				summaryParts = append(summaryParts, fmt.Sprintf("银心可见%.1fh", avgGalacticHours))
			}
		}

		summaryParts = append(summaryParts, weatherSummary)

		eval := Evaluation{
			ID:          fmt.Sprintf("eval_%s_%s_%s", m.ID, win.StartDate.Format("20060102"), win.EndDate.Format("20060102")),
			MonitorID:   m.ID,
			WindowStart: win.StartDate.Format("2006-01-02"),
			WindowEnd:   win.EndDate.Format("2006-01-02"),
			Score:       finalScore,
			Level:       level,
			Details: EvaluationDetails{
				Summary:           strings.Join(summaryParts, " · "),
				MoonPhaseName:     bestPhaseName,
				MoonIllumination:  math.Round(avgIllum*100) / 100,
				GalacticHours:     math.Round(avgGalacticHours*10) / 10,
				HolidayName:       win.HolidayName,
				LeaveDaysRequired: win.LeaveDays,
				WeatherCondition:  weatherSummary,
				CloudCoverAvg:     cloudAvg,
				RainProbAvg:       rainAvg,
				Scores: FactorScores{
					AstroScore:    astroScore,
					WeatherScore:  weatherScore,
					CalendarScore: calendarScore,
					BudgetScore:   budgetScore,
				},
			},
			EvaluatedAt: now,
		}
		evaluations = append(evaluations, eval)
	}

	// Sort evaluations: strongly recommended first, then highest score, then earliest date
	sort.Slice(evaluations, func(i, j int) bool {
		if (evaluations[i].Level == LevelStronglyRecommended) != (evaluations[j].Level == LevelStronglyRecommended) {
			return evaluations[i].Level == LevelStronglyRecommended
		}
		if evaluations[i].Score != evaluations[j].Score {
			return evaluations[i].Score > evaluations[j].Score
		}
		return evaluations[i].WindowStart < evaluations[j].WindowStart
	})

	return evaluations, nil
}
