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
}

// meanderMaxDeg: real roads wander around a segment's overall heading, so
// wind exposure changes along a long segment.
const meanderMaxDeg = 60.0

// stepHeading is a stable meander around the segment heading for step i.
func stepHeading(seg *model.Segment, i int) float64 {
	return seg.HeadingDeg + meanderMaxDeg*math.Sin(1.7*float64(i)+0.9*float64(seg.SegmentOrder))
}

// segmentSpeed is SpeedFromPower on a segment, including its headwind.
func segmentSpeed(power, massKg, cda float64, seg *model.Segment, handling int) float64 {
	return SpeedFromPowerWind(power, massKg, cda, seg.GradientPct, seg.SurfaceType, handling, seg.HeadwindMPS)
}

// segmentPower is the power needed to hold speed on a segment, with headwind.
func segmentPower(speed, massKg, cda float64, seg *model.Segment, handling int) float64 {
	air := speed + seg.HeadwindMPS
	fAero := 0.5 * cda * airDensity * air * math.Abs(air)
	return speed * (ForceGravity(massKg, seg.GradientPct) + ForceRolling(massKg, seg.GradientPct, seg.SurfaceType, handling) + fAero)
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
