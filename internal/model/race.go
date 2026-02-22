package model

import (
	"fmt"
	"time"
)

// RaceType represents the type of race.
type RaceType string

const (
	RaceTypeRoad  RaceType = "road"
	RaceTypeTT    RaceType = "tt"
	RaceTypeStage RaceType = "stage"
)

// Race represents a single race event.
type Race struct {
	ID               int64
	Name             string
	CourseID         int64
	RaceType         RaceType
	Date             time.Time
	Temperature      float64 // Celsius
	WindSpeedKmh     float64
	WindDirectionDeg float64 // 0=N, 90=E, 180=S, 270=W
}

// Validate checks race fields.
func (r *Race) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("race name is required")
	}
	if r.CourseID <= 0 {
		return fmt.Errorf("race must reference a valid course")
	}
	if r.RaceType != RaceTypeRoad && r.RaceType != RaceTypeTT && r.RaceType != RaceTypeStage {
		return fmt.Errorf("race type must be 'road', 'tt', or 'stage', got %q", r.RaceType)
	}
	if r.Temperature < -20 || r.Temperature > 50 {
		return fmt.Errorf("temperature must be between -20 and 50°C, got %.1f", r.Temperature)
	}
	if r.WindSpeedKmh < 0 || r.WindSpeedKmh > 120 {
		return fmt.Errorf("wind speed must be between 0 and 120 km/h, got %.1f", r.WindSpeedKmh)
	}
	return nil
}

// StageRace represents a multi-stage race (e.g., Grand Tour).
type StageRace struct {
	ID     int64
	Name   string
	Stages []StageRaceStage
}

// Validate checks stage race fields.
func (sr *StageRace) Validate() error {
	if sr.Name == "" {
		return fmt.Errorf("stage race name is required")
	}
	return nil
}

// StageRaceStage links a race to a stage race with ordering.
type StageRaceStage struct {
	ID            int64
	StageRaceID   int64
	RaceID        int64
	StageNumber   int
	RestDayBefore bool
}

// StageRaceNamePrefixes are race name prefixes that identify stages within stage races.
// They are used to distinguish stage-race stages from standalone one-day races.
var StageRaceNamePrefixes = []string{
	"TdF ", "Giro ", "Vuelta ", "Paris-Nice ", "Tirreno ",
	"Suisse ", "Dauphine ", "Basque ", "Romandie ",
}

// IsStageRaceStageName reports whether name matches a known stage-race stage prefix.
func IsStageRaceStageName(name string) bool {
	for _, p := range StageRaceNamePrefixes {
		if len(name) > len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}
