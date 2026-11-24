package simulation

import (
	"math"
	"sort"

	"cycling_sim/internal/model"
)

// Escapes. A team leader races to win (or, in a stage race, to gain time on
// GC rivals), so after the early phase they go when that pays from here, judged by physics on the
// remaining course rather than by gradient or distance rules:
//
//   - attack (attackPays): from a group with rival leaders, when it drops a
//     rival worth dropping who couldn't hold the wheel at the attacker's
//     burst speed here (on flat roads the draft makes that impossible; on
//     climbs and rough roads it isn't), and riding to the line alone at
//     threshold would beat the rest chasing in a rotating pace line (rivals
//     who follow sit on);
//   - commit (escapeHolds): in a road group with no rival leader ahead and a
//     threatening one behind (at least as strong as the group's weakest
//     leader), alone or as a front selection of rivals, ride to the line
//     (threshold plus W') and take turns, while the group's projected time as
//     a pace line still beats the chase behind plus the gap.
//
// The projection is what makes Milan-San Remo different from Strade Bianche:
// a fast run-in favours the chasing pace line, long climbs and gravel favour
// the rider ahead.

// escapeEligible: the rules apply to team leaders once the race is on.
func (s *RaceState) escapeEligible(rs *RiderState) bool {
	return s != nil && s.Phase != PhaseEarly && rs.TeamRole == RoleLeader &&
		len(s.Course) > 0 && !rs.DNF
}

func rivalLeaderOf(rs, other *RiderState) bool {
	return other != rs && !other.DNF && other.TeamRole == RoleLeader && other.Rider.Team != rs.Rider.Team
}

// chaseOf returns the riders who would chase rs from a group: other teams'
// non-sprinters, strongest first, as many as share a pace line.
func chaseOf(rs *RiderState, group []*RiderState) []*RiderState {
	var out []*RiderState
	for _, c := range group {
		if c != rs && !c.DNF && c.Rider.Team != rs.Rider.Team && c.TeamRole != RoleSprinter {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CurrentFTP() > out[j].CurrentFTP() })
	if len(out) > maxRotation {
		out = out[:maxRotation]
	}
	return out
}

// attackPays reports whether a leader should attack from their road group:
// the attack drops at least one rival worth dropping (in a one-day race any
// rival leader, all of them race for the win; in a stage race a favourite or
// a GC contender) who can't hold the wheel at the attacker's burst speed
// here, and the attacker alone would ride to the line faster than the rest
// of the group chasing. Rivals who can follow don't stop the attack (they
// go with it), but they sit on rather than share the work, so the attacker
// is judged by what they can do alone.
func (s *RaceState) attackPays(rs *RiderState, seg *model.Segment) bool {
	if !s.escapeEligible(rs) || s.rivalAhead(rs) {
		return false
	}
	bestFinisher := !s.IsStageRace && !s.outFinishedByRival(rs)
	km := s.attackKm(seg)
	// holdsOver: can a rival hold an attack ridden over the last km of this
	// climb or sector (his threshold and W' against the attacker's burst)?
	holdsOver := func(o *RiderState, km float64) bool { return s.holdsAttack(rs, o, seg, km) }
	holds := func(o *RiderState) bool { return holdsOver(o, km) }
	// One-day races: attack as late on the climb as it still works. If an
	// attack over the last half of what's left would drop as many rival
	// leaders, wait (Nov 2026: favourites went from the foot of the Poggio,
	// riding 3.8 km at their burst, and the whole bunch shattered; real
	// attacks come near the top, unless only a long effort drops a punchy
	// rival).
	if !s.IsStageRace && km/2 >= fineStepKm {
		dropped := func(km float64) int {
			n := 0
			for _, o := range s.Road {
				if rivalLeaderOf(rs, o) && !holdsOver(o, km) {
					n++
				}
			}
			return n
		}
		if now := dropped(km); now > 0 && dropped(km/2) >= now {
			return false
		}
	}
	own := []*RiderState{rs}
	inOwn := map[*RiderState]bool{rs: true}
	dropsThreat := false
	for _, o := range s.Road {
		if !rivalLeaderOf(rs, o) {
			continue
		}
		if holds(o) {
			if len(own) < maxRotation {
				own = append(own, o)
				inOwn[o] = true
			}
		} else if !s.IsStageRace || s.isDangerous(o) || s.gcContender(o) {
			dropsThreat = true
		}
	}
	if !dropsThreat {
		return false
	}
	// One-day races: the best finisher among the rival leaders here wins if
	// it comes to the finish, so he waits for it while a rival could follow
	// his attack; it's for his rivals to attack him (Nov 2026: at Flèche the
	// strongest climber attacked on the first ascent of the Mur, 60 km out).
	// One who rides every rival leader off his wheel takes the sure win
	// rather than gamble on the finish (Lombardia with Pogačar: nobody could
	// hold him on the Ganda, 49 km out, and he waited for a group finish he
	// lost a third of the time); the chase projection below still decides
	// whether the attack can stay away.
	if bestFinisher && len(own) > 1 {
		return false
	}
	// In a safe front selection, an attack that a faster finisher can follow
	// only tows him to the line: wait for one that drops him.
	if s.frontSafe(rs) {
		for _, o := range own[1:] {
			if s.outFinishes(o, rs) {
				return false
			}
		}
	}
	var rest []*RiderState
	for _, o := range s.Road {
		if !inOwn[o] {
			rest = append(rest, o)
		}
	}
	chase := chaseOf(rs, rest)
	if len(chase) == 0 {
		return false
	}
	// In a front selection no chase can catch (frontSafe), the rivals left
	// behind race each other for the podium rather than chase together: the
	// attack only has to beat the strongest of them alone.
	if s.frontSafe(rs) {
		chase = chase[:1]
		return s.timeToLine([]*RiderState{rs}) < s.timeToLine(chase)
	}
	// One-day races: the group left behind chases with all its legs, as the
	// group simulation's pooled chases do (Nov 2026: against a bare threshold
	// line, favourites attacked a full bunch 64 km from Flèche's finish).
	if !s.IsStageRace {
		helpers := 0
		for _, o := range rest {
			if !o.DNF && o.Rider.Team != rs.Rider.Team {
				helpers++
			}
		}
		return s.timeToLine([]*RiderState{rs}) < s.pooledTimeToLine(chase, helpers)
	}
	return s.timeToLine([]*RiderState{rs}) < s.timeToLine(chase)
}

