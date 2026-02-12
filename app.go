package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/seed"
	"cycling_sim/internal/selection"
	"cycling_sim/internal/simulation"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct holds application state and exposes methods to the frontend
type App struct {
	db     *database.DB
	ctx    context.Context
	dbPath string

	sessionsMu sync.Mutex
	sessions   map[string]*interactiveSession
}

// SimParams holds simulation parameters
type SimParams struct {
	HandlingEffect  float64 `json:"handlingEffect"`
	DraftFactor     float64 `json:"draftFactor"`
	FatigueRate     float64 `json:"fatigueRate"`
	AttackThreshold float64 `json:"attackThreshold"`
}

// NewApp creates a new App instance
func NewApp() *App {
	dbPath := "cycling_sim.db"
	if envDB := os.Getenv("CYCLING_SIM_DB"); envDB != "" {
		dbPath = envDB
	}

	db, err := database.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		return &App{}
	}

	return &App{db: db, dbPath: dbPath, sessions: make(map[string]*interactiveSession)}
}

// Startup is called by Wails to provide the runtime context.
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	runtime.WindowMaximise(ctx)
}

// RiderInfo is a simplified rider for the frontend
type RiderInfo struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Team        string  `json:"team"`
	Nationality string  `json:"nationality"`
	Weight      float64 `json:"weight"`
	FTP         int     `json:"ftp"`
	Sprint      int     `json:"sprint"`
	Climbing    int     `json:"climbing"`
	TT          int     `json:"tt"`
	Descending  int     `json:"descending"`
	Endurance   int     `json:"endurance"`
	Handling    int     `json:"handling"`
	Recovery    int     `json:"recovery"`
}

// RaceInfo represents a race for the frontend
type RaceInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	CourseID    int64  `json:"courseId"`
	CourseName  string `json:"courseName"`
	RaceType    string `json:"raceType"`
	Date        string `json:"date"`
	Distance    int    `json:"distance"`
	Temperature int    `json:"temperature"`
	WindSpeed   int    `json:"windSpeed"`
}

// StageRaceInfo represents a stage race for the frontend
type StageRaceInfo struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	StageCount      int     `json:"stageCount"`
	TotalDistanceKm float64 `json:"totalDistanceKm"`
}

// StageRaceStageInfo represents a stage within a stage race for the frontend
type StageRaceStageInfo struct {
	StageNumber   int    `json:"stageNumber"`
	RaceID        int64  `json:"raceId"`
	RaceName      string `json:"raceName"`
	CourseName    string `json:"courseName"`
	DistanceKm    int    `json:"distanceKm"`
	RaceType      string `json:"raceType"`
	Date          string `json:"date"`
	RestDayBefore bool   `json:"restDayBefore"`
}

// CourseInfo is a simplified course for the frontend
type CourseInfo struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Distance int    `json:"distance"`
	Type     string `json:"type"` // cobbles, climb, flat, hilly
}

// SimulationResult holds results from a simulation run
type SimulationResult struct {
	Position  int     `json:"position"`
	RiderID   int64   `json:"riderId"`
	RiderName string  `json:"riderName"`
	Team      string  `json:"team"`
	Time      float64 `json:"time"`
	Gap       float64 `json:"gap"`
	Handling  int     `json:"handling"`
}

// RaceResults holds full race results
type RaceResults struct {
	RaceID   int64              `json:"raceId"`
	RaceName string             `json:"raceName"`
	Results  []SimulationResult `json:"results"`
}

// ProgressGroup represents a live race group for the frontend.
type ProgressGroup struct {
	Name   string   `json:"name"`
	Gap    float64  `json:"gap"`
	Riders []string `json:"riders"`
}

// RiderTelemetryPoint represents realtime telemetry for a rider.
type RiderTelemetryPoint struct {
	SpeedKph   float64 `json:"speedKph"`
	PowerWatts float64 `json:"powerWatts"`
	EffortPct  float64 `json:"effortPct"`
	FatiguePct float64 `json:"fatiguePct"`
	WPrimePct  float64 `json:"wPrimePct"`
	RouteKm    float64 `json:"routeKm"`
	RoutePct   float64 `json:"routePct"`
}

// ProfilePoint represents one sampled point on the route profile.
type ProfilePoint struct {
	Km   float64 `json:"km"`
	Elev float64 `json:"elev"`
}

