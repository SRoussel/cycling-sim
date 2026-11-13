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

// W' goes with peak power (anaerobic capacity), not with endurance.
func TestWPrimeFollowsPeakPower(t *testing.T) {
	climber := testRider()
	climber.MaxSprintWatts = 1200
	puncheur := testRider()
	puncheur.MaxSprintWatts = 1700
	wc, wp := NewWPrimeState(climber).WPrimeMax, NewWPrimeState(puncheur).WPrimeMax
	if wp <= wc || wc < 15000 || wp > 28000 {
		t.Errorf("W' climber %.0f J, puncheur %.0f J; want puncheur > climber within 15-28 kJ", wc, wp)
	}
	enduring := *climber
	enduring.Endurance = 95
	if NewWPrimeState(&enduring).WPrimeMax != wc {
		t.Error("endurance should not change W'")
	}
}

func TestRepeatabilityAffectsReconRate(t *testing.T) {
	slowRec := testRider()
	slowRec.Repeatability = 50
	fastRec := testRider()
	fastRec.Repeatability = 90

	wSlow := NewWPrimeState(slowRec)
	wFast := NewWPrimeState(fastRec)

	if wFast.ReconRate <= wSlow.ReconRate {
		t.Errorf("high repeatability should reconstitute faster: %.4f vs %.4f", wFast.ReconRate, wSlow.ReconRate)
	}
	// Recovery is between days: it doesn't change W' reconstitution.
	daily := testRider()
	daily.Recovery = 95
	if NewWPrimeState(daily).ReconRate != NewWPrimeState(testRider()).ReconRate {
		t.Error("recovery (between days) should not change W' reconstitution")
	}
}

func TestPunchRaisesWPrime(t *testing.T) {
	diesel := testRider()
	diesel.Punch = 50
	punchy := testRider()
	punchy.Punch = 90
	if NewWPrimeState(punchy).WPrimeMax <= NewWPrimeState(diesel).WPrimeMax*1.1 {
		t.Error("a punchy rider should carry clearly more W' than a diesel with the same threshold")
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
	if effectiveFTP < int(float64(baseFTP)*0.78) {
		t.Error("FTP should not drop below 78% floor")
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

func TestDurabilityFadesWithWorkAndEndurance(t *testing.T) {
	if f := DurabilityFactor(0, 70); f != 1 {
		t.Errorf("no work should mean no fade, got %.3f", f)
	}
	avg := DurabilityFactor(4000, 70)
	durable := DurabilityFactor(4000, 95)
	fragile := DurabilityFactor(4000, 50)
	if !(fragile < avg && avg < durable && durable < 1) {
		t.Errorf("expected fragile < avg < durable < 1, got %.3f %.3f %.3f", fragile, avg, durable)
	}
	if avg < 0.88 || avg > 0.93 {
		t.Errorf("average rider after 4000 kJ should keep ~90%%, got %.3f", avg)
	}
	if DurabilityFactor(1e6, 30) < 1-durabilityMaxLoss-1e-9 {
		t.Error("fade must be capped")
	}
}
