package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

func TestPowerCurve(t *testing.T) {
	r := model.Rider{FTPWatts: 400, MaxSprintWatts: 1400, SprintAbility: 70, WeightKg: 68, Endurance: 70}
	secs := []float64{5, 60, 300, 1200, 3600, 4 * 3600}
	p := PowerCurve(&r, secs)
	if p[0] != SprintPower(&r) {
		t.Errorf("5 s: %.0f W, want the sprint %.0f W", p[0], SprintPower(&r))
	}
	for i := 1; i < len(p); i++ {
		if p[i] >= p[i-1] {
			t.Errorf("curve not falling: %.0f s %.0f W after %.0f W", secs[i], p[i], p[i-1])
		}
	}
	if p[3] < 400 || p[3] > 440 {
		t.Errorf("20 min: %.0f W, want a little over threshold", p[3])
	}
	if p[5] >= 400 {
		t.Errorf("4 h: %.0f W, want below threshold (durability)", p[5])
	}
}