// pressPays reports whether a one-day favourite presses here once the race
// is on (escapeEligible: after the early phase): on a hard
// section (a cobbled sector or a real climb) a favourite who is faster over
// the rest of the course than the group's fastest finisher surges over it,
// spending W' so that rivals have to spend theirs to hold the wheel and
// can't fully recover before the next one. Not an attack meant to stay away
// (attackPays), so it doesn't depend on beating a fresh chase: attrition
// over many sections is what decides hard classics.
func (s *RaceState) pressPays(rs *RiderState, seg *model.Segment) bool {
	if !s.escapeEligible(rs) || s.IsStageRace || !s.isDangerous(rs) || s.frontSafe(rs) {
		return false
	}
	if !seg.SurfaceType.Unpaved() && climbOf(seg) < terrainClimb {
		return false
	}
	// Only a rider who'd lose the finish (the group's best finisher beats
	// him: the sprint, or the climb of an uphill finish) gains from a hard
	// race, and only against a finisher he outrides over the rest of the
	// course. Judging the finish by flat sprint power had every climber
	// pressing at Flèche (a Mur de Huy finish suits them).
	finisher := s.bestFinisher(s.Road, rs)
	if finisher == nil || !s.outFinishes(finisher, rs) {
		return false
	}
	return s.timeToLine([]*RiderState{rs}) < s.timeToLine([]*RiderState{finisher})
}

// bestFinisher is the group's strongest finisher (finishTime) other than
// except.
func (s *RaceState) bestFinisher(group []*RiderState, except *RiderState) *RiderState {
	var best *RiderState
	for _, o := range group {
		if o != except && !o.DNF && (best == nil || s.outFinishes(o, best)) {
			best = o
		}
	}
	return best
}