// RouteProfile represents a route elevation profile for live rendering.
type RouteProfile struct {
	TotalKm float64        `json:"totalKm"`
	Points  []ProfilePoint `json:"points"`
}

// TeamState represents current controllable team tactical mode.
type TeamState struct {
	SprintTrainOrder []int64          `json:"sprintTrainOrder"`
	FollowMap        map[int64]*int64 `json:"followMap"`
	SprintTrainPhase string           `json:"sprintTrainPhase"`
	LaunchingRiderID *int64           `json:"launchingRiderId,omitempty"`
}

func mapTelemetry(in map[int64]simulation.RiderTelemetryPoint) map[int64]RiderTelemetryPoint {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int64]RiderTelemetryPoint, len(in))
	for riderID, point := range in {
		out[riderID] = RiderTelemetryPoint{
			SpeedKph:   point.SpeedKph,
			PowerWatts: point.PowerWatts,
			EffortPct:  point.EffortPct,
			FatiguePct: point.FatiguePct,
			WPrimePct:  point.WPrimePct,
			RouteKm:    point.RouteKm,
			RoutePct:   point.RoutePct,
		}
	}
	return out
}

func mapRouteProfile(in *simulation.RouteProfile) *RouteProfile {
	if in == nil {
		return nil
	}
	points := make([]ProfilePoint, len(in.Points))
	for i, p := range in.Points {
		points[i] = ProfilePoint{Km: p.Km, Elev: p.Elev}
	}
	return &RouteProfile{TotalKm: in.TotalKm, Points: points}
}

func mapTeamState(in *simulation.TeamState) *TeamState {
	if in == nil {
		return nil
	}
	order := make([]int64, len(in.SprintTrainOrder))
	copy(order, in.SprintTrainOrder)
	follow := make(map[int64]*int64, len(in.FollowMap))
	for riderID, followID := range in.FollowMap {
		if followID == nil {
			follow[riderID] = nil
			continue
		}
		id := *followID
		follow[riderID] = &id
	}
	var launching *int64
	if in.LaunchingRiderID != nil {
		id := *in.LaunchingRiderID
		launching = &id
	}
	return &TeamState{
		SprintTrainOrder: order,
		FollowMap:        follow,
		SprintTrainPhase: in.SprintTrainPhase,
		LaunchingRiderID: launching,
	}
}

// SimulationProgress is emitted during a race simulation.
type SimulationProgress struct {
	SessionID      string                        `json:"sessionId,omitempty"`
	RaceID         int64                         `json:"raceId"`
	KmCovered      float64                       `json:"kmCovered"`
	KmTotal        float64                       `json:"kmTotal"`
	ElapsedTime    float64                       `json:"elapsedTime"`
	ProgressPct    float64                       `json:"progress"`
	SegmentName    string                        `json:"segmentName"`
	Groups         []ProgressGroup               `json:"groups"`
	RiderGroups    map[int64]string              `json:"riderGroups,omitempty"`
	RiderTelemetry map[int64]RiderTelemetryPoint `json:"riderTelemetry,omitempty"`
	Profile        *RouteProfile                 `json:"profile,omitempty"`
	TeamState      *TeamState                    `json:"teamState,omitempty"`
	Events         []string                      `json:"events"`
}

// StageRaceProgress is emitted during a stage race simulation.
type StageRaceProgress struct {
	SessionID       string                        `json:"sessionId,omitempty"`
	StageRaceID     int64                         `json:"stageRaceId"`
	StageNumber     int                           `json:"stageNumber"`
	StageCount      int                           `json:"stageCount"`
	StageName       string                        `json:"stageName"`
	StageKm         float64                       `json:"stageKm"`
	StageKmTotal    float64                       `json:"stageKmTotal"`
	StageElapsed    float64                       `json:"stageElapsed"`
	OverallKm       float64                       `json:"overallKm"`
	OverallKmTotal  float64                       `json:"overallKmTotal"`
	OverallElapsed  float64                       `json:"overallElapsed"`
	ProgressStage   float64                       `json:"progressStage"`
	ProgressOverall float64                       `json:"progressOverall"`
	Groups          []ProgressGroup               `json:"groups"`
	RiderGroups     map[int64]string              `json:"riderGroups,omitempty"`
	RiderTelemetry  map[int64]RiderTelemetryPoint `json:"riderTelemetry,omitempty"`
	Profile         *RouteProfile                 `json:"profile,omitempty"`
	TeamState       *TeamState                    `json:"teamState,omitempty"`
	Events          []string                      `json:"events"`
}

