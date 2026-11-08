package simulation

import (
	"fmt"
	"math/rand"

	"cycling_sim/internal/model"
)

// FormGroups analyzes rider powers on a segment and splits them into groups.
// On flat/rolling terrain, the peloton stays together because of drafting,
// but riders who attack form a breakaway group.
// On climbs (gradient > 4%), riders split based on power-to-weight.
func FormGroups(riders []*RiderState, segment *model.Segment, isTT bool, remainingDistance float64, rng *rand.Rand, state *RaceState, params *TacticsParams) []*Group {
	if isTT || len(riders) == 0 {
		// TT: everyone in their own group
		groups := make([]*Group, len(riders))
		for i, r := range riders {
			r.GroupID = i
			r.GroupPosition = 1
			groups[i] = &Group{ID: i, Riders: []*RiderState{r}}
		}
		return groups
	}

	// Build a set of teams that have a leader present in this group
	leaderTeams := make(map[string]bool)
	for _, rs := range riders {
		if rs.TeamRole == RoleLeader || rs.TeamRole == RoleSprinter {
			leaderTeams[rs.Rider.Team] = true
		}
	}

	if params == nil {
		defaultParams := DefaultTacticsParams()
		params = &defaultParams
	}
	if state != nil {
		state.KmRemaining = remainingDistance
		state.Phase = PhaseFromKm(remainingDistance)
		state.Segment = segment
	}

	rps := make([]riderPower, len(riders))
	for i, rs := range riders {
		basePower := float64(rs.EffectiveFTP) * rs.FormFactor
		power := EffectivePower(rs.Rider, basePower, segment, false)
		hasLeader := leaderTeams[rs.Rider.Team]
		action := ActionHold
		if state != nil && state.Overrides != nil {
			if a, ok := state.Overrides.ConsumeOneShot(rs.Rider.ID); ok {
				action = a
			} else if a, ok := state.Overrides.ActionOverride(rs.Rider.ID); ok {
				action = a
			}
		}
		if state != nil && state.Overrides != nil {
			if sprintAction, ok := state.Overrides.SprintTrainDirective(rs.Rider.ID, rs.Rider.Team, remainingDistance); ok {
				if action == ActionHold || sprintAction == ActionAttack {
					action = sprintAction
				}
			}
		}
		if action == ActionHold {
			action = DecideAction(rs, state, hasLeader, rng, *params)
		}
		if state != nil {
			if state.LastActions == nil {
				state.LastActions = make(map[int64]Action)
			}
			previousAction := state.LastActions[rs.Rider.ID]
			if action == ActionAttack && previousAction != ActionAttack && state.Overrides != nil {
				state.LogEvent("attack", fmt.Sprintf("rider=%d team=%s", rs.Rider.ID, rs.Rider.Team))
			}
			state.LastActions[rs.Rider.ID] = action
		}
		if action != ActionAttack && state != nil {
			power *= IntensityForPhase(state.Phase)
		}
		power *= TacticsPowerMultiplier(action, *params)
		// Committed efforts are funded by W': attacks, and chases that matter
		// (a dangerous rider up the road, or sprint teams in the finale).
		committedChase := action == ActionChase && state != nil && (state.BreakawayHasDangerous || state.SprintChaseActive)
		if action == ActionAttack || committedChase {
			// Spend the W' available over the time this segment would take at
			// the rider's own speed.
			r := rs.Rider
			est := SegmentTime(SpeedFromPower(power, r.TotalMassKg(), r.CdA, segment.GradientPct, segment.SurfaceType, r.Handling), segment.DistanceKm)
			power += rs.WPrime.AvailablePowerAboveFTP(est)
		}
		if state != nil && state.Overrides != nil {
			if effortMultiplier, ok := state.Overrides.EffortMultiplier(rs.Rider.ID); ok {
				power *= effortMultiplier
			}
		}

		// Domestique sacrifice: when the team leader is in this group and we're
		// climbing (or in the final km), domestiques push 4% harder to pace for
		// their leader. This raises group speed and costs the domestique W'.
		if rs.TeamRole == RoleDomestique && leaderTeams[rs.Rider.Team] &&
			action != ActionConserve {
			if segment.GradientPct > 3.0 ||
				(state != nil && state.Phase == PhaseFinal && segment.GradientPct > -1.0) {
				power *= 1.04
			}
		}

		// W' depletion limits power
		if rs.WPrime.FractionRemaining() < 0.1 {
			// Can't sustain above FTP when W' is very low
			maxPower := float64(rs.EffectiveFTP) * rs.FormFactor
			if power > maxPower {
				power = maxPower
			}
		}
		rs.LastSegmentPower = power

		breakaway := action == ActionAttack
		// If no RaceState is provided, DecideAction() can't evaluate breakaway attempts.
		// Fall back to a single explicit breakaway roll for callers that pass nil state.
		if !breakaway && (state == nil || state.Segment == nil) {
			breakaway = BreakawayAttemptWithParams(rs, segment, remainingDistance, rng, hasLeader, *params)
		}
		rps[i] = riderPower{
			state:     rs,
			power:     power,
			wpw:       power / rs.Rider.WeightKg,
			breakaway: breakaway,
			action:    action,
		}
	}

	// Attackers ride away as their own group; everyone else follows the same
	// physical rule. Gaps then grow or shrink from speed differences alone.
	breakawayLeaderTeams := make(map[string]bool)
	for _, rp := range rps {
		if rp.breakaway && (rp.state.TeamRole == RoleLeader || rp.state.TeamRole == RoleSprinter) {
			breakawayLeaderTeams[rp.state.Rider.Team] = true
		}
	}
	var attackers, others []riderPower
	for _, rp := range rps {
		// Domestiques don't join a move that contains their own leader.
		joins := rp.breakaway && len(attackers) < maxBreakawaySize &&
			!(rp.state.TeamRole == RoleDomestique && breakawayLeaderTeams[rp.state.Rider.Team])
		if joins {
			attackers = append(attackers, rp)
		} else {
			others = append(others, rp)
		}
	}
	var groups []*Group
	if len(attackers) > 0 && len(others) > 0 {
		groups = followSplit(attackers, segment, 0)
		for _, g := range groups {
			g.IsBreakaway = true
		}
	} else {
		others = rps
	}
	return append(groups, followSplit(others, segment, len(groups))...)
}

