package main

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
	s.SetSprintTrainOrder([]int64{3, 3, 0, 4, 5}) // dupes and invalid IDs dropped
	if st := s.SprintTrainState(20); st == nil || len(st.SprintTrainOrder) != 3 {
		t.Fatalf("order not deduped: %+v", st)
	}

	if _, ok := s.SprintTrainDirective(3, "Team A", 20); ok {
		t.Error("no directive expected far from finish")
	}
	if _, ok := s.SprintTrainDirective(3, "Team B", 5); ok {
		t.Error("other teams must not be directed")
	}
	if a, _ := s.SprintTrainDirective(3, "Team A", 5); a != simulation.ActionChase {
		t.Errorf("lead-out rider should chase, got %v", a)
	}
	if a, _ := s.SprintTrainDirective(5, "Team A", 5); a != simulation.ActionConserve {
		t.Errorf("last in train (sprinter) should conserve, got %v", a)
	}
	if st := s.SprintTrainState(5); st.SprintTrainPhase != "following" {
		t.Errorf("phase = %s, want following", st.SprintTrainPhase)
	}

	s.LaunchSprint(5)
	if a, _ := s.SprintTrainDirective(5, "Team A", 1); a != simulation.ActionAttack {
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
