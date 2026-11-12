package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

// WPrimeState tracks the W' (anaerobic work capacity) balance within a stage.
type WPrimeState struct {
	WPrimeMax     float64 // Maximum W' in joules (15000-25000)
	WPrimeBase    float64 // fresh W' max (WPrimeMax fades with durability)
	WPrimeCurrent float64 // Current W' balance
	ReconRate     float64 // Reconstitution rate constant (from Recovery attribute)
}

// NewWPrimeState creates a W' state for a rider based on their attributes.
// Endurance 70 → W'max = 20000 J
// Endurance 90 → W'max = 24000 J
// Endurance 50 → W'max = 16000 J
// baseReconRate is the ReconRate of an average (Recovery 70) rider; a rider's
// recovery time constant scales with baseReconRate/ReconRate.
const baseReconRate = 0.03

// W' (anaerobic work capacity above threshold) is a physiological trait that
// goes with peak power: riders with a big sprint or punch carry more of it,
// pure climbers less, independent of endurance (which is durability, see
// DurabilityFactor). wPrimeSeconds scales peak power to W': 14 s gives the
// 15-28 kJ range reported for trained and pro cyclists (~21 kJ at a typical
// 1550 W). It decides short climbs, where W' matters as much as threshold.
const (
	wPrimeSeconds = 14.0
	defaultWPrime = 20000.0 // no peak power known
)

func NewWPrimeState(rider *model.Rider) *WPrimeState {
	wMax := defaultWPrime
	if rider.MaxSprintWatts > 0 {
		wMax = wPrimeSeconds * float64(rider.MaxSprintWatts)
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
		// Reconstitution (Skiba et al. 2012): exponential recovery toward
		// WPrimeMax with time constant tau = 546·e^(-0.01·(CP-P)) + 316 s,
		// i.e. ~5-9 minutes; the rider's Recovery attribute scales tau.
		deficit := w.WPrimeMax - w.WPrimeCurrent
		tau := 546*math.Exp(-0.01*(ftp-math.Max(powerWatts, 0))) + 316
		tau *= baseReconRate / w.ReconRate
		w.WPrimeCurrent += deficit * (1.0 - math.Exp(-durationSeconds/tau))
	}
}

