package director

import (
	"testing"

	"cycling_sim/internal/simulation"
)

// race builds a view: groups of rider IDs with gaps; riders 1-3 are ours
// (1 leader, 2-3 domestiques), 10+ are rivals (10 a favourite leader).
func race(km float64, groups [][]int64, gaps []float64, events ...simulation.TacticsEvent) *simulation.RaceView {
	v := &simulation.RaceView{KmRemaining: km, KmTotal: 250, Riders: map[int64]simulation.RiderBrief{}, Events: events}
	for gi, ids := range groups {
		v.Road = append(v.Road, simulation.RoadGroup{GapSeconds: gaps[gi], IDs: ids})
		for _, id := range ids {
			b := simulation.RiderBrief{Name: "R", Team: "Rivals", WPrimePct: 80, LegsPct: 95}
			switch {
			case id == 1:
				b.Team, b.Role = "Us", simulation.RoleLeader
			case id <= 3:
				b.Team = "Us"
			case id == 10:
				b.Role, b.Favourite = simulation.RoleLeader, true
			}
			v.Riders[id] = b
		}
	}
	return v
}

func bunch(from, to int64) []int64 {
	var out []int64
	for id := from; id <= to; id++ {
		out = append(out, id)
	}
	return out
}

func TestRivalAttackOffersFollow(t *testing.T) {
	d := New("Us")
	field := append([]int64{1, 2, 3}, bunch(11, 40)...)
	v := race(30, [][]int64{{10}, field}, []float64{0, 5},
		simulation.TacticsEvent{Type: "attack", RiderID: 10, Key: true})
	m := d.Next(v)
	if m == nil || m.Kind != "attack" {
		t.Fatalf("want an attack moment, got %+v", m)
	}
	o, ok := m.Option("follow")
	if !ok || len(o.Commands) != 1 || o.Commands[0].RiderID != 1 || o.Commands[0].Action != "chase" {
		t.Fatalf("follow should send our leader after him: %+v", o)
	}
	if _, ok := m.Option("mark"); !ok {
		t.Errorf("a domestique with the leader should be able to mark")
	}
	// The same rider attacking again 2 km later doesn't ask again.
	v.KmRemaining = 28
	if m2 := d.Next(v); m2 != nil && m2.Kind == "attack" {
		t.Errorf("attack cooldown ignored: %+v", m2)
	}
}

func TestOwnAttackIsNoMoment(t *testing.T) {
	d := New("Us")
	v := race(30, [][]int64{{1}, bunch(2, 40)}, []float64{0, 5},
		simulation.TacticsEvent{Type: "attack", RiderID: 1, Key: true})
	if m := d.Next(v); m != nil && m.Kind == "attack" {
		t.Fatalf("our own attack is not a decision: %+v", m)
	}
}

func TestBreakawayOnceThenLate(t *testing.T) {
	d := New("Us")
	field := append([]int64{1, 2, 3}, bunch(11, 60)...)
	v := race(150, [][]int64{{70, 71, 72}, field}, []float64{0, 180})
	m := d.Next(v)
	if m == nil || m.Kind != "break" || len(m.Options) != 3 {
		t.Fatalf("want a break moment, got %+v", m)
	}
	v.KmRemaining = 120
	if m := d.Next(v); m != nil {
		t.Fatalf("break asked twice: %+v", m)
	}
	v.KmRemaining = 65
	if m := d.Next(v); m == nil || m.Title != "⏱ The break is still away" {
		t.Fatalf("want the late break check, got %+v", m)
	}
}

func TestOurRiderInBreak(t *testing.T) {
	d := New("Us")
	v := race(150, [][]int64{{2, 70, 71}, append([]int64{1, 3}, bunch(11, 60)...)}, []float64{0, 180})
	m := d.Next(v)
	if m == nil || m.Title == "" || m.Options[0].Commands[0].RiderID != 2 {
		t.Fatalf("want a moment for our rider in the break, got %+v", m)
	}
}

func TestLeaderDropped(t *testing.T) {
	d := New("Us")
	front := append([]int64{10, 2}, bunch(11, 40)...)
	v := race(40, [][]int64{front, {1, 3}}, []float64{0, 30})
	m := d.Next(v)
	if m == nil || m.Kind != "dropped" {
		t.Fatalf("want a dropped moment, got %+v", m)
	}
	if o, ok := m.Option("wait"); !ok || o.Orders[0].Kind != "protect" {
		t.Fatalf("the team should be able to wait: %+v", m.Options)
	}
	// Early in the race a gap isn't a crisis yet.
	d2 := New("Us")
	if m := d2.Next(race(150, [][]int64{front, {1, 3}}, []float64{0, 30})); m != nil && m.Kind == "dropped" {
		t.Fatalf("dropped asked in the early phase")
	}
}

func TestKeyClimbScheduledAttack(t *testing.T) {
	d := New("Us")
	// 250 km route, a 4 km climb at 6% from km 230 (20 to go).
	pts := []simulation.ProfilePoint{{Km: 0, Elev: 0}, {Km: 230, Elev: 0}, {Km: 234, Elev: 240}, {Km: 250, Elev: 0}}
	d.SetRoute(&simulation.RouteProfile{TotalKm: 250, Points: pts})
	if len(d.Climbs()) != 1 {
		t.Fatalf("climbs: %+v", d.Climbs())
	}
	v := race(22, [][]int64{append([]int64{1, 2, 3, 10}, bunch(11, 40)...)}, []float64{0})
	m := d.Next(v)
	if m == nil || m.Kind != "climb" {
		t.Fatalf("want a climb moment, got %+v", m)
	}
	o, _ := m.Option("attack-top")
	if c := o.Commands[0]; c.RiderID != 1 || c.AtKmRemaining != 17 {
		t.Fatalf("attack near the top should be scheduled 3 km up the climb (17 km to go): %+v", c)
	}
	// The early climb rule doesn't fire 200 km out.
	d.SetRoute(&simulation.RouteProfile{TotalKm: 250, Points: []simulation.ProfilePoint{{Km: 0}, {Km: 30}, {Km: 34, Elev: 240}, {Km: 250}}})
	if m := d.Next(race(222, [][]int64{append([]int64{1, 2, 3, 10}, bunch(11, 40)...)}, []float64{0})); m != nil {
		t.Fatalf("an early climb isn't decisive: %+v", m)
	}
}

func TestFinaleSprint(t *testing.T) {
	d := New("Us")
	v := race(10, [][]int64{append([]int64{1, 2, 3}, bunch(11, 40)...)}, []float64{0})
	b := v.Riders[2]
	b.Role = simulation.RoleSprinter
	v.Riders[2] = b
	m := d.Next(v)
	if m == nil || m.Title != "🏁 Bunch sprint coming" {
		t.Fatalf("want the sprint finale, got %+v", m)
	}
	v.KmRemaining = 6
	if m := d.Next(v); m != nil {
		t.Fatalf("finale asked twice: %+v", m)
	}
}
