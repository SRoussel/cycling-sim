package simulation

import (
	"fmt"
	"math"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
)

// Race-speed plausibility: the winner's average speed in the simulation
// against the real race. Speeds integrate the whole physics model (power,
// drafting, wind, terrain), so they catch systematic errors that rankings
// can't. The seeded courses approximate the real routes, so the bar is loose.

// speedTolerance is the largest accepted relative speed error.
const speedTolerance = 0.06

// speedRuns is the number of simulations per one-day race.
const speedRuns = 3

// RealSpeed is a real winner's average speed for a seeded race.
type RealSpeed struct {
	DisplayName  string
	InternalName string
	StageRace    bool
	DistanceKm   float64
	WinningTime  float64 // seconds
}

// KMH is the real average speed.
func (r RealSpeed) KMH() float64 { return r.DistanceKm / (r.WinningTime / 3600) }

// KnownRealSpeeds: distances and winning times from the English Wikipedia
// race infoboxes (read Nov 2026).
func KnownRealSpeeds() []RealSpeed {
	hms := func(h, m, s float64) float64 { return h*3600 + m*60 + s }
	return []RealSpeed{
		{"Strade Bianche 2023", "Strade Bianche", false, 184, hms(4, 31, 41)},
		{"Milan-San Remo 2023", "Milan-San Remo", false, 294, hms(6, 25, 23)},
		{"Tour of Flanders 2023", "Tour of Flanders", false, 273.4, hms(6, 12, 7)},
		{"Paris-Roubaix 2023", "Paris-Roubaix", false, 256.6, hms(5, 28, 41)},
		{"Liege-Bastogne-Liege 2023", "Liege-Bastogne-Liege", false, 258.1, hms(6, 15, 49)},
		{"Il Lombardia 2023", "Il Lombardia", false, 238, hms(5, 55, 33)},
		{"Tour de France 2023", "Tour de France 2023", true, 3405.6, hms(82, 5, 42)},
		{"Tour de France 2024", "Tour de France", true, 3498, hms(83, 38, 56)},
	}
}

// RunSpeedValidation compares simulated and real winning speeds.
func RunSpeedValidation(db *database.DB) []ValidationSummary {
	if db == nil {
		return nil
	}
	riders, err := db.ListRiders()
	if err != nil || len(riders) == 0 {
		return nil
	}
	races, err := db.ListRaces()
	if err != nil {
		return nil
	}
	var out []ValidationSummary
	for _, ref := range KnownRealSpeeds() {
		var sim float64
		var err error
		if ref.StageRace {
			sim, err = stageRaceWinnerSpeed(db, riders, ref.InternalName)
		} else {
			sim, err = oneDayWinnerSpeed(db, riders, races, ref.InternalName)
		}
		s := ValidationSummary{Name: ref.DisplayName + " speed", IsRealRace: true}
		if err != nil {
			s.Details = err.Error()
			out = append(out, s)
			continue
		}
		rel := sim/ref.KMH() - 1
		s.Passed = math.Abs(rel) <= speedTolerance
		s.Details = fmt.Sprintf("Winner avg %.1f km/h vs real %.1f km/h (%+.1f%%, bar ±%.0f%%)",
			sim, ref.KMH(), rel*100, speedTolerance*100)
		out = append(out, s)
	}
	return out
}

func oneDayWinnerSpeed(db *database.DB, riders []model.Rider, races []model.Race, name string) (float64, error) {
	race := findRaceByName(races, name)
	if race == nil {
		return 0, fmt.Errorf("race %q not found", name)
	}
	course, err := db.GetCourse(race.CourseID)
	if err != nil {
		return 0, err
	}
	var sum float64
	for run := 0; run < speedRuns; run++ {
		start := selection.SelectStartList(riders, course, race, selection.DefaultConfig(race.RaceType, false, 0))
		res := SimulateRoadRace(RaceConfig{Race: race, Course: course, Riders: start, Seed: int64(run + 1)})
		if len(res) == 0 || res[0].FinishTimeSeconds <= 0 {
			return 0, fmt.Errorf("no finishers")
		}
		sum += course.TotalDistanceKm / (res[0].FinishTimeSeconds / 3600)
	}
	return sum / speedRuns, nil
}

func stageRaceWinnerSpeed(db *database.DB, riders []model.Rider, name string) (float64, error) {
	srs, err := db.ListStageRaces()
	if err != nil {
		return 0, err
	}
	var sr *model.StageRace
	for i := range srs {
		if srs[i].Name == name {
			sr, _ = db.GetStageRace(srs[i].ID)
		}
	}
	if sr == nil {
		return 0, fmt.Errorf("stage race %q not found", name)
	}
	var stages []StageConfig
	var infos []selection.StageInfo
	km := 0.0
	for _, st := range sr.Stages {
		race, err := db.GetRace(st.RaceID)
		if err != nil {
			continue
		}
		course, err := db.GetCourse(race.CourseID)
		if err != nil {
			continue
		}
		km += course.TotalDistanceKm
		stages = append(stages, StageConfig{Race: race, Course: course, StageNumber: st.StageNumber, RestDayBefore: st.RestDayBefore})
		infos = append(infos, selection.StageInfo{Course: course, RaceType: race.RaceType})
	}
	start := selection.SelectStageRaceStartList(riders, infos, selection.DefaultConfig(model.RaceTypeRoad, true, len(stages)))
	_, standings := SimulateStageRace(sr, stages, start, 1)
	if len(standings) == 0 || len(standings[len(standings)-1]) == 0 {
		return 0, fmt.Errorf("no GC")
	}
	return km / (standings[len(standings)-1][0].TotalTimeSeconds / 3600), nil
}