// GCStanding holds GC standings
type GCStanding struct {
	Position  int     `json:"position"`
	RiderName string  `json:"riderName"`
	Team      string  `json:"team"`
	TotalTime float64 `json:"totalTime"`
	Gap       float64 `json:"gap"`
}

type PointsStanding struct {
	Position  int    `json:"position"`
	RiderName string `json:"riderName"`
	Team      string `json:"team"`
	Points    int    `json:"points"`
	Gap       int    `json:"gap"`
}

type MountainsStanding struct {
	Position  int    `json:"position"`
	RiderName string `json:"riderName"`
	Team      string `json:"team"`
	Points    int    `json:"points"`
	Gap       int    `json:"gap"`
}

type YouthStanding struct {
	Position  int     `json:"position"`
	RiderName string  `json:"riderName"`
	Team      string  `json:"team"`
	TotalTime float64 `json:"totalTime"`
	Gap       float64 `json:"gap"`
}

// ValidationResult holds test validation results
type ValidationResult struct {
	TestName    string  `json:"testName"`
	Passed      bool    `json:"passed"`
	Correlation float64 `json:"correlation"`
	Top10Acc    float64 `json:"top10Accuracy"`
	Details     string  `json:"details"`
}

// GetRiders returns all riders from the database
func (a *App) GetRiders() []RiderInfo {
	if a.db == nil {
		return []RiderInfo{}
	}
	riders, err := a.db.ListRiders()
	if err != nil {
		return []RiderInfo{}
	}
	result := make([]RiderInfo, len(riders))
	for i, r := range riders {
		result[i] = RiderInfo{
			ID:          r.ID,
			Name:        r.Name,
			Team:        r.Team,
			Nationality: r.Nationality,
			Weight:      r.WeightKg,
			FTP:         r.FTPWatts,
			Sprint:      r.SprintAbility,
			Climbing:    r.ClimbingAbility,
			TT:          r.TTAbility,
			Descending:  r.DescendingAbility,
			Endurance:   r.Endurance,
			Handling:    r.Handling,
			Recovery:    r.Recovery,
		}
	}
	return result
}

// GetCourses returns available race courses
func (a *App) GetCourses() []CourseInfo {
	if a.db == nil {
		return []CourseInfo{}
	}
	courses, err := a.db.ListCourses()
	if err != nil {
		return []CourseInfo{}
	}
	result := make([]CourseInfo, len(courses))
	for i, c := range courses {
		courseType := classifyCourse(c)
		result[i] = CourseInfo{
			ID:       c.ID,
			Name:     c.Name,
			Distance: int(c.TotalDistanceKm),
			Type:     courseType,
		}
	}
	return result
}

// GetRaces returns all one-day races
func (a *App) GetRaces() []RaceInfo {
	if a.db == nil {
		return []RaceInfo{}
	}
	races, err := a.db.ListRaces()
	if err != nil {
		return []RaceInfo{}
	}

	// Filter out stage race stages (names starting with TdF, Giro, Vuelta, etc.)
	var oneDayRaces []RaceInfo
	for _, r := range races {
		if isStageRaceStage(r.Name) {
			continue
		}
		course, _ := a.db.GetCourse(r.CourseID)
		courseName := ""
		distance := 0
		if course != nil {
			courseName = course.Name
			distance = int(course.TotalDistanceKm)
		}
		oneDayRaces = append(oneDayRaces, RaceInfo{
			ID:          r.ID,
			Name:        r.Name,
			CourseID:    r.CourseID,
			CourseName:  courseName,
			RaceType:    string(r.RaceType),
			Date:        r.Date.Format("2006-01-02"),
			Distance:    distance,
			Temperature: int(r.Temperature),
			WindSpeed:   int(r.WindSpeedKmh),
		})
	}
	return oneDayRaces
}

