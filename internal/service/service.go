// Package service holds race-preparation and persistence logic shared by the
// CLI (cmd/cycling-sim) and the Wails GUI (package main). Both front-ends
// should load races, pick start lists and save outputs through these helpers
// so the two can never drift apart.
package service

import (
	"fmt"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
	"cycling_sim/internal/simulation"
)

// LoadRaceConfig loads a one-day race, its course and an auto-selected start
// list, returning a RaceConfig ready for simulation. Seed 0 means "random".
func LoadRaceConfig(db *database.DB, raceID int64, incidents bool, overrides simulation.ActionOverrideProvider) (simulation.RaceConfig, error) {
	race, err := db.GetRace(raceID)
	if err != nil {
		return simulation.RaceConfig{}, fmt.Errorf("get race %d: %w", raceID, err)
	}
	course, err := db.GetCourse(race.CourseID)
	if err != nil {
		return simulation.RaceConfig{}, fmt.Errorf("get course %d: %w", race.CourseID, err)
	}
	allRiders, err := db.ListRiders()
	if err != nil {
		return simulation.RaceConfig{}, fmt.Errorf("list riders: %w", err)
	}
	selected := selection.SelectStartList(allRiders, course, race, selection.DefaultConfig(race.RaceType, false, 0))
	return simulation.RaceConfig{
		Race:             race,
		Course:           course,
		Riders:           selected,
		IncidentsEnabled: incidents,
		ActionOverrides:  overrides,
	}, nil
}

// RunRace simulates a one-day race (road or TT) and returns its results.
// onProgress may be nil; it is ignored for time trials.
func RunRace(cfg simulation.RaceConfig, onProgress func(simulation.ProgressSnapshot)) []model.RaceResult {
	if cfg.Race.RaceType == model.RaceTypeTT {
		return simulation.SimulateTT(cfg)
	}
	if onProgress != nil {
		return simulation.SimulateRoadRaceWithProgress(cfg, onProgress)
	}
	return simulation.SimulateRoadRace(cfg)
}

// StageRacePlan is a stage race with its stage configs and selected riders.
type StageRacePlan struct {
	StageRace  *model.StageRace
	Stages     []simulation.StageConfig
	StageInfos []selection.StageInfo
	Riders     []model.Rider
}

// PrepareStageRace loads a stage race, builds its stage configs and selects a
// start list. When reset is true, any previously saved stage results and
// classification standings are cleared first.
func PrepareStageRace(db *database.DB, stageRaceID int64, reset bool) (*StageRacePlan, error) {
	sr, err := db.GetStageRace(stageRaceID)
	if err != nil {
		return nil, fmt.Errorf("get stage race %d: %w", stageRaceID, err)
	}
	if reset {
		if err := ClearStageRaceStandings(db, stageRaceID); err != nil {
			return nil, err
		}
	}
	stages, infos, err := BuildStageConfigs(db, sr, reset)
	if err != nil {
		return nil, err
	}
	allRiders, err := db.ListRiders()
	if err != nil {
		return nil, fmt.Errorf("list riders: %w", err)
	}
	selected := selection.SelectStageRaceStartList(allRiders, infos, selection.DefaultConfig(model.RaceTypeRoad, true, len(stages)))
	if len(selected) == 0 {
		return nil, fmt.Errorf("no riders selected for %s", sr.Name)
	}
	return &StageRacePlan{StageRace: sr, Stages: stages, StageInfos: infos, Riders: selected}, nil
}

// BuildStageConfigs builds StageConfig and StageInfo slices for a stage race.
// If clearResults is true, existing race results for each stage are removed.
func BuildStageConfigs(db *database.DB, sr *model.StageRace, clearResults bool) ([]simulation.StageConfig, []selection.StageInfo, error) {
	stages := make([]simulation.StageConfig, 0, len(sr.Stages))
	infos := make([]selection.StageInfo, 0, len(sr.Stages))
	for _, s := range sr.Stages {
		race, err := db.GetRace(s.RaceID)
		if err != nil {
			return nil, nil, fmt.Errorf("get stage %d race: %w", s.StageNumber, err)
		}
		if clearResults {
			if err := db.ClearRaceResults(race.ID); err != nil {
				return nil, nil, fmt.Errorf("clear stage %d results: %w", s.StageNumber, err)
			}
		}
		course, err := db.GetCourse(race.CourseID)
		if err != nil {
			return nil, nil, fmt.Errorf("get stage %d course: %w", s.StageNumber, err)
		}
		stages = append(stages, simulation.StageConfig{
			Race:          race,
			Course:        course,
			StageNumber:   s.StageNumber,
			RestDayBefore: s.RestDayBefore,
			Prestige:      s.Prestige,
		})
		infos = append(infos, selection.StageInfo{Course: course, RaceType: race.RaceType})
	}
	return stages, infos, nil
}

