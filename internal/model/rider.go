package model

import "fmt"

// Rider represents a professional cyclist with physical and ability attributes.
// MaxPeaks: the most form targets a rider can have in a season.
const MaxPeaks = 4

type Rider struct {
	ID                int64
	Name              string
	Team              string
	Nationality       string
	Age               int
	WeightKg          float64
	HeightCm          float64
	FTPWatts          int
	MaxSprintWatts    int
	VO2MaxWatts       int
	CdA               float64
	ClimbingAbility   int
	SprintAbility     int
	TTAbility         int
	DescendingAbility int
	Endurance         int
	Recovery          int
	Consistency       int
	TacticalAwareness int
	Handling          int
	// Punch: power above threshold over short climbs (1-5 min), as maximal
	// aerobic power over threshold; sets W' (Nov 2026). Repeatability: how
	// fast W' refills between efforts within a race (Recovery is recovery
	// between days). 0 means unset (average, 70).
	Punch         int
	Repeatability int
	// PeakDays: the days of the year his seasonal form peaks on, when the
	// player chose his targets (season mode; 0 = unused, all 0: the peaks of
	// his specialty, simulation.SeasonForm). An array, so Rider stays
	// comparable. Not stored with the riders.
	PeakDays [MaxPeaks]float64
}

// PunchRating is Punch, 70 when unset.
func (r *Rider) PunchRating() int {
	if r.Punch == 0 {
		return 70
	}
	return r.Punch
}

// RepeatabilityRating is Repeatability, 70 when unset.
func (r *Rider) RepeatabilityRating() int {
	if r.Repeatability == 0 {
		return 70
	}
	return r.Repeatability
}

// Validate checks that all rider fields are within acceptable ranges.
func (r *Rider) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("rider name is required")
	}
	if r.Team == "" {
		return fmt.Errorf("rider team is required")
	}
	if r.WeightKg < 45 || r.WeightKg > 100 {
		return fmt.Errorf("weight must be between 45 and 100 kg, got %.1f", r.WeightKg)
	}
	if r.HeightCm < 150 || r.HeightCm > 210 {
		return fmt.Errorf("height must be between 150 and 210 cm, got %.1f", r.HeightCm)
	}
	if r.FTPWatts < 200 || r.FTPWatts > 500 {
		return fmt.Errorf("FTP must be between 200 and 500 watts, got %d", r.FTPWatts)
	}
	if r.MaxSprintWatts < 800 || r.MaxSprintWatts > 2500 {
		return fmt.Errorf("max sprint watts must be between 800 and 2500, got %d", r.MaxSprintWatts)
	}
	if r.VO2MaxWatts < 300 || r.VO2MaxWatts > 600 {
		return fmt.Errorf("VO2max watts must be between 300 and 600, got %d", r.VO2MaxWatts)
	}
	if r.CdA < 0.2 || r.CdA > 0.5 {
		return fmt.Errorf("CdA must be between 0.2 and 0.5, got %.3f", r.CdA)
	}

	// Backwards-compat: if Age not set, assume a prime-age pro.
	if r.Age == 0 {
		r.Age = 28
	}
	if r.Age < 16 || r.Age > 50 {
		return fmt.Errorf("age must be between 16 and 50, got %d", r.Age)
	}

	// Backwards-compat: if Handling not set, default to 70 (average)
	if r.Handling == 0 {
		r.Handling = 70
	}
	abilities := map[string]int{
		"ClimbingAbility":   r.ClimbingAbility,
		"SprintAbility":     r.SprintAbility,
		"TTAbility":         r.TTAbility,
		"DescendingAbility": r.DescendingAbility,
		"Endurance":         r.Endurance,
		"Recovery":          r.Recovery,
		"Consistency":       r.Consistency,
		"TacticalAwareness": r.TacticalAwareness,
		"Handling":          r.Handling,
		"Punch":             r.PunchRating(),
		"Repeatability":     r.RepeatabilityRating(),
	}
	for name, val := range abilities {
		if val < 1 || val > 100 {
			return fmt.Errorf("%s must be between 1 and 100, got %d", name, val)
		}
	}
	return nil
}

// TotalMassKg returns rider weight plus assumed bike+gear weight (8 kg).
func (r *Rider) TotalMassKg() float64 {
	return r.WeightKg + 8.0
}

// Peaks returns the rider's chosen peak days (none: his specialty's).
func (r Rider) Peaks() []float64 {
	var out []float64
	for _, d := range r.PeakDays {
		if d > 0 {
			out = append(out, d)
		}
	}
	return out
}