// GetStageRaces returns all stage races
func (a *App) GetStageRaces() []StageRaceInfo {
	if a.db == nil {
		return []StageRaceInfo{}
	}
	stageRaces, err := a.db.ListStageRaces()
	if err != nil {
		return []StageRaceInfo{}
	}
	result := make([]StageRaceInfo, len(stageRaces))
	for i, sr := range stageRaces {
		// Estimate total distance: ~160km average per stage for grand tours
		stageCount := len(sr.Stages)
		avgStageKm := 160.0
		if stageCount <= 8 {
			avgStageKm = 175.0 // Shorter races tend to have longer stages
		}
		result[i] = StageRaceInfo{
			ID:              sr.ID,
			Name:            sr.Name,
			StageCount:      stageCount,
			TotalDistanceKm: float64(stageCount) * avgStageKm,
		}
	}
	return result
}

// GetStageRaceStages returns detailed stage info for a stage race
func (a *App) GetStageRaceStages(stageRaceID int64) []StageRaceStageInfo {
	if a.db == nil {
		return nil
	}
	sr, err := a.db.GetStageRace(stageRaceID)
	if err != nil {
		return nil
	}

	result := make([]StageRaceStageInfo, 0, len(sr.Stages))
	for _, s := range sr.Stages {
		race, err := a.db.GetRace(s.RaceID)
		if err != nil {
			return nil
		}
		courseName := ""
		distanceKm := 0
		if course, err := a.db.GetCourse(race.CourseID); err == nil && course != nil {
			courseName = course.Name
			distanceKm = int(course.TotalDistanceKm)
		}
		date := ""
		if !race.Date.IsZero() {
			date = race.Date.Format("2006-01-02")
		}
		result = append(result, StageRaceStageInfo{
			StageNumber:   s.StageNumber,
			RaceID:        race.ID,
			RaceName:      race.Name,
			CourseName:    courseName,
			DistanceKm:    distanceKm,
			RaceType:      string(race.RaceType),
			Date:          date,
			RestDayBefore: s.RestDayBefore,
		})
	}
	return result
}

// RunSimulation runs a race simulation
func (a *App) RunSimulation(raceID int64, enableIncidents bool) RaceResults {
	if a.db == nil {
		return RaceResults{}
	}

	race, err := a.db.GetRace(raceID)
	if err != nil {
		return raceResultsError(err)
	}

	course, err := a.db.GetCourse(race.CourseID)
	if err != nil {
		return raceResultsError(err)
	}

	riders, err := a.db.ListRiders()
	if err != nil {
		return raceResultsError(err)
	}

	// Select riders for the race
	config := selection.DefaultConfig(race.RaceType, false, 0)
	selected := selection.SelectStartList(riders, course, race, config)

	// Run simulation
	raceConfig := simulation.RaceConfig{
		Race:             race,
		Course:           course,
		Riders:           selected,
		Seed:             0,
		IncidentsEnabled: enableIncidents,
	}
	var simResults []model.RaceResult
	if race.RaceType == model.RaceTypeTT {
		simResults = simulation.SimulateTT(raceConfig)
	} else {
		progressCallback := func(snapshot simulation.ProgressSnapshot) {
			if a.ctx == nil {
				return
			}
			progressPct := 0.0
			if snapshot.KmTotal > 0 {
				progressPct = snapshot.KmCovered / snapshot.KmTotal * 100
			}
			groups := make([]ProgressGroup, len(snapshot.Groups))
			for i, g := range snapshot.Groups {
				groups[i] = ProgressGroup{
					Name:   g.Name,
					Gap:    g.GapSeconds,
					Riders: g.Riders,
				}
			}
			runtime.EventsEmit(a.ctx, "simulation_progress", SimulationProgress{
				RaceID:         raceID,
				KmCovered:      snapshot.KmCovered,
				KmTotal:        snapshot.KmTotal,
				ElapsedTime:    snapshot.ElapsedTime,
				ProgressPct:    progressPct,
				SegmentName:    snapshot.SegmentName,
				Groups:         groups,
				RiderGroups:    snapshot.RiderGroups,
				RiderTelemetry: mapTelemetry(snapshot.RiderTelemetry),
				Profile:        mapRouteProfile(snapshot.RouteProfile),
				TeamState:      mapTeamState(snapshot.TeamState),
				Events:         snapshot.Events,
			})
		}
		simResults = simulation.SimulateRoadRaceWithProgress(raceConfig, progressCallback)
	}
	riderMap := buildRiderMap(selected)
	results := mapSimulationResults(simResults, riderMap)

	// Store results
	a.db.SaveRaceResults(simResults)

	return RaceResults{
		RaceID:   raceID,
		RaceName: race.Name,
		Results:  results,
	}
}