// escapeHolds reports whether a leader ahead of their rivals commits to the
// line with their road group.
func (s *RaceState) escapeHolds(rs *RiderState) bool {
	if !s.escapeEligible(rs) || len(s.Chasers) == 0 {
		return false
	}
	for other := range s.Ahead {
		if rivalLeaderOf(rs, other) {
			return false
		}
	}
	// The chase that matters is the nearest group behind with a rival leader
	// in it (the next group on the road may be a few dropped domestiques).
	chasers, gap := s.rivalChase(rs)
	// Only a threat is worth racing for: a rival leader behind who is at
	// least as strong as the weakest other leader in this group (a dropped
	// weaker leader is no reason for a bunch of leaders to race flat out). A
	// leader with no other leader here is ahead of all their rivals: any
	// rival behind threatens their win.
	weakest := math.Inf(1)
	for _, o := range s.Road {
		if o != rs && o.TeamRole == RoleLeader && !o.DNF && wkg(o) < weakest {
			weakest = wkg(o)
		}
	}
	if math.IsInf(weakest, 1) {
		weakest = 0
	} else {
		weakest = math.Min(weakest, wkg(rs))
	}
	threat := false
	for _, c := range chasers {
		threat = threat || (rivalLeaderOf(rs, c) && wkg(c) >= weakest)
	}
	if !threat {
		return false
	}
	chase := chaseOf(rs, chasers)
	if len(chase) == 0 {
		return false
	}
	// Ahead, rs shares the work with the rest of their road group.
	own := []*RiderState{rs}
	for _, o := range s.Road {
		if o != rs && !o.DNF && len(own) < maxRotation {
			own = append(own, o)
		}
	}
	if !s.IsStageRace {
		// The chase pools its riders' legs (pooledTimeToLine), except those
		// whose leader is up here with rs.
		here := map[string]bool{}
		for _, o := range s.Road {
			if o.TeamRole == RoleLeader {
				here[o.Rider.Team] = true
			}
		}
		helpers := 0
		for _, c := range chasers {
			if !c.DNF && !here[c.Rider.Team] {
				helpers++
			}
		}
		return s.timeToLine(own) < s.pooledTimeToLine(chase, helpers)+gap
	}
	return s.timeToLine(own) < s.timeToLine(chase)+gap
}

// rivalChase returns the nearest road group behind rs's that holds a rival
// leader, and its gap in seconds (none: nil).
func (s *RaceState) rivalChase(rs *RiderState) ([]*RiderState, float64) {
	here := s.clusterOf(s.Road)
	if here < 0 {
		return s.Chasers, s.ChaseGap
	}
	for ci := here + 1; ci < len(s.Clusters); ci++ {
		for _, c := range s.Clusters[ci] {
			if rivalLeaderOf(rs, c) {
				return s.Clusters[ci], s.Clusters[ci][0].TotalTime - s.Road[0].TotalTime
			}
		}
	}
	return nil, 0
}

type lineKey struct {
	first *RiderState
	k     int
}

// timeToLine is the time a pace line of riders needs from here to the finish
// at threshold on each segment's terrain: their mean power, mass and CdA with
// the rotation's shelter, as paceLineSpeed models a group's pace.
func (s *RaceState) timeToLine(line []*RiderState) float64 {
	key := lineKey{line[0], len(line)}
	if t, ok := s.lineTime[key]; ok {
		return t
	}
	n := float64(len(line))
	t := 0.0
	for i := s.SegIndex; i < len(s.Course); i++ {
		seg := &s.Course[i]
		km := seg.DistanceKm
		if i == s.SegIndex && s.SegmentKmLeft > 0 && s.SegmentKmLeft < km {
			km = s.SegmentKmLeft
		}
		var p, m, cda, handling float64
		for _, rs := range line {
			p += EffectivePower(rs.Rider, rs.CurrentFTP(), seg, false)
			m += rs.Rider.TotalMassKg()
			cda += rs.Rider.CdA
			handling += float64(rs.Rider.Handling)
		}
		v := segmentSpeed(p/n, m/n, cda/n*rotationFactor(len(line), seg.SurfaceType), seg, int(handling/n))
		t += SegmentTime(v, km)
	}
	if s.lineTime == nil {
		s.lineTime = map[lineKey]float64{}
	}
	s.lineTime[key] = t
	return t
}

// wkg is a rider's current threshold per kilogram.
func wkg(rs *RiderState) float64 { return rs.CurrentFTP() / rs.Rider.TotalMassKg() }

// sprintLoserAttacks: in a one-day race, once the race is on, a leader in a
// group with rival leaders who would beat him in the sprint attacks on the
// hard sections whenever he has at least as much of his reserve left as
// they do: it is his only chance, and each attack costs everyone who answers
// it. Riders who can't follow are dropped (FormGroups); repeated attacks
// wear the sprinters down.
// holdsAttack reports whether o can hold rs's attack ridden over km here
// (o on the wheel with his threshold and W' against rs's burst).
func (s *RaceState) holdsAttack(rs, o *RiderState, seg *model.Segment, km float64) bool {
	r := rs.Rider
	threshold := EffectivePower(r, rs.CurrentFTP(), seg, false)
	v0 := segmentSpeed(threshold, r.TotalMassKg(), r.CdA, seg, r.Handling)
	burst := rs.maxPowerFor(threshold, threshold, SegmentTime(v0, km))
	// Speeds as the race rides them: nobody goes faster than he dares on a
	// descent (SpeedCap), so a burst there gains only what his descending
	// does (Nov 2026: uncapped, a 300%-of-threshold "attack" on a 6% descent
	// looked like it dropped everyone, and leaders burned W' on descents).
	v := math.Min(SpeedCap(r, seg), segmentSpeed(burst, r.TotalMassKg(), r.CdA, seg, r.Handling))
	or := o.Rider
	if v > SpeedCap(or, seg) {
		return false
	}
	need := segmentPower(v, or.TotalMassKg(), or.CdA*groupDraft(2, false, seg, v), seg, or.Handling)
	thrO := EffectivePower(or, o.CurrentFTP(), seg, false)
	avail := o.maxPowerFor(thrO, thrO, SegmentTime(v, km))
	return need <= avail
}

