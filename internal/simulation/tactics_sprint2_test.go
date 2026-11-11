package simulation

// Stage-race team tests: domestiques driving for their leader. (GC attacks
// are physics now, see TestAttackPaysOnClimbsNotFlat.)

import (
	"math/rand"
	"testing"

	"cycling_sim/internal/model"
)

// TestDomestiqueSacrificeClimbPower verifies that a domestique's power is boosted
// when climbing with their team leader in the group.
func TestDomestiqueDrivesWhenTerrainFavoursLeader(t *testing.T) {
	dom := makeRider("Dom", 380, 72, 70, 60, 70, 70, 75, 72, 70, 70)
	dom.ID = 1
	dom.Team = "Team A"

	leader := makeClimber("Leader")
	leader.ID = 2
	leader.Team = "Team A"

	// Flat: a sprinter is as fast as the climber, so there's no reason to drive.
	flatSeg := &model.Segment{GradientPct: 0.5, DistanceKm: 20, SurfaceType: model.SurfaceRoad}
	// Climb: the climber's W/kg wins, so the team drives to drop the sprinter.
	climbSeg := &model.Segment{GradientPct: 6.0, DistanceKm: 8, SurfaceType: model.SurfaceRoad}

	flatAction := domestiqueAction(&dom, &leader, flatSeg, 40, false)
	climbAction := domestiqueAction(&dom, &leader, climbSeg, 40, false)
	earlyStage := domestiqueAction(&dom, &leader, climbSeg, 150, true)
	earlyOneDay := domestiqueAction(&dom, &leader, climbSeg, 150, false)

	if climbAction != ActionChase {
		t.Errorf("domestique should drive (chase) on a climb favouring the leader, got %v", climbAction)
	}
	if flatAction == ActionChase {
		t.Errorf("domestique should not drive on the flat")
	}
	if earlyStage == ActionChase {
		t.Errorf("stage races: teams should not drive in the early phase")
	}
	if earlyOneDay != ActionChase {
		t.Errorf("one-day races: a favourite's team wears the field down from far out, got %v", earlyOneDay)
	}
}

// domestiqueAction runs FormGroups with a domestique, its climbing leader and
// a rival sprinter, returning the domestique's chosen action.
func domestiqueAction(dom, leader *model.Rider, seg *model.Segment, remaining float64, stageRace bool) Action {
	sprinter := makeSprinter("Rival")
	sprinter.ID = 3
	sprinter.Team = "Team B"
	rng := rand.New(rand.NewSource(1))
	domRS := &RiderState{Rider: dom, WPrime: NewWPrimeState(dom), FormFactor: 1.0, EffectiveFTP: dom.FTPWatts, TeamRole: RoleDomestique}
	leaderRS := &RiderState{Rider: leader, WPrime: NewWPrimeState(leader), FormFactor: 1.0, EffectiveFTP: leader.FTPWatts, TeamRole: RoleLeader}
	sprRS := &RiderState{Rider: &sprinter, WPrime: NewWPrimeState(&sprinter), FormFactor: 1.0, EffectiveFTP: sprinter.FTPWatts, TeamRole: RoleSprinter}
	riders := []*RiderState{domRS, leaderRS, sprRS}
	state := NewRaceState(riders, rng, stageRace)
	state.Segment = seg
	state.KmRemaining = remaining
	state.Phase = PhaseFromKm(remaining)
	params := DefaultTacticsParams()
	FormGroups(riders, seg, false, remaining, rng, state, &params)
	return state.LastActions[dom.ID]
}

// measureDomestiquePowers runs FormGroups n times and returns the domestique's LastSegmentPower.
func measureDomestiquePowers(dom *model.Rider, leader *model.Rider, seg *model.Segment, nRuns int, baseRemaining float64) []float64 {
	var powers []float64
	for seed := int64(0); seed < int64(nRuns); seed++ {
		rng := rand.New(rand.NewSource(seed))
		domRS := &RiderState{Rider: dom, WPrime: NewWPrimeState(dom), FormFactor: 1.0, EffectiveFTP: dom.FTPWatts, TeamRole: RoleDomestique}
		leaderRS := &RiderState{Rider: leader, WPrime: NewWPrimeState(leader), FormFactor: 1.0, EffectiveFTP: leader.FTPWatts, TeamRole: RoleLeader}
		riders := []*RiderState{domRS, leaderRS}
		state := NewRaceState(riders, rng, true)
		state.Segment = seg
		state.KmRemaining = baseRemaining
		state.Phase = PhaseFromKm(baseRemaining)
		params := DefaultTacticsParams()
		FormGroups(riders, seg, false, baseRemaining, rng, state, &params)
		powers = append(powers, domRS.LastSegmentPower)
	}
	return powers
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