// RunStageRace runs a complete stage race
func (a *App) RunStageRace(stageRaceID int64) []RaceResults {
	if a.db == nil {
		return nil
	}

	sr, err := a.db.GetStageRace(stageRaceID)
	if err != nil {
		return nil
	}
	if err := a.db.ClearGCStandings(stageRaceID); err != nil {
		return nil
	}
	_ = a.db.ClearPointsStandings(stageRaceID)
	_ = a.db.ClearMountainsStandings(stageRaceID)
	_ = a.db.ClearYouthStandings(stageRaceID)

	allRiders, err := a.db.ListRiders()
	if err != nil {
		return nil
	}

	var stages []simulation.StageConfig
	var stageInfos []selection.StageInfo
	for _, s := range sr.Stages {
		race, err := a.db.GetRace(s.RaceID)
		if err != nil {
			return nil
		}
		if err := a.db.ClearRaceResults(race.ID); err != nil {
			return nil
		}
		course, err := a.db.GetCourse(race.CourseID)
		if err != nil {
			return nil
		}
		stages = append(stages, simulation.StageConfig{
			Race:          race,
			Course:        course,
			StageNumber:   s.StageNumber,
			RestDayBefore: s.RestDayBefore,
		})
		stageInfos = append(stageInfos, selection.StageInfo{Course: course, RaceType: race.RaceType})
	}

	selConfig := selection.DefaultConfig(model.RaceTypeRoad, true, len(stages))
	selected := selection.SelectStageRaceStartList(allRiders, stageInfos, selConfig)
	if len(selected) == 0 {
		return nil
	}

	stageCount := len(stages)
	progressCallback := func(snapshot simulation.StageProgressSnapshot) {
		if a.ctx == nil {
			return
		}
		progressStage := 0.0
		if snapshot.StageKmTotal > 0 {
			progressStage = snapshot.StageKmCovered / snapshot.StageKmTotal * 100
		}
		progressOverall := 0.0
		if snapshot.OverallKmTotal > 0 {
			progressOverall = snapshot.OverallKmCovered / snapshot.OverallKmTotal * 100
		}
		groups := make([]ProgressGroup, len(snapshot.Groups))
		for i, g := range snapshot.Groups {
			groups[i] = ProgressGroup{
				Name:   g.Name,
				Gap:    g.GapSeconds,
				Riders: g.Riders,
			}
		}
		runtime.EventsEmit(a.ctx, "stage_race_progress", StageRaceProgress{
			StageRaceID:     stageRaceID,
			StageNumber:     snapshot.StageNumber,
			StageCount:      stageCount,
			StageName:       snapshot.StageName,
			StageKm:         snapshot.StageKmCovered,
			StageKmTotal:    snapshot.StageKmTotal,
			StageElapsed:    snapshot.StageElapsedTime,
			OverallKm:       snapshot.OverallKmCovered,
			OverallKmTotal:  snapshot.OverallKmTotal,
			OverallElapsed:  snapshot.OverallElapsed,
			ProgressStage:   progressStage,
			ProgressOverall: progressOverall,
			Groups:          groups,
			RiderGroups:     snapshot.RiderGroups,
			RiderTelemetry:  mapTelemetry(snapshot.RiderTelemetry),
			Profile:         mapRouteProfile(snapshot.RouteProfile),
			TeamState:       mapTeamState(snapshot.TeamState),
			Events:          snapshot.Events,
		})
	}

	allResults, allStandings := simulation.SimulateStageRaceWithProgress(sr, stages, selected, 0, progressCallback, nil)
	pointsByStage, komByStage, youthByStage := simulation.ComputeStageRaceCompetitions(sr.ID, stages, selected, allResults)

	riderMap := buildRiderMap(selected)
	finalResults := make([]RaceResults, 0, len(allResults))
	for i, results := range allResults {
		if i < len(stages) {
			_ = a.db.SaveRaceResults(results)
			finalResults = append(finalResults, RaceResults{
				RaceID:   stages[i].Race.ID,
				RaceName: stages[i].Race.Name,
				Results:  mapSimulationResults(results, riderMap),
			})
		}
	}

	for _, standings := range allStandings {
		_ = a.db.SaveGCStandings(standings)
	}
	for _, ps := range pointsByStage {
		_ = a.db.SavePointsStandings(ps)
	}
	for _, ks := range komByStage {
		_ = a.db.SaveMountainsStandings(ks)
	}
	for _, ys := range youthByStage {
		_ = a.db.SaveYouthStandings(ys)
	}

	return finalResults
}

