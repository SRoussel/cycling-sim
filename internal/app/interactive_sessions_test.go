package app

import (
	"testing"
	"time"

	"cycling_sim/internal/simulation"
)

func TestSessionPersistentAndOneShotCommands(t *testing.T) {
	s := newInteractiveSession("Team A")
	s.SetRiderCommand(1, simulation.ActionAttack, "persistent")
	if a, ok := s.ActionOverride(1); !ok || a != simulation.ActionAttack {
		t.Fatalf("persistent override = %v,%v", a, ok)
	}
	s.SetRiderCommand(1, simulation.ActionHold, "persistent")
	if _, ok := s.ActionOverride(1); ok {
		t.Error("hold should clear persistent override")
	}

	s.SetRiderCommand(2, simulation.ActionAttack, "oneshot")
	if a, ok := s.ConsumeOneShot(2); !ok || a != simulation.ActionAttack {
		t.Fatalf("one-shot = %v,%v", a, ok)
	}
	if _, ok := s.ConsumeOneShot(2); ok {
		t.Error("one-shot must be consumed once")
	}
}

func TestSessionEffortClamped(t *testing.T) {
	s := newInteractiveSession("T")
	for _, c := range []struct{ in, want float64 }{{0.1, 0.6}, {2, 1.3}, {1.1, 1.1}} {
		s.SetRiderEffort(1, c.in)
		if got, _ := s.EffortMultiplier(1); got != c.want {
			t.Errorf("effort %.2f -> %.2f, want %.2f", c.in, got, c.want)
		}
	}
	s.SetRiderEffort(1, 0)
	if _, ok := s.EffortMultiplier(1); ok {
		t.Error("effort 0 should clear")
	}
}

func TestSprintTrainDirectives(t *testing.T) {
	s := newInteractiveSession("Team A")
	group := []int64{5, 3, 4} // the team's riders in one road group, fastest finisher first

	if _, ok := s.SprintTrainDirective(3, "Team A", 20, group); ok {
		t.Error("no directive expected far from finish")
	}
	if _, ok := s.SprintTrainDirective(3, "Team B", 5, group); ok {
		t.Error("other teams must not be directed")
	}
	if _, ok := s.SprintTrainDirective(3, "Team A", 5, group); ok {
		t.Error("the train runs only on the lead-out order")
	}
	if st := s.SprintTrainState(5); st != nil {
		t.Errorf("no lead-out order: no train state, got %+v", st)
	}
	s.SetTeamOrder("leadout", simulation.OrderYes)
	if a, _ := s.SprintTrainDirective(3, "Team A", 5, group); a != simulation.ActionChase {
		t.Errorf("a teammate with the sprinter leads out, got %v", a)
	}
	if a, _ := s.SprintTrainDirective(5, "Team A", 5, group); a != simulation.ActionConserve {
		t.Errorf("the group's fastest finisher is the sprinter and sits in, got %v", a)
	}
	if _, ok := s.SprintTrainDirective(7, "Team A", 5, []int64{7}); ok {
		t.Error("a rider without a teammate in his group races on his own judgement")
	}
	// The player's choice of sprinter wins where he is in the group.
	s.SetSprintTrainOrder([]int64{0, 4})
	if a, _ := s.SprintTrainDirective(4, "Team A", 5, group); a != simulation.ActionConserve {
		t.Errorf("the chosen sprinter sits in, got %v", a)
	}
	if a, _ := s.SprintTrainDirective(5, "Team A", 5, group); a != simulation.ActionChase {
		t.Errorf("the fastest finisher leads out the chosen sprinter, got %v", a)
	}
	if a, _ := s.SprintTrainDirective(5, "Team A", 5, []int64{5, 3}); a != simulation.ActionConserve {
		t.Errorf("without the chosen sprinter, a group rides for its own fastest finisher, got %v", a)
	}
	if st := s.SprintTrainState(5); st.SprintTrainPhase != "following" || len(st.SprintTrainOrder) != 1 || st.SprintTrainOrder[0] != 4 {
		t.Errorf("state = %+v, want following for rider 4", st)
	}

	s.LaunchSprint(5)
	if a, _ := s.SprintTrainDirective(5, "Team A", 1, group); a != simulation.ActionAttack {
		t.Errorf("launching rider should attack, got %v", a)
	}
	if st := s.SprintTrainState(1); st.SprintTrainPhase != "launch" {
		t.Errorf("phase = %s, want launch", st.SprintTrainPhase)
	}
}

func TestSessionFollowIgnoresSelf(t *testing.T) {
	s := newInteractiveSession("T")
	s.SetRiderFollow(1, 1, true)
	if st := s.SprintTrainState(5); st != nil {
		t.Error("following yourself should be ignored")
	}
}

func TestSessionStopUnblocksPausedPace(t *testing.T) {
	s := newInteractiveSession("T")
	s.SetPaused(true)
	done := make(chan struct{})
	go func() { s.Pace(); close(done) }()
	s.Stop()
	s.Stop() // idempotent
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Pace did not return after Stop")
	}
	if !s.IsStopped() {
		t.Error("IsStopped false after Stop")
	}
}

func TestSessionSpeedBounds(t *testing.T) {
	s := newInteractiveSession("T")
	s.SetSpeed(-1)
	if s.speed != 1 {
		t.Errorf("speed<=0 -> %v, want 1", s.speed)
	}
	s.SetSpeed(100)
	if s.speed != 16 {
		t.Errorf("speed>16 -> %v, want 16", s.speed)
	}
}

func TestTeamOrdersApplyToControlledTeamOnly(t *testing.T) {
	s := newInteractiveSession("Team A")
	if got := s.TeamOrders("Team A"); got != (simulation.TeamOrders{}) {
		t.Fatalf("default orders = %+v, want auto", got)
	}
	if !s.SetTeamOrder("chase", simulation.OrderNo) || !s.SetTeamOrder("break", simulation.OrderYes) ||
		!s.SetTeamOrder("pace", simulation.OrderYes) || !s.SetTeamOrder("protect", simulation.OrderYes) ||
		!s.SetTeamOrder("leadout", simulation.OrderNo) || !s.SetTeamOrder("ambition", simulation.OrderYes) {
		t.Fatal("known order kinds rejected")
	}
	if s.SetTeamOrder("sprint", simulation.OrderYes) {
		t.Error("unknown order kind accepted")
	}
	want := simulation.TeamOrders{Chase: simulation.OrderNo, Break: simulation.OrderYes, Pace: simulation.OrderYes,
		Protect: simulation.OrderYes, Leadout: simulation.OrderNo, Ambition: simulation.OrderYes}
	if got := s.TeamOrders("Team A"); got != want {
		t.Errorf("controlled team orders = %+v, want %+v", got, want)
	}
	if got := s.TeamOrders("Team B"); got != (simulation.TeamOrders{}) {
		t.Errorf("other team orders = %+v, want auto", got)
	}
}

func TestRiderStyle(t *testing.T) {
	s := newInteractiveSession("Us")
	if d, p := s.RiderStyle(1); d != simulation.DigAuto || p != simulation.PaceAuto {
		t.Fatalf("default style: %v %v", d, p)
	}
	s.SetRiderStyle(1, "reserve", "tempo")
	if d, p := s.RiderStyle(1); d != simulation.DigKeepReserve || p != simulation.PaceTempo {
		t.Errorf("set style: %v %v", d, p)
	}
	s.SetRiderStyle(1, "auto", "auto")
	if _, ok := s.styles[1]; ok {
		t.Error("the default style should clear the entry")
	}
}
