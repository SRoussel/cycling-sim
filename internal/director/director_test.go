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
	o, ok := m.Option("follow-1")
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
	o, _ := m.Option("attack-top-1")
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

// withFavourite makes rider 3 (ours, a domestique by role) a favourite: the
// Van Aert riding for Jorgenson case.
func withFavourite(v *simulation.RaceView) *simulation.RaceView {
	b := v.Riders[3]
	b.Favourite, b.Name = true, "Fav"
	v.Riders[3] = b
	return v
}

func TestEveryCardCanAnswer(t *testing.T) {
	d := New("Us")
	v := withFavourite(race(30, [][]int64{{10}, append([]int64{1, 2, 3}, bunch(11, 40)...)}, []float64{0, 5},
		simulation.TacticsEvent{Type: "attack", RiderID: 10, Key: true}))
	m := d.Next(v)
	if m == nil {
		t.Fatal("want an attack moment")
	}
	for _, id := range []string{"follow-1", "follow-3"} {
		if _, ok := m.Option(id); !ok {
			t.Errorf("missing option %s: %+v", id, m.Options)
		}
	}
}

func TestDroppedLeaderWithFavouriteInFront(t *testing.T) {
	d := New("Us")
	front := append([]int64{10, 3}, bunch(11, 40)...)
	m := d.Next(withFavourite(race(40, [][]int64{front, {1, 2}}, []float64{0, 30})))
	if m == nil || m.Kind != "dropped" {
		t.Fatalf("want a dropped moment, got %+v", m)
	}
	if o, ok := m.Option("let"); !ok || o.Label != "Race for Fav instead" {
		t.Fatalf("should offer to ride for the favourite up front: %+v", m.Options)
	}
}

func TestDroppedFavouriteIsPacedByTeammates(t *testing.T) {
	d := New("Us")
	front := append([]int64{10, 1}, bunch(11, 40)...)
	m := d.Next(withFavourite(race(40, [][]int64{front, {3, 2}}, []float64{0, 30})))
	if m == nil || m.Title != "🚨 Fav is dropped" {
		t.Fatalf("a dropped favourite of ours is a moment too, got %+v", m)
	}
	o, ok := m.Option("wait")
	if !ok || len(o.Orders) != 0 || len(o.Commands) != 1 || o.Commands[0].RiderID != 2 {
		t.Fatalf("not the team leader: his teammate paces him (Protect waits for the leader): %+v", o)
	}
}

func TestSelectionWithRivalFavouritesIsNotABreak(t *testing.T) {
	d := New("Us")
	v := race(30, [][]int64{{2, 10, 11, 12}, append([]int64{1, 3}, bunch(13, 60)...)}, []float64{0, 70})
	if m := d.Next(v); m != nil && m.Kind == "break" {
		t.Fatalf("a front group with a rival favourite is the race, not a break: %+v", m)
	}
}

func roubaixLike() *simulation.RouteProfile {
	return &simulation.RouteProfile{TotalKm: 250,
		Points: []simulation.ProfilePoint{{Km: 0}, {Km: 250}},
		Sectors: []simulation.RouteSector{
			{Name: "Early", StartKm: 60, LengthKm: 3, Stars: 4},                 // too far out
			{Name: "Arenberg", StartKm: 160, LengthKm: 2.1, Stars: 5},           // 52
			{Name: "Small", StartKm: 180, LengthKm: 1, Stars: 2},                // 4: too easy
			{Name: "Bersee", StartKm: 200, LengthKm: 2.6, Stars: 4},             // 42
			{Name: "Mons", StartKm: 205, LengthKm: 2.9, Stars: 5},               // 72
			{Name: "Hornaing", StartKm: 170, LengthKm: 3.5, Stars: 3},           // 31
			{Name: "Camphin", StartKm: 228, LengthKm: 1.7, Stars: 4},            // 27
			{Name: "Carrefour de l'Arbre", StartKm: 231, LengthKm: 2, Stars: 5}, // 50, last 4-5★
			{Name: "Roubaix", StartKm: 243, LengthKm: 0.4, Stars: 1},
		}}
}

