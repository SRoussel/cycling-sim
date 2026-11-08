package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

const (
	gravity     = 9.8067 // m/s²
	airDensity  = 1.225  // kg/m³ at sea level, 15°C
	crr         = 0.004  // rolling resistance coefficient (road)
	crrCobbles  = 0.012  // rolling resistance coefficient (cobbles) — 3x road
	bikeWeight  = 8.0    // kg
	minSpeedMPS = 1.0    // minimum speed 1 m/s (~3.6 km/h) to avoid stalling
	// HandlingEffectPerPoint controls how much each handling point (1-100)
	// changes the effective rolling resistance on cobbles.
	// At 0.022: H=70 → 1.0x, H=93 → 0.494x, H=55 → 1.33x
	HandlingEffectPerPoint = 0.022
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
func ForceRolling(totalMassKg float64, gradientPct float64, surface model.SurfaceType, handling int) float64 {
	angle := math.Atan(gradientPct / 100.0)
	c := crr

	if surface == model.SurfaceCobbles {
		// Apply handling-based rolling resistance on cobbles
		c = crrCobbles * HandlingFactor(handling)
	}

	return c * totalMassKg * gravity * math.Cos(angle)
}

// HandlingFactor returns a multiplier to apply to cobbles rolling resistance
// based on a rider's handling skill (1-100). Higher handling reduces the
// effective crr on cobbles, giving a physics-plausible speed advantage.
// H=70 → 1.0, H=93 → 0.586 (eff crr ~0.007), H=55 → 1.27 (eff crr ~0.015).
func HandlingFactor(handling int) float64 {
	f := 1.0 - float64(handling-70)*HandlingEffectPerPoint
	if f < 0.5 {
		f = 0.5
	}
	if f > 1.5 {
		f = 1.5
	}
	return f
}

// CobbleWPrimeDrain returns extra W' depletion rate (watts) on cobbles
// based on handling skill. Poor handlers waste energy on corrections,
// braking, and fighting the bike. This is the attrition mechanism —
// over many cobble sectors, poor handlers deplete their W' and crack.
// H=93: -20W (saves energy), H=70: 0W, H=55: +30W extra drain.
func CobbleWPrimeDrain(handling int) float64 {
	drain := float64(70-handling) * 2.5
	if drain < -35 {
		drain = -35
	}
	if drain > 45 {
		drain = 45
	}
	return drain
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
func SpeedFromPower(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int) float64 {
	return SpeedFromPowerWind(powerWatts, totalMassKg, cda, gradientPct, surface, handling, 0)
}

// SpeedFromPowerWind is SpeedFromPower with a headwind (m/s, negative for a
// tailwind): aerodynamic drag depends on air speed v+w, not ground speed.
func SpeedFromPowerWind(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int, headwindMPS float64) float64 {
	if headwindMPS == 0 {
		return speedFromPowerCalm(powerWatts, totalMassKg, cda, gradientPct, surface, handling)
	}
	fStatic := ForceGravity(totalMassKg, gradientPct) + ForceRolling(totalMassKg, gradientPct, surface, handling)
	f := func(v float64) float64 {
		air := v + headwindMPS
		return (0.5*cda*airDensity*air*math.Abs(air)+fStatic)*v - powerWatts
	}
	lo, hi := minSpeedMPS, 40.0
	for f(hi) < 0 && hi < 200 {
		hi *= 2
	}
	for i := 0; i < 100 && hi-lo > 1e-8; i++ {
		mid := (lo + hi) / 2
		if f(mid) < 0 {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

func speedFromPowerCalm(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int) float64 {
	fGrav := ForceGravity(totalMassKg, gradientPct)
	fRoll := ForceRolling(totalMassKg, gradientPct, surface, handling)

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

// SpeedCap is the fastest a rider dares to go on this segment, set by
// descending skill and how technical the road is. It only binds at descent
// speeds, so it applies on every segment.
func SpeedCap(r *model.Rider, segment *model.Segment) float64 {
	return speedCapMPS(r.DescendingAbility, segment.IsTechnical)
}

func speedCapMPS(descendingAbility int, isTechnical bool) float64 {
	maxKmh := 60.0 + float64(descendingAbility)*0.35 // 70 → ~85 km/h, 95 → ~93 km/h
	if isTechnical {
		maxKmh *= 0.80 // technical descents are ~20% slower
	}
	return maxKmh / 3.6
}

// DescentSpeed calculates speed on a descent, capped by descending ability and
// technical difficulty. On descents, gravity assists, so the rider may be
// going faster than their power alone would suggest.
func DescentSpeed(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int, descendingAbility int, isTechnical bool) float64 {
	// For descents, calculate the terminal velocity where drag = gravity - rolling - braking
	// First get the powered speed
	speed := SpeedFromPower(powerWatts, totalMassKg, cda, gradientPct, surface, handling)

	maxDescentMPS := speedCapMPS(descendingAbility, isTechnical)

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
// applying the relevant ability modifier. Handling does NOT affect power —
// it affects rolling resistance via HandlingFactor in ForceRolling.
func EffectivePower(rider *model.Rider, basePower float64, segment *model.Segment, isTT bool) float64 {
	var modifier int
	switch {
	case isTT:
		modifier = rider.TTAbility
	case segment.GradientPct > 4.0:
		modifier = rider.ClimbingAbility
	default:
		modifier = 70
	}
	power := basePower * ModifierFactor(modifier)
	if !isTT && segment.SurfaceType == model.SurfaceCobbles {
		// Reduce effective power for poor handlers on cobbles.
		handlingFactor := 1.0 + float64(rider.Handling-70)*0.006
		if handlingFactor < 0.7 {
			handlingFactor = 0.7
		}
		if handlingFactor > 1.06 {
			handlingFactor = 1.06
		}
		power *= handlingFactor
	}
	return power
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
