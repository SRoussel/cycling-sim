package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

func stateFor(id int64, team string, ftp int, kg float64, role TeamRole) *RiderState {
	r := &model.Rider{ID: id, Name: team, Team: team, FTPWatts: ftp, WeightKg: kg, ClimbingAbility: 70, CdA: 0.32, Handling: 70, DescendingAbility: 70, Endurance: 70}
	return &RiderState{Rider: r, TeamRole: role, EffectiveFTP: ftp, FormFactor: 1, WPrime: NewWPrimeState(r)}
}

func TestAssignCoLeaders(t *testing.T) {
	var states []*RiderState
	// Team A: leader plus a domestique who is one of the strongest in the field.
	states = append(states, stateFor(1, "A", 450, 65, RoleLeader), stateFor(2, "A", 430, 64, RoleDomestique))
	// Ten weaker riders on other teams.
	for i := int64(10); i < 20; i++ {
		states = append(states, stateFor(i, "B", 330, 70, RoleDomestique))
	}
	weak := states[len(states)-1]

	AssignCoLeaders(states, nil, false)
	if states[1].TeamRole != RoleDomestique {
		t.Fatal("one-day races keep a single leader")
	}
	AssignCoLeaders(states, nil, true)
	if states[1].TeamRole != RoleLeader {
		t.Error("a top-10 climber in the field should be a co-leader in a stage race")
	}
	if weak.TeamRole != RoleDomestique {
		t.Error("a weak domestique outside the GC top 10 should stay a domestique")
	}
	// Once on GC, a weak rider sitting in the top 10 is protected too.
	gaps := map[int64]float64{1: 0, 2: 10, weak.Rider.ID: 20}
	for i := int64(10); i < 19; i++ {
		gaps[i] = 3600
	}
	AssignCoLeaders(states, gaps, true)
	if weak.TeamRole != RoleLeader {
		t.Error("a rider in the GC top 10 should ride for their own GC")
	}
}

func TestGCRivalAhead(t *testing.T) {
	me := stateFor(1, "A", 400, 62, RoleLeader)
	rival := stateFor(2, "B", 400, 62, RoleLeader)
	mate := stateFor(3, "A", 400, 62, RoleLeader)
	s := &RaceState{IsStageRace: true, GCGaps: map[int64]float64{1: 60, 2: 300, 3: 0}}

	s.Ahead = map[*RiderState]float64{rival: 120}
	if s.gcRivalAhead(me) {
		t.Error("a rival 4 min down on GC and 2 min up the road doesn't threaten my GC place")
	}
	s.Ahead = map[*RiderState]float64{rival: 400}
	if !s.gcRivalAhead(me) {
		t.Error("a rival who would pass me on GC must be chased")
	}
	s.Ahead = map[*RiderState]float64{mate: 400}
	if s.gcRivalAhead(me) {
		t.Error("a teammate up the road is not a rival")
	}
	s.IsStageRace = false
	s.Ahead = map[*RiderState]float64{rival: 400}
	if s.gcRivalAhead(me) {
		t.Error("one-day races have no GC to defend")
	}
}

func TestLeaderTempoSpeed(t *testing.T) {
	leader := stateFor(1, "A", 420, 62, RoleLeader)
	climb := &model.Segment{GradientPct: 7, DistanceKm: 2, SurfaceType: model.SurfaceRoad}
	v := leaderTempoSpeed(leader, climb)
	r := leader.Rider
	threshold := EffectivePower(r, leader.CurrentFTP(), climb, false)
	need := segmentPower(v, r.TotalMassKg(), r.CdA*draftFor(2, climb.SurfaceType), climb, r.Handling)
	if d := need - threshold; d > 1 || d < -1 {
		t.Errorf("tempo needs %.0f W from the leader on the wheel, want threshold %.0f W", need, threshold)
	}
	if solo := segmentSpeed(threshold, r.TotalMassKg(), r.CdA, climb, r.Handling); v <= solo {
		t.Errorf("drafting should let the leader follow a pace above their solo threshold speed (%.2f vs %.2f)", v, solo)
	}
}
