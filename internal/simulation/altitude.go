package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

// Altitude.
//
// Thin air helps and hurts: lower air density cuts aerodynamic drag, but less
// oxygen lowers sustainable power. On high mountain passes the power loss
// wins, which is part of why the highest climbs are the most selective.

// AirDensityAt is the standard-atmosphere air density (kg/m³) at an altitude.
func AirDensityAt(elevationM float64) float64 {
	if elevationM <= 0 {
		return airDensity
	}
	return airDensity * math.Pow(1-2.25577e-5*elevationM, 4.2559)
}

// AltitudePowerFactor is the fraction of sea-level aerobic power available at
// an altitude, from Bassett et al. (1999) for altitude-acclimatised athletes:
// %P = 99.921 − 1.8991·h − 1.122·h² (h in km). ~95% at 1500 m, ~92% at 2000 m,
// ~87% at 2600 m.
func AltitudePowerFactor(elevationM float64) float64 {
	if elevationM <= 0 {
		return 1
	}
	h := elevationM / 1000
	f := (99.921 - 1.8991*h - 1.122*h*h) / 99.921
	if f > 1 {
		return 1
	}
	if f < 0.5 {
		return 0.5
	}
	return f
}

// PrepareSegments returns a race-ready copy of a course's segments: wind
// components, altitude (elevation profile from the gradients, the course's
// lowest point taken as startElevationM) and cached climb factors.
func PrepareSegments(segments []model.Segment, race *model.Race, startElevationM float64) []model.Segment {
	out := append([]model.Segment(nil), segments...)
	if race != nil {
		out = WithWind(out, race.WindSpeedKmh, race.WindDirectionDeg)
	}
	withElevation(out, startElevationM)
	for i := range out {
		refreshClimb(&out[i])
	}
	return out
}

// withElevation integrates the gradient profile into midpoint altitudes. Real
// start altitudes aren't in the data, so the lowest point of the course is
// set to base metres: absolute altitude is approximate, relative is exact.
func withElevation(segs []model.Segment, base float64) {
	alt, low := 0.0, 0.0
	mid := make([]float64, len(segs))
	for i, s := range segs {
		rise := s.DistanceKm * 1000 * s.GradientPct / 100
		mid[i] = alt + rise/2
		alt += rise
		low = math.Min(low, alt)
	}
	for i := range segs {
		segs[i].ElevationM = mid[i] - low + base
		segs[i].AirDensity = AirDensityAt(segs[i].ElevationM)
	}
}
