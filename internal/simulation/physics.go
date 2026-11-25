package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

const (
	gravity    = 9.8067 // m/s²
	airDensity = 1.225  // kg/m³ at sea level, 15°C
	crr        = 0.004  // rolling resistance coefficient (road)
	crrCobbles = 0.012  // rolling resistance coefficient (cobbles) — 3x road
	// crrGravel: compacted white gravel (strade bianche), a 2-star sector;
	// tyre tests put good gravel at ~0.007-0.011, a little under pavé of the
	// same grade (see gravelRoughness).
	crrGravel   = 0.009
	bikeWeight  = 8.0 // kg
	minSpeedMPS = 1.0 // minimum speed 1 m/s (~3.6 km/h) to avoid stalling
)

// Drafting: the CdA multiplier rises from the second wheel's value towards a
// packed bunch's deep shelter smoothly with position (wind tunnel / CFD:
// ~35% saving on one wheel, ~50% a few riders back, ~75% deep in a bunch).
const (
	draftSecondWheel = 0.65 // CdA multiplier on one wheel
	draftDeepBunch   = 0.25 // deep inside a packed bunch
	draftDecayRiders = 6.4  // riders over which shelter deepens by 1/e
)

// DraftFactor returns the aerodynamic drag multiplier for a position in a
// group (1 = on the front, no shelter).
func DraftFactor(position int) float64 {
	if position <= 1 {
		return 1.0
	}
	if position < len(draftTable) {
		return draftTable[position]
	}
	return draftFactorAt(position)
}

func draftFactorAt(position int) float64 {
	return draftDeepBunch + (draftSecondWheel-draftDeepBunch)*math.Exp(-float64(position-2)/draftDecayRiders)
}

// draftTable: DraftFactor by position, computed once (the exponential was
// recomputed on every call).
var draftTable = func() []float64 {
	t := make([]float64, 256)
	for p := 2; p < len(t); p++ {
		t[p] = draftFactorAt(p)
	}
	return t
}()

// ForceGravity returns the gravitational force component along the slope.
// Positive = uphill resistance, negative = downhill assistance.
func ForceGravity(totalMassKg float64, gradientPct float64) float64 {
	g := gradientPct / 100.0
	return totalMassKg * gravity * g / math.Sqrt(1+g*g) // sin(atan g), without the trig
}

// ForceRolling returns the rolling resistance force.
func ForceRolling(totalMassKg float64, gradientPct float64, surface model.SurfaceType, handling int) float64 {
	return forceRolling(totalMassKg, gradientPct, surface, handling, 1)
}

// forceRolling is ForceRolling with cobbles' resistance scaled by roughness
// (see cobbleRoughness).
func forceRolling(totalMassKg float64, gradientPct float64, surface model.SurfaceType, handling int, roughness float64) float64 {
	g := gradientPct / 100.0
	c := crr

	switch surface {
	case model.SurfaceCobbles:
		// Handling and body mass: what the rider loses to the stones.
		c = crrCobbles * roughness * roughResistanceFactor(handling, totalMassKg-bikeWeight, surface)
	case model.SurfaceGravel:
		c = crrGravel * roughness * roughResistanceFactor(handling, totalMassKg-bikeWeight, surface)
	}

	return c * totalMassKg * gravity / math.Sqrt(1+g*g) // cos(atan g)
}

// cobbleRoughness scales cobbles' rolling resistance by a sector's star
// rating (the race organisers' difficulty grading): crrCobbles is a 3-star
// sector; 1-star pavé rolls at ~0.008, a 5-star sector like the Trouée
// d'Arenberg at ~0.020 (vibration losses on rough pavé). Unknown: 1.
func cobbleRoughness(stars int) float64 {
	switch stars {
	case 1:
		return 0.67
	case 2:
		return 0.83
	case 4:
		return 1.33
	case 5:
		return 1.67
	}
	return 1
}

// gravelRoughness scales gravel's rolling resistance by a sector's stars
// (the mod grades gravel 1-3): ~0.007 for 1 star, 0.009 for 2, 0.011 for 3.
func gravelRoughness(stars int) float64 {
	switch stars {
	case 1:
		return 0.78
	case 3:
		return 1.22
	case 4:
		return 1.44
	case 5:
		return 1.67
	}
	return 1
}

// sectorRoughness is the roughness of a segment's surface by its stars.
func sectorRoughness(seg *model.Segment) float64 {
	if seg.SurfaceType == model.SurfaceGravel {
		return gravelRoughness(seg.CobbleStars)
	}
	return cobbleRoughness(seg.CobbleStars)
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
	return speedFromPowerAir(powerWatts, totalMassKg, cda, gradientPct, surface, handling, headwindMPS)
}

func speedFromPowerAir(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int, headwindMPS float64) float64 {
	return speedFromPowerRough(powerWatts, totalMassKg, cda, gradientPct, surface, handling, headwindMPS, 1)
}

// speedFromPowerRough is speedFromPowerAir with cobble roughness.
func speedFromPowerRough(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int, headwindMPS, roughness float64) float64 {
	if headwindMPS == 0 {
		return speedFromPowerCalmRough(powerWatts, totalMassKg, cda, gradientPct, surface, handling, roughness)
	}
	fStatic := ForceGravity(totalMassKg, gradientPct) + forceRolling(totalMassKg, gradientPct, surface, handling, roughness)
	k := 0.5 * cda * airDensity
	return solveSpeed(func(v float64) (float64, float64) {
		air := v + headwindMPS
		return (k*air*math.Abs(air)+fStatic)*v - powerWatts, k*(air*math.Abs(air)+2*math.Abs(air)*v) + fStatic
	})
}

