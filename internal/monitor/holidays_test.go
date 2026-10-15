package monitor

import (
	"testing"
	"time"
)

func TestIsDayOff(t *testing.T) {
	// Normal weekend
	sat := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC) // Saturday
	isOff, name := IsDayOff(sat, nil)
	if !isOff || name != "周末" {
		t.Fatalf("expected Saturday to be day off, got isOff=%v, name=%s", isOff, name)
	}

	// 端午节 (2026-06-19)
	duanwu := time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC)
	isOffDW, nameDW := IsDayOff(duanwu, nil)
	if !isOffDW || nameDW != "端午节" {
		t.Fatalf("expected 2026-06-19 to be 端午节, got isOff=%v, name=%s", isOffDW, nameDW)
	}

	// 调休补班 (2026-02-07 春节调休补班)
	workday := time.Date(2026, 2, 7, 0, 0, 0, 0, time.UTC)
	isOffWork, _ := IsDayOff(workday, nil)
	if isOffWork {
		t.Fatalf("expected 2026-02-07 compensation Saturday to be a workday")
	}
}

func TestFindCandidateWindows(t *testing.T) {
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	windows := FindCandidateWindows(start, end, 1, 2, 4, nil)
	if len(windows) == 0 {
		t.Fatalf("expected candidate travel windows in June, got none")
	}

	foundDuanwu := false
	for _, w := range windows {
		if w.HolidayName == "端午节" {
			foundDuanwu = true
			t.Logf("Found Duanwu window: %s ~ %s (%s, dur=%d, leave=%d)",
				w.StartDate.Format("2006-01-02"), w.EndDate.Format("2006-01-02"), w.Summary, w.DurationDays, w.LeaveDays)
			break
		}
	}
	if !foundDuanwu {
		t.Fatalf("expected to find 端午节 travel window in June 2026")
	}
}
