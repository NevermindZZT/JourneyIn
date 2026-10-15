package monitor

import (
	"fmt"
	"time"
)

type CandidateWindow struct {
	StartDate    time.Time `json:"start_date"`
	EndDate      time.Time `json:"end_date"`
	DurationDays int       `json:"duration_days"`
	OffDays      int       `json:"off_days"`
	LeaveDays    int       `json:"leave_days"`
	HolidayName  string    `json:"holiday_name,omitempty"`
	Summary      string    `json:"summary"`
}

// Built-in China statutory holidays and compensation workdays (2025-2027)
var defaultHolidays = map[string]HolidayRecord{
	// 2026 元旦
	"2026-01-01": {CalendarDate: "2026-01-01", HolidayName: "元旦", IsHoliday: true, Year: 2026},
	"2026-01-02": {CalendarDate: "2026-01-02", HolidayName: "元旦", IsHoliday: true, Year: 2026},
	"2026-01-03": {CalendarDate: "2026-01-03", HolidayName: "元旦", IsHoliday: true, Year: 2026},
	// 2026 春节
	"2026-02-07": {CalendarDate: "2026-02-07", HolidayName: "春节调休", IsWorkday: true, Year: 2026},
	"2026-02-15": {CalendarDate: "2026-02-15", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-16": {CalendarDate: "2026-02-16", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-17": {CalendarDate: "2026-02-17", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-18": {CalendarDate: "2026-02-18", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-19": {CalendarDate: "2026-02-19", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-20": {CalendarDate: "2026-02-20", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-21": {CalendarDate: "2026-02-21", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-22": {CalendarDate: "2026-02-22", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-23": {CalendarDate: "2026-02-23", HolidayName: "春节", IsHoliday: true, Year: 2026},
	"2026-02-28": {CalendarDate: "2026-02-28", HolidayName: "春节调休", IsWorkday: true, Year: 2026},
	// 2026 清明节
	"2026-04-04": {CalendarDate: "2026-04-04", HolidayName: "清明节", IsHoliday: true, Year: 2026},
	"2026-04-05": {CalendarDate: "2026-04-05", HolidayName: "清明节", IsHoliday: true, Year: 2026},
	"2026-04-06": {CalendarDate: "2026-04-06", HolidayName: "清明节", IsHoliday: true, Year: 2026},
	// 2026 劳动节
	"2026-04-26": {CalendarDate: "2026-04-26", HolidayName: "劳动节调休", IsWorkday: true, Year: 2026},
	"2026-05-01": {CalendarDate: "2026-05-01", HolidayName: "劳动节", IsHoliday: true, Year: 2026},
	"2026-05-02": {CalendarDate: "2026-05-02", HolidayName: "劳动节", IsHoliday: true, Year: 2026},
	"2026-05-03": {CalendarDate: "2026-05-03", HolidayName: "劳动节", IsHoliday: true, Year: 2026},
	"2026-05-04": {CalendarDate: "2026-05-04", HolidayName: "劳动节", IsHoliday: true, Year: 2026},
	"2026-05-05": {CalendarDate: "2026-05-05", HolidayName: "劳动节", IsHoliday: true, Year: 2026},
	"2026-05-09": {CalendarDate: "2026-05-09", HolidayName: "劳动节调休", IsWorkday: true, Year: 2026},
	// 2026 端午节
	"2026-06-19": {CalendarDate: "2026-06-19", HolidayName: "端午节", IsHoliday: true, Year: 2026},
	"2026-06-20": {CalendarDate: "2026-06-20", HolidayName: "端午节", IsHoliday: true, Year: 2026},
	"2026-06-21": {CalendarDate: "2026-06-21", HolidayName: "端午节", IsHoliday: true, Year: 2026},
	// 2026 中秋节
	"2026-09-25": {CalendarDate: "2026-09-25", HolidayName: "中秋节", IsHoliday: true, Year: 2026},
	"2026-09-26": {CalendarDate: "2026-09-26", HolidayName: "中秋节", IsHoliday: true, Year: 2026},
	"2026-09-27": {CalendarDate: "2026-09-27", HolidayName: "中秋节", IsHoliday: true, Year: 2026},
	// 2026 国庆节
	"2026-10-01": {CalendarDate: "2026-10-01", HolidayName: "国庆节", IsHoliday: true, Year: 2026},
	"2026-10-02": {CalendarDate: "2026-10-02", HolidayName: "国庆节", IsHoliday: true, Year: 2026},
	"2026-10-03": {CalendarDate: "2026-10-03", HolidayName: "国庆节", IsHoliday: true, Year: 2026},
	"2026-10-04": {CalendarDate: "2026-10-04", HolidayName: "国庆节", IsHoliday: true, Year: 2026},
	"2026-10-05": {CalendarDate: "2026-10-05", HolidayName: "国庆节", IsHoliday: true, Year: 2026},
	"2026-10-06": {CalendarDate: "2026-10-06", HolidayName: "国庆节", IsHoliday: true, Year: 2026},
	"2026-10-07": {CalendarDate: "2026-10-07", HolidayName: "国庆节", IsHoliday: true, Year: 2026},
	"2026-10-10": {CalendarDate: "2026-10-10", HolidayName: "国庆节调休", IsWorkday: true, Year: 2026},
}

// IsDayOff checks whether a given date is a non-working day (weekend or public holiday,
// excluding compensation workdays).
func IsDayOff(t time.Time, customHolidays map[string]HolidayRecord) (isOff bool, name string) {
	dateStr := t.Format("2006-01-02")
	// 1. Check custom or built-in holiday table
	rec, ok := customHolidays[dateStr]
	if !ok {
		rec, ok = defaultHolidays[dateStr]
	}
	if ok {
		if rec.IsWorkday {
			return false, ""
		}
		if rec.IsHoliday {
			return true, rec.HolidayName
		}
	}

	// 2. Default weekend check
	weekday := t.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return true, "周末"
	}
	return false, ""
}

// FindCandidateWindows scans a date range and identifies contiguous blocks of days
// that fit the required duration and leave constraints (e.g. 2-day weekends, 3-day holidays,
// or smart leave-bridging windows like "take 1 day off for 4 days travel").
func FindCandidateWindows(
	startDate, endDate time.Time,
	maxLeaveDays, minDuration, maxDuration int,
	customHolidays map[string]HolidayRecord,
) []CandidateWindow {
	if minDuration <= 0 {
		minDuration = 2
	}
	if maxDuration < minDuration {
		maxDuration = minDuration
	}

	// Scan dates day by day
	var windows []CandidateWindow
	seen := make(map[string]bool)

	// Clamp start and end to midnight
	cur := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())
	end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, endDate.Location())

	for d := cur; !d.After(end); d = d.AddDate(0, 0, 1) {
		for dur := minDuration; dur <= maxDuration; dur++ {
			winEnd := d.AddDate(0, 0, dur-1)
			if winEnd.After(end) {
				continue
			}

			// Count off days and required leave days
			offDays := 0
			leaveDays := 0
			holidayName := ""

			for step := 0; step < dur; step++ {
				dayDate := d.AddDate(0, 0, step)
				isOff, name := IsDayOff(dayDate, customHolidays)
				if isOff {
					offDays++
					if name != "周末" && holidayName == "" {
						holidayName = name
					}
				} else {
					leaveDays++
				}
			}

			// A valid travel window must not exceed maxLeaveDays,
			// and must contain at least 1 natural off-day (weekend or holiday)
			if leaveDays <= maxLeaveDays && offDays >= 1 {
				key := fmt.Sprintf("%s_%s", d.Format("2006-01-02"), winEnd.Format("2006-01-02"))
				if seen[key] {
					continue
				}
				seen[key] = true

				var summary string
				if holidayName != "" {
					if leaveDays == 0 {
						summary = fmt.Sprintf("%s连休 %d 天 (免请假)", holidayName, dur)
					} else {
						summary = fmt.Sprintf("%s拼假 %d 天 (请 %d 休 %d)", holidayName, dur, leaveDays, dur)
					}
				} else {
					if leaveDays == 0 {
						summary = fmt.Sprintf("周末连休 %d 天 (免请假)", dur)
					} else {
						summary = fmt.Sprintf("周末拼假 %d 天 (请 %d 休 %d)", dur, leaveDays, dur)
					}
				}

				windows = append(windows, CandidateWindow{
					StartDate:    d,
					EndDate:      winEnd,
					DurationDays: dur,
					OffDays:      offDays,
					LeaveDays:    leaveDays,
					HolidayName:  holidayName,
					Summary:      summary,
				})
			}
		}
	}
	return windows
}
