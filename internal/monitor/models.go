package monitor

import (
	"encoding/json"
	"time"
)

type EvaluationLevel string

const (
	LevelStronglyRecommended EvaluationLevel = "strongly_recommended"
	LevelAcceptable          EvaluationLevel = "acceptable"
	LevelWaiting             EvaluationLevel = "waiting"
)

type MonitorCriteria struct {
	Months          []int `json:"months,omitempty"`            // 适游月份 1-12
	StargazingMode  bool  `json:"stargazing_mode,omitempty"`  // 是否开启星空/银河摄影模式
	MaxLeaveDays    int   `json:"max_leave_days,omitempty"`    // 可接受请假天数 (0=纯周末/节假日)
	MaxCloudCover   int   `json:"max_cloud_cover,omitempty"`   // 最高允许云量百分比 (0-100, 默认30)
	MaxRainProb     int   `json:"max_rain_prob,omitempty"`     // 最高允许降水概率百分比 (0-100, 默认20)
	MinDurationDays int   `json:"min_duration_days,omitempty"` // 期望最短行程天数 (默认2)
	MaxDurationDays int   `json:"max_duration_days,omitempty"` // 期望最长行程天数 (默认4)
}

type FactorScores struct {
	AstroScore    float64  `json:"astro_score"`
	WeatherScore  *float64 `json:"weather_score,omitempty"`
	CalendarScore float64  `json:"calendar_score"`
	BudgetScore   float64  `json:"budget_score"`
}

type EvaluationDetails struct {
	Summary           string       `json:"summary"`
	MoonPhaseName     string       `json:"moon_phase_name"`
	MoonIllumination  float64      `json:"moon_illumination"` // 0.0 - 1.0 (照亮比例)
	GalacticHours     float64      `json:"galactic_hours"`    // 银河核心纯黑夜可见时长(小时)
	HolidayName       string       `json:"holiday_name,omitempty"`
	LeaveDaysRequired int          `json:"leave_days_required"`
	WeatherCondition  string       `json:"weather_condition,omitempty"`
	TempMinC          *float64     `json:"temp_min_c,omitempty"`
	TempMaxC          *float64     `json:"temp_max_c,omitempty"`
	CloudCoverAvg     *float64     `json:"cloud_cover_avg,omitempty"`
	RainProbAvg       *float64     `json:"rain_prob_avg,omitempty"`
	Scores            FactorScores `json:"scores"`
}
type Monitor struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	PlaceName  string          `json:"place_name"`
	Lat        float64         `json:"lat"`
	Lng        float64         `json:"lng"`
	CRS        string          `json:"crs"`
	Criteria   MonitorCriteria `json:"criteria"`
	OriginCity string          `json:"origin_city,omitempty"`
	MaxBudget  float64         `json:"max_budget,omitempty"`
	Enabled    bool            `json:"enabled"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`

	LatestEvaluation *Evaluation `json:"latest_evaluation,omitempty"`
}

type Evaluation struct {
	ID          string            `json:"id"`
	MonitorID   string            `json:"monitor_id"`
	WindowStart string            `json:"window_start"` // YYYY-MM-DD
	WindowEnd   string            `json:"window_end"`   // YYYY-MM-DD
	Score       float64           `json:"score"`        // 0 - 100
	Level       EvaluationLevel   `json:"level"`
	Details     EvaluationDetails `json:"details"`
	EvaluatedAt time.Time         `json:"evaluated_at"`
}

type RadarSummary struct {
	TotalMonitors    int         `json:"total_monitors"`
	RecommendedCount int         `json:"recommended_count"`
	TopOpportunity   *Evaluation `json:"top_opportunity,omitempty"`
	TopPlaceName     string      `json:"top_place_name,omitempty"`
}

type HolidayRecord struct {
	CalendarDate string `json:"calendar_date"` // YYYY-MM-DD
	HolidayName  string `json:"holiday_name"`
	IsHoliday    bool   `json:"is_holiday"`
	IsWorkday    bool   `json:"is_workday"` // 调休补班
	Year         int    `json:"year"`
}

func (c MonitorCriteria) ToJSON() (string, error) {
	bytes, err := json.Marshal(c)
	return string(bytes), err
}

func CriteriaFromJSON(raw string) (MonitorCriteria, error) {
	var c MonitorCriteria
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return c, err
	}
	if c.MaxCloudCover <= 0 {
		c.MaxCloudCover = 35
	}
	if c.MaxRainProb <= 0 {
		c.MaxRainProb = 25
	}
	if c.MinDurationDays <= 0 {
		c.MinDurationDays = 2
	}
	if c.MaxDurationDays <= 0 {
		c.MaxDurationDays = 4
	}
	return c, nil
}
