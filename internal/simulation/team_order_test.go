package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

// orders is a minimal ActionOverrideProvider with team orders.
type orders map[string]TeamOrders

func (orders) ActionOverride(int64) (Action, bool)                        { return 0, false }
func (orders) ConsumeOneShot(int64) (Action, bool)                        { return 0, false }
func (orders) EffortMultiplier(int64) (float64, bool)                     { return 0, false }
func (orders) SprintTrainDirective(int64, string, float64) (Action, bool) { return 0, false }
func (orders) SprintTrainState(float64) *TeamState                        { return nil }
func (o orders) TeamOrders(team string) TeamOrders                        { return o[team] }

// A player's chase order decides whether their team joins the chase of a
// break: Chase makes an uninvolved team chase, Don't chase keeps a team
// with a reason to chase (here: its sprinter) out of it.
func TestTeamChaseOrder(t *testing.T) {
	course := []model.Segment{{DistanceKm: 60, GradientPct: 0, SurfaceType: model.SurfaceRoad}}
	brk := stateFor(1, "Break", 400, 70, RoleDomestique)
	var group []*RiderState
	for i := int64(10); i < 14; i++ {
		group = append(group, stateFor(i, "A", 400, 70, RoleDomestique))
	}
	sprinter := stateFor(20, "S", 380, 75, RoleSprinter)
	group = append(group, sprinter)
	for i := int64(21); i < 25; i++ {
		group = append(group, stateFor(i, "S", 400, 70, RoleDomestique))
	}
	plan := func(o orders) chasePlan {
		s := &RaceState{IsStageRace: true, Course: course, SegIndex: 0, SegmentKmLeft: 60, Road: group,
			Ahead: map[*RiderState]float64{brk: 120}, Overrides: o, GCGaps: map[int64]float64{}}
		return s.planChase(group)
	}
	if p := plan(orders{}); p.teams["A"] {
		t.Error("team A has no reason to chase on its own")
	}
	if p := plan(orders{"A": {Chase: OrderYes}}); !p.teams["A"] || p.intensity <= 0 {
		t.Errorf("ordered to chase, team A should chase: %+v", p)
	}
	if p := plan(orders{}); !p.teams["S"] {
		t.Skip("the sprinter's team doesn't chase here anyway; nothing to override")
	}
	if p := plan(orders{"S": {Chase: OrderNo}}); p.teams["S"] {
		t.Error("ordered not to chase, the sprinter's team should sit in")
	}
}

// Ordered into the break, a team sends its two strongest domestiques with W'
// left, until one of its riders is up the road; told to stay together, its
// domestiques ignore their own wish to go.
func TestTeamBreakOrder(t *testing.T) {
	strong := stateFor(1, "A", 420, 65, RoleDomestique)
	second := stateFor(2, "A", 400, 65, RoleDomestique)
	weak := stateFor(3, "A", 330, 70, RoleDomestique)
	tired := stateFor(4, "A", 430, 65, RoleDomestique)
	tired.WPrime.WPrimeCurrent = 0.3 * tired.WPrime.WPrimeMax
	leader := stateFor(5, "A", 440, 64, RoleLeader)
	other := stateFor(6, "B", 430, 65, RoleDomestique)
	group := []*RiderState{strong, second, weak, tired, leader, other}
	s := &RaceState{Overrides: orders{"A": {Break: OrderYes}}, Phase: PhaseMid, Road: group}

	picks := s.breakPicks(group)
	if !picks[1] || !picks[2] || len(picks) != 2 {
		t.Errorf("picks = %v, want riders 1 and 2", picks)
	}
	if !s.wantsBreak(strong, picks) || s.wantsBreak(other, picks) {
		t.Error("only the picked riders go with moves")
	}
	s.BreakawayRiders = map[int64]bool{weak.Rider.ID: true}
	if p := s.breakPicks(group); len(p) != 0 {
		t.Errorf("with a teammate in the break nobody else goes: %v", p)
	}
	s.BreakawayRiders, s.Phase = nil, PhaseFinal
	if p := s.breakPicks(group); len(p) != 0 {
		t.Errorf("no break hunting in the finale: %v", p)
	}

	s = &RaceState{Overrides: orders{"A": {Break: OrderNo}}, WantsBreak: map[int64]bool{1: true, 5: true, 6: true}}
	if s.wantsBreak(strong, nil) {
		t.Error("told to stay together, a domestique doesn't go with moves")
	}
	if !s.wantsBreak(leader, nil) || !s.wantsBreak(other, nil) {
		t.Error("leaders and other teams are unaffected")
	}
}

// Pace orders: Set the pace makes a team drive for its best climbing leader in
// the group; Sit in stops a team driving on its own judgement.
func TestTeamPaceOrder(t *testing.T) {
	l1 := stateFor(1, "A", 400, 70, RoleLeader)
	l2 := stateFor(2, "A", 420, 62, RoleLeader)
	dom := stateFor(3, "A", 380, 70, RoleDomestique)
	b := stateFor(4, "B", 430, 64, RoleLeader)
	group := []*RiderState{l1, l2, dom, b}
	s := &RaceState{Overrides: orders{"A": {Pace: OrderYes}, "B": {Pace: OrderNo}}}
	driving := map[string]*RiderState{"B": b}
	s.applyPaceOrders(driving, group)
	if driving["A"] != l2 {
		t.Errorf("team A should drive for its best climber, got %v", driving["A"])
	}
	if _, ok := driving["B"]; ok {
		t.Error("team B was told to sit in")
	}
}
