package simulation

import (
	"math"
	"sort"

	"cycling_sim/internal/model"
)

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

// Co-leaders.
//
// In a stage race a team protects every rider who can finish in the GC top 10
// (UCI points, prestige): a domestique who is one of the strongest climbers in
// the race, or already high on GC, rides for their own result instead of
// emptying themselves for the leader (e.g. Almeida 4th and Yates 6th at the
// 2024 Tour while riding for Pogačar; Jorgenson 8th riding for Vingegaard).

// gcPlacesProtected: teams protect a top-10 GC place.
const gcPlacesProtected = 10

// planSpread: how sharply a team's decision to free a domestique falls off
// with his climbing rank in the field (in places). Teams don't cut at exactly
// 10th: the 8th-best climber is usually freed (~84%), the 12th sometimes
// (~27%), the 15th rarely (~5%). Without it a ±1% rating change flipped a
// rider between leader and domestique and moved him ~500 s on GC.
const planSpread = 1.5

// freedByPlan reports whether a team frees a domestique ranked rank (1 = the
// strongest climber in the field) given the plan draw u in [0,1).
func freedByPlan(rank int, u float64) bool {
	p := 1 / (1 + math.Exp((float64(rank)-(gcPlacesProtected+0.5))/planSpread))
	return u < p
}

// climbingWKg is a rider's threshold W/kg on a real climb.
func climbingWKg(r *model.Rider) float64 {
	if r.WeightKg <= 0 {
		return 0
	}
	return float64(r.FTPWatts) * ModifierFactor(r.ClimbingAbility) / r.WeightKg
}

// topIDs returns the n riders with the lowest score.
func topIDs(ids []int64, score func(int64) float64, n int) map[int64]bool {
	sort.Slice(ids, func(i, j int) bool {
		if score(ids[i]) != score(ids[j]) {
			return score(ids[i]) < score(ids[j])
		}
		return ids[i] < ids[j]
	})
	out := map[int64]bool{}
	for i := 0; i < len(ids) && i < n; i++ {
		out[ids[i]] = true
	}
	return out
}

// AssignCoLeaders promotes domestiques who are realistic GC top-10 riders to
// (co-)leaders in a stage race: the strongest climbers in the field and, once
// there is a GC, riders in its top 10. gcGaps (rider → seconds behind the GC
// leader) may be nil before the first stage.
func AssignCoLeaders(states []*RiderState, gcGaps map[int64]float64, plan map[int64]float64, stageRace bool) {
	if !stageRace {
		return
	}
	// A team's GC leader is its best climber: climbing W/kg decides a stage
	// race, not the average of ability ratings AssignTeamRoles goes by (which
	// made Bilbao Bahrain's leader over Buitrago at the 2024 Tour).
	best := map[string]*RiderState{}
	for _, rs := range states {
		if rs.DNF || rs.TeamRole == RoleSprinter {
			continue
		}
		if b := best[rs.Rider.Team]; b == nil || climbingWKg(rs.Rider) > climbingWKg(b.Rider) {
			best[rs.Rider.Team] = rs
		}
	}
	for _, rs := range states {
		if b := best[rs.Rider.Team]; b != nil && rs.TeamRole == RoleLeader && rs != b {
			rs.TeamRole = RoleDomestique // back as a co-leader below if they qualify
		}
	}
	for _, b := range best {
		b.TeamRole = RoleLeader
	}
	wkg := map[int64]float64{}
	var ids []int64
	for _, rs := range states {
		if !rs.DNF {
			wkg[rs.Rider.ID] = climbingWKg(rs.Rider)
			ids = append(ids, rs.Rider.ID)
		}
	}
	topIDs(ids, func(id int64) float64 { return -wkg[id] }, 0) // sorts strongest first
	rank := make(map[int64]int, len(ids))
	for i, id := range ids {
		rank[id] = i + 1
	}
	freed := func(id int64) bool {
		if plan == nil {
			return rank[id] <= gcPlacesProtected
		}
		return freedByPlan(rank[id], plan[id])
	}
	topGC := map[int64]bool{}
	if len(gcGaps) > 0 {
		gcIDs := make([]int64, 0, len(gcGaps))
		for id := range gcGaps {
			gcIDs = append(gcIDs, id)
		}
		topGC = topIDs(gcIDs, func(id int64) float64 { return gcGaps[id] }, gcPlacesProtected)
	}
	for _, rs := range states {
		if rs.TeamRole == RoleDomestique && (freed(rs.Rider.ID) || topGC[rs.Rider.ID]) {
			rs.TeamRole = RoleLeader
		}
	}
}