// GetRaceResults returns stored results for a race
func (a *App) GetRaceResults(raceID int64) RaceResults {
	if a.db == nil {
		return RaceResults{}
	}

	race, _ := a.db.GetRace(raceID)
	results, err := a.db.GetRaceResults(raceID)
	if err != nil {
		return RaceResults{}
	}

	riders, _ := a.db.ListRiders()
	riderMap := buildRiderMap(riders)
	simResults := mapSimulationResults(results, riderMap)

	raceName := ""
	if race != nil {
		raceName = race.Name
	}
	return RaceResults{
		RaceID:   raceID,
		RaceName: raceName,
		Results:  simResults,
	}
}

// GetGCStandings returns GC standings after a stage
func (a *App) GetGCStandings(stageRaceID int64, afterStage int) []GCStanding {
	if a.db == nil {
		return nil
	}

	standings, err := a.db.GetGCStandings(stageRaceID, afterStage)
	if err != nil {
		return nil
	}

	riders, _ := a.db.ListRiders()
	riderMap := buildRiderMap(riders)

	result := make([]GCStanding, len(standings))
	var leaderTime float64
	if len(standings) > 0 {
		leaderTime = standings[0].TotalTimeSeconds
	}
	for i, s := range standings {
		rider := riderMap[s.RiderID]
		result[i] = GCStanding{
			Position:  i + 1,
			RiderName: rider.Name,
			Team:      rider.Team,
			TotalTime: s.TotalTimeSeconds,
			Gap:       s.TotalTimeSeconds - leaderTime,
		}
	}
	return result
}

func (a *App) GetPointsStandings(stageRaceID int64, afterStage int) []PointsStanding {
	if a.db == nil {
		return nil
	}
	standings, err := a.db.GetPointsStandings(stageRaceID, afterStage)
	if err != nil {
		return nil
	}

	riders, _ := a.db.ListRiders()
	riderMap := buildRiderMap(riders)

	result := make([]PointsStanding, len(standings))
	leaderPoints := 0
	if len(standings) > 0 {
		leaderPoints = standings[0].Points
	}
	for i, s := range standings {
		r := riderMap[s.RiderID]
		result[i] = PointsStanding{
			Position:  i + 1,
			RiderName: r.Name,
			Team:      r.Team,
			Points:    s.Points,
			Gap:       leaderPoints - s.Points,
		}
	}
	return result
}

func (a *App) GetMountainsStandings(stageRaceID int64, afterStage int) []MountainsStanding {
	if a.db == nil {
		return nil
	}
	standings, err := a.db.GetMountainsStandings(stageRaceID, afterStage)
	if err != nil {
		return nil
	}

	riders, _ := a.db.ListRiders()
	riderMap := buildRiderMap(riders)

	result := make([]MountainsStanding, len(standings))
	leaderPoints := 0
	if len(standings) > 0 {
		leaderPoints = standings[0].Points
	}
	for i, s := range standings {
		r := riderMap[s.RiderID]
		result[i] = MountainsStanding{
			Position:  i + 1,
			RiderName: r.Name,
			Team:      r.Team,
			Points:    s.Points,
			Gap:       leaderPoints - s.Points,
		}
	}
	return result
}

func (a *App) GetYouthStandings(stageRaceID int64, afterStage int) []YouthStanding {
	if a.db == nil {
		return nil
	}
	standings, err := a.db.GetYouthStandings(stageRaceID, afterStage)
	if err != nil {
		return nil
	}

	riders, _ := a.db.ListRiders()
	riderMap := buildRiderMap(riders)

	result := make([]YouthStanding, len(standings))
	leaderTime := 0.0
	if len(standings) > 0 {
		leaderTime = standings[0].TotalTimeSeconds
	}
	for i, s := range standings {
		r := riderMap[s.RiderID]
		result[i] = YouthStanding{
			Position:  i + 1,
			RiderName: r.Name,
			Team:      r.Team,
			TotalTime: s.TotalTimeSeconds,
			Gap:       s.TotalTimeSeconds - leaderTime,
		}
	}
	return result
}