func (s *RaceState) sprintLoserAttacks(rs *RiderState, seg *model.Segment) bool {
	if !s.escapeEligible(rs) || s.IsStageRace {
		return false
	}
	if !seg.SurfaceType.Unpaved() && climbOf(seg) < terrainClimb {
		return false
	}
	// Only an attack that drops someone who'd beat him at the finish (over
	// the rest of this climb or sector) is worth it (Nov 2026: attacks that
	// dropped nobody went on every small climb from 60 km out at Flèche and
	// split the whole bunch).
	km := s.attackKm(seg)
	faster, drops := false, false
	for _, o := range s.Road {
		if !rivalLeaderOf(rs, o) || !s.outFinishes(o, rs) {
			continue
		}
		faster = true
		if o.WPrime.FractionRemaining() > rs.WPrime.FractionRemaining() {
			return false // he has more left: wait for a better moment
		}
		drops = drops || !s.holdsAttack(rs, o, seg, km)
	}
	return faster && drops
}

// winGone: one-day races, a group behind the front of the race (which holds
// a rival leader) that can't catch it: the front's projected time to the
// line beats this group's best pace line plus the gap. The win is gone, so
// nobody here chases for it: riders ride for placings (their own tempo,
// attacks for the podium) and domestiques stop working. A chase that still
// can catch the front chases as before.
func (s *RaceState) winGone(rs *RiderState) bool {
	if s == nil || s.IsStageRace || s.Phase == PhaseEarly || len(s.Ahead) == 0 || len(s.Course) == 0 {
		return false
	}
	gap := 0.0
	for _, g := range s.Ahead {
		gap = math.Max(gap, g)
	}
	var front []*RiderState
	rival := false
	for o, g := range s.Ahead {
		if g < gap-contactSeconds || o.DNF {
			continue
		}
		front = append(front, o)
		if o.TeamRole == RoleLeader && o.Rider.Team != rs.Rider.Team {
			rival = true
		}
		if o.TeamRole == RoleLeader && o.Rider.Team == rs.Rider.Team {
			return false // our leader is winning: not our chase anyway
		}
	}
	if !rival {
		return false
	}
	// The win isn't gone to the main field: a group behind a bigger group
	// rejoins it (behindMainField), it doesn't ride for places. Nov 2026:
	// at Flèche 50 riders 20 s behind a bunch of 86, 66 km out, gave up.
	if len(front) > len(s.Road) {
		return false
	}
	var chase []*RiderState
	for _, o := range s.Road {
		if !o.DNF && o.TeamRole != RoleSprinter {
			chase = append(chase, o)
		}
	}
	if len(chase) == 0 {
		return false
	}
	// Helpers: everyone here who'd work in the chase (not riders whose own
	// leader is in front); everyone in the front group works for it.
	helpers := 0
	ahead := map[string]bool{}
	for _, o := range front {
		if o.TeamRole == RoleLeader {
			ahead[o.Rider.Team] = true
		}
	}
	for _, o := range chase {
		if !ahead[o.Rider.Team] {
			helpers++
		}
	}
	frontHelpers := len(front)
	sort.Slice(chase, func(i, j int) bool { return wkg(chase[i]) > wkg(chase[j]) })
	sort.Slice(front, func(i, j int) bool { return wkg(front[i]) > wkg(front[j]) })
	if len(chase) > maxRotation {
		chase = chase[:maxRotation]
	}
	frontK := maxRotation
	if len(ahead) >= 2 {
		// A front of rival leaders (two teams or more) watches itself:
		// nobody tows the others to the finish, so it rides as a
		// disorganised group (as placings groups do), with no pooling.
		frontK, frontHelpers = disorganisedRotation, 0
	}
	if len(front) > frontK {
		front = front[:frontK]
	}
	return s.pooledTimeToLine(front, frontHelpers) < s.pooledTimeToLine(chase, helpers)+gap
}

