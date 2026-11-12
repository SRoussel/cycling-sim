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
	state.applyPaceOrders(driving, riders)
	squad := driveSquads(riders, driving, segment, state)
	sendToBreak := state.breakPicks(riders)
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
	// Protect orders: a dropped leader is paced back by the domestiques with
	// them; those up the road wait.
	protectBack := state.protectChase(riders)
	for t := range protectBack {
		teamChase[t] = true
	}
	waiting := state.protectWaits(riders)
	// Lead-out orders: a team told not to lead out races the finale rider
	// by rider.
	for t := range leaderTeams {
		if state.teamOrders(t).Leadout == OrderNo {
			leaderTeams[t] = false
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
		commanded := false // the player gave this rider a command
		if state != nil && state.Overrides != nil {
			if a, ok := state.Overrides.ConsumeOneShot(rs.Rider.ID); ok {
				action, commanded = a, true
			} else if a, ok := state.Overrides.ActionOverride(rs.Rider.ID); ok {
				action, commanded = a, true
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
		// Break orders: riders sent after the break keep trying until a
		// teammate is in it; a team told to stay together doesn't go.
		if !commanded && rs.TeamRole != RoleLeader {
			switch {
			case sendToBreak[rs.Rider.ID] && (action == ActionHold || action == ActionConserve) &&
				rng.Float64() < ScaleToDistance(orderedBreakChance, segment.DistanceKm):
				action = ActionAttack
			case state.teamOrders(rs.Rider.Team).Break == OrderNo && action == ActionAttack:
				action = ActionHold
			}
		}
		chaseAt := 0.0
		if plan.chasing(rs, helpers) && action != ActionAttack {
			action, chaseAt = ActionChase, plan.intensity
		}
		// A team ordered not to chase: its domestiques sit in (a rider's own
		// command still wins).
		holdBack := rs.TeamRole == RoleDomestique && state.teamOrders(rs.Rider.Team).Chase == OrderNo && !commanded
		if holdBack && action == ActionChase {
			action = ActionHold
		}
		if state != nil {
			if state.LastActions == nil {
				state.LastActions = make(map[int64]Action)
			}
			previousAction := state.LastActions[rs.Rider.ID]
			if action == ActionAttack && previousAction != ActionAttack && state.Overrides != nil {
				// key: a rider the race reacts to (a favourite, a GC
				// contender or a team leader); the app pauses only for these.
				key := 0
				if state.isDangerous(rs) || state.gcContender(rs) || rs.TeamRole == RoleLeader {
					key = 1
				}
				state.LogEvent("attack", fmt.Sprintf("rider=%d; key=%d; name=%s; team=%s", rs.Rider.ID, key, rs.Rider.Name, rs.Rider.Team))
				last := &state.Events[len(state.Events)-1]
				last.RiderID, last.Key = rs.Rider.ID, key == 1
			}
			state.LastActions[rs.Rider.ID] = action
		}
		driveFor := driving[rs.Rider.Team]
		if squad != nil && rs.TeamRole == RoleDomestique && !squad[rs] {
			driveFor = nil // sheltered in the bunch, saved for later
		}
		power, action = riderEffort(rs, action, segment, remainingDistance, state, driveFor, leaderTeams[rs.Rider.Team], teamChase[rs.Rider.Team] && !holdBack, holdBack, chaseAt, *params)
		if protectBack[rs.Rider.Team] && !commanded && action != ActionAttack && action != ActionChase {
			if l, _ := state.protectedLeader(rs.Rider.Team); l == rs {
				// The dropped leader races back with the team (see leaderGoes).
				power, action = burstPower(rs, segment, remainingDistance), ActionChase
			}
		}
		if waiting[rs.Rider.ID] && !commanded {
			// Sits up for the leader behind.
			power = EffectivePower(rs.Rider, rs.CurrentFTP(), segment, false) * easyIntensity
			action = ActionConserve
		}
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
	var attackers, others, waiters []riderPower
	for _, rp := range rps {
		if waiting[rp.state.Rider.ID] && rp.action == ActionConserve {
			waiters = append(waiters, rp) // ride on as their own group
			continue
		}
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
			if state != nil && state.wantsBreak(rp.state, sendToBreak) && rp.action != ActionChase &&
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
	groups = append(groups, followSplit(others, segment, len(groups), chasing, finale)...)
	return append(groups, followSplit(waiters, segment, len(groups), false, false)...)
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
	// Early in a race nobody drives: the break goes and the bunch eases off;
	// the race gets hard later (in classics, driving from km 0 split the
	// field 150 km out).
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
		// One-day races: a favourite's team wears the field down on cobbles
		// and climbs when its leader is faster over the rest of the course
		// than the group's fastest finisher (the same judgement as pressing):
		// on a sector the draft protects the finisher this time, but not
		// over the whole race.
		hard := segment.SurfaceType == model.SurfaceCobbles || climbOf(segment) >= terrainClimb
		wearDown := !state.IsStageRace && hard && state.isDangerous(rs) &&
			state.timeToLine([]*RiderState{rs}) < state.timeToLine([]*RiderState{finisher})
		if wearDown || state.tempoDrops(rs, finisher, segment, finisherFTP) {
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
func riderEffort(rs *RiderState, action Action, segment *model.Segment, remainingKm float64, state *RaceState, driveFor *RiderState, protectedTeammate, teamChase, holdBack bool, chaseAt float64, params TacticsParams) (float64, Action) {
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
		if state != nil && (state.IsStageRace || state.calmBunch()) && state.HasBreakaway && !state.BreakawayRiders[r.ID] && !state.behindMainField() {
			return threshold * easyIntensity
		}
		return threshold * IntensityForPhase(phase)
	}

	// A rival up the road is chased to the line: by the leader's domestiques
	// when the team has them here (the leader sits in, see shelteredFor),
	// otherwise by the rider themself.
	if !holdBack && teamChase && rs.TeamRole == RoleDomestique {
		return burstOver(remainingKm), ActionChase
	}
	// Stage races: a GC contender with another team's contender up the road
	// rides to the line as hard as he can sustain: every second lost is lost
	// on GC (a dropped contender doesn't ride tempo).
	if !holdBack && state.limitsLosses(rs) {
		return burstOver(remainingKm), ActionChase
	}
	if !holdBack && state.rivalAhead(rs) {
		// One-day races: a chase of rivals is no pace line. A rider who would
		// win the sprint of his chase group sits on (why tow the others?);
		// the rest ride their threshold, each waiting for the others.
		if state != nil && !state.IsStageRace {
			if state.outsprintsChase(rs) {
				return steady(), ActionHold
			}
			return threshold, ActionChase
		}
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

// orderedBreakChance: a rider the player sent after the break tries to get
// away with this chance per 5 km (ScaleToDistance) until a teammate is in it.
const orderedBreakChance = 0.5

// breakSendCount is how many riders a team sends after the break.
const breakSendCount = 2

// breakPicks returns the riders whose team was ordered into the break and
// who go after it: while no teammate is up the road and before the finale,
// the team's breakSendCount strongest domestiques (threshold W/kg) in this
// group with at least half their W' left.
func (s *RaceState) breakPicks(riders []*RiderState) map[int64]bool {
	if s == nil || s.Overrides == nil || s.Phase == PhaseFinal {
		return nil
	}
	inBreak := map[string]bool{} // teams with a rider up the road
	for _, rs := range append(append([]*RiderState{}, s.Road...), riders...) {
		if s.BreakawayRiders[rs.Rider.ID] && !rs.DNF {
			inBreak[rs.Rider.Team] = true
		}
	}
	byTeam := map[string][]*RiderState{}
	for _, rs := range riders {
		t := rs.Rider.Team
		if rs.TeamRole != RoleDomestique || rs.DNF || inBreak[t] || s.BreakawayRiders[rs.Rider.ID] ||
			rs.WPrime.FractionRemaining() < 0.5 || s.teamOrders(t).Break != OrderYes {
			continue
		}
		byTeam[t] = append(byTeam[t], rs)
	}
	out := map[int64]bool{}
	for _, cands := range byTeam {
		sort.Slice(cands, func(i, j int) bool {
			wi := float64(cands[i].CurrentFTP()) / cands[i].Rider.WeightKg
			wj := float64(cands[j].CurrentFTP()) / cands[j].Rider.WeightKg
			if wi != wj {
				return wi > wj
			}
			return cands[i].Rider.ID < cands[j].Rider.ID
		})
		for i := 0; i < len(cands) && i < breakSendCount; i++ {
			out[cands[i].Rider.ID] = true
		}
	}
	return out
}

// wantsBreak reports whether a rider goes with a move today: they decided to
// this morning (setBreakIntent) or were sent by the player, unless their
// team was told to stay together.
func (s *RaceState) wantsBreak(rs *RiderState, sent map[int64]bool) bool {
	if sent[rs.Rider.ID] {
		return true
	}
	if rs.TeamRole != RoleLeader && s.teamOrders(rs.Rider.Team).Break == OrderNo {
		return false
	}
	return s.WantsBreak[rs.Rider.ID]
}

// applyPaceOrders adjusts the driving teams to the player's pace orders: a
// team told to sit in doesn't drive; a team told to set the pace drives for
// its strongest climbing leader in the group (if it has one here).
func (s *RaceState) applyPaceOrders(driving map[string]*RiderState, riders []*RiderState) {
	if s == nil || s.Overrides == nil {
		return
	}
	for _, rs := range riders {
		t := rs.Rider.Team
		switch s.teamOrders(t).Pace {
		case OrderNo:
			delete(driving, t)
		case OrderYes:
			if rs.TeamRole != RoleLeader || rs.DNF {
				continue
			}
			if cur := driving[t]; cur == nil || climbingWKg(rs.Rider) > climbingWKg(cur.Rider) {
				driving[t] = rs
			}
		}
	}
}

// protectedLeader is the rider a team protects: its best-climbing leader
// still in the race, else its sprinter.
func (s *RaceState) protectedLeader(team string) (*RiderState, int) {
	var best *RiderState
	bestAt := -1
	for ci, c := range s.Clusters {
		for _, rs := range c {
			if rs.Rider.Team != team || rs.DNF {
				continue
			}
			better := best == nil ||
				(rs.TeamRole == RoleLeader && best.TeamRole != RoleLeader) ||
				(rs.TeamRole == best.TeamRole && climbingWKg(rs.Rider) > climbingWKg(best.Rider))
			if (rs.TeamRole == RoleLeader || rs.TeamRole == RoleSprinter) && better {
				best, bestAt = rs, ci
			}
		}
	}
	return best, bestAt
}

// clusterOf is the index of the road cluster holding riders (front first).
func (s *RaceState) clusterOf(riders []*RiderState) int {
	if len(riders) == 0 {
		return -1
	}
	for ci, c := range s.Clusters {
		for _, rs := range c {
			if rs == riders[0] {
				return ci
			}
		}
	}
	return -1
}

// protectChase returns the teams ordered to protect their leader whose
// leader is in this group, dropped behind the main field: their domestiques
// here pace the leader back.
func (s *RaceState) protectChase(riders []*RiderState) map[string]bool {
	if s == nil || s.Overrides == nil || !s.behindMainField() {
		return nil
	}
	out := map[string]bool{}
	for _, rs := range riders {
		t := rs.Rider.Team
		if out[t] || s.teamOrders(t).Protect != OrderYes {
			continue
		}
		if l, _ := s.protectedLeader(t); l == rs {
			out[t] = true
		}
	}
	return out
}

// protectWaits returns the domestiques in this group who wait for their
// protected leader in a road group behind (not riders in a break: they are
// up the road on purpose).
func (s *RaceState) protectWaits(riders []*RiderState) map[int64]bool {
	if s == nil || s.Overrides == nil {
		return nil
	}
	here := s.clusterOf(riders)
	out := map[int64]bool{}
	for _, rs := range riders {
		t := rs.Rider.Team
		if rs.TeamRole != RoleDomestique || s.BreakawayRiders[rs.Rider.ID] || s.teamOrders(t).Protect != OrderYes {
			continue
		}
		if l, at := s.protectedLeader(t); l != nil && at > here {
			out[rs.Rider.ID] = true
		}
	}
	return out
}

// burstPower is what a rider can sustain to the line: threshold plus their W'
// spread over the time left at threshold pace.
func burstPower(rs *RiderState, segment *model.Segment, remainingKm float64) float64 {
	r := rs.Rider
	threshold := EffectivePower(r, rs.CurrentFTP(), segment, false)
	v := segmentSpeed(threshold, r.TotalMassKg(), r.CdA, segment, r.Handling)
	return threshold + rs.WPrime.AvailablePowerAboveFTP(SegmentTime(v, remainingKm))
}

// driveSquads picks, for each driving team, the domestiques who ride on the
// front: the fewest whose pace line reaches the pace the team wants
// (leaderTempoSpeed at the phase's intensity), freshest first, so the work
// rotates through the team as riders tire and the rest stay sheltered for
// later, as real teams spend one or two riders at a time.
func driveSquads(riders []*RiderState, driving map[string]*RiderState, segment *model.Segment, state *RaceState) map[*RiderState]bool {
	if len(driving) == 0 {
		return nil
	}
	phase := PhaseEarly
	if state != nil {
		phase = state.Phase
	}
	out := map[*RiderState]bool{}
	for team, leader := range driving {
		var doms []*RiderState
		for _, rs := range riders {
			if rs.Rider.Team == team && rs.TeamRole == RoleDomestique && !rs.DNF {
				doms = append(doms, rs)
			}
		}
		fresh := func(rs *RiderState) float64 {
			return rs.WPrime.FractionRemaining() * rs.CurrentFTP()
		}
		sort.SliceStable(doms, func(i, j int) bool { return fresh(doms[i]) > fresh(doms[j]) })
		target := leaderTempoSpeed(leader, segment, IntensityForPhase(phase))
		var p, m, cda float64
		for k, rs := range doms {
			out[rs] = true
			r := rs.Rider
			p += EffectivePower(r, rs.CurrentFTP(), segment, false)
			m += r.TotalMassKg()
			cda += r.CdA
			n := float64(k + 1)
			if segmentSpeed(p/n, m/n, cda/n*rotationFactor(k+1, segment.SurfaceType), segment, r.Handling) >= target {
				break
			}
		}
	}
	return out
}

// calmBunch: early in a one-day race, once the early break is away and
// threatens nobody, the bunch eases off and nobody drives; the race gets
// hard later (stage races ride easy behind a break throughout, see steady).
func (s *RaceState) calmBunch() bool {
	return s != nil && !s.IsStageRace && s.Phase == PhaseEarly &&
		s.HasBreakaway && !s.BreakawayHasDangerous
}

// outsprintsChase reports whether rs is in the faster-finishing half of the
// riders in his road group chasing a rival.
func (s *RaceState) outsprintsChase(rs *RiderState) bool {
	mine := FatiguedSprintPower(rs)
	faster, chasers := 0, 0
	for _, o := range s.Road {
		if o.DNF || !s.rivalAhead(o) {
			continue
		}
		chasers++
		if o != rs && FatiguedSprintPower(o) > mine {
			faster++
		}
	}
	return chasers > 1 && faster < chasers/2
}

// limitsLosses reports whether a GC contender in a stage race has a rival
// contender (another team's) in a road group ahead.
func (s *RaceState) limitsLosses(rs *RiderState) bool {
	if s == nil || !s.IsStageRace || !s.gcContender(rs) || s.BreakawayRiders[rs.Rider.ID] {
		return false
	}
	for o := range s.Ahead {
		if !o.DNF && o.Rider.Team != rs.Rider.Team && s.gcContender(o) {
			return true
		}
	}
	return false
}
