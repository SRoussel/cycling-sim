package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

// orders is a minimal ActionOverrideProvider with team chase orders.
type orders map[string]ChaseOrder

func (orders) ActionOverride(int64) (Action, bool)                        { return 0, false }
func (orders) ConsumeOneShot(int64) (Action, bool)                        { return 0, false }
func (orders) EffortMultiplier(int64) (float64, bool)                     { return 0, false }
func (orders) SprintTrainDirective(int64, string, float64) (Action, bool) { return 0, false }
func (orders) SprintTrainState(float64) *TeamState                        { return nil }
func (o orders) ChaseOrder(team string) ChaseOrder                        { return o[team] }

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
	if p := plan(orders{"A": ChaseYes}); !p.teams["A"] || p.intensity <= 0 {
		t.Errorf("ordered to chase, team A should chase: %+v", p)
	}
	if p := plan(orders{}); !p.teams["S"] {
		t.Skip("the sprinter's team doesn't chase here anyway; nothing to override")
	}
	if p := plan(orders{"S": ChaseNo}); p.teams["S"] {
		t.Error("ordered not to chase, the sprinter's team should sit in")
	}
}
