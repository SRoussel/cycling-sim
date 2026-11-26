package app

import (
	"testing"

	"cycling_sim/internal/director"
	"cycling_sim/internal/simulation"
)

func TestResolveAppliesTimedAndScheduledCommands(t *testing.T) {
	s := newInteractiveSession("Us")
	s.Observe(nil, &simulation.RouteProfile{TotalKm: 100}, 30, 0)
	s.director.pending = &director.Moment{ID: "m1", Options: []director.Option{{
		ID: "go",
		Commands: []director.Command{
			{RiderID: 1, Action: "chase", Mode: "persistent", ForKm: 3},
			{RiderID: 2, Action: "attack", Mode: "oneshot", AtKmRemaining: 25},
		},
		Orders: []director.OrderChange{{Kind: "pace", Value: "yes", ForKm: 4}},
	}}}
	s.paused = true
	if _, err := s.Resolve("m1", "go"); err != nil {
		t.Fatal(err)
	}
	if s.paused || s.PendingMoment() != nil {
		t.Fatalf("resolving should resume the race")
	}
	if a, ok := s.ActionOverride(1); !ok || a != simulation.ActionChase {
		t.Fatalf("rider 1 should chase now")
	}
	if _, ok := s.ConsumeOneShot(2); ok {
		t.Fatalf("rider 2's attack is scheduled for 25 km to go, not now")
	}
	if s.TeamOrders("Us").Pace != simulation.OrderYes {
		t.Fatalf("pace order not applied")
	}
	s.Observe(nil, nil, 26.5, 0) // 3.5 km later: the chase has ended, the order not yet
	if _, ok := s.ActionOverride(1); ok {
		t.Errorf("the 3 km chase should be over")
	}
	if s.TeamOrders("Us").Pace != simulation.OrderYes {
		t.Errorf("the 4 km pace order ended early")
	}
	s.Observe(nil, nil, 24.9, 0)
	if a, ok := s.ConsumeOneShot(2); !ok || a != simulation.ActionAttack {
		t.Errorf("the scheduled attack should fire at 25 km to go")
	}
	if s.TeamOrders("Us").Pace != simulation.OrderAuto {
		t.Errorf("the pace order should revert to auto")
	}
}

func TestPlayerOrderCancelsTimedRevert(t *testing.T) {
	s := newInteractiveSession("Us")
	s.Observe(nil, nil, 30, 0)
	s.director.pending = &director.Moment{ID: "m1", Options: []director.Option{{ID: "x",
		Orders: []director.OrderChange{{Kind: "protect", Value: "yes", ForKm: 5}}}}}
	if _, err := s.Resolve("m1", "x"); err != nil {
		t.Fatal(err)
	}
	s.SetTeamOrder("protect", simulation.OrderNo)
	s.Observe(nil, nil, 20, 0)
	if s.TeamOrders("Us").Protect != simulation.OrderNo {
		t.Fatalf("the player's own order must not be reverted by an old timer")
	}
}

func TestResumeDismissesMoment(t *testing.T) {
	s := newInteractiveSession("Us")
	s.director.pending = &director.Moment{ID: "m1"}
	s.SetPaused(false)
	if s.PendingMoment() != nil {
		t.Fatal("pressing play should dismiss the open decision")
	}
	if _, err := s.Resolve("m1", "x"); err == nil {
		t.Fatal("a dismissed moment can't be resolved")
	}
}

func TestRacePlanSchedulesHardSectionAndAttack(t *testing.T) {
	s := newInteractiveSession("Us")
	pts := []director.PlanPoint{{Key: "s1", Kind: "cobbles", StartKm: 230, LengthKm: 2, ToGoKm: 20}}
	s.SetPlan(RacePlan{LeaderID: 0, Break: "no", HardOn: []string{"s1"}, AttackOn: "s1"}, pts)
	if s.TeamOrders("Us").Break != simulation.OrderNo {
		t.Fatal("break order applies at once")
	}
	view := &simulation.RaceView{Riders: map[int64]simulation.RiderBrief{
		7: {Team: "Us", Role: simulation.RoleLeader}, 8: {Team: "Us"}, 9: {Team: "Them", Role: simulation.RoleLeader}}}
	s.Observe(view, nil, 30, 0)
	if s.TeamOrders("Us").Pace != simulation.OrderAuto {
		t.Fatal("the hard section starts 1 km before the sector, not at 30 km to go")
	}
	s.Observe(nil, nil, 20.9, 0)
	if s.TeamOrders("Us").Pace != simulation.OrderYes {
		t.Fatal("the team drives from 21 km to go into the sector")
	}
	s.Observe(nil, nil, 19.2, 0)
	if a, ok := s.ConsumeOneShot(7); !ok || a != simulation.ActionAttack {
		t.Fatal("the team's leader (from the race view) attacks a third into the sector")
	}
	s.Observe(nil, nil, 17.5, 0)
	if s.TeamOrders("Us").Pace != simulation.OrderAuto {
		t.Fatal("after the sector the team decides the pace again")
	}
	if _, ok := s.LeaderChoice("Us"); ok {
		t.Fatal("no leader chosen: the team decides")
	}
	s.SetPlan(RacePlan{LeaderID: 8}, nil)
	if id, ok := s.LeaderChoice("Us"); !ok || id != 8 {
		t.Fatal("the chosen leader")
	}
	if _, ok := s.LeaderChoice("Them"); ok {
		t.Fatal("only the player's team")
	}
}

