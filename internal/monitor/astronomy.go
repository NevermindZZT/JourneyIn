package monitor

import (
	"math"
	"time"
)

// SynodicMonth is the average length of a lunar synodic cycle in days.
const SynodicMonth = 29.53058867

// ReferenceNewMoonJD is the Julian Date of a known new moon (2000-01-06 18:14 UTC).
const ReferenceNewMoonJD = 2451549.26

// MoonPhase calculates the moon age in days, illumination fraction [0.0, 1.0], and descriptive name.
func MoonPhase(t time.Time) (age float64, illumination float64, phaseName string) {
	utc := t.UTC()
	// Julian Day calculation
	unixSec := float64(utc.Unix())
	jd := (unixSec / 86400.0) + 2440587.5

	daysSinceNew := math.Mod(jd-ReferenceNewMoonJD, SynodicMonth)
	if daysSinceNew < 0 {
		daysSinceNew += SynodicMonth
	}
	age = daysSinceNew

	// Phase angle (0 = New Moon, pi = Full Moon, 2pi = next New Moon)
	phaseAngle := (age / SynodicMonth) * 2 * math.Pi
	illumination = (1.0 - math.Cos(phaseAngle)) / 2.0

	// Descriptive classification
	switch {
	case illumination < 0.15:
		phaseName = "新月/朔月 (暗夜极佳)"
	case illumination < 0.35:
		phaseName = "蛾眉月/残月 (较暗少光害)"
	case illumination < 0.65:
		phaseName = "弦月 (半暗半明)"
	case illumination < 0.85:
		phaseName = "凸月 (月光明显)"
	default:
		phaseName = "满月/望月 (月光强/光害显著)"
	}
	return age, illumination, phaseName
}

// GalacticCoreHours estimates the number of hours the galactic core (Sagittarius region)
// is visible above 15 degrees altitude during full astronomical dark night (Sun altitude <= -18 deg).
func GalacticCoreHours(t time.Time, lat float64) (hours float64, isOptimal bool) {
	month := int(t.UTC().Month())
	// In northern hemisphere mid-latitudes, the galactic center (RA 17h45m, Dec -29deg)
	// has strong seasonal visibility.
	if lat < -50 || lat > 55 {
		// Too far north/south for optimal Sagittarius visibility
		return 0, false
	}

	// Seasonal baseline for dark night visibility hours:
	// Nov - Jan: core is in daytime sky with the Sun (0 hours)
	// Feb: 0.5 - 1.5h before dawn
	// Mar - Apr: 2.0 - 4.0h pre-dawn
	// May - Jul: 4.5 - 6.5h throughout the night (Summer peak)
	// Aug - Sep: 3.5 - 5.0h in early/mid evening
	// Oct: 1.5 - 2.5h after dusk
	var baseHours float64
	switch month {
	case 11, 12, 1:
		baseHours = 0
	case 2:
		baseHours = 1.0
	case 3:
		baseHours = 2.5
	case 4:
		baseHours = 3.8
	case 5:
		baseHours = 5.0
	case 6:
		baseHours = 6.0
	case 7:
		baseHours = 5.8
	case 8:
		baseHours = 4.8
	case 9:
		baseHours = 3.5
	case 10:
		baseHours = 2.0
	}

	// Latitude adjustment: higher latitudes see Sagittarius lower in the southern sky
	if lat > 38 {
		baseHours = math.Max(0, baseHours-(lat-38)*0.1)
	}

	_, illum, _ := MoonPhase(t)
	// Optimal stargazing requires at least 2.5 hours of core visibility AND low moon illumination (< 0.20)
	isOptimal = baseHours >= 2.5 && illum < 0.20
	return baseHours, isOptimal
}
