package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

// Wind and echelons.
//
// Headwind changes air speed, so it enters the aero term of the speed solver.
// Crosswind shifts the sheltered zone sideways: riders shelter diagonally in
// an echelon whose size is limited by road width. Riders beyond it ride in the
// gutter and lose shelter in proportion to the yaw angle, so a hard pace in a
// crosswind splits the bunch.
const (
	// echelonCapacity is how many riders fit across the road in an echelon.
	echelonCapacity = 10
	// fullExposureYawDeg: at this yaw angle a gutter rider has no shelter left.
	fullExposureYawDeg = 25.0
)

// WithWind returns a copy of the segments with headwind/crosswind components
// for a race's wind (speed km/h, direction it blows FROM, compass degrees).
func WithWind(segments []model.Segment, windKmh, windFromDeg float64) []model.Segment {
	out := append([]model.Segment(nil), segments...)
	w := windKmh / 3.6
	for i := range out {
		out[i].WindMPS = w
		out[i].WindFromDeg = windFromDeg
		setWindComponents(&out[i], out[i].HeadingDeg)
	}
	return out
}

// setWindComponents sets head/crosswind for a direction of travel.
func setWindComponents(seg *model.Segment, headingDeg float64) {
	rel := (seg.WindFromDeg - headingDeg) * math.Pi / 180
	seg.HeadwindMPS = seg.WindMPS * math.Cos(rel)
	seg.CrosswindMPS = math.Abs(seg.WindMPS * math.Sin(rel))
	refreshClimb(seg)
}

// meanderMaxDeg: real roads wander around a segment's overall heading, so
// wind exposure changes along a long segment.
const meanderMaxDeg = 60.0

// stepHeading is a stable meander around the segment heading for step i.
func stepHeading(seg *model.Segment, i int) float64 {
	return stepHeadingAt(seg, float64(i))
}

// stepHeadingAt is stepHeading at a fractional coarse-step position.
func stepHeadingAt(seg *model.Segment, i float64) float64 {
	return seg.HeadingDeg + meanderMaxDeg*math.Sin(1.7*i+0.9*float64(seg.SegmentOrder))
}

// segmentSpeed is SpeedFromPower on a segment, including its headwind.
// It includes headwind and the air density at the segment's altitude.
func segmentSpeed(power, massKg, cda float64, seg *model.Segment, handling int) float64 {
	return speedFromPowerRough(power, massKg, cda*densityRatio(seg), seg.GradientPct, seg.SurfaceType, handling, seg.HeadwindMPS, sectorRoughness(seg))
}

// densityRatio scales drag for altitude: thinner air acts like a smaller CdA.
func densityRatio(seg *model.Segment) float64 {
	if seg == nil || seg.AirDensity <= 0 {
		return 1
	}
	return seg.AirDensity / airDensity
}

// segmentPower is the power needed to hold speed on a segment, with headwind.
func segmentPower(speed, massKg, cda float64, seg *model.Segment, handling int) float64 {
	air := speed + seg.HeadwindMPS
	fAero := 0.5 * cda * densityRatio(seg) * airDensity * air * math.Abs(air)
	return speed * (ForceGravity(massKg, seg.GradientPct) + forceRolling(massKg, seg.GradientPct, seg.SurfaceType, handling, sectorRoughness(seg)) + fAero)
}

// segmentPowerSlope is d(segmentPower)/d(speed).
func segmentPowerSlope(speed, massKg, cda float64, seg *model.Segment, handling int) float64 {
	air := speed + seg.HeadwindMPS
	k := 0.5 * cda * densityRatio(seg) * airDensity
	return k*(air*math.Abs(air)+2*math.Abs(air)*speed) + ForceGravity(massKg, seg.GradientPct) + forceRolling(massKg, seg.GradientPct, seg.SurfaceType, handling, sectorRoughness(seg))
}

// gutterExposure is the fraction of shelter lost by riders beyond the
// echelon at this speed: 0 in calm or pure head/tailwind, 1 at high yaw.
func gutterExposure(seg *model.Segment, speed float64) float64 {
	if seg == nil || seg.CrosswindMPS <= 0 {
		return 0
	}
	yaw := math.Atan2(seg.CrosswindMPS, math.Max(speed+seg.HeadwindMPS, 0.1)) * 180 / math.Pi
	return math.Min(1, yaw/fullExposureYawDeg)
}

// crosswindDraft applies echelon limits to a draft factor for a position.
func crosswindDraft(d float64, position int, seg *model.Segment, speed float64) float64 {
	if position <= echelonCapacity {
		return d
	}
	return 1 - (1-d)*(1-gutterExposure(seg, speed))
}

// Reference rider for terrain classification: 70 kg, 5.5 W/kg, CdA 0.32.
const (
	refRiderMassKg = 78.0 // incl. bike
	refRiderPower  = 385.0
	refRiderCdA    = 0.32
)

// ClimbFactor is the share of a reference rider's power that goes into
// lifting their weight on this segment (wind included): ~0 on the flat,
// above ~0.6 on real climbs, negative downhill. It is the physical reason
// climbs are selective: W/kg decides, and drafting barely helps.
func ClimbFactor(seg *model.Segment) float64 {
	p := refRiderPower * AltitudePowerFactor(seg.ElevationM)
	v := segmentSpeed(p, refRiderMassKg, refRiderCdA, seg, 70)
	return ForceGravity(refRiderMassKg, seg.GradientPct) * v / p
}

// refreshClimb caches a segment's climb factor (after wind/terrain changes).
func refreshClimb(seg *model.Segment) {
	seg.ClimbSet = false
	seg.Climb = ClimbFactor(seg)
	seg.ClimbSet = true
}

// Terrain classes by ClimbFactor (still-air gradient equivalents in comments).
const (
	terrainClimb   = 0.73  // a real climb (~5%)
	terrainHill    = 0.65  // a hill (~4%)
	terrainRolling = 0.60  // rolling (~3.5%)
	terrainDrag    = 0.54  // a drag (~3%)
	terrainFlat    = 0.40  // flat or nearly (~2%)
	terrainDescent = -0.26 // descending (~-1%)
)

// climbOf returns a segment's climb factor, cached when the segment was
// prepared for a race and computed otherwise.
func climbOf(seg *model.Segment) float64 {
	if seg.ClimbSet {
		return seg.Climb
	}
	return ClimbFactor(seg)
}
