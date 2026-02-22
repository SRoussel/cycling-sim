package simulation

// Sprint 2 tests: GC mountain attack tuning, GC gap awareness, domestique sacrifice.

import (
	"math/rand"
	"testing"

	"cycling_sim/internal/model"
)

// buildStageRaceState creates a minimal RaceState for a stage-race mountain scenario.
func buildStageRaceState(segment *model.Segment, kmRemaining float64, hasDangerousBreakaway bool, breakawayGap float64, gcGaps map[int64]float64, riders []*RiderState) *RaceState {
	state := NewRaceState(riders, rand.New(rand.NewSource(1)), true)
	state.Segment = segment
	state.KmRemaining = kmRemaining
	state.Phase = PhaseFromKm(kmRemaining)
	state.BreakawayHasDangerous = hasDangerousBreakaway
	state.HasBreakaway = hasDangerousBreakaway || breakawayGap > 0
	state.BreakawayGap = breakawayGap
	state.GCGaps = gcGaps
	return state
}

// makeGCLeader creates a leader-role rider and a corresponding RiderState.
func makeGCLeader(id int64, team string) (*model.Rider, *RiderState) {
	r := makeClimber("Leader")
	r.ID = id
	r.Team = team
	r.TacticalAwareness = 85
	rs := &RiderState{
		Rider:        &r,
		WPrime:       NewWPrimeState(&r),
		FormFactor:   1.0,
		EffectiveFTP: r.FTPWatts,
		TeamRole:     RoleLeader,
	}
	return &r, rs
}

// TestGCLeaderMountainAttackFrequency verifies that GC leaders attack in the mountain
// final at a meaningful rate — substantially more than the old 3% ceiling.
func TestGCLeaderMountainAttackFrequency(t *testing.T) {
	_, rs := makeGCLeader(1, "Team A")

	// Make a domestique so the team context can be built.
	dom := makeRider("Dom", 380, 72, 70, 60, 70, 70, 75, 72, 70, 70)
	dom.ID = 2
	dom.Team = "Team A"
	domRS := &RiderState{Rider: &dom, WPrime: NewWPrimeState(&dom), FormFactor: 1.0, EffectiveFTP: dom.FTPWatts, TeamRole: RoleDomestique}

	// Rival favorite (another strong climber).
	rival := makeClimber("Rival")
	rival.ID = 3
	rival.Team = "Team B"
	rivalRS := &RiderState{Rider: &rival, WPrime: NewWPrimeState(&rival), FormFactor: 1.0, EffectiveFTP: rival.FTPWatts, TeamRole: RoleLeader}

	riders := []*RiderState{rs, domRS, rivalRS}
	allRiders := []*RiderState{rs, domRS, rivalRS}

	// HC climb, 8km from summit (PhaseFinal), gradient 7%.
	seg := &model.Segment{GradientPct: 7.0, DistanceKm: 8.0, SurfaceType: model.SurfaceRoad}

	n := 10000
	params := DefaultTacticsParams()

	// No GC deficit — rs is GC leader.
	gcGaps := map[int64]float64{1: 0, 2: 120, 3: 30}

	state := buildStageRaceState(seg, 8.0, false, 0, gcGaps, allRiders)
	_ = riders
	rng := rand.New(rand.NewSource(42))

	attacks := 0
	for i := 0; i < n; i++ {
		action := DecideAction(rs, state, true, rng, params)
		if action == ActionAttack {
			attacks++
		}
	}

	attackPct := float64(attacks) * 100.0 / float64(n)
	t.Logf("GC leader mountain attack rate (8km from summit, gradient 7%%): %.1f%%", attackPct)

	// Should attack at least 5% of the time (old cap was ~4%, now should be ≥8%).
	if attackPct < 5.0 {
		t.Errorf("GC leader mountain attack rate %.1f%% is too low — stage races feel flat", attackPct)
	}
}

// TestGCDeficitBoostsAttackProbability validates that a rider far behind in GC
// attacks more often than a rider close to the lead.
func TestGCDeficitBoostsAttackProbability(t *testing.T) {
	makeLeaderState := func(id int64, gcGap float64, rivals []*RiderState) (*RiderState, *RaceState) {
		_, rs := makeGCLeader(id, "Team X")
		gcGaps := map[int64]float64{id: gcGap}
		seg := &model.Segment{GradientPct: 7.0, DistanceKm: 8.0, SurfaceType: model.SurfaceRoad}
		state := buildStageRaceState(seg, 8.0, false, 0, gcGaps, append(rivals, rs))
		return rs, state
	}

	// Build a shared set of rivals.
	rival1 := makeClimber("R1")
	rival1.ID = 99
	rival1.Team = "TeamR"
	r1RS := &RiderState{Rider: &rival1, WPrime: NewWPrimeState(&rival1), FormFactor: 1.0, EffectiveFTP: rival1.FTPWatts, TeamRole: RoleLeader}
	rivals := []*RiderState{r1RS}

	rsDose, stateDose := makeLeaderState(1, 0, rivals)   // leads the race
	rsDeficit, stateDeficit := makeLeaderState(2, 1200, rivals) // 20 minutes down

	n := 10000
	params := DefaultTacticsParams()

	rng1 := rand.New(rand.NewSource(77))
	rng2 := rand.New(rand.NewSource(77))

	leaderAttacks := 0
	deficitAttacks := 0
	for i := 0; i < n; i++ {
		if DecideAction(rsDose, stateDose, true, rng1, params) == ActionAttack {
			leaderAttacks++
		}
		if DecideAction(rsDeficit, stateDeficit, true, rng2, params) == ActionAttack {
			deficitAttacks++
		}
	}

	t.Logf("GC leader (0s deficit): %.1f%% attacks", float64(leaderAttacks)*100/float64(n))
	t.Logf("20-min down rider: %.1f%% attacks", float64(deficitAttacks)*100/float64(n))

	if deficitAttacks <= leaderAttacks {
		t.Errorf("rider 20-min down (%d attacks) should attack more than GC leader (%d attacks)",
			deficitAttacks, leaderAttacks)
	}
}

