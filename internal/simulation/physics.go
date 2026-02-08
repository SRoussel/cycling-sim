package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

const (
	gravity     = 9.8067 // m/s²
	airDensity  = 1.225  // kg/m³ at sea level, 15°C
	crr         = 0.004  // rolling resistance coefficient (road)
	crrCobbles  = 0.008  // rolling resistance coefficient (cobbles)
	bikeWeight  = 8.0    // kg
	minSpeedMPS = 1.0    // minimum speed 1 m/s (~3.6 km/h) to avoid stalling
)

// DraftFactor returns the aerodynamic drag reduction based on position in group.
func DraftFactor(position int) float64 {
	switch {
	case position <= 1:
		return 1.0
	case position == 2:
		return 0.65
	case position <= 5:
		return 0.55
	case position <= 15:
		return 0.40
	default:
		return 0.25
	}
}

// ForceGravity returns the gravitational force component along the slope.
// Positive = uphill resistance, negative = downhill assistance.
func ForceGravity(totalMassKg float64, gradientPct float64) float64 {
	angle := math.Atan(gradientPct / 100.0)
	return totalMassKg * gravity * math.Sin(angle)
}

// ForceRolling returns the rolling resistance force.
func ForceRolling(totalMassKg float64, gradientPct float64, surface model.SurfaceType) float64 {
	angle := math.Atan(gradientPct / 100.0)
	c := crr
	if surface == model.SurfaceCobbles {
		c = crrCobbles
	}
	return c * totalMassKg * gravity * math.Cos(angle)
}

// ForceAero returns the aerodynamic drag force at a given speed.
func ForceAero(cda float64, speedMPS float64) float64 {
	return 0.5 * cda * airDensity * speedMPS * speedMPS
}

// SpeedFromPower solves for the velocity at which a rider can sustain a given
// power output on a given gradient. Uses Newton's method on the equation:
//
//	P = v * (Fgrav + Froll + Faero)
//
// Returns speed in m/s.
func SpeedFromPower(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType) float64 {
	fGrav := ForceGravity(totalMassKg, gradientPct)
	fRoll := ForceRolling(totalMassKg, gradientPct, surface)

	// f(v) = 0.5 * cda * rho * v³ + (fGrav + fRoll) * v - P = 0
	// f'(v) = 1.5 * cda * rho * v² + (fGrav + fRoll)
	aeroCoeff := 0.5 * cda * airDensity
	staticForce := fGrav + fRoll

	// Use bisection for robustness: find v where aeroCoeff*v³ + staticForce*v - P = 0
	// f(v) is monotonically increasing for v>0 (since aeroCoeff>0)
	lo := minSpeedMPS
	hi := 40.0 // ~144 km/h
	// On descents (staticForce < 0), terminal velocity can be high; expand upper bound
	for {
		fHi := aeroCoeff*hi*hi*hi + staticForce*hi - powerWatts
		if fHi > 0 {
			break
		}
		hi *= 2
		if hi > 200 { // cap at ~720 km/h, unreachable
			break
		}
	}

	for i := 0; i < 100; i++ {
		mid := (lo + hi) / 2
		fMid := aeroCoeff*mid*mid*mid + staticForce*mid - powerWatts
		if math.Abs(fMid) < 1e-6 || (hi-lo) < 1e-8 {
			return mid
		}
		if fMid < 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// DescentSpeed calculates speed on a descent, capped by descending ability and
// technical difficulty. On descents, gravity assists, so the rider may be
// going faster than their power alone would suggest.
func DescentSpeed(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, descendingAbility int, isTechnical bool) float64 {
	// For descents, calculate the terminal velocity where drag = gravity - rolling - braking
	// First get the powered speed
	speed := SpeedFromPower(powerWatts, totalMassKg, cda, gradientPct, surface)

	// Maximum descent speed based on ability (km/h)
	// Ability 70 (average WT) → ~80 km/h max on non-technical
	// Ability 95 → ~90 km/h max
	// Ability 40 → ~70 km/h max
	maxDescentKmh := 60.0 + float64(descendingAbility)*0.35
	if isTechnical {
		maxDescentKmh *= 0.80 // technical descents are ~20% slower
	}
	maxDescentMPS := maxDescentKmh / 3.6

	if speed > maxDescentMPS {
		speed = maxDescentMPS
	}
	return speed
}

// ModifierFactor returns the power multiplier for a given ability rating.
// 70 is average WT rider → factor 1.0
// 90 → 1.04 (+4%), 50 → 0.96 (-4%)
func ModifierFactor(ability int) float64 {
	return 1.0 + float64(ability-70)*0.002
}

// EffectivePower calculates a rider's effective power output for a segment,
// applying the relevant ability modifier.
func EffectivePower(rider *model.Rider, basePower float64, segment *model.Segment, isTT bool) float64 {
	var modifier int
	switch {
	case isTT:
		modifier = rider.TTAbility
	case segment.GradientPct > 4.0:
		modifier = rider.ClimbingAbility
	default:
		modifier = 70 // no modifier on flat/rolling terrain for sustained power
	}
	return basePower * ModifierFactor(modifier)
}

// SprintPower calculates a rider's sprint power output.
func SprintPower(rider *model.Rider) float64 {
	return float64(rider.MaxSprintWatts) * ModifierFactor(rider.SprintAbility)
}

// SegmentTime calculates the time in seconds for a rider to traverse a segment.
func SegmentTime(speed float64, distanceKm float64) float64 {
	if speed <= 0 {
		return math.Inf(1)
	}
	return (distanceKm * 1000.0) / speed
}
