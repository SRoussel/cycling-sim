package main

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
