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
	r := rs.Rider
	threshold := EffectivePower(r, rs.CurrentFTP(), seg, false)
	v0 := segmentSpeed(threshold, r.TotalMassKg(), r.CdA, seg, r.Handling)
	burst := threshold + rs.WPrime.AvailablePowerAboveFTP(SegmentTime(v0, seg.DistanceKm))
	v := segmentSpeed(burst, r.TotalMassKg(), r.CdA, seg, r.Handling)
	holds := func(o *RiderState) bool {
		or := o.Rider
		need := segmentPower(v, or.TotalMassKg(), or.CdA*groupDraft(2, false, seg, v), seg, or.Handling)
		avail := EffectivePower(or, o.CurrentFTP(), seg, false) + o.WPrime.AvailablePowerAboveFTP(SegmentTime(v, seg.DistanceKm))
		return need <= avail
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
	if !s.escapeEligible(rs) || s.IsStageRace || !s.isDangerous(rs) {
		return false
	}
	if seg.SurfaceType != model.SurfaceCobbles && climbOf(seg) < terrainClimb {
		return false
	}
	var finisher *RiderState
	for _, o := range s.Road {
		if o != rs && !o.DNF && (finisher == nil || SprintPower(o.Rider) > SprintPower(finisher.Rider)) {
			finisher = o
		}
	}
	if finisher == nil {
		return false
	}
	return s.timeToLine([]*RiderState{rs}) < s.timeToLine([]*RiderState{finisher})
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
