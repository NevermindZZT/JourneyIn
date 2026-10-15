package monitor

import (
	"testing"
	"time"
)

func TestMoonPhase(t *testing.T) {
	// 2026-06-15 is near astronomical new moon
	newMoonTime := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	age, illum, name := MoonPhase(newMoonTime)

	if illum > 0.15 {
		t.Fatalf("expected low illumination near new moon, got %.2f", illum)
	}
	if name == "" {
		t.Fatalf("expected non-empty phase name")
	}
	t.Logf("2026-06-15: age=%.1f days, illum=%.2f, name=%s", age, illum, name)
}

func TestGalacticCoreHours(t *testing.T) {
	// June at 30 deg North: peak galactic core visibility
	juneTime := time.Date(2026, 6, 20, 0, 0, 0, 0, time.UTC)
	hours, isOpt := GalacticCoreHours(juneTime, 30.5)

	if hours < 4.0 {
		t.Fatalf("expected >= 4.0 galactic core hours in June, got %.1f", hours)
	}
	t.Logf("June galactic core hours: %.1f, isOptimal=%v", hours, isOpt)

	// December: daytime, 0 hours
	decTime := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	decHours, _ := GalacticCoreHours(decTime, 30.5)
	if decHours > 0.5 {
		t.Fatalf("expected ~0 galactic core hours in December, got %.1f", decHours)
	}
}