// pooledTimeToLine is a pace line's time to the line when helpers riders
// share the work: each is on the front only k/N of the time, so the line
// can spend its W' N/k times faster (as the group simulation's pooled
// chases do: peloton_follow.go), up to maximal aerobic power. Nov 2026: with
// a bare threshold line, a group of 94 (86 domestiques) judged a 30-rider
// move 15 s ahead uncatchable 40 km from Milan-San Remo's finish and rode
// for places.
func (s *RaceState) pooledTimeToLine(line []*RiderState, helpers int) float64 {
	t0 := s.timeToLine(line)
	if len(line) == 0 || t0 <= 0 || helpers <= 0 {
		return t0
	}
	var thr, wp, gap float64
	for _, rs := range line {
		thr += rs.CurrentFTP()
		wp += rs.WPrime.WPrimeCurrent
		gap += punchGap(rs.Rider)
	}
	k := float64(len(line))
	thr, wp, gap = thr/k, wp/k, gap/k
	depth := math.Max(1, float64(helpers)/k)
	f := math.Min(1+gap, 1+depth*wp/t0/thr)
	return s.timeToLineAt(line, f)
}

// frontSafe: one-day races, rs is in the front selection of rival leaders
// (no rival leader ahead, at least one in his group) and no group behind can
// catch it. The race is now among themselves: a favourite stops pressing
// (surges that wear down a bunch only empty him in a small selection) and
// keeps his reserve for the attack that drops the others (attackPays).
func (s *RaceState) frontSafe(rs *RiderState) bool {
	if s == nil || s.IsStageRace || s.Phase == PhaseEarly || len(s.Course) == 0 {
		return false
	}
	for o := range s.Ahead {
		if rivalLeaderOf(rs, o) {
			return false
		}
	}
	var own []*RiderState
	rival := false
	for _, o := range s.Road {
		if o.DNF {
			continue
		}
		own = append(own, o)
		rival = rival || rivalLeaderOf(rs, o)
	}
	if !rival {
		return false
	}
	chasers, gap := s.rivalChase(rs)
	if len(chasers) == 0 {
		return true
	}
	chase := append([]*RiderState(nil), chasers...)
	sort.Slice(chase, func(i, j int) bool { return wkg(chase[i]) > wkg(chase[j]) })
	sort.Slice(own, func(i, j int) bool { return wkg(own[i]) > wkg(own[j]) })
	if len(chase) > maxRotation {
		chase = chase[:maxRotation]
	}
	if len(own) > maxRotation {
		own = own[:maxRotation]
	}
	return s.timeToLine(own) < s.timeToLine(chase)+gap
}

// attackKm: the stretch an attack is ridden over. In a one-day race an
// attack on a hard section (a climb, cobbles, gravel) lasts the rest of it:
// the decisive Kwaremont attack is a two- or three-minute effort that empties
// the followers, not a 20-second surge anyone with a little reserve can
// match (Nov 2026: judged per step, a faster sprinter always held Van der
// Poel's wheel in Flanders). Otherwise (and in stage races) the step.
func (s *RaceState) attackKm(seg *model.Segment) float64 {
	if s == nil || s.IsStageRace || len(s.Course) == 0 || s.SegIndex >= len(s.Course) {
		return seg.DistanceKm
	}
	hard := func(g *model.Segment) bool { return g.SurfaceType.Unpaved() || climbOf(g) >= terrainClimb }
	if !hard(seg) {
		return seg.DistanceKm
	}
	km := 0.0
	for i := s.SegIndex; i < len(s.Course) && hard(&s.Course[i]); i++ {
		d := s.Course[i].DistanceKm
		if i == s.SegIndex && s.SegmentKmLeft > 0 && s.SegmentKmLeft < d {
			d = s.SegmentKmLeft
		}
		km += d
	}
	// What's left of the climb from here, at least one fine step (Nov 2026:
	// this was at least the whole course segment, so near the top of a
	// one-segment climb an attack was judged as if from its foot).
	return math.Max(km, fineStepKm)
}

// outFinishedByRival reports whether a rival leader in rs's road group would
// beat him at the finish (finishTime).
func (s *RaceState) outFinishedByRival(rs *RiderState) bool {
	for _, o := range s.Road {
		if !o.DNF && rivalLeaderOf(rs, o) && s.outFinishes(o, rs) {
			return true
		}
	}
	return false
}