// assignTeamPositions reorders riders in a group so domestiques ride at the front
// (low position = more wind) to shelter their team's leader/sprinter.
// Only applies on flat/rolling terrain where drafting matters.
func assignTeamPositions(rps []riderPower) {
	if len(rps) <= 1 {
		return
	}

	// Identify which teams have a protected rider (Leader or Sprinter) in this group
	protectedTeams := make(map[string]bool)
	for _, rp := range rps {
		if rp.state.TeamRole == RoleLeader || rp.state.TeamRole == RoleSprinter {
			protectedTeams[rp.state.Rider.Team] = true
		}
	}

	if len(protectedTeams) == 0 {
		// No protected riders — keep power-ranked positions
		for i, rp := range rps {
			rp.state.GroupPosition = i + 1
		}
		return
	}

	// Separate into: domestiques pulling for a team, protected riders, and others
	var frontDomestiques []riderPower
	var protectedRiders []riderPower
	var others []riderPower

	for _, rp := range rps {
		if rp.state.TeamRole == RoleDomestique && protectedTeams[rp.state.Rider.Team] {
			frontDomestiques = append(frontDomestiques, rp)
		} else if (rp.state.TeamRole == RoleLeader || rp.state.TeamRole == RoleSprinter) && protectedTeams[rp.state.Rider.Team] {
			protectedRiders = append(protectedRiders, rp)
		} else {
			others = append(others, rp)
		}
	}

	// Order: domestiques at front (in the wind), then other riders, then protected riders (sheltered)
	pos := 1
	for _, rp := range frontDomestiques {
		rp.state.GroupPosition = pos
		pos++
	}
	for _, rp := range others {
		rp.state.GroupPosition = pos
		pos++
	}
	for _, rp := range protectedRiders {
		rp.state.GroupPosition = pos
		pos++
	}
}