func TestStagePlanAskedOncePerStageAndCleared(t *testing.T) {
	s := newInteractiveSession("Us")
	s.SetDecisionLevel("all")
	pts := []director.PlanPoint{{Key: "c1", Kind: "climb", StartKm: 150, LengthKm: 8, ToGoKm: 10}}
	s.Observe(nil, &simulation.RouteProfile{TotalKm: 160}, 160, 2)
	if r := s.RequestStagePlan(2, "Stage 2", pts); r == nil || !s.paused {
		t.Fatal("the race waits for the stage plan")
	}
	if r := s.RequestStagePlan(2, "Stage 2", pts); r != nil {
		t.Fatal("asked once per stage")
	}
	s.SetStagePlan(RacePlan{LeaderID: 5, HardOn: []string{"c1"}})
	if s.paused {
		t.Fatal("planning starts the stage")
	}
	if id, ok := s.LeaderChoice("Us"); !ok || id != 5 {
		t.Fatal("the stage's leader")
	}
	s.Observe(nil, &simulation.RouteProfile{TotalKm: 180}, 180, 3) // next stage
	if _, ok := s.LeaderChoice("Us"); ok {
		t.Fatal("a stage's plan ends with the stage")
	}
	if len(s.director.scheduledOrders) != 0 {
		t.Fatal("the last stage's hard sections are cleared")
	}
	s.SetDecisionLevel("off")
	if r := s.RequestStagePlan(3, "Stage 3", pts); r != nil {
		t.Fatal("with decisions off the race doesn't stop for plans")
	}
}

func TestLiveReportsWhoSetWhat(t *testing.T) {
	s := newInteractiveSession("Us")
	s.Observe(nil, nil, 50, 0)
	s.director.pending = &director.Moment{ID: "m1", Title: "⚡ Pogačar attacks!", Options: []director.Option{{ID: "f",
		Commands: []director.Command{{RiderID: 3, Action: "chase", Mode: "persistent", ForKm: 3}},
		Orders:   []director.OrderChange{{Kind: "pace", Value: "yes", ForKm: 4}}}}}
	if _, err := s.Resolve("m1", "f"); err != nil {
		t.Fatal(err)
	}
	s.SetTeamOrder("chase", simulation.OrderNo)
	s.SetRiderCommand(4, simulation.ActionConserve, "persistent")
	live := s.Live()
	if o := live.Orders["pace"]; o.Value != "yes" || o.Source != "decision" || o.Note != "Pogačar attacks!" || o.KmLeft != 4 {
		t.Errorf("pace: %+v", o)
	}
	if o := live.Orders["chase"]; o.Value != "no" || o.Source != "you" {
		t.Errorf("chase: %+v", o)
	}
	if o := live.Orders["break"]; o.Value != "auto" || o.Source != "" {
		t.Errorf("break: %+v", o)
	}
	if r := live.Riders[3]; r.Action != "chase" || r.Source != "decision" || r.KmLeft != 3 {
		t.Errorf("rider 3: %+v", r)
	}
	if r := live.Riders[4]; r.Action != "conserve" || r.Source != "you" || r.KmLeft != 0 {
		t.Errorf("rider 4: %+v", r)
	}
	s.Observe(nil, nil, 45, 0)
	live = s.Live()
	if _, ok := live.Riders[3]; ok {
		t.Error("the decision's 3 km chase is over")
	}
	if o := live.Orders["pace"]; o.Value != "auto" || o.Source != "" {
		t.Errorf("the decision's pace order handed back: %+v", o)
	}
}

// A break rider's "take turns" order lasts while he is up the road and ends
// once the bunch catches him (no chasing the bunch's front for 100 km).
func TestBreakOrderEndsWhenCaught(t *testing.T) {
	s := newInteractiveSession("Us")
	s.Observe(nil, &simulation.RouteProfile{TotalKm: 200}, 150, 0)
	s.director.pending = &director.Moment{ID: "m1", Options: []director.Option{{
		ID: "work", Commands: []director.Command{{RiderID: 1, Action: "chase", Mode: "persistent", WhileAhead: true}},
	}}}
	if _, err := s.Resolve("m1", "work"); err != nil {
		t.Fatal(err)
	}
	view := func(km float64, groups ...[]int64) *simulation.RaceView {
		v := &simulation.RaceView{KmRemaining: km, KmTotal: 200, Riders: map[int64]simulation.RiderBrief{}}
		for _, ids := range groups {
			v.Road = append(v.Road, simulation.RoadGroup{IDs: ids})
			for _, id := range ids {
				v.Riders[id] = simulation.RiderBrief{Team: "Rivals"}
			}
		}
		return v
	}
	bunch := []int64{10, 11, 12, 13, 14, 15}
	s.Observe(view(120, []int64{1, 2}, bunch), nil, 120, 0)
	if a, ok := s.ActionOverride(1); !ok || a != simulation.ActionChase {
		t.Fatalf("still in the break: the order holds")
	}
	s.Observe(view(90, append([]int64{1, 2}, bunch...)), nil, 90, 0)
	if _, ok := s.ActionOverride(1); ok {
		t.Fatalf("caught: the break order should end")
	}
}

func TestStageTitle(t *testing.T) {
	for in, want := range map[string]string{
		"Tirreno 2026 Stage 2: Camaiore - San Gimignano": "Camaiore - San Gimignano",
		"Basque Stage 1 - Rolling":                       "Basque Stage 1 - Rolling",
	} {
		if got := stageTitle(in); got != want {
			t.Errorf("stageTitle(%q) = %q, want %q", in, got, want)
		}
	}
}