func TestDecisiveSectors(t *testing.T) {
	d := New("Us")
	d.SetRoute(roubaixLike())
	want := map[float64]bool{160: true, 200: true, 205: true, 231: true}
	if len(d.decisiveSectors) != len(want) {
		t.Fatalf("decisive sectors %v, want Arenberg, Bersee, Mons, Carrefour", d.decisiveSectors)
	}
	for k := range want {
		if !d.decisiveSectors[k] {
			t.Errorf("missing sector at km %.0f", k)
		}
	}
}

func TestCobblesMoment(t *testing.T) {
	d := New("Us")
	d.SetRoute(roubaixLike())
	m := d.Next(race(48, [][]int64{append([]int64{1, 2, 3, 10}, bunch(11, 40)...)}, []float64{0}))
	if m == nil || m.Kind != "cobbles" || m.Title != "🪨 Mons in 3.0 km" {
		t.Fatalf("want the Mons moment, got %+v", m)
	}
	o, ok := m.Option("attack-1")
	if !ok || o.Commands[0].AtKmRemaining > 45 || o.Commands[0].AtKmRemaining < 44 {
		t.Fatalf("attack a third of the way into the sector (~44 km to go): %+v", o)
	}
}

func TestCobbledClimbIsNamed(t *testing.T) {
	d := New("Us")
	d.SetRoute(&simulation.RouteProfile{TotalKm: 250,
		Points:  []simulation.ProfilePoint{{Km: 0}, {Km: 230}, {Km: 230.4, Elev: 55}, {Km: 250, Elev: 0}},
		Sectors: []simulation.RouteSector{{Name: "Paterberg", StartKm: 230, LengthKm: 0.5, Stars: 3}}})
	m := d.Next(race(21, [][]int64{append([]int64{1, 2, 3, 10}, bunch(11, 40)...)}, []float64{0}))
	if m == nil || m.Title != "⛰ Paterberg in 1.0 km" {
		t.Fatalf("want the named Paterberg moment, got %+v", m)
	}
	if len(d.decisiveSectors) != 0 {
		t.Errorf("a cobbled climb is the climb's moment, not a sector's")
	}
}

func TestLongGravelSectorWithARiseInIt(t *testing.T) {
	d := New("Us")
	// Monte Sante Marie: 10 km of 3★ gravel from km 133 (82 to go) with a
	// 1.4 km rise inside it.
	d.SetRoute(&simulation.RouteProfile{TotalKm: 215,
		Points:  []simulation.ProfilePoint{{Km: 0}, {Km: 136}, {Km: 137.4, Elev: 80}, {Km: 215}},
		Sectors: []simulation.RouteSector{{Name: "Monte Sante Marie", StartKm: 133, LengthKm: 10, Stars: 3, Surface: "gravel"}}})
	if !d.decisiveSectors[133] {
		t.Fatalf("Monte Sante Marie should be decisive: %v", d.decisiveSectors)
	}
	if len(d.decisive) != 0 {
		t.Errorf("the rise inside it belongs to the sector's moment: %v", d.decisive)
	}
	m := d.Next(race(85, [][]int64{append([]int64{1, 2, 3, 10}, bunch(11, 40)...)}, []float64{0}))
	if m == nil || m.Title != "💨 Monte Sante Marie in 3.0 km" || m.Kind != "gravel" {
		t.Fatalf("want the gravel moment, got %+v", m)
	}
}

func TestKeyOnlySkipsEarlyAttacksAndLateBreakChecks(t *testing.T) {
	field := append([]int64{1, 2, 3}, bunch(11, 40)...)
	attack := func(km float64) *simulation.RaceView {
		return race(km, [][]int64{{10}, field}, []float64{0, 5}, simulation.TacticsEvent{Type: "attack", RiderID: 10, Key: true})
	}
	d := New("Us")
	d.KeyOnly = true
	if m := d.Next(attack(45)); m != nil {
		t.Fatalf("key moments: no attack 45 km out, got %+v", m)
	}
	if m := d.Next(attack(28)); m == nil || m.Kind != "attack" {
		t.Fatalf("key moments: a favourite's attack at 28 km is one, got %+v", m)
	}
	b := New("Us")
	b.KeyOnly = true
	v := race(150, [][]int64{{70, 71, 72}, field}, []float64{0, 180})
	if m := b.Next(v); m == nil || m.Kind != "break" {
		t.Fatalf("the first break is a key moment, got %+v", m)
	}
	v.KmRemaining = 65
	if m := b.Next(v); m != nil {
		t.Fatalf("key moments: no second look at the break, got %+v", m)
	}
}

