package model

import "fmt"

// Rider represents a professional cyclist with physical and ability attributes.
type Rider struct {
	ID               int64
	Name             string
	Team             string
	Nationality      string
	WeightKg         float64
	HeightCm         float64
	FTPWatts         int
	MaxSprintWatts   int
	VO2MaxWatts      int
	CdA              float64
	ClimbingAbility  int
	SprintAbility    int
	TTAbility        int
	DescendingAbility int
	Endurance        int
	Recovery         int
	Consistency      int
	TacticalAwareness int
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
	abilities := map[string]int{
		"ClimbingAbility":   r.ClimbingAbility,
		"SprintAbility":     r.SprintAbility,
		"TTAbility":         r.TTAbility,
		"DescendingAbility": r.DescendingAbility,
		"Endurance":         r.Endurance,
		"Recovery":          r.Recovery,
		"Consistency":       r.Consistency,
		"TacticalAwareness": r.TacticalAwareness,
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
