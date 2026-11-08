package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

func TestTimeCutFractionScalesWithClimbing(t *testing.T) {
	flat := &model.Course{Segments: []model.Segment{{DistanceKm: 200, GradientPct: 0}}}
	mountain := &model.Course{Segments: []model.Segment{{DistanceKm: 120, GradientPct: 0}, {DistanceKm: 40, GradientPct: 7}}}
	f, m := TimeCutFraction(flat, model.RaceTypeRoad), TimeCutFraction(mountain, model.RaceTypeRoad)
	if f != timeCutBase {
		t.Errorf("flat cut = %.3f, want %.3f", f, timeCutBase)
	}
	if m <= f || m > timeCutMax {
		t.Errorf("mountain cut %.3f should exceed flat %.3f and be capped", m, f)
	}
	if TimeCutFraction(flat, model.RaceTypeTT) != timeCutTT {
		t.Error("TT should use the TT cut")
	}
}

func TestApplyTimeCutEliminatesSlowRiders(t *testing.T) {
	flat := &model.Course{Segments: []model.Segment{{DistanceKm: 200}}}
	results := make([]model.RaceResult, 20)
	for i := range results {
		results[i] = model.RaceResult{RiderID: int64(i + 1), Position: i + 1, FinishTimeSeconds: 18000}
	}
	results[19].FinishTimeSeconds = 18000 * 1.10 // 10% down on a 5% flat cut
	if n := ApplyTimeCut(results, flat, model.RaceTypeRoad); n != 1 || results[19].Position != 0 {
		t.Fatalf("expected last rider cut, got n=%d pos=%d", n, results[19].Position)
	}
}

func TestApplyTimeCutWaivedWhenTooManyOutside(t *testing.T) {
	flat := &model.Course{Segments: []model.Segment{{DistanceKm: 200}}}
	results := make([]model.RaceResult, 10)
	for i := range results {
		results[i] = model.RaceResult{RiderID: int64(i + 1), Position: i + 1, FinishTimeSeconds: 18000}
		if i >= 5 {
			results[i].FinishTimeSeconds = 20000
		}
	}
	if n := ApplyTimeCut(results, flat, model.RaceTypeRoad); n != 0 {
		t.Errorf("cut should be waived when half the field misses it, cut %d", n)
	}
}