// UpdateWithHandling applies W' balance changes with extra cobble fatigue.
// On cobbles, poor handlers waste energy fighting the bike (extra W' drain),
// while skilled handlers ride more efficiently (W' preservation).
// This is the attrition mechanism that makes P-R a race of elimination.
func (w *WPrimeState) UpdateWithHandling(powerWatts float64, ftpWatts int, durationSeconds float64, handlingValue int, isOnCobbles bool) {
	if isOnCobbles {
		// Add extra metabolic cost from bike handling on cobbles
		extraDrain := CobbleWPrimeDrain(handlingValue)
		w.Update(powerWatts+extraDrain, ftpWatts, durationSeconds)
	} else {
		w.Update(powerWatts, ftpWatts, durationSeconds)
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

// Multi-day fatigue: a typical rider accumulates ~10 fatigue units over a
// Grand Tour (after rest days), which should cost ~3-5% of threshold as seen
// in Grand Tour power data. In-race fading is modelled separately by
// DurabilityFactor, so this only carries what's left over night to night.
const (
	stageFatigueFTPPerUnit = 0.004
	stageFatigueFloor      = 0.85
)

// EffectiveFTP returns the FTP adjusted for cumulative fatigue.
// effectiveFTP = baseFTP * (1 - cumulativeFatigue * stageFatigueFTPPerUnit)
func (f *StageFatigueState) EffectiveFTP(baseFTP int) int {
	factor := 1.0 - f.CumulativeFatigue*stageFatigueFTPPerUnit
	if factor < stageFatigueFloor {
		factor = stageFatigueFloor
	}
	return int(math.Round(float64(baseFTP) * factor))
}

func stageDifficulty(course *model.Course, raceType model.RaceType) float64 {
	if course == nil {
		return 1.0
	}

	distanceKm := course.TotalDistanceKm
	if distanceKm <= 0 {
		for _, seg := range course.Segments {
			distanceKm += seg.DistanceKm
		}
	}

	totalClimbM := 0.0
	cobblesKm := 0.0
	for _, seg := range course.Segments {
		if seg.GradientPct > 0 {
			totalClimbM += seg.DistanceKm * 1000.0 * seg.GradientPct / 100.0
		}
		if seg.SurfaceType.Unpaved() {
			cobblesKm += seg.DistanceKm
		}
	}

	// Rough difficulty index in a typical 0.7–2.0 range.
	// ~200km flat road: ~1.5
	// Grand tour mountain stage: ~1.8–2.0
	difficulty := 0.55 + (distanceKm / 220.0) + (totalClimbM / 4500.0) + (cobblesKm / 45.0)
	if raceType == model.RaceTypeTT {
		difficulty += 0.15
	}
	if difficulty < 0.7 {
		difficulty = 0.7
	}
	if difficulty > 2.0 {
		difficulty = 2.0
	}
	return difficulty
}

func stageEffortFactor(timeGapSeconds float64) float64 {
	if timeGapSeconds < 0 {
		timeGapSeconds = 0
	}
	// Near the front = high stress; far back = can ride easier.
	// gap=0s -> 1.20, gap~10min -> ~0.95, very large gaps -> ~0.80
	return 0.8 + 0.4*math.Exp(-timeGapSeconds/600.0)
}

func StageStress(course *model.Course, raceType model.RaceType, timeGapSeconds float64) float64 {
	return stageDifficulty(course, raceType) * stageEffortFactor(timeGapSeconds)
}

// Durability: a rider's sustainable power fades with the work done in a race.
// Pro data shows roughly 1-4% loss per 1000 kJ depending on the rider, which
// is why riders who spend hours in the wind (breakaways, domestiques) fade in
// the finale while sheltered leaders don't.
const (
	durabilityLossPer1000kJ = 0.025 // average rider (Endurance 70)
	durabilityMaxLoss       = 0.15
)

// DurabilityFactor returns the fraction of threshold still available after
// workKJ of work, scaled by the rider's Endurance attribute.
func DurabilityFactor(workKJ float64, endurance int) float64 {
	rate := durabilityLossPer1000kJ * (1 - float64(endurance-70)*0.02)
	if rate < 0.005 {
		rate = 0.005
	}
	loss := rate * workKJ / 1000
	if loss > durabilityMaxLoss {
		loss = durabilityMaxLoss
	}
	return 1 - loss
}

// CurrentFTP is the rider's threshold right now: stage-race fatigue (in
// EffectiveFTP), daily form, in-race durability and fuel combined.
func (rs *RiderState) CurrentFTP() float64 {
	return rs.freshFTPToday() * rs.fuelFactor()
}

// freshFTPToday is the threshold before fuel: what intensity is measured
// against when burning carbohydrate.
func (rs *RiderState) freshFTPToday() float64 {
	return float64(rs.EffectiveFTP) * rs.FormFactor * DurabilityFactor(rs.WorkKJ, rs.Rider.Endurance)
}

// thresholdIntOn is the rider's critical power on a segment, rounded for W'
// bookkeeping. It is the same threshold the rider paces at (terrain ability
// and altitude included), so riding at threshold neither drains nor refills W'.
func (rs *RiderState) thresholdIntOn(seg *model.Segment) int {
	return int(EffectivePower(rs.Rider, rs.CurrentFTP(), seg, false) + 0.5)
}

// addWork records work done (watts over seconds) for durability.
func (rs *RiderState) addWork(powerWatts, seconds float64) {
	if powerWatts > 0 && seconds > 0 {
		rs.WorkKJ += powerWatts * seconds / 1000
	}
	rs.burnFuel(math.Max(0, powerWatts), seconds, rs.freshFTPToday())
	// The anaerobic reserve fades with the day's work like threshold does
	// (W' falls after prolonged exercise as CP does; Clark et al. 2019).
	if rs.WPrime != nil {
		if rs.WPrime.WPrimeBase == 0 {
			rs.WPrime.WPrimeBase = rs.WPrime.WPrimeMax
		}
		rs.WPrime.WPrimeMax = rs.WPrime.WPrimeBase * DurabilityFactor(rs.WorkKJ, rs.Rider.Endurance)
		if rs.WPrime.WPrimeCurrent > rs.WPrime.WPrimeMax {
			rs.WPrime.WPrimeCurrent = rs.WPrime.WPrimeMax
		}
	}
}