// SeedDatabase seeds the database with initial data
func (a *App) SeedDatabase() string {
	if a.db == nil {
		return "Error: database not connected"
	}

	res, err := seed.Seed(a.db)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	if res.Riders == 0 && res.Courses == 0 && res.Races == 0 && res.StageRaces == 0 {
		return "Database already contains data"
	}
	if res.Warnings > 0 {
		return fmt.Sprintf("Seed complete with warnings: %d riders, %d courses, %d races, %d stage races (%d warnings)", res.Riders, res.Courses, res.Races, res.StageRaces, res.Warnings)
	}
	return fmt.Sprintf("Seed complete: %d riders, %d courses, %d races, %d stage races", res.Riders, res.Courses, res.Races, res.StageRaces)
}

// ClearDatabase clears and recreates the database
func (a *App) ClearDatabase() string {
	if a.dbPath == "" {
		a.dbPath = "cycling_sim.db"
	}

	a.sessionsMu.Lock()
	for _, s := range a.sessions {
		s.Stop()
	}
	a.sessions = make(map[string]*interactiveSession)
	a.sessionsMu.Unlock()

	if a.db != nil {
		_ = a.db.Close()
		a.db = nil
	}

	if err := seed.ClearDBFile(a.dbPath); err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	db, err := database.Open(a.dbPath)
	if err != nil {
		return fmt.Sprintf("Error reopening database: %v", err)
	}
	a.db = db
	return "Database cleared"
}

// IsDatabaseSeeded checks if the database has data
func (a *App) IsDatabaseSeeded() bool {
	if a.db == nil {
		return false
	}
	riders, _ := a.db.ListRiders()
	return len(riders) > 0
}

// RunValidation runs validation tests
func (a *App) RunValidation() []ValidationResult {
	results := simulation.RunValidationSuite()
	output := make([]ValidationResult, len(results))
	for i, r := range results {
		output[i] = ValidationResult{
			TestName:    r.Name,
			Passed:      r.Passed,
			Correlation: r.Correlation,
			Top10Acc:    r.Top10Accuracy,
			Details:     r.Details,
		}
	}
	return output
}

// GetSimulationParams returns current simulation parameters
func (a *App) GetSimulationParams() SimParams {
	return SimParams{
		HandlingEffect:  0.018,
		DraftFactor:     0.25,
		FatigueRate:     1.0,
		AttackThreshold: 0.7,
	}
}

// SaveSimulationParams saves simulation parameters
func (a *App) SaveSimulationParams(params SimParams) error {
	return nil
}

// Helper functions

func classifyCourse(c model.Course) string {
	hasCobbles := false
	totalClimbing := 0.0
	for _, seg := range c.Segments {
		if seg.SurfaceType == model.SurfaceCobbles {
			hasCobbles = true
		}
		if seg.GradientPct > 3 {
			totalClimbing += seg.DistanceKm * seg.GradientPct / 100
		}
	}

	if hasCobbles {
		return "cobbles"
	}
	if totalClimbing > 3000 {
		return "climb"
	}
	if totalClimbing > 1000 {
		return "hilly"
	}
	return "flat"
}

func isStageRaceStage(name string) bool {
	prefixes := []string{"TdF ", "Giro ", "Vuelta ", "Paris-Nice ", "Tirreno ", "Suisse ", "Dauphine ", "Basque ", "Romandie "}
	for _, p := range prefixes {
		if len(name) > len(p) && name[:len(p)] == p {
			return true
		}
	}
	return false
}

func raceResultsError(err error) RaceResults {
	return RaceResults{RaceName: fmt.Sprintf("Error: %v", err)}
}

func buildRiderMap(riders []model.Rider) map[int64]model.Rider {
	riderMap := make(map[int64]model.Rider, len(riders))
	for _, r := range riders {
		riderMap[r.ID] = r
	}
	return riderMap
}

func mapSimulationResults(results []model.RaceResult, riderMap map[int64]model.Rider) []SimulationResult {
	simResults := make([]SimulationResult, len(results))
	for i, r := range results {
		rider := riderMap[r.RiderID]
		simResults[i] = SimulationResult{
			Position:  r.Position,
			RiderID:   r.RiderID,
			RiderName: rider.Name,
			Team:      rider.Team,
			Time:      r.FinishTimeSeconds,
			Gap:       r.TimeGapSeconds,
			Handling:  rider.Handling,
		}
	}
	return simResults
}
