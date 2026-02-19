package simulation

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
