package simulation

import (
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
func AssignCoLeaders(states []*RiderState, gcGaps map[int64]float64, stageRace bool) {
	if !stageRace {
		return
	}
	wkg := map[int64]float64{}
	var ids []int64
	for _, rs := range states {
		if !rs.DNF {
			wkg[rs.Rider.ID] = climbingWKg(rs.Rider)
			ids = append(ids, rs.Rider.ID)
		}
	}
	strongest := topIDs(ids, func(id int64) float64 { return -wkg[id] }, gcPlacesProtected)
	topGC := map[int64]bool{}
	if len(gcGaps) > 0 {
		gcIDs := make([]int64, 0, len(gcGaps))
		for id := range gcGaps {
			gcIDs = append(gcIDs, id)
		}
		topGC = topIDs(gcIDs, func(id int64) float64 { return gcGaps[id] }, gcPlacesProtected)
	}
	for _, rs := range states {
		if rs.TeamRole == RoleDomestique && (strongest[rs.Rider.ID] || topGC[rs.Rider.ID]) {
			rs.TeamRole = RoleLeader
		}
	}
}