// ClearStageRaceStandings removes all saved classification standings.
func ClearStageRaceStandings(db *database.DB, stageRaceID int64) error {
	if err := db.ClearGCStandings(stageRaceID); err != nil {
		return fmt.Errorf("clear GC standings: %w", err)
	}
	if err := db.ClearPointsStandings(stageRaceID); err != nil {
		return fmt.Errorf("clear points standings: %w", err)
	}
	if err := db.ClearMountainsStandings(stageRaceID); err != nil {
		return fmt.Errorf("clear mountains standings: %w", err)
	}
	if err := db.ClearYouthStandings(stageRaceID); err != nil {
		return fmt.Errorf("clear youth standings: %w", err)
	}
	return nil
}

// StageRaceOutput bundles everything a stage race simulation produces.
type StageRaceOutput struct {
	Results   [][]model.RaceResult
	GC        [][]model.GCStanding
	Points    [][]model.PointsStanding
	Mountains [][]model.MountainsStanding
	Youth     [][]model.YouthStanding
}

// RunStageRace simulates every stage and computes all classifications.
// seed 0 means random; onProgress and overrides may be nil.
func RunStageRace(plan *StageRacePlan, seed int64, onProgress func(simulation.StageProgressSnapshot), overrides simulation.ActionOverrideProvider) StageRaceOutput {
	var results [][]model.RaceResult
	var gc [][]model.GCStanding
	if onProgress != nil || overrides != nil {
		results, gc = simulation.SimulateStageRaceWithProgress(plan.StageRace, plan.Stages, plan.Riders, seed, onProgress, overrides)
	} else {
		results, gc = simulation.SimulateStageRace(plan.StageRace, plan.Stages, plan.Riders, seed)
	}
	return CompleteStageRaceOutput(plan, results, gc)
}

// CompleteStageRaceOutput computes the secondary classifications for
// already-simulated stage results.
func CompleteStageRaceOutput(plan *StageRacePlan, results [][]model.RaceResult, gc [][]model.GCStanding) StageRaceOutput {
	pts, kom, youth := simulation.ComputeStageRaceCompetitions(plan.StageRace.ID, plan.Stages, plan.Riders, results)
	return StageRaceOutput{Results: results, GC: gc, Points: pts, Mountains: kom, Youth: youth}
}

// SaveStageRaceOutput persists stage results and all classification standings.
// It keeps going after a failure and returns the first error encountered.
func SaveStageRaceOutput(db *database.DB, out StageRaceOutput) error {
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	for _, r := range out.Results {
		keep(db.SaveRaceResults(r))
	}
	for _, s := range out.GC {
		keep(db.SaveGCStandings(s))
	}
	for _, s := range out.Points {
		keep(db.SavePointsStandings(s))
	}
	for _, s := range out.Mountains {
		keep(db.SaveMountainsStandings(s))
	}
	for _, s := range out.Youth {
		keep(db.SaveYouthStandings(s))
	}
	return first
}

// RiderNames maps rider IDs to display names.
func RiderNames(riders []model.Rider) map[int64]string {
	m := make(map[int64]string, len(riders))
	for _, r := range riders {
		m[r.ID] = r.Name
	}
	return m
}

// OneDayRaces returns races that are not stages of any stage race.
func OneDayRaces(db *database.DB) ([]model.Race, error) {
	races, err := db.ListRaces()
	if err != nil {
		return nil, err
	}
	srs, err := db.ListStageRaces()
	if err != nil {
		return nil, err
	}
	inStageRace := make(map[int64]bool)
	for _, sr := range srs {
		full, err := db.GetStageRace(sr.ID)
		if err != nil {
			continue
		}
		for _, st := range full.Stages {
			inStageRace[st.RaceID] = true
		}
	}
	out := make([]model.Race, 0, len(races))
	for _, r := range races {
		if !inStageRace[r.ID] && r.RaceType != model.RaceTypeStage {
			out = append(out, r)
		}
	}
	return out, nil
}
