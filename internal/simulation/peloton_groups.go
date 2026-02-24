package simulation

import (
	"fmt"
	"math/rand"
	"sort"

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
		power *= TacticsPowerMultiplier(action, *params)
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
		}
	}

	if segment.GradientPct <= 5.0 {
		// Flat/rolling: split into breakaway group and peloton
		sort.Slice(rps, func(i, j int) bool { return rps[i].power > rps[j].power })

		var breakawayRPs []riderPower
		var pelotonRPs []riderPower

		// First pass: identify which leaders/sprinters are in the breakaway
		breakawayLeaderTeams := make(map[string]bool)
		for _, rp := range rps {
			if rp.breakaway && (rp.state.TeamRole == RoleLeader || rp.state.TeamRole == RoleSprinter) {
				breakawayLeaderTeams[rp.state.Rider.Team] = true
			}
		}

		for _, rp := range rps {
			if rp.breakaway && len(breakawayRPs) < maxBreakawaySize {
				// Domestiques don't join a breakaway that contains their own leader
				if rp.state.TeamRole == RoleDomestique && breakawayLeaderTeams[rp.state.Rider.Team] {
					pelotonRPs = append(pelotonRPs, rp)
					continue
				}
				breakawayRPs = append(breakawayRPs, rp)
			} else {
				pelotonRPs = append(pelotonRPs, rp)
			}
		}

		// If no breakaway riders, everyone stays in the peloton (or splits on cobbles)
		if len(breakawayRPs) == 0 {
			if segment.SurfaceType == model.SurfaceCobbles {
				// On cobbles with no explicit breakaway, the peloton still splinters.
				// The strongest cobbles rider sets the pace; others drop if they can't
				// sustain it with reduced drafting (cobbles break up draft lines).
				// Sort by cobbles-adjusted power: raw power / HandlingFactor so that
				// handlers who are efficient on pavé lead the group.
				sort.Slice(rps, func(i, j int) bool {
					hfi := HandlingFactor(rps[i].state.Rider.Handling)
					hfj := HandlingFactor(rps[j].state.Rider.Handling)
					return rps[i].power/hfi > rps[j].power/hfj
				})
				cobbleLeader := rps[0]
				cobbleLeaderSpeed := SpeedFromPower(cobbleLeader.power, cobbleLeader.state.Rider.TotalMassKg(),
					cobbleLeader.state.Rider.CdA, segment.GradientPct, segment.SurfaceType, cobbleLeader.state.Rider.Handling)

				var mainGroup []*riderPower
				var droppedGroups []*Group
				for _, rp := range rps {
					// Cobbles draft is severely limited — riders must mostly ride on their own.
					cobbleDraft := 0.85
					requiredPower := powerForSpeed(cobbleLeaderSpeed, rp.state.Rider.TotalMassKg(),
						rp.state.Rider.CdA*cobbleDraft, segment.GradientPct, segment.SurfaceType, rp.state.Rider.Handling)
					if rp.power >= requiredPower*0.95 {
						rp := rp
						mainGroup = append(mainGroup, &rp)
					} else {
						droppedSpeed := SpeedFromPower(rp.power, rp.state.Rider.TotalMassKg(),
							rp.state.Rider.CdA, segment.GradientPct, segment.SurfaceType, rp.state.Rider.Handling)
						droppedGroup := &Group{ID: len(droppedGroups) + 1, Speed: droppedSpeed, Riders: []*RiderState{rp.state}}
						rp.state.GroupID = droppedGroup.ID
						rp.state.GroupPosition = 1
						rp.state.Dropped = true
						droppedGroups = append(droppedGroups, droppedGroup)
					}
				}
				if len(mainGroup) == 0 {
					mainGroup = []*riderPower{&rps[0]}
				}
				frontGroup := &Group{ID: 0, Speed: cobbleLeaderSpeed, Riders: make([]*RiderState, len(mainGroup))}
				for i, rp := range mainGroup {
					rp.state.GroupID = 0
					rp.state.GroupPosition = i + 1
					frontGroup.Riders[i] = rp.state
				}
				result := []*Group{frontGroup}
				result = append(result, droppedGroups...)
				return result
			}

			group := &Group{ID: 0, Riders: make([]*RiderState, len(rps))}
			for i, rp := range rps {
				rp.state.GroupID = 0
				group.Riders[i] = rp.state
			}
			assignTeamPositions(rps)
			leader := rps[0]
			leaderSpeed := SpeedFromPower(leader.power, leader.state.Rider.TotalMassKg(),
				leader.state.Rider.CdA, segment.GradientPct, segment.SurfaceType, leader.state.Rider.Handling)
			group.Speed = leaderSpeed
			return []*Group{group}
		}

		// Breakaway group (rides at front, reduced drafting)
		breakGroup := &Group{ID: 0, Riders: make([]*RiderState, len(breakawayRPs)), IsBreakaway: true}
		for i, rp := range breakawayRPs {
			rp.state.GroupID = 0
			rp.state.GroupPosition = i + 1
			breakGroup.Riders[i] = rp.state
		}
		// Breakaway speed: strongest rider sets pace (less draft benefit in small group)
		breakLeader := breakawayRPs[0]
		breakSpeed := SpeedFromPower(breakLeader.power, breakLeader.state.Rider.TotalMassKg(),
			breakLeader.state.Rider.CdA, segment.GradientPct, segment.SurfaceType, breakLeader.state.Rider.Handling)
		breakGroup.Speed = breakSpeed

		// Peloton group with dropping logic on technical sections
		groups := []*Group{breakGroup}

		if len(pelotonRPs) == 0 {
			return groups // Only breakaway, no peloton
		}

		pelotonLeader := pelotonRPs[0]
		pelotonSpeed := SpeedFromPower(pelotonLeader.power, pelotonLeader.state.Rider.TotalMassKg(),
			pelotonLeader.state.Rider.CdA, segment.GradientPct, segment.SurfaceType, pelotonLeader.state.Rider.Handling)

		// On technical sections (cobbles), apply dropping for riders who can't maintain pace
		// This allows specialization to matter: riders with poor handling naturally get dropped
		if segment.SurfaceType == model.SurfaceCobbles {
			// Technical terrain: riders with insufficient power get dropped.
			// Draft efficiency on cobbles is severely limited (0.85 vs 0.65 on smooth road)
			// because riders must constantly adapt line and can't sit smoothly in a wheel.
			var mainPeloton []*riderPower
			droppedGroups := make([]*Group, 0)

			for _, rp := range pelotonRPs {
				// Calculate power needed to stay with peloton (less draft benefit on cobbles)
				draftFactor := 0.85
				requiredPower := powerForSpeed(pelotonSpeed, rp.state.Rider.TotalMassKg(),
					rp.state.Rider.CdA*draftFactor, segment.GradientPct, segment.SurfaceType, rp.state.Rider.Handling)

				// Drop riders who can't sustain 95% of group pace (cobbles are unforgiving)
				if rp.power >= requiredPower*0.95 {
					mainPeloton = append(mainPeloton, &rp)
				} else {
					// Dropped — create own group at their speed
					droppedSpeed := SpeedFromPower(rp.power, rp.state.Rider.TotalMassKg(),
						rp.state.Rider.CdA, segment.GradientPct, segment.SurfaceType, rp.state.Rider.Handling)
					droppedGroup := &Group{ID: len(groups) + len(droppedGroups), Speed: droppedSpeed, Riders: []*RiderState{rp.state}}
					rp.state.GroupID = droppedGroup.ID
					rp.state.GroupPosition = 1
					rp.state.Dropped = true
					droppedGroups = append(droppedGroups, droppedGroup)
				}
			}

			// Main peloton (or remaining peloton after drops)
			if len(mainPeloton) > 0 {
				pelotonGroup := &Group{ID: 1, Speed: pelotonSpeed, Riders: make([]*RiderState, len(mainPeloton))}
				for i, rp := range mainPeloton {
					rp.state.GroupID = 1
					rp.state.GroupPosition = i + 1
					pelotonGroup.Riders[i] = rp.state
				}
				groups = append(groups, pelotonGroup)
			}
			groups = append(groups, droppedGroups...)
		} else {
			// Non-technical: peloton stays together with team-aware positioning
			pelotonGroup := &Group{ID: 1, Riders: make([]*RiderState, len(pelotonRPs)), Speed: pelotonSpeed}
			for i, rp := range pelotonRPs {
				rp.state.GroupID = 1
				pelotonGroup.Riders[i] = rp.state
			}
			assignTeamPositions(pelotonRPs)
			groups = append(groups, pelotonGroup)
		}

		return groups
	}

	// Climbing: split by W/kg — drafting benefit is minimal on climbs
	sort.Slice(rps, func(i, j int) bool { return rps[i].wpw > rps[j].wpw })

	var groups []*Group
	currentGroup := &Group{ID: 0}

	for i, rp := range rps {
		if i == 0 {
			rp.state.GroupID = 0
			rp.state.GroupPosition = 1
			currentGroup.Riders = append(currentGroup.Riders, rp.state)

			// Leader's speed on the climb
			pacePower := rp.power
			// Climb pacing: groups rarely ride at the absolute max power of the single
			// strongest rider for an entire segment; this was creating unrealistic
			// multi-minute gaps to 4th/5th on summit finishes.
			pacePower *= 0.93
			speed := SpeedFromPower(pacePower, rp.state.Rider.TotalMassKg(),
				rp.state.Rider.CdA, segment.GradientPct, segment.SurfaceType, rp.state.Rider.Handling)
			currentGroup.Speed = speed
			continue
		}

		// Can this rider stay with the current group?
		// On climbs, calculate their individual speed and compare
		riderSpeed := SpeedFromPower(rp.power, rp.state.Rider.TotalMassKg(),
			rp.state.Rider.CdA, segment.GradientPct, segment.SurfaceType, rp.state.Rider.Handling)

		// Allow a small drafting benefit even on climbs (reduced)
		climbDraftFactor := 0.92 // enough to keep groups together longer
		requiredPower := powerForSpeed(currentGroup.Speed, rp.state.Rider.TotalMassKg(),
			rp.state.Rider.CdA*climbDraftFactor, segment.GradientPct, segment.SurfaceType, rp.state.Rider.Handling)

		// On climbs, riders can (briefly) exceed FTP by spending W'. Without this,
		// the model shatters too early and produces multi-minute GC gaps.
		segTimeAtPace := SegmentTime(currentGroup.Speed, segment.DistanceKm)
		wPrimeBurst := segTimeAtPace
		if wPrimeBurst > 600 {
			wPrimeBurst = 600
		}
		followPower := rp.power + rp.state.WPrime.AvailablePowerAboveFTP(wPrimeBurst)
		if followPower >= requiredPower*0.99 {
			// Can stay with group
			rp.state.GroupID = currentGroup.ID
			rp.state.GroupPosition = len(currentGroup.Riders) + 1
			currentGroup.Riders = append(currentGroup.Riders, rp.state)
		} else {
			// Dropped — start new group
			groups = append(groups, currentGroup)
			newID := len(groups)
			currentGroup = &Group{ID: newID, Speed: riderSpeed}
			rp.state.GroupID = newID
			rp.state.GroupPosition = 1
			rp.state.Dropped = true
			currentGroup.Riders = append(currentGroup.Riders, rp.state)
		}
	}
	groups = append(groups, currentGroup)

	return groups
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
