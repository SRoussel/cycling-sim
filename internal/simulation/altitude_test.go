package simulation

import (
	"math"
	"testing"

	"cycling_sim/internal/model"
)

func TestAirDensityAt(t *testing.T) {
	if got := AirDensityAt(0); got != airDensity {
		t.Errorf("sea level %.3f, want %.3f", got, airDensity)
	}
	// Standard atmosphere: ~1.007 kg/m³ at 2000 m.
	if got := AirDensityAt(2000); math.Abs(got-1.007) > 0.01 {
		t.Errorf("2000 m: %.3f, want ~1.007", got)
	}
}

func TestAltitudePowerFactor(t *testing.T) {
	for _, c := range []struct{ m, lo, hi float64 }{
		{0, 1, 1}, {1500, 0.94, 0.96}, {2000, 0.91, 0.93}, {2600, 0.86, 0.89},
	} {
		if got := AltitudePowerFactor(c.m); got < c.lo || got > c.hi {
			t.Errorf("%.0f m: %.3f, want %.2f-%.2f", c.m, got, c.lo, c.hi)
		}
	}
}

func TestPrepareSegmentsElevation(t *testing.T) {
	segs := []model.Segment{
		{Name: "down", DistanceKm: 10, GradientPct: -2, SurfaceType: model.SurfaceRoad},
		{Name: "up", DistanceKm: 10, GradientPct: 8, SurfaceType: model.SurfaceRoad},
	}
	out := PrepareSegments(segs, nil, 0)
	// Lowest point (end of the descent, -200 m) becomes 0 m: the descent's
	// midpoint is at 100 m and the climb's at 400 m.
	if math.Abs(out[0].ElevationM-100) > 1e-6 || math.Abs(out[1].ElevationM-400) > 1e-6 {
		t.Errorf("elevations %.0f, %.0f; want 100, 400", out[0].ElevationM, out[1].ElevationM)
	}
	if !out[1].ClimbSet || out[1].Climb <= terrainClimb {
		t.Errorf("8%% climb should be cached as a climb, got %.2f (set %v)", out[1].Climb, out[1].ClimbSet)
	}
	if segs[0].ElevationM != 0 {
		t.Error("PrepareSegments must not modify its input")
	}
}

func TestClimbingAbilityBlendsWithTerrain(t *testing.T) {
	r := &model.Rider{ClimbingAbility: 95}
	flat := &model.Segment{GradientPct: 0, SurfaceType: model.SurfaceRoad}
	drag := &model.Segment{GradientPct: 2.5, SurfaceType: model.SurfaceRoad}
	climb := &model.Segment{GradientPct: 8, SurfaceType: model.SurfaceRoad}
	pf, pd, pc := EffectivePower(r, 400, flat, false), EffectivePower(r, 400, drag, false), EffectivePower(r, 400, climb, false)
	if pf != 400 || !(pd > pf && pc > pd) {
		t.Errorf("climbing bonus should grow smoothly with the climb: flat %.1f, drag %.1f, climb %.1f", pf, pd, pc)
	}
	if want := 400 * ModifierFactor(95); math.Abs(pc-want) > 0.5 {
		t.Errorf("full bonus on a real climb: %.1f, want %.1f", pc, want)
	}
}

func TestWPrimeUsesTerrainThreshold(t *testing.T) {
	// A strong climber riding exactly at their (ability-raised) threshold on a
	// climb must not drain W'.
	r := model.Rider{FTPWatts: 400, WeightKg: 60, ClimbingAbility: 95, Endurance: 70, Handling: 70, CdA: 0.3}
	rs := &RiderState{Rider: &r, EffectiveFTP: 400, FormFactor: 1, WPrime: NewWPrimeState(&r)}
	seg := &model.Segment{GradientPct: 8, SurfaceType: model.SurfaceRoad}
	before := rs.WPrime.FractionRemaining()
	p := EffectivePower(&r, rs.CurrentFTP(), seg, false)
	rs.WPrime.Update(p, rs.thresholdIntOn(seg), 600)
	if rs.WPrime.FractionRemaining() < before-0.01 {
		t.Errorf("W' drained riding at threshold: %.2f → %.2f", before, rs.WPrime.FractionRemaining())
	}
}

func TestFatiguedSprintPower(t *testing.T) {
	r := model.Rider{MaxSprintWatts: 1600, SprintAbility: 70, Endurance: 70, FTPWatts: 400}
	fresh := &RiderState{Rider: &r, WPrime: NewWPrimeState(&r)}
	tired := &RiderState{Rider: &r, WPrime: NewWPrimeState(&r), WorkKJ: 4000}
	tired.WPrime.Update(800, 400, 60) // dig deep
	if FatiguedSprintPower(fresh) != SprintPower(&r) {
		t.Errorf("fresh sprint %.0f, want %.0f", FatiguedSprintPower(fresh), SprintPower(&r))
	}
	if f := FatiguedSprintPower(tired) / SprintPower(&r); f > 0.95 || f < 0.8 {
		t.Errorf("sprint after 4000 kJ and an emptied W' at %.0f%% of fresh, want 80-95%%", f*100)
	}
}

func TestTierCorrelation(t *testing.T) {
	pos := map[int64]float64{1: 1, 2: 2, 3: 3, 4: 5, 5: 4}
	// Riders 3-5 share a tier, so their mutual order doesn't matter.
	if c := validationTierCorrelation([][]int64{{1}, {2}, {3, 4, 5}}, pos); c < 0.85 {
		t.Errorf("tiered correlation %.2f, want high", c)
	}
	if c := validationSpearman([]int64{1, 2, 3, 4, 5}, pos); c >= 1 || c < 0.8 {
		t.Errorf("strict correlation %.2f, want just below 1", c)
	}
}

func TestFindIntrudersSkipsUnnamedPlaces(t *testing.T) {
	entry := RealRaceEntry{Top10RiderIDs: []int64{1, 2, 0, 0, 0, 0, 0, 0, 0, 0}}
	top10 := map[int64]int{1: 10, 2: 10, 10: 10, 11: 9, 12: 8, 13: 8, 14: 8, 15: 8, 16: 8, 17: 8, 18: 6}
	pos := func(id int64) float64 { return float64(id) }
	got := findIntruders(entry, top10, map[int64]int{}, 10, pos, pos)
	// 9 frequent non-reference riders, 8 unnamed places: only the last is judged.
	if len(got) != 1 || got[0].RiderID != 18 {
		t.Errorf("intruders %+v, want only rider 18", got)
	}
}

func TestWithStarters(t *testing.T) {
	pool := []model.Rider{{ID: 1}, {ID: 2}, {ID: 3}}
	got := withStarters([]model.Rider{{ID: 1}}, pool, []int64{3, 1})
	if len(got) != 2 || got[1].ID != 3 {
		t.Errorf("start list %+v, want riders 1 and 3", got)
	}
}
