package simulation

import (
	"fmt"
	"math/rand"
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

	AssignCoLeaders(states, nil, nil, false)
	if states[1].TeamRole != RoleDomestique {
		t.Fatal("one-day races keep a single leader")
	}
	AssignCoLeaders(states, nil, nil, true)
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
	AssignCoLeaders(states, gaps, nil, true)
	if weak.TeamRole != RoleLeader {
		t.Error("a rider in the GC top 10 should ride for their own GC")
	}
}

func TestGCRivalAhead(t *testing.T) {
	me := stateFor(1, "A", 400, 62, RoleLeader)
	rival := stateFor(2, "B", 400, 62, RoleLeader)
	mate := stateFor(3, "A", 400, 62, RoleLeader)
	s := &RaceState{IsStageRace: true, GCGaps: map[int64]float64{1: 60, 2: 300, 3: 0}, reach: 1800}

	s.Ahead = map[*RiderState]float64{rival: 120}
	if s.rivalAhead(me) {
		t.Error("a rival 4 min down on GC and 2 min up the road doesn't threaten my GC place")
	}
	s.Ahead = map[*RiderState]float64{rival: 400}
	if !s.rivalAhead(me) {
		t.Error("a rival who would pass me on GC must be chased")
	}
	s.Ahead = map[*RiderState]float64{mate: 400}
	if s.rivalAhead(me) {
		t.Error("a teammate up the road is not a rival")
	}
	s.reach = 30 // a break could gain 30 s today: I'm out of contention
	s.Ahead = map[*RiderState]float64{rival: 400}
	if s.rivalAhead(me) {
		t.Error("a leader out of GC contention doesn't race a rival for places")
	}
	s.reach = 1800
	s.IsStageRace = false
	s.Ahead = map[*RiderState]float64{rival: 120}
	s.Phase = PhaseEarly
	if s.rivalAhead(me) {
		t.Error("early in a one-day race nobody chases a rival yet")
	}
	s.Phase = PhaseMid
	if !s.rivalAhead(me) {
		t.Error("later in a one-day race, a rival leader up the road must be chased")
	}
	dom := stateFor(4, "A", 400, 62, RoleDomestique)
	if !s.rivalAhead(dom) {
		t.Error("a domestique whose leader isn't ahead races for placings")
	}
	s.Ahead = map[*RiderState]float64{rival: 120, mate: 60}
	if s.rivalAhead(dom) {
		t.Error("riders don't chase when their own leader is up the road")
	}
}

