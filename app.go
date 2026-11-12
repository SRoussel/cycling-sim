package main

import (
	"context"
	"cycling_sim/internal/director"
	"fmt"
	"os"
	"sort"
	"sync"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/seed"
	"cycling_sim/internal/service"
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
	Size   int      `json:"size"`
}

// SimulationProgress is emitted during a race simulation.
type SimulationProgress struct {
	SessionID      string                                   `json:"sessionId,omitempty"`
	RaceID         int64                                    `json:"raceId"`
	KmCovered      float64                                  `json:"kmCovered"`
	KmTotal        float64                                  `json:"kmTotal"`
	ElapsedTime    float64                                  `json:"elapsedTime"`
	ProgressPct    float64                                  `json:"progress"`
	SegmentName    string                                   `json:"segmentName"`
	Groups         []ProgressGroup                          `json:"groups"`
	RiderGroups    map[int64]string                         `json:"riderGroups,omitempty"`
	RiderTelemetry map[int64]simulation.RiderTelemetryPoint `json:"riderTelemetry,omitempty"`
	Profile        *simulation.RouteProfile                 `json:"profile,omitempty"`
	TeamState      *simulation.TeamState                    `json:"teamState,omitempty"`
	Events         []string                                 `json:"events"`
	Moment         *director.Moment                         `json:"moment,omitempty"`
	Goal           *director.Goal                           `json:"goal,omitempty"`
}

// StageRaceProgress is emitted during a stage race simulation.
type StageRaceProgress struct {
	SessionID       string                                   `json:"sessionId,omitempty"`
	StageRaceID     int64                                    `json:"stageRaceId"`
	StageNumber     int                                      `json:"stageNumber"`
	StageCount      int                                      `json:"stageCount"`
	StageName       string                                   `json:"stageName"`
	StageKm         float64                                  `json:"stageKm"`
	StageKmTotal    float64                                  `json:"stageKmTotal"`
	StageElapsed    float64                                  `json:"stageElapsed"`
	OverallKm       float64                                  `json:"overallKm"`
	OverallKmTotal  float64                                  `json:"overallKmTotal"`
	OverallElapsed  float64                                  `json:"overallElapsed"`
	ProgressStage   float64                                  `json:"progressStage"`
	ProgressOverall float64                                  `json:"progressOverall"`
	Groups          []ProgressGroup                          `json:"groups"`
	RiderGroups     map[int64]string                         `json:"riderGroups,omitempty"`
	RiderTelemetry  map[int64]simulation.RiderTelemetryPoint `json:"riderTelemetry,omitempty"`
	Profile         *simulation.RouteProfile                 `json:"profile,omitempty"`
	TeamState       *simulation.TeamState                    `json:"teamState,omitempty"`
	Events          []string                                 `json:"events"`
	Moment          *director.Moment                         `json:"moment,omitempty"`
	Goal            *director.Goal                           `json:"goal,omitempty"`
	GCGoal          *director.Goal                           `json:"gcGoal,omitempty"`
	// GC: the race leader and the player's riders, live (on the true
	// end-of-segment frames only).
	GC []GCLine `json:"gc,omitempty"`
}

// GCLine is a rider's place on GC, before the stage and live.
type GCLine struct {
	RiderID int64   `json:"riderId"`
	Name    string  `json:"name"`
	Team    string  `json:"team"`
	Pos     int     `json:"pos"`
	Gap     float64 `json:"gap"`
	LivePos int     `json:"livePos"`
	LiveGap float64 `json:"liveGap"`
}

// gcLines lists the live GC leader and the team's riders, live order.
func gcLines(v *simulation.RaceView, team string) []GCLine {
	if v == nil || !v.StageRace {
		return nil
	}
	var out []GCLine
	for id, r := range v.Riders {
		if r.LiveGCPos == 0 || (r.LiveGCPos != 1 && r.Team != team) {
			continue
		}
		out = append(out, GCLine{RiderID: id, Name: r.Name, Team: r.Team, Pos: r.GCPos, Gap: r.GCGap, LivePos: r.LiveGCPos, LiveGap: r.LiveGCGap})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LivePos < out[j].LivePos })
	return out
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
	TestName      string  `json:"testName"`
	Passed        bool    `json:"passed"`
	Correlation   float64 `json:"correlation"`
	Top10Acc      float64 `json:"top10Accuracy"`
	TimeGapMAE    float64 `json:"timeGapMAE"`    // mean absolute time-gap error vs FC reference (seconds)
	MatchedRiders int     `json:"matchedRiders"` // riders matched between FC reference and internal DB
	IsRealRace    bool    `json:"isRealRace"`    // true for FC-backed cases
	Details       string  `json:"details"`
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
		result[i] = riderToInfo(r)
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
		if model.IsStageRaceStageName(r.Name) {
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
	raceConfig, err := service.LoadRaceConfig(a.db, raceID, enableIncidents, nil)
	if err != nil {
		return raceResultsError(err)
	}
	simResults := service.RunRace(raceConfig, a.makeRaceProgressCallback(raceID, nil))
	_ = a.db.SaveRaceResults(simResults)
	return RaceResults{
		RaceID:   raceID,
		RaceName: raceConfig.Race.Name,
		Results:  mapSimulationResults(simResults, buildRiderMap(raceConfig.Riders)),
	}
}

