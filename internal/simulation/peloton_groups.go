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

	driving := drivingTeams(riders, segment, state)
	rps := make([]riderPower, len(riders))
	for i, rs := range riders {
		basePower := rs.CurrentFTP()
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
		// (a dangerous rider up the road, sprint teams in the finale, or any
		// break still away in the final phase).
		committedChase := action == ActionChase && state != nil &&
			(state.BreakawayHasDangerous || state.SprintChaseActive || (state.HasBreakaway && state.Phase == PhaseFinal))
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

		// Team strategy: when this terrain favours the team's leader over the
		// group's fastest finisher, domestiques drive the pace (a committed
		// effort funded by W') to make the race hard and shed the sprinters.
		if rs.TeamRole == RoleDomestique && driving[rs.Rider.Team] && action != ActionConserve && action != ActionAttack {
			r := rs.Rider
			est := SegmentTime(SpeedFromPower(power, r.TotalMassKg(), r.CdA, segment.GradientPct, segment.SurfaceType, r.Handling), segment.DistanceKm)
			power = EffectivePower(r, rs.CurrentFTP(), segment, false) + rs.WPrime.AvailablePowerAboveFTP(est)
			action = ActionChase
		}
		if state != nil && state.LastActions != nil {
			state.LastActions[rs.Rider.ID] = action
		}

		// W' depletion limits power
		if rs.WPrime.FractionRemaining() < 0.1 {
			// Can't sustain above FTP when W' is very low
			maxPower := rs.CurrentFTP()
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

// drivingTeams returns the teams that want to make this segment hard: their
// leader is present and a pace at the leader's threshold would put the
// group's fastest finisher at their limit, even sitting in the draft. On flat
// roads drafting makes that impossible, so nobody drives; on climbs W/kg wins.
func drivingTeams(riders []*RiderState, segment *model.Segment, state *RaceState) map[string]bool {
	out := map[string]bool{}
	if state == nil || state.Phase == PhaseEarly || len(riders) < 3 {
		return out
	}
	var finisher *RiderState
	for _, rs := range riders {
		if finisher == nil || SprintPower(rs.Rider) > SprintPower(finisher.Rider) {
			finisher = rs
		}
	}
	f := finisher.Rider
	finisherFTP := EffectivePower(f, finisher.CurrentFTP(), segment, false)
	for _, rs := range riders {
		if rs.TeamRole != RoleLeader || rs.Rider.Team == f.Team {
			continue
		}
		r := rs.Rider
		pace := SpeedFromPower(EffectivePower(r, rs.CurrentFTP(), segment, false),
			r.TotalMassKg(), r.CdA, segment.GradientPct, segment.SurfaceType, r.Handling)
		segTime := SegmentTime(pace, segment.DistanceKm)
		required := powerForSpeed(pace, f.TotalMassKg(), f.CdA*draftFor(3, segment.SurfaceType),
			segment.GradientPct, segment.SurfaceType, f.Handling)
		available := finisherFTP + finisher.WPrime.AvailablePowerAboveFTP(segTime)
		if required >= available*(1-driveMargin) {
			out[r.Team] = true
		}
	}
	return out
}

// driveMargin: a team drives when its leader's pace would take the finisher to
// within this fraction of their limit (they may crack with fatigue or surges).
const driveMargin = 0.03
