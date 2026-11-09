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

// breakawayProbability is a rider's chance (per 5 km) of trying to get away.
// It is intent only: aggressive riders try more, a rider low on W' can't,
// a domestique riding for a leader in the group rarely goes, and a team's
// sprinter waits for the sprint. Where a move sticks is left to physics: on
// flat roads the bunch rides it down in the draft, on climbs and rough roads
// it can stay away.
func breakawayProbability(rs *RiderState, segment *model.Segment, remainingKm float64, leaderInGroup ...bool) float64 {
	if rs.TeamRole == RoleSprinter {
		return 0
	}
	wFrac := rs.WPrime.FractionRemaining()
	if wFrac < 0.2 {
		return 0 // spent
	}
	wFactor := (wFrac - 0.2) / 0.8 // 0 at 20% W', 1 when fresh
	if rs.TeamRole == RoleDomestique && len(leaderInGroup) > 0 && leaderInGroup[0] {
		wFactor *= 0.1 // working for the leader
	}
	// TacticalAwareness 70 -> 0.07 per 5 km, 90 -> 0.12, 50 -> 0.035.
	ta := float64(rs.Rider.TacticalAwareness)
	return ta * ta / 70000.0 * wFactor
}

// DailyFormFactor generates a daily form multiplier for a rider.
// High consistency → small variance. Low consistency → larger variance.
func DailyFormFactor(rider *model.Rider, rng *rand.Rand) float64 {
	stddev := 0.02 * (1.0 - float64(rider.Consistency)/150.0)
	return 1.0 + rng.NormFloat64()*stddev
}
