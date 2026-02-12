package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"cycling_sim/internal/model"
)

// TeamRole represents a rider's role within their team for a race.
type TeamRole int

const (
	RoleDomestique TeamRole = iota
	RoleLeader              // best GC rider on the team
	RoleSprinter            // designated sprint finisher
)

// RiderState tracks a rider's current state during a race simulation.
type RiderState struct {
	Rider            *model.Rider
	WPrime           *WPrimeState
	FormFactor       float64  // daily form multiplier (from randomness)
	TotalTime        float64  // accumulated time in seconds
	GroupID          int      // which group the rider is in
	GroupPosition    int      // position within group
	Dropped          bool     // can't stay with group
	EffectiveFTP     int      // FTP after fatigue adjustment
	LastSegmentPower float64  // power used for the current segment (after tactics)
	DNF              bool     // did not finish (e.g., crash)
	TotalPenalty     float64  // cumulative penalty time from incidents
	TeamRole         TeamRole // role within team (leader, sprinter, domestique)
}

// Calculate each rider's sustainable power for this segment
type riderPower struct {
	state     *RiderState
	power     float64 // effective power for this segment
	wpw       float64 // watts per kg
	breakaway bool
}

// Group represents a cluster of riders traveling together.
type Group struct {
	ID          int
	Riders      []*RiderState
	Speed       float64 // group speed in m/s
	GapTime     float64 // cumulative time gap from the race leader
	IsBreakaway bool    // whether this is a breakaway group
}