// solveSpeed finds the speed where power balance f(v) (with derivative)
// crosses zero: Newton steps kept inside a bisection bracket (falling back
// to bisection when a step would leave it), to the bisection's tolerance.
// Power from speed is convex and negative at rest, so the positive root is
// unique and both methods find it; Newton takes ~5 steps where bisection
// took ~30 (the solver was ~45% of all simulation time, Nov 2026).
func solveSpeed(f func(v float64) (float64, float64)) float64 {
	lo, hi := minSpeedMPS, 40.0
	if fl, _ := f(lo); fl >= 0 {
		return lo
	}
	for fh, _ := f(hi); fh < 0 && hi < 200; fh, _ = f(hi) {
		hi *= 2
	}
	v := (lo + hi) / 2
	for i := 0; i < 100; i++ {
		fv, d := f(v)
		if fv < 0 {
			lo = v
		} else {
			hi = v
		}
		if hi-lo < 1e-8 || math.Abs(fv) < 1e-9 {
			return v
		}
		next := v - fv/d
		if d <= 0 || next <= lo || next >= hi {
			next = (lo + hi) / 2
		}
		v = next
	}
	return v
}

func speedFromPowerCalm(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int) float64 {
	return speedFromPowerCalmRough(powerWatts, totalMassKg, cda, gradientPct, surface, handling, 1)
}

func speedFromPowerCalmRough(powerWatts float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int, roughness float64) float64 {
	// aeroCoeff·v³ + staticForce·v = P (calm air)
	aeroCoeff := 0.5 * cda * airDensity
	staticForce := ForceGravity(totalMassKg, gradientPct) + forceRolling(totalMassKg, gradientPct, surface, handling, roughness)
	return solveSpeed(func(v float64) (float64, float64) {
		return aeroCoeff*v*v*v + staticForce*v - powerWatts, 3*aeroCoeff*v*v + staticForce
	})
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

// EffectivePower calculates a rider's effective power output for a segment:
// the ability modifier for the terrain and the altitude. Climbing ability
// blends in with the climb factor (how much of the effort is lifting weight),
// so there's no gradient at which it switches on.
func EffectivePower(rider *model.Rider, basePower float64, segment *model.Segment, isTT bool) float64 {
	var factor float64
	if isTT {
		factor = ModifierFactor(rider.TTAbility)
	} else {
		w := clamp01(climbOf(segment) / terrainClimb)
		factor = 1 + (ModifierFactor(rider.ClimbingAbility)-1)*w
	}
	return basePower * factor * AltitudePowerFactor(segment.ElevationM)
}

// Rough roads (cobbles, gravel): what a rider loses to the road, not what he
// puts into it. Handling (line choice: the crown or the gutter, floating over
// the stones, less braking) scales the rough-road rolling resistance by
// handlingPerPoint per point around the field's average of 70; a light rider
// is shaken about more and loses more to vibration, (mass / jostleRefKg)^
// -jostleExp, half as much on gravel. As resistance, the skill matters most
// at speed on flat pavé and little on a slow, steep berg (Nov 2026: it was a
// power bonus, the same share on both, and handling also scaled the
// resistance a second time at 2.2% a point).
const (
	handlingPerPoint = 0.01
	jostleRefKg      = 72.0
	jostleExp        = 0.5
)

// roughResistanceFactor scales a rough road's rolling resistance for a rider
// (handling and body mass; 1 for an average 72 kg rider of Handling 70).
func roughResistanceFactor(handling int, riderMassKg float64, surface model.SurfaceType) float64 {
	f := 1.0 - float64(handling-70)*handlingPerPoint
	if riderMassKg > 0 {
		exp := jostleExp
		if surface == model.SurfaceGravel {
			exp /= 2
		}
		f *= math.Pow(riderMassKg/jostleRefKg, -exp)
	}
	return math.Max(0.6, math.Min(1.5, f))
}

// SprintPower calculates a rider's fresh sprint power output.
func SprintPower(rider *model.Rider) float64 {
	return float64(rider.MaxSprintWatts) * ModifierFactor(rider.SprintAbility)
}

// sprintWPrimeShare: how much of peak sprint power depends on anaerobic
// reserves left (an emptied rider loses this fraction).
const sprintWPrimeShare = 0.10

// FatiguedSprintPower is the sprint power a rider has left at the finish:
// peak power fades with the day's work like threshold does (durability; pro
// data show sprint power falling several % after 3-4000 kJ) and with how
// deep the rider has dug into W'.
func FatiguedSprintPower(rs *RiderState) float64 {
	wFrac := 1.0
	if rs.WPrime != nil {
		wFrac = rs.WPrime.FractionRemaining()
	}
	return SprintPower(rs.Rider) * DurabilityFactor(rs.WorkKJ, rs.Rider.Endurance) *
		(1 - sprintWPrimeShare*(1-wFrac))
}

// SegmentTime calculates the time in seconds for a rider to traverse a segment.
func SegmentTime(speed float64, distanceKm float64) float64 {
	if speed <= 0 {
		return math.Inf(1)
	}
	return (distanceKm * 1000.0) / speed
}

// clamp01 limits x to [0, 1] (plain comparisons: math.Min/Max's NaN and
// signed-zero handling cost time in hot paths, and x is never NaN here).
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