// AssignCourseLeaders picks each team's leader for a one-day race: the
// rider who would be fastest over the course's hard sections (see
// decisiveTime), so climbers lead on long climbs, puncheurs on short steep
// ones and big engines on flatter courses. A sprinter who is his team's
// best there leads too (a fast puncheur is no pure sprinter); other
// sprinters keep their role. The previous leader rides for the new one.
// (AssignTeamRoles goes by an average of ability ratings, which made
// Kwiatkowski INEOS's Milan-San Remo leader over Ganna.)
func AssignCourseLeaders(states []*RiderState, segments []model.Segment) {
	best := map[string]*RiderState{}
	bestTime := map[string]float64{}
	for _, rs := range states {
		if rs.DNF {
			continue
		}
		t := decisiveTime(rs, segments)
		if b := best[rs.Rider.Team]; b == nil || t < bestTime[rs.Rider.Team] {
			best[rs.Rider.Team], bestTime[rs.Rider.Team] = rs, t
		}
	}
	for _, rs := range states {
		if b := best[rs.Rider.Team]; b != nil && rs.TeamRole == RoleLeader && rs != b {
			rs.TeamRole = RoleDomestique
		}
	}
	for _, b := range best {
		b.TeamRole = RoleLeader
	}
}

// decisiveTime is a rider's time over a course's hard sections (climbs and
// cobbles), where one-day races are decided: each ridden alone at
// threshold plus their W' spread over it (a short steep climb is a
// puncheur's, a long one a climber's). Flat roads are ridden in the bunch,
// at everyone's speed, so they don't count. A course with no hard section
// falls back to the whole course at threshold (courseTime).
func decisiveTime(rs *RiderState, segments []model.Segment) float64 {
	r := rs.Rider
	t, hard := 0.0, false
	for i := range segments {
		seg := &segments[i]
		if !seg.SurfaceType.Unpaved() && climbOf(seg) < terrainClimb {
			continue
		}
		hard = true
		thr := EffectivePower(r, rs.CurrentFTP(), seg, false)
		v := segmentSpeed(thr, r.TotalMassKg(), r.CdA, seg, r.Handling)
		p := rs.maxPowerFor(thr, thr, SegmentTime(v, seg.DistanceKm))
		v = math.Min(SpeedCap(r, seg), segmentSpeed(p, r.TotalMassKg(), r.CdA, seg, r.Handling))
		t += SegmentTime(v, seg.DistanceKm)
	}
	if !hard {
		return courseTime(rs, segments)
	}
	return t
}

// courseTime is a rider's time over the segments alone at threshold.
func courseTime(rs *RiderState, segments []model.Segment) float64 {
	r := rs.Rider
	t := 0.0
	for i := range segments {
		seg := &segments[i]
		v := segmentSpeed(EffectivePower(r, rs.CurrentFTP(), seg, false), r.TotalMassKg(), r.CdA, seg, r.Handling)
		t += SegmentTime(v, seg.DistanceKm)
	}
	return t
}

// HardTime is a fresh rider's time over a course's hard sections (climbs and
// cobbles) ridden alone at threshold plus W' (decisiveTime, as one-day
// leaders are picked), and whether the course has any.
func HardTime(r model.Rider, segments []model.Segment) (float64, bool) {
	rs := &RiderState{Rider: &r, WPrime: NewWPrimeState(&r), FormFactor: 1, EffectiveFTP: r.FTPWatts}
	hard := false
	for i := range segments {
		if segments[i].SurfaceType.Unpaved() || climbOf(&segments[i]) >= terrainClimb {
			hard = true
			break
		}
	}
	if !hard {
		return 0, false
	}
	return decisiveTime(rs, segments), true
}

// CourseTime is a fresh rider's time over a whole course alone at threshold
// (a time trial's measure).
func CourseTime(r model.Rider, segments []model.Segment) float64 {
	rs := &RiderState{Rider: &r, WPrime: NewWPrimeState(&r), FormFactor: 1, EffectiveFTP: r.FTPWatts}
	return courseTime(rs, segments)
}
