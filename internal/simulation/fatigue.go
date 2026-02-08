package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

// WPrimeState tracks the W' (anaerobic work capacity) balance within a stage.
type WPrimeState struct {
	WPrimeMax     float64 // Maximum W' in joules (15000-25000)
	WPrimeCurrent float64 // Current W' balance
	ReconRate     float64 // Reconstitution rate constant (from Recovery attribute)
}

// NewWPrimeState creates a W' state for a rider based on their attributes.
// Endurance 70 → W'max = 20000 J
// Endurance 90 → W'max = 24000 J
// Endurance 50 → W'max = 16000 J
func NewWPrimeState(rider *model.Rider) *WPrimeState {
	wMax := 20000.0 + float64(rider.Endurance-70)*200.0
	if wMax < 15000 {
		wMax = 15000
	}
	if wMax > 25000 {
		wMax = 25000
	}
	// Recovery attribute determines reconstitution rate
	// 70 → rate 0.03, higher = faster reconstitution
	reconRate := 0.03 + float64(rider.Recovery-70)*0.0005
	if reconRate < 0.01 {
		reconRate = 0.01
	}
	return &WPrimeState{
		WPrimeMax:     wMax,
		WPrimeCurrent: wMax,
		ReconRate:     reconRate,
	}
}

// Update applies the W' balance change for a given duration at a given power.
// If power > FTP, W' depletes. If power < FTP, W' reconstitutes exponentially.
func (w *WPrimeState) Update(powerWatts float64, ftpWatts int, durationSeconds float64) {
	ftp := float64(ftpWatts)
	if powerWatts > ftp {
		// Depletion: W' decreases at (Power - FTP) joules per second
		depletion := (powerWatts - ftp) * durationSeconds
		w.WPrimeCurrent -= depletion
		if w.WPrimeCurrent < 0 {
			w.WPrimeCurrent = 0
		}
	} else {
		// Reconstitution: exponential recovery toward WPrimeMax
		// dW'/dt = reconRate * (WPrimeMax - WPrimeCurrent) * (1 - Power/FTP)
		deficit := w.WPrimeMax - w.WPrimeCurrent
		intensityFactor := 1.0 - powerWatts/ftp
		recovery := deficit * (1.0 - math.Exp(-w.ReconRate*intensityFactor*durationSeconds))
		w.WPrimeCurrent += recovery
	}
}

// AvailablePowerAboveFTP returns the maximum sustainable above-FTP effort
// given current W' balance and desired duration.
func (w *WPrimeState) AvailablePowerAboveFTP(durationSeconds float64) float64 {
	if durationSeconds <= 0 || w.WPrimeCurrent <= 0 {
		return 0
	}
	return w.WPrimeCurrent / durationSeconds
}

// FractionRemaining returns W'/W'max as a 0-1 value.
func (w *WPrimeState) FractionRemaining() float64 {
	if w.WPrimeMax <= 0 {
		return 0
	}
	return w.WPrimeCurrent / w.WPrimeMax
}

// StageFatigueState tracks cumulative fatigue across a multi-stage race.
type StageFatigueState struct {
	CumulativeFatigue float64 // accumulated fatigue index
	Recovery          int     // rider's Recovery attribute
}

// NewStageFatigueState creates a fresh fatigue tracker for a rider.
func NewStageFatigueState(rider *model.Rider) *StageFatigueState {
	return &StageFatigueState{
		CumulativeFatigue: 0,
		Recovery:          rider.Recovery,
	}
}

// AddStageStress adds fatigue from completing a stage.
// stageStress is a normalized measure of stage difficulty (e.g., 0.5-2.0).
func (f *StageFatigueState) AddStageStress(stageStress float64) {
	f.CumulativeFatigue += stageStress * (1.0 - float64(f.Recovery)/150.0)
}

// ApplyRestDay reduces cumulative fatigue by ~40%.
func (f *StageFatigueState) ApplyRestDay() {
	f.CumulativeFatigue *= 0.6
}

// EffectiveFTP returns the FTP adjusted for cumulative fatigue.
// effectiveFTP = baseFTP * (1 - cumulativeFatigue * 0.001)
func (f *StageFatigueState) EffectiveFTP(baseFTP int) int {
	factor := 1.0 - f.CumulativeFatigue*0.001
	if factor < 0.85 { // floor at 85% of base FTP
		factor = 0.85
	}
	return int(math.Round(float64(baseFTP) * factor))
}
