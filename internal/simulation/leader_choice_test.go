package simulation

import "testing"

type choice struct {
	orders
	team string
	id   int64
}

func (c choice) LeaderChoice(team string) (int64, bool) {
	if team == c.team {
		return c.id, true
	}
	return 0, false
}

func TestLeaderChoiceOnlyForThePlayersTeam(t *testing.T) {
	lead := stateFor(1, "A", 450, 65, RoleLeader)
	dom := stateFor(2, "A", 400, 65, RoleDomestique)
	sprinter := stateFor(3, "A", 380, 70, RoleSprinter)
	other := stateFor(4, "B", 450, 65, RoleLeader)
	states := []*RiderState{lead, dom, sprinter, other}
	applyLeaderChoices(states, choice{team: "A", id: 2})
	if dom.TeamRole != RoleLeader || lead.TeamRole != RoleDomestique {
		t.Fatalf("chosen rider leads, the old leader works for him: %v %v", dom.TeamRole, lead.TeamRole)
	}
	if sprinter.TeamRole != RoleSprinter || other.TeamRole != RoleLeader {
		t.Fatalf("the sprinter and other teams are untouched")
	}
}