// AssignTeamRoles infers team roles from rider stats.
// For each team: best GC scorer becomes Leader, best sprinter (if sprint > GC score)
// becomes Sprinter, everyone else is Domestique. Single-rider teams get Leader.
func AssignTeamRoles(states []*RiderState) {
	// Group by team
	teams := make(map[string][]*RiderState)
	for _, rs := range states {
		teams[rs.Rider.Team] = append(teams[rs.Rider.Team], rs)
	}

	for _, members := range teams {
		if len(members) == 1 {
			members[0].TeamRole = RoleLeader
			continue
		}

		// Find best GC rider
		bestGCIdx := 0
		bestGCScore := 0.0
		for i, rs := range members {
			gcScore := float64(rs.Rider.ClimbingAbility+rs.Rider.TTAbility+rs.Rider.Endurance) / 3.0
			if gcScore > bestGCScore {
				bestGCScore = gcScore
				bestGCIdx = i
			}
		}
		members[bestGCIdx].TeamRole = RoleLeader

		// Find best sprinter among remaining riders
		bestSprintIdx := -1
		bestSprintScore := 0.0
		for i, rs := range members {
			if i == bestGCIdx {
				continue
			}
			sprintScore := float64(rs.Rider.SprintAbility)
			gcScore := float64(rs.Rider.ClimbingAbility+rs.Rider.TTAbility+rs.Rider.Endurance) / 3.0
			if sprintScore > gcScore && sprintScore > bestSprintScore {
				bestSprintScore = sprintScore
				bestSprintIdx = i
			}
		}
		if bestSprintIdx >= 0 {
			members[bestSprintIdx].TeamRole = RoleSprinter
		}

		// Everyone else is domestique (default zero value)
	}
}

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
		if action == ActionAttack && state != nil && state.Overrides != nil {
			state.LogEvent("attack", fmt.Sprintf("rider=%d team=%s", rs.Rider.ID, rs.Rider.Team))
		}
		power *= TacticsPowerMultiplier(action, *params)
		if state != nil && state.Overrides != nil {
			if effortMultiplier, ok := state.Overrides.EffortMultiplier(rs.Rider.ID); ok {
				power *= effortMultiplier
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

	if segment.GradientPct <= 4.0 {
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

		// If no breakaway riders, everyone stays in the peloton
		if len(breakawayRPs) == 0 {
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
			// Technical terrain: riders with insufficient power get dropped
			var mainPeloton []*riderPower
			droppedGroups := make([]*Group, 0)

			for _, rp := range pelotonRPs {
				// Calculate power needed to stay with peloton (less draft benefit on cobbles)
				draftFactor := 0.65
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

// powerForSpeed calculates the power needed to sustain a given speed.
func powerForSpeed(speedMPS float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int) float64 {
	fGrav := ForceGravity(totalMassKg, gradientPct)
	fRoll := ForceRolling(totalMassKg, gradientPct, surface, handling)
	fAero := ForceAero(cda, speedMPS)
	return speedMPS * (fGrav + fRoll + fAero)
}

// ApplyIncidents randomly applies crashes/punctures to riders in a segment.
// Crashes are more likely on technical terrain, punctures are rare throughout.
// Probability is very low to not overwhelm race results with random bad luck.
func ApplyIncidents(riders []*RiderState, segment *model.Segment, rng *rand.Rand) {
	// Base incident probability per segment: low overall, higher on technical terrain
	baseProbability := 0.003
	if segment.IsTechnical {
		baseProbability = 0.008
	}
	if segment.SurfaceType == model.SurfaceCobbles {
		baseProbability = 0.015
	}
	if segment.GradientPct < -3.0 {
		baseProbability = 0.006
	}

	for _, rs := range riders {
		if rs.DNF {
			continue // already out of race
		}

		probability := baseProbability
		if segment.SurfaceType == model.SurfaceCobbles || segment.IsTechnical || segment.GradientPct < -3.0 {
			handlingMod := 1.0 + float64(70-rs.Rider.Handling)*0.01
			if handlingMod < 0.6 {
				handlingMod = 0.6
			}
			if handlingMod > 1.6 {
				handlingMod = 1.6
			}
			probability *= handlingMod
		}

		if rng.Float64() > probability {
			continue // no incident for this rider
		}

		// Determine incident type: 70% puncture, 30% crash
		if rng.Float64() < 0.65 {
			// Puncture: 30-60 second penalty
			penalty := 30.0 + rng.Float64()*30.0
			rs.TotalPenalty += penalty
			rs.TotalTime += penalty
		} else {
			// Crash: chance it ends race (DNF) depends on handling and terrain
			dnfChance := 0.08
			if segment.SurfaceType == model.SurfaceCobbles || segment.IsTechnical || segment.GradientPct < -3.0 {
				dnfChance = 0.15
			}
			if segment.SurfaceType == model.SurfaceCobbles {
				dnfChance = 0.18
			}
			if rs.Rider.Handling < 65 {
				dnfChance += float64(65-rs.Rider.Handling) * 0.006
			}
			if dnfChance > 0.2 {
				dnfChance = 0.2
			}
			if rng.Float64() < dnfChance {
				rs.DNF = true // severe crash, rider out
			} else {
				// Major crash penalty: 2-5 minutes + potential injury fatigue
				penalty := 120.0 + rng.Float64()*180.0
				rs.TotalPenalty += penalty
				rs.TotalTime += penalty
				// Reduce FTP temporarily from injuries
				rs.EffectiveFTP = int(float64(rs.EffectiveFTP) * 0.90)
			}
		}
	}
}

// partitionByTimeGap splits riders into road clusters based on accumulated time gaps.
// Riders who are too far apart on the road cannot regroup. Returns clusters sorted
// by leader time (fastest cluster first).
func partitionByTimeGap(riders []*RiderState, segment *model.Segment) [][]*RiderState {
	if len(riders) <= 1 {
		return [][]*RiderState{riders}
	}

	// Sort by accumulated time (road position)
	sorted := make([]*RiderState, len(riders))
	copy(sorted, riders)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TotalTime < sorted[j].TotalTime
	})

	// Gap threshold: how close riders must be to potentially regroup.
	// On flat terrain, riders within ~15s can draft back on.
	// On climbs, gaps are sticky — only very close riders regroup.
	// On descents, some natural regrouping occurs.
	gapThreshold := 45.0 // flat road: peloton draft can close moderate gaps
	if segment.SurfaceType == model.SurfaceCobbles {
		gapThreshold = 10.0 // cobbles shatter groups; gaps stick
	} else if segment.GradientPct > 4.0 {
		gapThreshold = 8.0 // gaps are hard to close on climbs, but small regrouping happens
	} else if segment.GradientPct < -2.0 {
		gapThreshold = 60.0 // descents allow significant regrouping
	}

	var clusters [][]*RiderState
	cluster := []*RiderState{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		gap := sorted[i].TotalTime - sorted[i-1].TotalTime
		if gap <= gapThreshold {
			cluster = append(cluster, sorted[i])
		} else {
			clusters = append(clusters, cluster)
			cluster = []*RiderState{sorted[i]}
		}
	}
	clusters = append(clusters, cluster)
	return clusters
}

// SimulateSegment processes all riders through a segment, returning their times.
func SimulateSegment(riders []*RiderState, segment *model.Segment, isTT bool, distanceRemaining float64, rng *rand.Rand, state *RaceState, params *TacticsParams, incidentsEnabled ...bool) {
	localParams := params
	if localParams == nil {
		defaultParams := DefaultTacticsParams()
		localParams = &defaultParams
	}
	if state != nil {
		state.KmRemaining = distanceRemaining
		state.Phase = PhaseFromKm(distanceRemaining)
		state.Segment = segment
		// Sprint trains only really matter on flat/rolling finishes.
		state.SprintChaseActive = state.Phase == PhaseFinal && state.Rival.SprintTeamsChasing && segment.GradientPct < 3.0 && segment.SurfaceType != model.SurfaceCobbles
		if state.SprintChaseActive && !state.LastSprintChase {
			state.LogEvent("sprint-chase", "sprinter teams drive the chase")
		}
		state.LastSprintChase = state.SprintChaseActive
		adjusted := ApplyEventModifiers(*localParams, state)
		localParams = &adjusted
	}

	// Apply random incidents (crashes/punctures) first, if enabled
	enableIncidents := len(incidentsEnabled) > 0 && incidentsEnabled[0]
	if enableIncidents {
		ApplyIncidents(riders, segment, rng)
	}

	// Remove DNF riders from active participation
	var activeRiders []*RiderState
	for _, r := range riders {
		if !r.DNF {
			activeRiders = append(activeRiders, r)
		}
	}

	// Partition riders into road clusters by accumulated time gaps.
	// Dropped riders can't teleport back to the front group.
	clusters := partitionByTimeGap(activeRiders, segment)

	// Form groups within each cluster independently
	var groups []*Group
	groupIDOffset := 0
	for _, cluster := range clusters {
		clusterGroups := FormGroups(cluster, segment, isTT, distanceRemaining, rng, state, localParams)
		// Offset group IDs to avoid collisions between clusters
		for _, g := range clusterGroups {
			g.ID += groupIDOffset
			for _, rs := range g.Riders {
				rs.GroupID += groupIDOffset
			}
		}
		groupIDOffset += len(clusterGroups)
		groups = append(groups, clusterGroups...)
	}

	// Calculate breakaway time gap if there's a breakaway group on flat terrain
	var breakawayGap float64
	if len(groups) >= 2 && groups[0].IsBreakaway {
		breakGroup := groups[0]
		pelotonGroup := groups[1]
		if state != nil {
			state.BreakawayHasDangerous = false
			for _, rs := range breakGroup.Riders {
				if state.Rival.DangerousRiders[rs.Rider.ID] {
					state.BreakawayHasDangerous = true
					break
				}
			}
		}

		// Average power of breakaway riders
		var breakPowerSum float64
		for _, rs := range breakGroup.Riders {
			if rs.LastSegmentPower > 0 {
				breakPowerSum += rs.LastSegmentPower
				continue
			}
			basePower := float64(rs.EffectiveFTP) * rs.FormFactor
			breakPowerSum += EffectivePower(rs.Rider, basePower, segment, false)
		}
		breakAvgPower := breakPowerSum / float64(len(breakGroup.Riders))

		// Peloton leader power (strongest rider sets chase pace)
		var pelotonLeaderPower float64
		if len(pelotonGroup.Riders) > 0 {
			rs := pelotonGroup.Riders[0]
			if rs.LastSegmentPower > 0 {
				pelotonLeaderPower = rs.LastSegmentPower
			} else {
				basePower := float64(rs.EffectiveFTP) * rs.FormFactor
				pelotonLeaderPower = EffectivePower(rs.Rider, basePower, segment, false)
			}
		}

		cohesion := BreakawayCohesion(breakGroup.Riders)
		breakawayGap = EvaluateBreakawayWithCohesion(breakAvgPower, pelotonLeaderPower,
			segment.DistanceKm, distanceRemaining, len(breakGroup.Riders), cohesion, *localParams)
		if state != nil {
			if state.BreakawayHasDangerous {
				breakawayGap /= localParams.GCChaseAggression
			}
			if state.SprintChaseActive {
				breakawayGap /= localParams.SprintTrainEfficiency
			}
		}
		if state != nil {
			if !state.BreakawayActive {
				composition := breakawayComposition(breakGroup.Riders, 6)
				state.LogEvent("breakaway", composition)
			}
			state.HasBreakaway = true
			state.BreakawayGap = breakawayGap
			state.BreakawayActive = true
		}
	} else if state != nil {
		if state.BreakawayActive {
			state.LogEvent("breakaway", "breakaway caught")
		}
		state.HasBreakaway = false
		state.BreakawayGap = 0
		state.BreakawayHasDangerous = false
		state.BreakawayActive = false
	}

	for _, group := range groups {
		var groupSpeed float64

		if segment.GradientPct < -2.0 {
			// Descent: each rider has individual speed capped by ability
			for _, rs := range group.Riders {
				basePower := float64(rs.EffectiveFTP) * rs.FormFactor * 0.5 // coast on descents
				speed := DescentSpeed(basePower, rs.Rider.TotalMassKg(),
					rs.Rider.CdA, segment.GradientPct, segment.SurfaceType,
					rs.Rider.Handling, rs.Rider.DescendingAbility, segment.IsTechnical)
				segTime := SegmentTime(speed, segment.DistanceKm)
				rs.TotalTime += segTime

				// W' recovery during descent (easy effort)
				// Use handling-based fatigue on technical terrain
				if segment.SurfaceType == model.SurfaceCobbles {
					rs.WPrime.UpdateWithHandling(basePower, rs.EffectiveFTP, segTime, rs.Rider.Handling, true)
				} else {
					rs.WPrime.Update(basePower, rs.EffectiveFTP, segTime)
				}
			}
			continue
		}

		// Normal segment: group travels together
		groupSpeed = group.Speed
		if groupSpeed < minSpeedMPS {
			groupSpeed = minSpeedMPS
		}
		segTime := SegmentTime(groupSpeed, segment.DistanceKm)

		for _, rs := range group.Riders {
			adjustedTime := segTime
			// Breakaway riders get a time bonus from the gap they've built
			if group.IsBreakaway && breakawayGap > 0 {
				adjustedTime -= breakawayGap
				if adjustedTime < segTime*0.9 {
					adjustedTime = segTime * 0.9 // cap gap gain at 10% of segment time
				}
			}
			rs.TotalTime += adjustedTime

			// Calculate actual power this rider is producing
			draftFactor := DraftFactor(rs.GroupPosition)
			if group.IsBreakaway {
				// Small breakaway group gets less draft benefit
				draftFactor = math.Max(draftFactor, 0.65)
			}
			if segment.GradientPct > 4.0 {
				draftFactor = math.Max(draftFactor, 0.85) // reduced draft on climbs
			}
			actualPower := powerForSpeed(groupSpeed, rs.Rider.TotalMassKg(),
				rs.Rider.CdA*draftFactor, segment.GradientPct, segment.SurfaceType, rs.Rider.Handling)
			if actualPower < 0 {
				actualPower = 0
			}

			rs.WPrime.UpdateWithHandling(actualPower, rs.EffectiveFTP, segTime, rs.Rider.Handling, segment.SurfaceType == model.SurfaceCobbles)
		}
	}
}

func breakawayComposition(riders []*RiderState, limit int) string {
	if len(riders) == 0 {
		return "breakaway formed"
	}
	if limit <= 0 {
		limit = 1
	}
	if len(riders) < limit {
		limit = len(riders)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, riders[i].Rider.Name)
	}
	if len(riders) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(riders)-limit))
	}
	return "breakaway: " + strings.Join(parts, ", ")
}
