package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

// Time-cut rules (approximating UCI/organiser tables, which set the limit as
// a percentage of the winner's time by stage type and speed).
const (
	timeCutBase          = 0.05  // flat stage
	timeCutPerClimbMPerK = 0.006 // +0.6% per metre of climbing per km
	timeCutMax           = 0.20
	timeCutTT            = 0.25
	// If more riders than this fraction miss the cut, the jury waives it.
	timeCutWaiveFraction = 0.20
)

// TimeCutFraction returns the allowed margin over the winner's time.
func TimeCutFraction(course *model.Course, raceType model.RaceType) float64 {
	if raceType == model.RaceTypeTT {
		return timeCutTT
	}
	if course == nil {
		return timeCutMax
	}
	dist, climb := 0.0, 0.0
	for _, s := range course.Segments {
		dist += s.DistanceKm
		if s.GradientPct > 0 {
			climb += s.DistanceKm * 1000 * s.GradientPct / 100
		}
	}
	if dist <= 0 {
		return timeCutMax
	}
	return math.Min(timeCutMax, timeCutBase+timeCutPerClimbMPerK*climb/dist)
}

// ApplyTimeCut marks finishers outside the time limit as eliminated
// (Position 0) and returns how many were cut. Results must be ordered by
// finish time. The cut is waived when too many riders would miss it.
func ApplyTimeCut(results []model.RaceResult, course *model.Course, raceType model.RaceType) int {
	if len(results) == 0 {
		return 0
	}
	winner := -1.0
	finishers := 0
	for _, r := range results {
		if r.Position > 0 {
			finishers++
			if winner < 0 || r.FinishTimeSeconds < winner {
				winner = r.FinishTimeSeconds
			}
		}
	}
	if winner <= 0 {
		return 0
	}
	limit := winner * (1 + TimeCutFraction(course, raceType))
	var outside []int
	for i, r := range results {
		if r.Position > 0 && r.FinishTimeSeconds > limit {
			outside = append(outside, i)
		}
	}
	if float64(len(outside)) > timeCutWaiveFraction*float64(finishers) {
		return 0
	}
	for _, i := range outside {
		results[i].Position = 0
	}
	return len(outside)
}
