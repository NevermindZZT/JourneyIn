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

		// 5. Weather evaluation (strictly based on confirmed real weather forecast <= 15 days)
		hasRealWeather := false
		var weatherScore *float64
		var cloudAvg *float64
		var rainAvg *float64
		var tempMin *float64
		var tempMax *float64
		var visKm *float64
		var aodVal *float64
		var seeingVal *float64
		var transparencyVal *float64
		weatherSummary := "超出预报范围 (待临近观测)"

		daysFromNow := int(win.StartDate.Sub(now).Hours() / 24)
		if daysFromNow >= 0 && daysFromNow <= 15 && e.weatherSvc != nil {
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
				hasRealWeather = true
				if snap.TempMinC != nil {
					tMin := *snap.TempMinC
					tempMin = &tMin
				}
				if snap.TempMaxC != nil {
					tMax := *snap.TempMaxC
					tempMax = &tMax
				}

				// 优先消费真实云量 (CloudCoverPct)
				if snap.CloudCoverPct != nil {
					c := float64(*snap.CloudCoverPct)
					cloudAvg = &c
				}

				// 优先消费真实降水概率 (RainProbPct)
				if snap.RainProbPct != nil {
					r := float64(*snap.RainProbPct)
					rainAvg = &r
				}

				// 高阶气象与大气通透度指标
				if snap.VisibilityKm != nil {
					v := *snap.VisibilityKm
					visKm = &v
				}
				if snap.AOD != nil {
					a := *snap.AOD
					aodVal = &a
				}
				if snap.Seeing != nil {
					s := *snap.Seeing
					seeingVal = &s
				}
				if snap.Transparency != nil {
					tr := *snap.Transparency
					transparencyVal = &tr
				}

				// 若开启星空摄影模式且在 3 天高精预报期内，主动融合 7Timer 专业天文视宁度与透明度
				if m.Criteria.StargazingMode && daysFromNow <= 3 && (seeingVal == nil || transparencyVal == nil) {
					if sevenSnap, err := e.weatherSvc.Weather(ctx, weather.Provider7Timer, wReq); err == nil && sevenSnap.Available {
						if sevenSnap.Seeing != nil && seeingVal == nil {
							s := *sevenSnap.Seeing
							seeingVal = &s
						}
						if sevenSnap.Transparency != nil && transparencyVal == nil {
							tr := *sevenSnap.Transparency
							transparencyVal = &tr
						}
					}
				}

				cond := strings.ToLower(snap.Condition)
				isRainSnow := strings.Contains(cond, "雨") || strings.Contains(cond, "rain") || strings.Contains(cond, "雪") || strings.Contains(cond, "snow") || strings.Contains(cond, "雷") || strings.Contains(cond, "storm")
				isCloudy := strings.Contains(cond, "多云") || strings.Contains(cond, "cloud") || strings.Contains(cond, "阴")
				isSunny := strings.Contains(cond, "晴") || strings.Contains(cond, "clear") || strings.Contains(cond, "sunny")

				// 若无真实数值则按天气现象兜底推算
				if cloudAvg == nil {
					if isSunny {
						c := 10.0
						cloudAvg = &c
					} else if isCloudy {
						c := 40.0
						cloudAvg = &c
					} else if isRainSnow {
						c := 90.0
						cloudAvg = &c
					} else {
						c := 25.0
						cloudAvg = &c
					}
				}
				if rainAvg == nil {
					if isSunny {
						r := 5.0
						rainAvg = &r
					} else if isCloudy {
						r := 15.0
						rainAvg = &r
					} else if isRainSnow {
						r := 75.0
						rainAvg = &r
					} else {
						r := 10.0
						rainAvg = &r
					}
				}

				// 科学定量打分: 云量与降水概率线性扣分
				cVal := *cloudAvg
				rVal := *rainAvg
				ws := 100.0 - (cVal * 0.45) - (rVal * 0.45)

				// 降水惩罚
				if isRainSnow || rVal >= 50.0 {
					ws = math.Min(ws, 25.0)
				}

				// 能见度与大气通透度奖惩
				if visKm != nil {
					if *visKm >= 15.0 {
						ws = math.Min(100.0, ws+5.0) // 通透天奖励
					} else if *visKm < 5.0 {
						ws = math.Max(10.0, ws-20.0) // 雾霾/低能见扣分
					}
				}
				if aodVal != nil && *aodVal > 0.5 {
					ws = math.Max(10.0, ws-15.0) // 重度气溶胶厚度扣分
				}

				ws = math.Round(math.Max(5.0, math.Min(100.0, ws))*10) / 10
				weatherScore = &ws

				// 组织人类可读摘要
				var conditionCategory string
				if isSunny || cVal <= 25.0 {
					conditionCategory = "晴空少云"
				} else if isRainSnow || rVal >= 50.0 {
					conditionCategory = "有降水"
				} else {
					conditionCategory = "多云适宜"
				}
				detailDesc := snap.Condition
				if visKm != nil && *visKm > 0 {
					detailDesc = fmt.Sprintf("%s, 能见度%.0fkm", detailDesc, *visKm)
				}
				weatherSummary = fmt.Sprintf("%s (%s)", conditionCategory, detailDesc)
			} else {
				weatherSummary = "气象暂不可用 (待临近观测)"
			}
		}

		budgetScore := 80.0

		// 6. Multi-factor weighted composite scoring
		var finalScore float64
		if hasRealWeather && weatherScore != nil {
			if m.Criteria.StargazingMode {
				finalScore = 0.40*astroScore + 0.35*(*weatherScore) + 0.25*calendarScore
			} else {
				finalScore = 0.40*calendarScore + 0.35*(*weatherScore) + 0.15*astroScore + 0.10*budgetScore
			}
		} else {
			// No real weather data: compute preliminary score based on known calendar/astronomy/budget
			// without fabricating weather. Capped at 55 so it remains strictly within LevelWaiting (< 60).
			if m.Criteria.StargazingMode {
				finalScore = 0.50*astroScore + 0.50*calendarScore
			} else {
				finalScore = 0.50*calendarScore + 0.30*astroScore + 0.20*budgetScore
			}
			if finalScore > 55 {
				finalScore = 55.0
			}
		}
		finalScore = math.Round(math.Max(0, math.Min(100, finalScore))*10) / 10

		// 7. Level determination
		// As per user specification: without reliable real weather, all candidate windows are strictly LevelWaiting ("待观察").
		var level EvaluationLevel
		if !hasRealWeather {
			level = LevelWaiting
		} else {
			// Adverse weather (rain/snow/severe, weatherScore <= 35) or exceeding user limits enforces LevelWaiting
			isAdverseWeather := weatherScore != nil && *weatherScore <= 35
			isStargazingUnfit := m.Criteria.StargazingMode && (
				(visKm != nil && *visKm < 8.0) ||
					(aodVal != nil && *aodVal > 0.55) ||
					(transparencyVal != nil && *transparencyVal >= 5.0))
			if isAdverseWeather || isStargazingUnfit ||
				(cloudAvg != nil && m.Criteria.MaxCloudCover > 0 && int(*cloudAvg) > m.Criteria.MaxCloudCover) ||
				(rainAvg != nil && m.Criteria.MaxRainProb > 0 && int(*rainAvg) > m.Criteria.MaxRainProb) {
				level = LevelWaiting
			} else {
				switch {
				case finalScore >= 80:
					level = LevelStronglyRecommended
				case finalScore >= 60:
					level = LevelAcceptable
				default:
					level = LevelWaiting
				}
			}
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
			if transparencyVal != nil && *transparencyVal <= 2.0 {
				summaryParts = append(summaryParts, "水晶级通透")
			} else if visKm != nil && *visKm >= 15.0 {
				summaryParts = append(summaryParts, "远眺极通透")
			}
			if aodVal != nil && *aodVal > 0.55 {
				summaryParts = append(summaryParts, "气溶胶偏重")
			} else if visKm != nil && *visKm < 8.0 {
				summaryParts = append(summaryParts, "低空视线受阻")
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
				TempMinC:          tempMin,
				TempMaxC:          tempMax,
				CloudCoverAvg:     cloudAvg,
				RainProbAvg:       rainAvg,
				VisibilityKm:      visKm,
				AOD:               aodVal,
				Seeing:            seeingVal,
				Transparency:      transparencyVal,
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

	// Sort evaluations: strongly recommended first, then acceptable, then highest score, then earliest date
	sort.Slice(evaluations, func(i, j int) bool {
		if (evaluations[i].Level == LevelStronglyRecommended) != (evaluations[j].Level == LevelStronglyRecommended) {
			return evaluations[i].Level == LevelStronglyRecommended
		}
		if (evaluations[i].Level == LevelAcceptable) != (evaluations[j].Level == LevelAcceptable) {
			return evaluations[i].Level == LevelAcceptable
		}
		if evaluations[i].Score != evaluations[j].Score {
			return evaluations[i].Score > evaluations[j].Score
		}
		return evaluations[i].WindowStart < evaluations[j].WindowStart
	})

	return evaluations, nil
}
