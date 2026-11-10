package simulation

import (
	"fmt"
	"math"
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

	driving := drivingTeams(riders, segment, state)
	// Shelter needs teammates: a protected rider with no domestique in the
	// group has to take turns like everyone else.
	helpers := map[string]int{}
	for _, rs := range riders {
		if rs.TeamRole == RoleDomestique {
			helpers[rs.Rider.Team]++
		}
	}
	// A leader with a rival up the road is chased for by their domestiques in
	// the group; the leader sits in while they have them (see shelteredFor).
	teamChase := map[string]bool{}
	for _, rs := range riders {
		if rs.TeamRole == RoleLeader && state.rivalAhead(rs) {
			teamChase[rs.Rider.Team] = true
		}
	}
	// Stage races: a team keeps its domestiques for today only if it needs
	// them: a sprinter who can contest the finish, or a leader who is a
	// favourite or GC contender. Other teams' riders are free to go in the
	// break (sprint teams on a mountain stage, a team out of contention).
	needsHelp := map[string]bool{}
	if state != nil && state.IsStageRace {
		for _, rs := range riders {
			switch {
			case rs.TeamRole == RoleSprinter && len(state.Course) > 0 && state.sprinterCanFollow(rs, riders):
				needsHelp[rs.Rider.Team] = true
			case rs.TeamRole == RoleLeader && (state.isDangerous(rs) || state.gcContender(rs)):
				needsHelp[rs.Rider.Team] = true
			}
		}
	}
	// Stage races: interested teams' domestiques chase the groups ahead to a
	// plan (chase.go) instead of everyone chasing by rule.
	plan := state.planChase(riders)
	rps := make([]riderPower, len(riders))
	for i, rs := range riders {
		var power float64
		hasLeader := leaderTeams[rs.Rider.Team]
		if state != nil && state.IsStageRace {
			hasLeader = needsHelp[rs.Rider.Team]
		}
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
		chaseAt := 0.0
		if plan.chasing(rs, helpers) && action != ActionAttack {
			action, chaseAt = ActionChase, plan.intensity
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
		power, action = riderEffort(rs, action, segment, remainingDistance, state, driving[rs.Rider.Team], leaderTeams[rs.Rider.Team], teamChase[rs.Rider.Team], chaseAt, *params)
		if state != nil && state.Overrides != nil {
			if effortMultiplier, ok := state.Overrides.EffortMultiplier(rs.Rider.ID); ok {
				power *= effortMultiplier
			}
		}
		if state != nil && state.LastActions != nil {
			state.LastActions[rs.Rider.ID] = action
		}

		// W' depletion limits power
		if rs.WPrime.FractionRemaining() < 0.1 {
			// Can't sustain above FTP when W' is very low
			maxPower := EffectivePower(rs.Rider, rs.CurrentFTP(), segment, false)
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
			sheltered: shelteredFor(rs, action, state) && (action == ActionConserve || helpers[rs.Rider.Team] > 0),
		}
	}

	// A leader chasing a rival sits in while the team does the chasing. When
	// the teammates here can't stop the rival's group gaining and the leader
	// would be faster alone (flat-terrain domestiques on a climb), the leader
	// goes to the front.
	for i, rp := range rps {
		if rp.sheltered && rp.action == ActionChase && rp.state.TeamRole == RoleLeader {
			var mates []*RiderState
			for _, o := range rps {
				if o.state != rp.state && o.state.Rider.Team == rp.state.Rider.Team && !o.sheltered {
					mates = append(mates, o.state)
				}
			}
			if state.leaderGoes(rp.state, mates) {
				rps[i].sheltered = false
			}
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
	// An attack is let go (it rides away as its own group) only when it
	// doesn't threaten the race. In the finale, or when a dangerous rider
	// goes, the group responds at once: the attack sets the pace and whoever
	// can hold the wheel stays with it (followSplit decides physically).
	finale := state != nil && state.Phase == PhaseFinal
	threatening := func(rp riderPower) bool {
		return finale || state.isDangerous(rp.state) || state.gcContender(rp.state)
	}
	var attackers, others []riderPower
	for _, rp := range rps {
		// Domestiques don't join a move that contains their own leader.
		joins := rp.breakaway && !threatening(rp) && len(attackers) < maxBreakawaySize &&
			!(rp.state.TeamRole == RoleDomestique && breakawayLeaderTeams[rp.state.Rider.Team])
		if joins {
			attackers = append(attackers, rp)
		} else {
			others = append(others, rp)
		}
	}
	var groups []*Group
	if len(attackers) > 0 && len(others) > 0 {
		// Riders who want the break today go with the move if they can hold
		// the wheel (state.WantsBreak); those who can't stay in the bunch.
		move := attackers
		var bunch []riderPower
		for _, rp := range others {
			if state != nil && state.WantsBreak[rp.state.Rider.ID] && rp.action != ActionChase &&
				!state.BreakawayRiders[rp.state.Rider.ID] {
				move = append(move, rp)
			} else {
				bunch = append(bunch, rp)
			}
		}
		for i, g := range followSplit(move, segment, 0, false, true) {
			var kept []*RiderState
			for _, rs := range g.Riders {
				if i > 0 && !riderPowerOf(rs, move).breakaway {
					bunch = append(bunch, riderPowerOf(rs, move)) // couldn't hold it
				} else {
					kept = append(kept, rs)
				}
			}
			if len(kept) == 0 {
				continue
			}
			g.Riders, g.ID, g.IsBreakaway = kept, len(groups), true
			for _, rs := range kept {
				rs.GroupID = g.ID
			}
			groups = append(groups, g)
		}
		others = bunch
	} else {
		others = rps
	}
	// A group behind a break chases cooperatively.
	chasing := false
	if state != nil && state.HasBreakaway {
		for _, rp := range others {
			if !state.BreakawayRiders[rp.state.Rider.ID] {
				chasing = true
				break
			}
		}
	}
	if len(others) == 0 {
		return groups
	}
	return append(groups, followSplit(others, segment, len(groups), chasing, finale)...)
}

func riderPowerOf(rs *RiderState, rps []riderPower) riderPower {
	for _, rp := range rps {
		if rp.state == rs {
			return rp
		}
	}
	return riderPower{state: rs}
}

// assignTeamPositions reorders riders in a group so domestiques ride at the front
// (low position = more wind) to shelter their team's leader/sprinter.
// Only applies on flat/rolling terrain where drafting matters.
func assignTeamPositions(rps []riderPower, crosswind bool) {
	if len(rps) == 1 {
		rps[0].state.GroupPosition = 1 // alone in the wind
	}
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

	// Order: domestiques at front (in the wind), then other riders, then
	// protected riders (sheltered deep in the bunch). In a crosswind the deep
	// bunch is the gutter, so each team rides as a unit with its protected
	// riders right behind a couple of domestiques (see crosswindTeamOrder).
	order := [][]riderPower{frontDomestiques, others, protectedRiders}
	if crosswind {
		order = crosswindTeamOrder(frontDomestiques, protectedRiders, others)
	}
	pos := 1
	for _, part := range order {
		for _, rp := range part {
			rp.state.GroupPosition = pos
			pos++
		}
	}
}

// drivingTeams returns the teams that want to make this segment hard, each
// with the leader they ride for: their leader is present and the team's tempo
// (the leader's threshold scaled by the phase intensity: a tempo far from the
// finish, flat out in the finale) would put the group's fastest finisher at
// their limit, even sitting in the draft. On flat roads drafting makes that impossible, so
// nobody drives; on climbs W/kg wins.
func drivingTeams(riders []*RiderState, segment *model.Segment, state *RaceState) map[string]*RiderState {
	out := map[string]*RiderState{}
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
		if state.tempoDrops(rs, finisher, segment, finisherFTP) {
			r := rs.Rider
			if cur := out[r.Team]; cur == nil || climbingWKg(r) > climbingWKg(cur.Rider) {
				out[r.Team] = rs // co-leaders: ride for the strongest
			}
		}
	}
	return out
}

// leaderTempoSpeed is the speed at which a leader sitting second wheel rides
// at the given fraction of threshold on this segment.
func leaderTempoSpeed(leader *RiderState, segment *model.Segment, intensity float64) float64 {
	r := leader.Rider
	threshold := EffectivePower(r, leader.CurrentFTP(), segment, false) * intensity
	cda := r.CdA * draftFor(2, segment.SurfaceType)
	lo, hi := minSpeedMPS, SpeedCap(r, segment)
	if segmentPower(hi, r.TotalMassKg(), cda, segment, r.Handling) <= threshold {
		return hi
	}
	for i := 0; i < 50; i++ {
		mid := (lo + hi) / 2
		if segmentPower(mid, r.TotalMassKg(), cda, segment, r.Handling) < threshold {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

// driveMargin: a team drives when its leader's pace would take the finisher to
// within this fraction of their limit (they may crack with fatigue or surges).
const driveMargin = 0.03

// leadOutKm is where domestiques stop pacing and empty themselves for their
// leader or sprinter.
const leadOutKm = 3.0

// riderEffort decides how hard a rider rides this step. Each mode draws on W'
// once, over one horizon: a burst spends it over this step, racing to the
// line spreads it over the time left, and steady riding doesn't touch it.
func riderEffort(rs *RiderState, action Action, segment *model.Segment, remainingKm float64, state *RaceState, driveFor *RiderState, protectedTeammate, teamChase bool, chaseAt float64, params TacticsParams) (float64, Action) {
	r := rs.Rider
	threshold := EffectivePower(r, rs.CurrentFTP(), segment, false)
	burstOver := func(km float64) float64 {
		v := segmentSpeed(threshold, r.TotalMassKg(), r.CdA, segment, r.Handling)
		return threshold + rs.WPrime.AvailablePowerAboveFTP(SegmentTime(v, km))
	}
	phase := PhaseEarly
	if state != nil {
		phase = state.Phase
	}
	steady := func() float64 {
		if phase == PhaseFinal {
			return threshold
		}
		// Stage races: with a break up the road and nobody chasing it to
		// a plan, the bunch rides easy (chasers return earlier, see
		// chaseAt). A group dropped behind the main field rides its tempo
		// (not easy). In a one-day race the race is the result: the bunch
		// keeps racing for placings.
		if state != nil && state.IsStageRace && state.HasBreakaway && !state.BreakawayRiders[r.ID] && !state.behindMainField() {
			return threshold * easyIntensity
		}
		return threshold * IntensityForPhase(phase)
	}

	// A rival up the road is chased to the line: by the leader's domestiques
	// when the team has them here (the leader sits in, see shelteredFor),
	// otherwise by the rider themself.
	if state.rivalAhead(rs) || (teamChase && rs.TeamRole == RoleDomestique) {
		return burstOver(remainingKm), ActionChase
	}
	// A leader clear of their rivals who can hold it commits to the line.
	if state.escapeHolds(rs) {
		return burstOver(remainingKm), ActionChase
	}
	// A planned chase rides at the plan's intensity (all in: to the line).
	if chaseAt > 0 {
		if chaseAt > chaseAllIn {
			return burstOver(remainingKm), ActionChase
		}
		return math.Min(threshold*chaseAt, burstOver(remainingKm)), ActionChase
	}
	// Stage races: a break with a group behind rides together to stay away,
	// at the pace its riders can sustain to the finish.
	if state != nil && state.IsStageRace && state.BreakawayRiders[r.ID] && len(state.Chasers) > 0 &&
		action != ActionAttack && len(state.Road) > 0 && len(state.Course) > 0 {
		hours := state.timeToLine(topLine(state.Road)) / 3600
		return threshold * sustainableFraction(hours), ActionChase
	}
	if action == ActionConserve {
		return steady() * params.ConservePowerPenalty, action
	}
	segmentLeft := segment.DistanceKm
	if state != nil && state.SegmentKmLeft > segmentLeft {
		segmentLeft = state.SegmentKmLeft
	}
	// Attacks are bursts: W' spent over this step.
	if action == ActionAttack {
		return burstOver(segment.DistanceKm), action
	}
	// Domestiques of a driving team set the pace their leader can hold on the
	// wheel at the phase's intensity (a tempo mid-race, threshold in the
	// finale): hard for everyone weaker, comfortable for the leader. They
	// can't exceed what they can sustain to the top.
	if driveFor != nil && rs.TeamRole == RoleDomestique {
		v := leaderTempoSpeed(driveFor, segment, IntensityForPhase(phase))
		need := segmentPower(v, r.TotalMassKg(), r.CdA, segment, r.Handling)
		return math.Min(need, burstOver(segmentLeft)), ActionChase
	}
	// Committed chases and racing to the line are sustained to the finish.
	// A committed chase needs something to chase: a dangerous rider up the
	// road, or a break in the finale (sprint teams included). Without a break,
	// "chasing" teams just control the tempo.
	committedChase := action == ActionChase && state != nil && state.HasBreakaway &&
		(state.BreakawayHasDangerous || state.SprintChaseActive || phase == PhaseFinal)
	if committedChase {
		return burstOver(remainingKm), action
	}
	if phase == PhaseFinal {
		// A domestique riding for a teammate holds a controlled tempo and only
		// empties himself in the lead-out; everyone else races to the line.
		if rs.TeamRole == RoleDomestique && protectedTeammate && remainingKm > leadOutKm {
			return threshold * TacticsPowerMultiplier(action, params), action
		}
		return burstOver(remainingKm), action
	}
	return steady() * TacticsPowerMultiplier(action, params), action
}

func phaseOf(state *RaceState) RacePhase {
	if state == nil {
		return PhaseEarly
	}
	return state.Phase
}

// crosswindTeamOrder lines teams up for an echelon: strongest teams first,
// each with up to two domestiques directly ahead of its protected riders;
// remaining domestiques and unattached riders follow.
func crosswindTeamOrder(domestiques, protected, others []riderPower) [][]riderPower {
	byTeam := map[string][]riderPower{}
	strength := map[string]float64{}
	var teams []string
	for _, rp := range protected {
		t := rp.state.Rider.Team
		if _, ok := byTeam[t]; !ok {
			teams = append(teams, t)
		}
		byTeam[t] = append(byTeam[t], rp)
	}
	domByTeam := map[string][]riderPower{}
	for _, rp := range domestiques {
		t := rp.state.Rider.Team
		domByTeam[t] = append(domByTeam[t], rp)
		strength[t] += rp.power
	}
	sort.SliceStable(teams, func(i, j int) bool { return strength[teams[i]] > strength[teams[j]] })
	var front, rest []riderPower
	for _, t := range teams {
		doms := domByTeam[t]
		n := len(doms)
		if n > 2 {
			n = 2
		}
		front = append(front, doms[:n]...)
		front = append(front, byTeam[t]...)
		rest = append(rest, doms[n:]...)
	}
	return [][]riderPower{front, rest, others}
}
