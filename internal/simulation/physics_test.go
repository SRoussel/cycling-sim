package simulation

import (
	"math"
	"testing"

	"cycling_sim/internal/model"
)

func TestSpeedFromPowerFlat(t *testing.T) {
	// 400W, 75kg rider + 8kg bike = 83kg, flat, CdA=0.32
	// Expected: ~45 km/h
	speed := SpeedFromPower(400, 83, 0.32, 0, model.SurfaceRoad, 70)
	speedKmh := speed * 3.6
	if speedKmh < 43 || speedKmh > 47 {
		t.Errorf("400W flat speed = %.1f km/h, expected ~45 km/h", speedKmh)
	}
}

func TestSpeedFromPowerClimb(t *testing.T) {
	// 380W, 65kg rider + 8kg bike = 73kg, 8% gradient, CdA=0.32
	// Expected: ~21 km/h
	speed := SpeedFromPower(380, 73, 0.32, 8.0, model.SurfaceRoad, 70)
	speedKmh := speed * 3.6
	if speedKmh < 19 || speedKmh > 23 {
		t.Errorf("380W 8%% climb speed = %.1f km/h, expected ~21 km/h", speedKmh)
	}
}

func TestDraftingPowerSavings(t *testing.T) {
	// At 45 km/h, drafting with factor 0.25 should save ~35-40% power
	baseCdA := 0.32
	speed := 45.0 / 3.6 // m/s
	mass := 83.0        // kg

	// Power needed solo
	fGrav := ForceGravity(mass, 0)
	fRoll := ForceRolling(mass, 0, model.SurfaceRoad, 70)
	fAero := ForceAero(baseCdA, speed)
	powerSolo := speed * (fGrav + fRoll + fAero)

	// Power needed in draft
	fAeroDraft := ForceAero(baseCdA*0.25, speed)
	powerDraft := speed * (fGrav + fRoll + fAeroDraft)

	savings := 1.0 - powerDraft/powerSolo
	// At 45 km/h, aero is ~70% of total drag. Reducing CdA by 75% saves ~50-70% total.
	if savings < 0.50 || savings > 0.75 {
		t.Errorf("drafting power savings = %.1f%%, expected 50-70%%", savings*100)
	}
}

func TestDraftFactors(t *testing.T) {
	tests := []struct {
		pos      int
		expected float64
	}{
		{1, 1.0},
		{2, 0.65},
		{3, 0.55},
		{5, 0.55},
		{6, 0.40},
		{15, 0.40},
		{16, 0.25},
		{100, 0.25},
	}
	for _, tt := range tests {
		if got := DraftFactor(tt.pos); got != tt.expected {
			t.Errorf("DraftFactor(%d) = %.2f, want %.2f", tt.pos, got, tt.expected)
		}
	}
}

func TestModifierFactor(t *testing.T) {
	if f := ModifierFactor(70); math.Abs(f-1.0) > 0.001 {
		t.Errorf("ModifierFactor(70) = %.4f, want 1.0", f)
	}
	if f := ModifierFactor(90); math.Abs(f-1.04) > 0.001 {
		t.Errorf("ModifierFactor(90) = %.4f, want 1.04", f)
	}
	if f := ModifierFactor(50); math.Abs(f-0.96) > 0.001 {
		t.Errorf("ModifierFactor(50) = %.4f, want 0.96", f)
	}
}

func TestSpeedOnSteepClimb(t *testing.T) {
	// Very steep climb (15%) - speed should still be reasonable
	speed := SpeedFromPower(400, 73, 0.32, 15.0, model.SurfaceRoad, 70)
	speedKmh := speed * 3.6
	if speedKmh < 8 || speedKmh > 14 {
		t.Errorf("400W 15%% climb speed = %.1f km/h, expected 8-14 km/h", speedKmh)
	}
}

func TestDescentSpeed(t *testing.T) {
	// Descent at -8%, should be fast but capped
	speed := DescentSpeed(200, 83, 0.32, -8.0, model.SurfaceRoad, 70, 70, false)
	speedKmh := speed * 3.6
	if speedKmh < 50 || speedKmh > 90 {
		t.Errorf("descent speed = %.1f km/h, expected 50-90 km/h", speedKmh)
	}

	// Technical descent should be slower
	techSpeed := DescentSpeed(200, 83, 0.32, -8.0, model.SurfaceRoad, 70, 70, true)
	if techSpeed >= speed {
		t.Errorf("technical descent should be slower: %.1f >= %.1f", techSpeed*3.6, speed*3.6)
	}
}

func TestSegmentTime(t *testing.T) {
	// 10 km at 10 m/s (36 km/h) = 1000 seconds
	time := SegmentTime(10.0, 10.0)
	if math.Abs(time-1000.0) > 0.1 {
		t.Errorf("SegmentTime = %.1f, want 1000", time)
	}
}

func TestCobblesPenalty(t *testing.T) {
	// Cobbles should be slower than road at same power
	roadSpeed := SpeedFromPower(350, 83, 0.32, 0, model.SurfaceRoad, 70)
	cobbleSpeed := SpeedFromPower(350, 83, 0.32, 0, model.SurfaceCobbles, 70)
	if cobbleSpeed >= roadSpeed {
		t.Errorf("cobbles should be slower: %.2f >= %.2f m/s", cobbleSpeed, roadSpeed)
	}
}