func TestGoalFromStrength(t *testing.T) {
	mk := func(rank int) *simulation.RaceView {
		v := race(200, [][]int64{append([]int64{1, 2, 3, 10}, bunch(11, 40)...)}, []float64{0})
		for id, b := range v.Riders {
			b.Rank = int(id) + 50
			v.Riders[id] = b
		}
		b := v.Riders[1]
		b.Rank, b.Name = rank, "Lead"
		v.Riders[1] = b
		return v
	}
	climbs := &simulation.RouteProfile{TotalKm: 250, Points: []simulation.ProfilePoint{{Km: 0}, {Km: 230}, {Km: 234, Elev: 240}, {Km: 250}}}
	for _, tc := range []struct {
		rank int
		want string
	}{{1, "Win with Lead"}, {4, "Podium with Lead"}, {8, "Top 5 with Lead"}, {15, "Top 10 with Lead"}, {40, "Get a rider in the break"}} {
		d := New("Us")
		d.SetRoute(climbs)
		d.Next(mk(tc.rank))
		if g := d.Goal(); g == nil || g.Text != tc.want {
			t.Errorf("rank %d: goal %+v, want %q", tc.rank, g, tc.want)
		}
	}
}

func TestBreakGoalTracked(t *testing.T) {
	d := New("Us")
	field := append([]int64{1, 3}, bunch(11, 60)...)
	d.Next(race(150, [][]int64{{2, 70, 71}, field}, []float64{0, 90}))
	if !d.BreakMade() {
		t.Fatal("our rider 90 s clear of the peloton made the break")
	}
}

func TestGoalNamesTheLeader(t *testing.T) {
	d := New("Us")
	d.SetRoute(&simulation.RouteProfile{TotalKm: 250, Points: []simulation.ProfilePoint{{Km: 0}, {Km: 230}, {Km: 234, Elev: 240}, {Km: 250}}})
	v := race(200, [][]int64{append([]int64{1, 2, 3, 10}, bunch(11, 40)...)}, []float64{0})
	for id, b := range v.Riders {
		b.Rank = int(id) + 50
		v.Riders[id] = b
	}
	lead, tt := v.Riders[1], v.Riders[2] // 1 is our leader, 2 a domestique faster on course time
	lead.Rank, lead.Name = 9, "Leader"
	tt.Rank, tt.Name = 3, "Rouleur"
	v.Riders[1], v.Riders[2] = lead, tt
	d.Next(v)
	if g := d.Goal(); g == nil || g.Text != "Top 5 with Leader" {
		t.Fatalf("the goal is the leader's, at his rank: %+v", g)
	}
}

func TestGCGoalKeptAcrossStages(t *testing.T) {
	d := New("Us")
	v := race(150, [][]int64{append([]int64{1, 2, 3, 10}, bunch(11, 40)...)}, []float64{0})
	v.StageRace = true
	for id, b := range v.Riders {
		b.ClimbWKg = 5.0
		v.Riders[id] = b
	}
	b := v.Riders[3]
	b.ClimbWKg, b.Name = 6.5, "Climber" // ours, the best climber in the field
	v.Riders[3] = b
	d.Next(v)
	if g := d.GCGoal(); g == nil || g.Text != "Win the GC with Climber" {
		t.Fatalf("GC goal: %+v", g)
	}
	d.SetRoute(&simulation.RouteProfile{TotalKm: 180}) // the next stage
	if g := d.GCGoal(); g == nil || g.RiderID != 3 {
		t.Fatalf("the GC goal lasts the whole race: %+v", g)
	}
}