// TestCounterAttackDangerousBreakaway checks that when a dangerous rival is in
// a breakaway on a mountain finish, GC leaders respond with high urgency.
func TestCounterAttackDangerousBreakaway(t *testing.T) {
	rival := makeClimber("Rival")
	rival.ID = 10
	rival.Team = "Team B"
	rivalRS := &RiderState{Rider: &rival, WPrime: NewWPrimeState(&rival), FormFactor: 1.0, EffectiveFTP: rival.FTPWatts, TeamRole: RoleLeader}

	_, rs := makeGCLeader(1, "Team A")
	riders := []*RiderState{rs, rivalRS}

	seg := &model.Segment{GradientPct: 7.5, DistanceKm: 6.0, SurfaceType: model.SurfaceRoad}
	gcGaps := map[int64]float64{1: 30, 10: 0} // rival leads by 30s

	state := buildStageRaceState(seg, 6.0, true /* dangerous */, 45, gcGaps, riders)

	n := 10000
	params := DefaultTacticsParams()
	rng := rand.New(rand.NewSource(99))

	attacks := 0
	for i := 0; i < n; i++ {
		if DecideAction(rs, state, true, rng, params) == ActionAttack {
			attacks++
		}
	}

	attackPct := float64(attacks) * 100.0 / float64(n)
	t.Logf("Counter-attack vs dangerous breakaway (mountain final): %.1f%%", attackPct)

	// Should attack at least 15% — rival is up the road on a mountain, this is urgent.
	if attackPct < 15.0 {
		t.Errorf("counter-attack probability %.1f%% is too low when dangerous rival is in breakaway", attackPct)
	}
}

// TestMountainAttackNotBlockedByBreakaway validates the ordering fix:
// a GC leader on a mountain should be able to ActionAttack even when HasBreakaway
// is true (previously they would always ActionChase).
func TestMountainAttackNotBlockedByBreakaway(t *testing.T) {
	rival := makeClimber("Rival")
	rival.ID = 2
	rival.Team = "Team B"
	rivalRS := &RiderState{Rider: &rival, WPrime: NewWPrimeState(&rival), FormFactor: 1.0, EffectiveFTP: rival.FTPWatts, TeamRole: RoleLeader}

	_, rs := makeGCLeader(1, "Team A")
	riders := []*RiderState{rs, rivalRS}

	seg := &model.Segment{GradientPct: 6.5, DistanceKm: 8.0, SurfaceType: model.SurfaceRoad}
	// A big gap breakaway is active — old code would force ActionChase.
	gcGaps := map[int64]float64{1: 180, 2: 0}
	state := buildStageRaceState(seg, 8.0, false, 60 /* big gap */, gcGaps, riders)

	n := 10000
	params := DefaultTacticsParams()
	rng := rand.New(rand.NewSource(11))

	attacks := 0
	for i := 0; i < n; i++ {
		if DecideAction(rs, state, true, rng, params) == ActionAttack {
			attacks++
		}
	}

	if attacks == 0 {
		t.Error("GC leader on mountain should sometimes attack even when a breakaway is active — ordering bug still present")
	}
	t.Logf("GC leader attacks despite active breakaway: %d/%d (%.1f%%)", attacks, n, float64(attacks)*100/float64(n))
}

// TestDomestiqueSacrificeClimbPower verifies that a domestique's power is boosted
// when climbing with their team leader in the group.
func TestDomestiqueSacrificeClimbPower(t *testing.T) {
	dom := makeRider("Dom", 380, 72, 70, 60, 70, 70, 75, 72, 70, 70)
	dom.ID = 1
	dom.Team = "Team A"

	leader := makeClimber("Leader")
	leader.ID = 2
	leader.Team = "Team A"

	// Scenario 1: flat stage (gradient 0.5%) — no sacrifice
	flatSeg := &model.Segment{GradientPct: 0.5, DistanceKm: 20, SurfaceType: model.SurfaceRoad}
	// Scenario 2: climb (gradient 6%) — sacrifice fires
	climbSeg := &model.Segment{GradientPct: 6.0, DistanceKm: 8, SurfaceType: model.SurfaceRoad}

	flatPowers := measureDomestiquePowers(&dom, &leader, flatSeg, 100, 500)
	climbPowers := measureDomestiquePowers(&dom, &leader, climbSeg, 100, 500)

	avgFlat := avg(flatPowers)
	avgClimb := avg(climbPowers)

	t.Logf("Domestique avg power — flat: %.1fW, climb (with leader): %.1fW", avgFlat, avgClimb)

	if avgClimb <= avgFlat {
		t.Errorf("domestique should push harder on climbs (%.1fW) than flat (%.1fW) when leader is in group",
			avgClimb, avgFlat)
	}
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
