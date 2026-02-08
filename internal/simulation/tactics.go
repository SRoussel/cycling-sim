package simulation

import (
	"math/rand"
	"sort"

	"cycling_sim/internal/model"
)

// SprintFinish resolves a bunch sprint among riders who arrive together.
// It modifies riders' TotalTime to create small gaps based on SprintAbility.
func SprintFinish(riders []*RiderState, rng *rand.Rand) {
	if len(riders) <= 1 {
		return
	}

	type sprintResult struct {
		rs         *RiderState
		sprintTime float64
	}

	results := make([]sprintResult, len(riders))
	for i, rs := range riders {
		// Sprint power with modifier and randomness
		sprintPower := SprintPower(rs.Rider) * rs.FormFactor
		// Add sprint-specific randomness (sprints are volatile)
		sprintPower *= 1.0 + rng.NormFloat64()*0.03

		// Sprint is ~200m, time = distance / speed
		// Approximate: higher power = faster sprint
		speed := SpeedFromPower(sprintPower, rs.Rider.TotalMassKg(),
			rs.Rider.CdA, 0, model.SurfaceRoad)
		sprintTime := 200.0 / speed // time for final 200m

		results[i] = sprintResult{rs: rs, sprintTime: sprintTime}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].sprintTime < results[j].sprintTime
	})

	// The sprint winner gets their time, others get small gaps
	winnerTime := results[0].sprintTime
	for i, res := range results {
		gap := res.sprintTime - winnerTime
		// Sprints produce very small time gaps (usually <1 second across top finishers)
		if i > 0 && gap < 0.01 {
			gap = 0.01 * float64(i) // minimum separation
		}
		res.rs.TotalTime += gap
	}
}

// BreakawayAttempt decides if a rider should attempt a breakaway on a given segment.
// Returns true if the rider decides to attack.
func BreakawayAttempt(rs *RiderState, segment *model.Segment, remainingKm float64, rng *rand.Rand) bool {
	// Factors that increase breakaway probability:
	// - High tactical awareness
	// - Plenty of W' remaining
	// - Not too far from the finish (but not too close for non-climbers)
	// - Hilly/undulating terrain

	if rs.WPrime.FractionRemaining() < 0.3 {
		return false // not enough juice
	}

	probability := float64(rs.Rider.TacticalAwareness) / 1000.0 // base 0.07 for avg rider

	// Boost on hills
	if segment.GradientPct > 4.0 {
		probability *= 2.0
	}

	// Distance factor: most likely 30-60km from finish
	if remainingKm > 30 && remainingKm < 60 {
		probability *= 1.5
	}
	if remainingKm < 10 {
		probability *= 0.3 // too close to finish, peloton will chase
	}

	return rng.Float64() < probability
}

// EvaluateBreakaway determines if a breakaway succeeds based on gap and peloton dynamics.
// This is a simplified model: breakaways succeed if they build enough time gap
// relative to distance remaining.
func EvaluateBreakaway(breakawayPower float64, pelotonPower float64, distanceKm float64) float64 {
	// Time gap per km based on power differential
	// A 20W advantage on flat at ~45km/h gives ~5 seconds per km
	if pelotonPower <= 0 {
		return 0
	}
	powerRatio := breakawayPower / pelotonPower
	if powerRatio <= 1.0 {
		return 0 // breakaway can't gain time
	}
	// Approximate gap gain: proportional to power difference
	gapPerKm := (powerRatio - 1.0) * 30.0 // ~seconds per km
	return gapPerKm * distanceKm
}

// DailyFormFactor generates a daily form multiplier for a rider.
// High consistency → small variance. Low consistency → larger variance.
func DailyFormFactor(rider *model.Rider, rng *rand.Rand) float64 {
	stddev := 0.02 * (1.0 - float64(rider.Consistency)/150.0)
	return 1.0 + rng.NormFloat64()*stddev
}
