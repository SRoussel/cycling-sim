package simulation

import (
	"math"
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
	return rng.Float64() < probability
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
	return rng.Float64() < probability
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
	if segment.GradientPct > 4.0 {
		climbFactor := 1.0 + float64(rs.Rider.ClimbingAbility-70)*0.03
		if climbFactor < 0.5 {
			climbFactor = 0.5
		}
		probability *= 2.0 * climbFactor
	}

	// Handling boost on cobbles only — specialists attack on technical terrain
	if segment.SurfaceType == model.SurfaceCobbles {
		// H=95: 4x, H=85: 2x, H=70: 1x, H<70: 0.5x
		handlingFactor := 1.0 + float64(rs.Rider.Handling-70)*0.12
		if handlingFactor < 0.5 {
			handlingFactor = 0.5
		}
		if handlingFactor > 4.0 {
			handlingFactor = 4.0
		}
		probability *= handlingFactor
	}

	// Sprinters are less likely to break away early (they want a bunch sprint)
	if rs.Rider.SprintAbility > 85 && remainingKm > 20 {
		probability *= 0.3
	}

	// Distance factor: most likely 30-60km from finish (optimal attack zone)
	if remainingKm > 30 && remainingKm < 60 {
		probability *= 1.5
	}
	// Flat road suppression: breakaways are less likely on flat stages
	if segment.GradientPct >= -1.0 && segment.GradientPct <= 2.0 && remainingKm > 15 {
		probability *= 0.5
	}
	// Too close to finish — peloton will chase hard
	if remainingKm < 10 {
		probability *= 0.3
	}
	// Very far from finish — breakaway unlikely to survive
	if remainingKm > 100 {
		probability *= 0.7
	}

	return probability
}

// EvaluateBreakaway determines the time gap a breakaway gains over a segment.
// Returns gap in seconds (positive = breakaway gains time).
// Models: power differential, drafting advantage of peloton, cooperation in breakaway,
// and increasing peloton chase effort closer to the finish.
func EvaluateBreakaway(breakawayPower float64, pelotonPower float64, segmentDistanceKm float64, remainingKm float64, breakawaySize int) float64 {
	return EvaluateBreakawayWithCohesion(breakawayPower, pelotonPower, segmentDistanceKm, remainingKm, breakawaySize, 0.5, DefaultTacticsParams())
}

func EvaluateBreakawayWithCohesion(breakawayPower float64, pelotonPower float64, segmentDistanceKm float64, remainingKm float64, breakawaySize int, cohesion float64, params TacticsParams) float64 {
	if pelotonPower <= 0 || breakawayPower <= 0 {
		return 0
	}

	// Peloton has drafting advantage: leaders only need ~70% of solo power
	// to maintain speed. (If this is too low, breakaways never stick.)
	pelotonEffective := pelotonPower / 0.70

	powerRatio := breakawayPower / pelotonEffective
	if powerRatio <= 1.0 {
		return 0 // breakaway can't gain time against organized peloton
	}

	// Base gap gain: ~seconds per km from power differential
	gapPerKm := (powerRatio - 1.0) * 26.0
	gapPerKm *= params.BreakawaySuccessBias

	// Cooperation factor: solo breakaway is hardest, 2-4 riders can share turns
	// Solo = 0.65, 2 riders = 1.0, 3-4 riders = 1.2, 5+ = 1.2 (cap)
	coopFactor := math.Min(float64(breakawaySize), 4)*0.3 + 0.35
	coopFactor *= 1.0 + params.BreakawayCohesionWeight*(cohesion-0.5)
	if coopFactor < 0.4 {
		coopFactor = 0.4
	}
	if coopFactor > 1.5 {
		coopFactor = 1.5
	}
	gapPerKm *= coopFactor

	// Distance-remaining decay: peloton organizes a more serious chase closer to finish
	// Far from finish (>60km) → peloton is relaxed, breakaway gains easily
	// Close to finish (<20km) → peloton chases hard, but breakaway can still win
	chaseFactor := 1.0
	if remainingKm < 20 {
		chaseFactor = 0.7 + remainingKm/65.0 // 0.7 at finish, ~1.0 at 20km
	} else if remainingKm < 40 {
		chaseFactor = 0.85 + 0.15*(remainingKm-20)/20.0 // 0.85-1.0
	}
	gapPerKm *= chaseFactor

	gap := gapPerKm * segmentDistanceKm
	maxGap := math.Min(segmentDistanceKm*10.0, 90.0)
	if gap > maxGap {
		gap = maxGap
	}
	return gap
}

// DailyFormFactor generates a daily form multiplier for a rider.
// High consistency → small variance. Low consistency → larger variance.
func DailyFormFactor(rider *model.Rider, rng *rand.Rand) float64 {
	stddev := 0.02 * (1.0 - float64(rider.Consistency)/150.0)
	return 1.0 + rng.NormFloat64()*stddev
}