// RunStageRace runs a complete stage race
func (a *App) RunStageRace(stageRaceID int64) []RaceResults {
	if a.db == nil {
		return nil
	}
	plan, err := service.PrepareStageRace(a.db, stageRaceID, true)
	if err != nil {
		return nil
	}
	out := service.RunStageRace(plan, 0, a.makeStageRaceProgressCallback(stageRaceID, len(plan.Stages), nil), nil)
	_ = service.SaveStageRaceOutput(a.db, out)

	riderMap := buildRiderMap(plan.Riders)
	finalResults := make([]RaceResults, 0, len(out.Results))
	for i, results := range out.Results {
		if i < len(plan.Stages) {
			finalResults = append(finalResults, RaceResults{
				RaceID:   plan.Stages[i].Race.ID,
				RaceName: plan.Stages[i].Race.Name,
				Results:  mapSimulationResults(results, riderMap),
			})
		}
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

// RunValidation runs synthetic + real-race validation tests.
func (a *App) RunValidation() []ValidationResult {
	synthetic := simulation.RunValidationSuite()
	realCases := append(simulation.RunRealRaceValidation(a.db), simulation.RunRealStageRaceValidation(a.db)...)
	realCases = append(realCases, simulation.RunHoldoutValidation(a.db)...)
	realCases = append(realCases, simulation.RunSpeedValidation(a.db)...)

	all := append(synthetic, realCases...)
	output := make([]ValidationResult, len(all))
	for i, r := range all {
		output[i] = ValidationResult{
			TestName:      r.Name,
			Passed:        r.Passed,
			Correlation:   r.Correlation,
			Top10Acc:      r.Top10Accuracy,
			TimeGapMAE:    r.MeanTimeGapErrorSecs,
			MatchedRiders: r.MatchedRiders,
			IsRealRace:    r.IsRealRace,
			Details:       r.Details,
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

// riderToInfo converts a model.Rider to the frontend-facing RiderInfo DTO.
func riderToInfo(r model.Rider) RiderInfo {
	return RiderInfo{
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

// makeRaceProgressCallback returns a progress callback for a single-race simulation.
// sessionID is included in the emitted event (empty for non-interactive runs).
// If pace is non-nil it is called after each update (for interactive sessions).
func (a *App) makeRaceProgressCallback(raceID int64, sess *interactiveSession) func(simulation.ProgressSnapshot) {
	sessionID := ""
	if sess != nil {
		sessionID = sess.id
	}
	return func(snapshot simulation.ProgressSnapshot) {
		if a.ctx == nil {
			return
		}
		progressPct := 0.0
		if snapshot.KmTotal > 0 {
			progressPct = snapshot.KmCovered / snapshot.KmTotal * 100
		}
		groups := make([]ProgressGroup, len(snapshot.Groups))
		for i, g := range snapshot.Groups {
			groups[i] = ProgressGroup{Name: g.Name, Gap: g.GapSeconds, Riders: g.Riders, Size: g.Size}
		}
		var moment *director.Moment
		var goal *director.Goal
		if sess != nil {
			moment = sess.Observe(snapshot.Race, snapshot.RouteProfile, snapshot.KmTotal-snapshot.KmCovered, 0)
			goal, _ = sess.Goal()
		}
		runtime.EventsEmit(a.ctx, "simulation_progress", SimulationProgress{
			SessionID:      sessionID,
			RaceID:         raceID,
			KmCovered:      snapshot.KmCovered,
			KmTotal:        snapshot.KmTotal,
			ElapsedTime:    snapshot.ElapsedTime,
			ProgressPct:    progressPct,
			SegmentName:    snapshot.SegmentName,
			Groups:         groups,
			RiderGroups:    snapshot.RiderGroups,
			RiderTelemetry: snapshot.RiderTelemetry,
			Profile:        snapshot.RouteProfile,
			TeamState:      snapshot.TeamState,
			Events:         snapshot.Events,
			Moment:         moment,
			Goal:           goal,
		})
		if sess != nil {
			sess.Pace()
		}
	}
}

// makeStageRaceProgressCallback returns a progress callback for a stage race simulation.
func (a *App) makeStageRaceProgressCallback(stageRaceID int64, stageCount int, sess *interactiveSession) func(simulation.StageProgressSnapshot) {
	sessionID := ""
	if sess != nil {
		sessionID = sess.id
	}
	return func(snapshot simulation.StageProgressSnapshot) {
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
			groups[i] = ProgressGroup{Name: g.Name, Gap: g.GapSeconds, Riders: g.Riders, Size: g.Size}
		}
		var moment *director.Moment
		var goal, gcGoal *director.Goal
		var gc []GCLine
		if sess != nil {
			moment = sess.Observe(snapshot.Race, snapshot.RouteProfile, snapshot.StageKmTotal-snapshot.StageKmCovered, snapshot.StageNumber)
			goal, _ = sess.Goal()
			gcGoal = sess.GCGoal()
			gc = gcLines(snapshot.Race, sess.controlledTeam)
		}
		runtime.EventsEmit(a.ctx, "stage_race_progress", StageRaceProgress{
			SessionID:       sessionID,
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
			RiderTelemetry:  snapshot.RiderTelemetry,
			Profile:         snapshot.RouteProfile,
			TeamState:       snapshot.TeamState,
			Events:          snapshot.Events,
			Moment:          moment,
			Goal:            goal,
			GCGoal:          gcGoal,
			GC:              gc,
		})
		if sess != nil {
			sess.Pace()
		}
	}
}

func classifyCourse(c model.Course) string {
	hasCobbles, hasGravel := false, false
	totalClimbing := 0.0
	for _, seg := range c.Segments {
		switch seg.SurfaceType {
		case model.SurfaceCobbles:
			hasCobbles = true
		case model.SurfaceGravel:
			hasGravel = true
		}
		if seg.GradientPct > 3 {
			totalClimbing += seg.DistanceKm * seg.GradientPct / 100
		}
	}

	if hasCobbles {
		return "cobbles"
	}
	if hasGravel {
		return "gravel"
	}
	if totalClimbing > 3000 {
		return "climb"
	}
	if totalClimbing > 1000 {
		return "hilly"
	}
	return "flat"
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
