package simulation

import (
	"math"
	"testing"

	"cycling_sim/internal/model"
)

func testRider() *model.Rider {
	return &model.Rider{
		Name: "Test Rider", Team: "Test Team", Nationality: "TST",
		WeightKg: 70, HeightCm: 180, FTPWatts: 400, MaxSprintWatts: 1400,
		VO2MaxWatts: 460, CdA: 0.32, ClimbingAbility: 70, SprintAbility: 70,
		TTAbility: 70, DescendingAbility: 70, Endurance: 70, Recovery: 70,
		Consistency: 70, TacticalAwareness: 70,
	}
}

func TestWPrimeDepletion(t *testing.T) {
	rider := testRider()
	w := NewWPrimeState(rider)
	initialW := w.WPrimeCurrent

	// Ride at 500W for 60 seconds (100W above FTP of 400)
	w.Update(500, 400, 60)
	expectedDepletion := 100.0 * 60.0 // 6000 J
	actualDepletion := initialW - w.WPrimeCurrent

	if math.Abs(actualDepletion-expectedDepletion) > 1.0 {
		t.Errorf("W' depletion = %.0f J, expected %.0f J", actualDepletion, expectedDepletion)
	}
}

func TestWPrimeReconstitution(t *testing.T) {
	rider := testRider()
	w := NewWPrimeState(rider)

	// Deplete some W'
	w.Update(500, 400, 120) // 12000 J depleted
	depleted := w.WPrimeCurrent

	// Recover at 200W for 300 seconds
	w.Update(200, 400, 300)
	if w.WPrimeCurrent <= depleted {
		t.Error("W' should recover below FTP")
	}
	if w.WPrimeCurrent > w.WPrimeMax {
		t.Error("W' should not exceed max")
	}
}

func TestWPrimeFloorAtZero(t *testing.T) {
	rider := testRider()
	w := NewWPrimeState(rider)

	// Massive depletion
	w.Update(800, 400, 100) // 40000 J depletion attempt
	if w.WPrimeCurrent < 0 {
		t.Errorf("W' went below 0: %.0f", w.WPrimeCurrent)
	}
}

func TestEnduranceAffectsWPrimeMax(t *testing.T) {
	lowEndurance := testRider()
	lowEndurance.Endurance = 50
	highEndurance := testRider()
	highEndurance.Endurance = 90

	wLow := NewWPrimeState(lowEndurance)
	wHigh := NewWPrimeState(highEndurance)

	if wHigh.WPrimeMax <= wLow.WPrimeMax {
		t.Errorf("high endurance should have more W': %.0f vs %.0f", wHigh.WPrimeMax, wLow.WPrimeMax)
	}
}

func TestRecoveryAffectsReconRate(t *testing.T) {
	slowRec := testRider()
	slowRec.Recovery = 50
	fastRec := testRider()
	fastRec.Recovery = 90

	wSlow := NewWPrimeState(slowRec)
	wFast := NewWPrimeState(fastRec)

	if wFast.ReconRate <= wSlow.ReconRate {
		t.Errorf("high recovery should reconstitute faster: %.4f vs %.4f", wFast.ReconRate, wSlow.ReconRate)
	}
}

func TestStageFatigue(t *testing.T) {
	rider := testRider()
	f := NewStageFatigueState(rider)

	baseFTP := rider.FTPWatts
	if f.EffectiveFTP(baseFTP) != baseFTP {
		t.Error("no fatigue should mean full FTP")
	}

	// Add several stages of stress
	for i := 0; i < 10; i++ {
		f.AddStageStress(1.0)
	}
	effectiveFTP := f.EffectiveFTP(baseFTP)
	if effectiveFTP >= baseFTP {
		t.Error("accumulated fatigue should reduce FTP")
	}
	if effectiveFTP < int(float64(baseFTP)*0.85) {
		t.Error("FTP should not drop below 85% floor")
	}
}

func TestRestDayRecovery(t *testing.T) {
	rider := testRider()
	f := NewStageFatigueState(rider)

	for i := 0; i < 5; i++ {
		f.AddStageStress(1.5)
	}
	fatigueBeforeRest := f.CumulativeFatigue
	f.ApplyRestDay()
	if f.CumulativeFatigue >= fatigueBeforeRest {
		t.Error("rest day should reduce fatigue")
	}
	if math.Abs(f.CumulativeFatigue-fatigueBeforeRest*0.6) > 0.01 {
		t.Errorf("rest day should reduce fatigue by ~40%%: %.2f vs %.2f", f.CumulativeFatigue, fatigueBeforeRest*0.6)
	}
}

func TestHighRecoveryLessFatigue(t *testing.T) {
	lowRec := testRider()
	lowRec.Recovery = 40
	highRec := testRider()
	highRec.Recovery = 90

	fLow := NewStageFatigueState(lowRec)
	fHigh := NewStageFatigueState(highRec)

	for i := 0; i < 5; i++ {
		fLow.AddStageStress(1.0)
		fHigh.AddStageStress(1.0)
	}
	if fHigh.CumulativeFatigue >= fLow.CumulativeFatigue {
		t.Errorf("high recovery should accumulate less fatigue: %.2f vs %.2f",
			fHigh.CumulativeFatigue, fLow.CumulativeFatigue)
	}
}
