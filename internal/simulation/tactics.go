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
		sprintPower := FatiguedSprintPower(rs) * rs.FormFactor
		// Add sprint-specific randomness (sprints are volatile)
		sprintPower *= 1.0 + rng.NormFloat64()*0.03

		// Sprint is ~200m, time = distance / speed
		// Approximate: higher power = faster sprint
		speed := SpeedFromPower(sprintPower, rs.Rider.TotalMassKg(),
			rs.Rider.CdA, 0, model.SurfaceRoad, rs.Rider.Handling)
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

// maxBreakawaySize caps the number of riders that can form a breakaway.
// More than this is essentially a group split, not a breakaway.
const maxBreakawaySize = 12

// BreakawayAttempt decides if a rider should attempt a breakaway on a given segment.
// leaderInGroup indicates whether the rider's team leader is in the same group.
// Returns true if the rider decides to attack.
func BreakawayAttempt(rs *RiderState, segment *model.Segment, remainingKm float64, rng *rand.Rand, leaderInGroup ...bool) bool {
	probability := breakawayProbability(rs, segment, remainingKm, leaderInGroup...)
	return rng.Float64() < ScaleToDistance(probability, segment.DistanceKm)
}

func BreakawayAttemptWithParams(rs *RiderState, segment *model.Segment, remainingKm float64, rng *rand.Rand, leaderInGroup bool, params TacticsParams) bool {
	probability := breakawayProbability(rs, segment, remainingKm, leaderInGroup)
	probability *= params.AttackAggression
	if probability > 0.9 {
		probability = 0.9
	}
	if probability < 0.0 {
		probability = 0.0
	}
	return rng.Float64() < ScaleToDistance(probability, segment.DistanceKm)
}

func breakawayProbability(rs *RiderState, segment *model.Segment, remainingKm float64, leaderInGroup ...bool) float64 {
	// W' fraction as a continuous factor — exhausted riders are much less likely to attack
	wFrac := rs.WPrime.FractionRemaining()
	if wFrac < 0.2 {
		return 0.0 // completely spent, won't attack
	}
	// Scale probability by W' availability (full W' = 1.0, 20% W' = ~0.0)
	wFactor := (wFrac - 0.2) / 0.8 // 0.0 at 20%, 1.0 at 100%

	// Domestique role suppression: stay with leader to do team work
	hasLeader := len(leaderInGroup) > 0 && leaderInGroup[0]
	if rs.TeamRole == RoleDomestique && hasLeader {
		wFactor *= 0.1 // drastically reduce breakaway probability
	}

	// Base probability from tactical awareness (0-100 scale)
	// TacticalAwareness 70 → base 0.07, 90 → 0.12, 50 → 0.035
	baseProbability := float64(rs.Rider.TacticalAwareness) * float64(rs.Rider.TacticalAwareness) / 70000.0

	probability := baseProbability * wFactor

	// Climbing ability boost on hills — climbers attack on climbs
	if ClimbFactor(segment) > terrainClimb {
		climbFactor := 1.0 + float64(rs.Rider.ClimbingAbility-70)*0.03
		if climbFactor < 0.5 {
			climbFactor = 0.5
		}
		probability *= 2.0 * climbFactor
	}

	// Handling boost on cobbles only — specialists attack on technical terrain
	if segment.SurfaceType == model.SurfaceCobbles {
		// H=99: ~2.2x, H=90: ~1.8x, H=80: ~1.4x, H=70: 1x, H<70: capped at 0.5x
		handlingFactor := 1.0 + float64(rs.Rider.Handling-70)*0.04
		if handlingFactor < 0.5 {
			handlingFactor = 0.5
		}
		if handlingFactor > 2.5 {
			handlingFactor = 2.5
		}
		probability *= handlingFactor
	}

	// Sprinters are less likely to break away early (they want a bunch sprint),
	// but cobbles specialists who are also good sprinters (e.g. vdP, van Aert)
	// should NOT be suppressed on cobbled sectors where they are the strongest.
	if rs.Rider.SprintAbility > 85 && remainingKm > 20 {
		if segment.SurfaceType != model.SurfaceCobbles || rs.Rider.Handling <= 85 {
			probability *= 0.3
		}
	}

	// Distance factor: most likely 30-60km from finish (optimal attack zone)
	if remainingKm > 30 && remainingKm < 60 {
		probability *= 1.5
	}
	// Flat non-cobble road suppression: peloton always controls pace on flat.
	// Close to the finish (≤15 km) this lifts to allow sprint-finish attacks.
	if isFlatish(segment) &&
		segment.SurfaceType != model.SurfaceCobbles && remainingKm > 15 {
		if remainingKm > 80 {
			// Opening flat — almost no breakaway survives this far out
			probability *= 0.05
		} else {
			probability *= 0.35
		}
	}
	// Too close to finish — peloton will chase hard
	if remainingKm < 10 {
		probability *= 0.3
	}
	// Very far from finish on non-flat terrain (climbs/cobbles handled above)
	if remainingKm > 100 &&
		!(isFlatish(segment) && segment.SurfaceType != model.SurfaceCobbles) {
		probability *= 0.7
	}

	return probability
}

// DailyFormFactor generates a daily form multiplier for a rider.
// High consistency → small variance. Low consistency → larger variance.
func DailyFormFactor(rider *model.Rider, rng *rand.Rand) float64 {
	stddev := 0.02 * (1.0 - float64(rider.Consistency)/150.0)
	return 1.0 + rng.NormFloat64()*stddev
}

// isFlatish reports flat or gently rolling terrain (neither climb nor descent).
func isFlatish(seg *model.Segment) bool {
	c := ClimbFactor(seg)
	return c >= terrainDescent && c <= terrainFlat
}
