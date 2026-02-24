package simulation

import (
	"math/rand"
	"testing"

	"cycling_sim/internal/model"
)

type staticActionOverride struct {
	riderID int64
	action  Action
}

func (o *staticActionOverride) ActionOverride(riderID int64) (Action, bool) {
	if riderID != o.riderID {
		return ActionHold, false
	}
	return o.action, true
}

func (o *staticActionOverride) ConsumeOneShot(int64) (Action, bool)    { return ActionHold, false }
func (o *staticActionOverride) EffortMultiplier(int64) (float64, bool) { return 1.0, false }
func (o *staticActionOverride) SprintTrainDirective(int64, string, float64) (Action, bool) {
	return ActionHold, false
}
func (o *staticActionOverride) SprintTrainState(float64) *TeamState { return nil }

func TestAttackEventOnlyOnTransition(t *testing.T) {
	r := testRider()
	r.ID = 1
	rs := &RiderState{Rider: r, WPrime: NewWPrimeState(r), FormFactor: 1.0, EffectiveFTP: r.FTPWatts}
	states := []*RiderState{rs}

	rng := rand.New(rand.NewSource(1))
	state := NewRaceState(states, rng, false, false)
	overrides := &staticActionOverride{riderID: r.ID, action: ActionAttack}
	state.Overrides = overrides

	seg := &model.Segment{GradientPct: 0, DistanceKm: 1.0, SurfaceType: model.SurfaceRoad}

	// Repeated attack overrides across multiple segments should only emit one "attack" event.
	for kmRemaining := 50.0; kmRemaining > 45.0; kmRemaining-- {
		_ = FormGroups(states, seg, false, kmRemaining, rng, state, nil)
	}

	attackEvents := 0
	for _, e := range state.Events {
		if e.Type == "attack" {
			attackEvents++
		}
	}
	if attackEvents != 1 {
		t.Fatalf("expected 1 attack event for continuous ActionAttack, got %d", attackEvents)
	}

	// If the rider stops attacking and later attacks again, we should emit a new event.
	overrides.action = ActionConserve
	_ = FormGroups(states, seg, false, 44.0, rng, state, nil)
	overrides.action = ActionAttack
	_ = FormGroups(states, seg, false, 43.0, rng, state, nil)

	attackEvents = 0
	for _, e := range state.Events {
		if e.Type == "attack" {
			attackEvents++
		}
	}
	if attackEvents != 2 {
		t.Fatalf("expected 2 attack events after stopping and re-attacking, got %d", attackEvents)
	}
}