func TestLeaderTempoSpeed(t *testing.T) {
	leader := stateFor(1, "A", 420, 62, RoleLeader)
	climb := &model.Segment{GradientPct: 7, DistanceKm: 2, SurfaceType: model.SurfaceRoad}
	v := leaderTempoSpeed(leader, climb, 1)
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

// In a stage race the team's best climber leads, whoever the ability ratings
// favoured; the old leader rides for them unless they qualify as co-leader.
func TestStageRaceLeaderIsBestClimber(t *testing.T) {
	var states []*RiderState
	old := stateFor(1, "A", 380, 66, RoleLeader)         // 5.76 W/kg
	climber := stateFor(2, "A", 400, 60, RoleDomestique) // 6.67 W/kg
	states = append(states, old, climber)
	for i := int64(10); i < 30; i++ { // twenty stronger riders elsewhere
		states = append(states, stateFor(i, fmt.Sprint("T", i), 460, 62, RoleLeader))
	}
	AssignCoLeaders(states, nil, nil, true)
	if climber.TeamRole != RoleLeader || old.TeamRole != RoleDomestique {
		t.Errorf("roles: climber %v, old leader %v; want leader, domestique", climber.TeamRole, old.TeamRole)
	}
}

// With a team plan, freeing a domestique falls off smoothly with climbing
// rank instead of cutting at exactly 10th.
func TestFreedByPlanIsSmooth(t *testing.T) {
	share := func(rank int) float64 {
		n := 0
		for i := 0; i < 1000; i++ {
			if freedByPlan(rank, (float64(i)+0.5)/1000) {
				n++
			}
		}
		return float64(n) / 1000
	}
	prev := 1.0
	for rank := 1; rank <= 20; rank++ {
		p := share(rank)
		if p > prev || prev-p > 0.2 {
			t.Errorf("rank %d: freed %.2f after %.2f; want a smooth decline", rank, p, prev)
		}
		prev = p
	}
	if share(5) < 0.9 || share(16) > 0.1 {
		t.Errorf("freed: rank 5 %.2f, rank 16 %.2f", share(5), share(16))
	}
}

// With a rival up the road, a leader's domestiques chase and the leader sits
// in; a leader without teammates takes turns.
func TestTeamChasesForLeader(t *testing.T) {
	rival := stateFor(1, "R", 420, 62, RoleLeader)
	rival.TotalTime = 0
	leader := stateFor(2, "A", 420, 62, RoleLeader)
	dom := stateFor(3, "A", 400, 68, RoleDomestique)
	lone := stateFor(4, "B", 420, 62, RoleLeader)
	state := &RaceState{Phase: PhaseMid, Ahead: map[*RiderState]float64{rival: 30}}
	seg := &model.Segment{DistanceKm: 2, SurfaceType: model.SurfaceRoad}
	groups := FormGroups([]*RiderState{leader, dom, lone}, seg, false, 50, rand.New(rand.NewSource(1)), state, nil)
	if len(groups) == 0 {
		t.Fatal("no groups")
	}
	g := groups[0]
	if g.Workers[leader] {
		t.Error("a leader with a domestique present should sit in during the chase")
	}
	if !g.Workers[dom] {
		t.Error("the domestique should chase for the leader")
	}
	if !g.Workers[lone] {
		t.Error("a leader without teammates should take turns")
	}
}

// A one-day team rides for whoever is fastest over the course: the big
// engine on the flat, the light climber uphill.
func TestCourseLeaders(t *testing.T) {
	flat := []model.Segment{{DistanceKm: 50, SurfaceType: model.SurfaceRoad}}
	hill := []model.Segment{{DistanceKm: 10, GradientPct: 8, SurfaceType: model.SurfaceRoad}}
	for _, c := range []struct {
		name string
		segs []model.Segment
		want int64
	}{{"flat", flat, 2}, {"climb", hill, 1}} {
		climber := stateFor(1, "A", 380, 58, RoleLeader)    // 6.6 W/kg
		engine := stateFor(2, "A", 470, 82, RoleDomestique) // 5.7 W/kg
		AssignCourseLeaders([]*RiderState{climber, engine}, c.segs)
		lead := climber
		if engine.TeamRole == RoleLeader {
			lead = engine
		}
		if lead.Rider.ID != c.want || climber.TeamRole == engine.TeamRole {
			t.Errorf("%s: leader %d, want %d (roles %v/%v)", c.name, lead.Rider.ID, c.want, climber.TeamRole, engine.TeamRole)
		}
	}
}

// A one-day leader attacks where the strongest rival can't hold the wheel and
// the line can be reached first: on a long climb, not on the flat.
func TestAttackPaysOnClimbsNotFlat(t *testing.T) {
	for _, c := range []struct {
		name  string
		grade float64
		stage bool
		want  bool
	}{{"climb", 8, false, true}, {"flat", 0, false, false}, {"stage-race climb", 8, true, true}, {"stage-race flat", 0, true, false}} {
		strong := stateFor(1, "A", 460, 62, RoleLeader) // 7.4 W/kg
		rival := stateFor(2, "B", 380, 66, RoleLeader)  // 5.8 W/kg
		dom := stateFor(3, "B", 380, 70, RoleDomestique)
		course := []model.Segment{{DistanceKm: 12, GradientPct: c.grade, SurfaceType: model.SurfaceRoad}}
		s := &RaceState{Phase: PhaseFinal, IsStageRace: c.stage, Road: []*RiderState{strong, rival, dom}, Course: course, SegIndex: 0, SegmentKmLeft: 12}
		if got := s.attackPays(strong, &course[0]); got != c.want {
			t.Errorf("%s: attackPays = %v, want %v", c.name, got, c.want)
		}
	}
}
