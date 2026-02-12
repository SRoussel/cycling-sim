package main

import (
	"sort"
	"strings"

	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
	"cycling_sim/internal/simulation"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type InteractiveSessionInfo struct {
	SessionID      string      `json:"sessionId"`
	RaceID         int64       `json:"raceId"`
	StageRaceID    int64       `json:"stageRaceId"`
	ControlledTeam string      `json:"controlledTeam"`
	AllRiders      []RiderInfo `json:"allRiders"`
	TeamRiders     []RiderInfo `json:"teamRiders"`
}

type InteractiveComplete struct {
	SessionID   string `json:"sessionId"`
	RaceID      int64  `json:"raceId"`
	StageRaceID int64  `json:"stageRaceId"`
}

func (a *App) getSession(sessionID string) *interactiveSession {
	a.sessionsMu.Lock()
	defer a.sessionsMu.Unlock()
	return a.sessions[sessionID]
}

func (a *App) putSession(s *interactiveSession) {
	a.sessionsMu.Lock()
	defer a.sessionsMu.Unlock()
	a.sessions[s.id] = s
}

func (a *App) deleteSession(sessionID string) {
	a.sessionsMu.Lock()
	defer a.sessionsMu.Unlock()
	delete(a.sessions, sessionID)
}

// PreviewRaceStartList returns the selected start list for a race (same selection logic as simulation).
func (a *App) PreviewRaceStartList(raceID int64) []RiderInfo {
	if a.db == nil {
		return nil
	}
	race, err := a.db.GetRace(raceID)
	if err != nil {
		return nil
	}
	course, err := a.db.GetCourse(race.CourseID)
	if err != nil {
		return nil
	}
	riders, err := a.db.ListRiders()
	if err != nil {
		return nil
	}

	config := selection.DefaultConfig(race.RaceType, false, 0)
	selected := selection.SelectStartList(riders, course, race, config)
	infos := make([]RiderInfo, 0, len(selected))
	for _, r := range selected {
		infos = append(infos, RiderInfo{
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
		})
	}
	return sortRidersByTeamThenName(infos)
}

// PreviewStageRaceStartList returns the selected start list for a stage race.
func (a *App) PreviewStageRaceStartList(stageRaceID int64) []RiderInfo {
	if a.db == nil {
		return nil
	}
	sr, err := a.db.GetStageRace(stageRaceID)
	if err != nil {
		return nil
	}
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
	infos := make([]RiderInfo, 0, len(selected))
	for _, r := range selected {
		infos = append(infos, RiderInfo{
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
		})
	}
	return sortRidersByTeamThenName(infos)
}

func sortRidersByTeamThenName(infos []RiderInfo) []RiderInfo {
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].Team != infos[j].Team {
			return infos[i].Team < infos[j].Team
		}
		return infos[i].Name < infos[j].Name
	})
	return infos
}

// StartInteractiveSimulation starts a race simulation in interactive mode (user controls one team).
// It returns immediately with a session ID; progress is emitted via events.
func (a *App) StartInteractiveSimulation(raceID int64, enableIncidents bool, controlledTeam string) InteractiveSessionInfo {
	if a.db == nil {
		return InteractiveSessionInfo{}
	}
	controlledTeam = strings.TrimSpace(controlledTeam)

	race, err := a.db.GetRace(raceID)
	if err != nil {
		return InteractiveSessionInfo{}
	}
	course, err := a.db.GetCourse(race.CourseID)
	if err != nil {
		return InteractiveSessionInfo{}
	}
	riders, err := a.db.ListRiders()
	if err != nil {
		return InteractiveSessionInfo{}
	}

	config := selection.DefaultConfig(race.RaceType, false, 0)
	selected := selection.SelectStartList(riders, course, race, config)
	if len(selected) == 0 {
		return InteractiveSessionInfo{}
	}
	if controlledTeam == "" {
		controlledTeam = selected[0].Team
	}

	sess := newInteractiveSession(controlledTeam)
	a.putSession(sess)

	allInfos := make([]RiderInfo, 0, len(selected))
	teamInfos := make([]RiderInfo, 0)
	for _, r := range selected {
		info := RiderInfo{
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
		allInfos = append(allInfos, info)
		if r.Team == controlledTeam {
			teamInfos = append(teamInfos, info)
		}
	}

	go func() {
		defer a.deleteSession(sess.id)
		if a.ctx == nil {
			return
		}

		raceConfig := simulation.RaceConfig{
			Race:             race,
			Course:           course,
			Riders:           selected,
			Seed:             0,
			IncidentsEnabled: enableIncidents,
			ActionOverrides:  sess,
		}

		if race.RaceType == model.RaceTypeTT {
			results := simulation.SimulateTT(raceConfig)
			_ = a.db.SaveRaceResults(results)
			runtime.EventsEmit(a.ctx, "interactive_simulation_complete", InteractiveComplete{SessionID: sess.id, RaceID: raceID})
			return
		}

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
				groups[i] = ProgressGroup{Name: g.Name, Gap: g.GapSeconds, Riders: g.Riders}
			}
			runtime.EventsEmit(a.ctx, "simulation_progress", SimulationProgress{
				SessionID:      sess.id,
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
			sess.Pace()
		}

		results := simulation.SimulateRoadRaceWithProgress(raceConfig, progressCallback)
		_ = a.db.SaveRaceResults(results)
		runtime.EventsEmit(a.ctx, "interactive_simulation_complete", InteractiveComplete{SessionID: sess.id, RaceID: raceID})
	}()

	return InteractiveSessionInfo{
		SessionID:      sess.id,
		RaceID:         raceID,
		ControlledTeam: controlledTeam,
		AllRiders:      sortRidersByTeamThenName(allInfos),
		TeamRiders:     sortRidersByTeamThenName(teamInfos),
	}
}

// StartInteractiveStageRace starts a stage race simulation in interactive mode.
func (a *App) StartInteractiveStageRace(stageRaceID int64, controlledTeam string) InteractiveSessionInfo {
	if a.db == nil {
		return InteractiveSessionInfo{}
	}
	controlledTeam = strings.TrimSpace(controlledTeam)

	sr, err := a.db.GetStageRace(stageRaceID)
	if err != nil {
		return InteractiveSessionInfo{}
	}
	if err := a.db.ClearGCStandings(stageRaceID); err != nil {
		return InteractiveSessionInfo{}
	}
	_ = a.db.ClearPointsStandings(stageRaceID)
	_ = a.db.ClearMountainsStandings(stageRaceID)
	_ = a.db.ClearYouthStandings(stageRaceID)

	allRiders, err := a.db.ListRiders()
	if err != nil {
		return InteractiveSessionInfo{}
	}

	var stages []simulation.StageConfig
	var stageInfos []selection.StageInfo
	for _, s := range sr.Stages {
		r, err := a.db.GetRace(s.RaceID)
		if err != nil {
			return InteractiveSessionInfo{}
		}
		if err := a.db.ClearRaceResults(r.ID); err != nil {
			return InteractiveSessionInfo{}
		}
		c, err := a.db.GetCourse(r.CourseID)
		if err != nil {
			return InteractiveSessionInfo{}
		}
		stages = append(stages, simulation.StageConfig{Race: r, Course: c, StageNumber: s.StageNumber, RestDayBefore: s.RestDayBefore})
		stageInfos = append(stageInfos, selection.StageInfo{Course: c, RaceType: r.RaceType})
	}

	selConfig := selection.DefaultConfig(model.RaceTypeRoad, true, len(stages))
	selected := selection.SelectStageRaceStartList(allRiders, stageInfos, selConfig)
	if len(selected) == 0 {
		return InteractiveSessionInfo{}
	}
	if controlledTeam == "" {
		controlledTeam = selected[0].Team
	}

	sess := newInteractiveSession(controlledTeam)
	a.putSession(sess)

	allInfos := make([]RiderInfo, 0, len(selected))
	teamInfos := make([]RiderInfo, 0)
	for _, r := range selected {
		info := RiderInfo{
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
		allInfos = append(allInfos, info)
		if r.Team == controlledTeam {
			teamInfos = append(teamInfos, info)
		}
	}

	go func() {
		defer a.deleteSession(sess.id)
		if a.ctx == nil {
			return
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
				groups[i] = ProgressGroup{Name: g.Name, Gap: g.GapSeconds, Riders: g.Riders}
			}
			runtime.EventsEmit(a.ctx, "stage_race_progress", StageRaceProgress{
				SessionID:       sess.id,
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
			sess.Pace()
		}

		allResults, allStandings := simulation.SimulateStageRaceWithProgress(sr, stages, selected, 0, progressCallback, sess)
		pointsByStage, komByStage, youthByStage := simulation.ComputeStageRaceCompetitions(sr.ID, stages, selected, allResults)

		for i, results := range allResults {
			if i < len(stages) {
				_ = a.db.SaveRaceResults(results)
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

		runtime.EventsEmit(a.ctx, "interactive_stage_race_complete", InteractiveComplete{SessionID: sess.id, StageRaceID: stageRaceID})
	}()

	return InteractiveSessionInfo{
		SessionID:      sess.id,
		StageRaceID:    stageRaceID,
		ControlledTeam: controlledTeam,
		AllRiders:      sortRidersByTeamThenName(allInfos),
		TeamRiders:     sortRidersByTeamThenName(teamInfos),
	}
}

func (a *App) SetInteractivePaused(sessionID string, paused bool) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	s.SetPaused(paused)
}

func (a *App) SetInteractiveSpeed(sessionID string, speed float64) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	s.SetSpeed(speed)
}

// SetRiderCommand sets a rider command for an interactive session.
// action: "hold" clears override; "attack"/"chase"/"conserve" apply.
// mode: "persistent" or "oneshot" (for "attack now").
func (a *App) SetRiderCommand(sessionID string, riderID int64, action string, mode string) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	action = strings.ToLower(strings.TrimSpace(action))
	mode = strings.ToLower(strings.TrimSpace(mode))

	act := simulation.ActionHold
	switch action {
	case "attack":
		act = simulation.ActionAttack
	case "chase":
		act = simulation.ActionChase
	case "conserve":
		act = simulation.ActionConserve
	case "hold", "":
		act = simulation.ActionHold
	default:
		return
	}
	if mode != "oneshot" {
		mode = "persistent"
	}
	s.SetRiderCommand(riderID, act, mode)
}

// SetRiderEffort sets a rider's effort target as a percentage [60..130].
func (a *App) SetRiderEffort(sessionID string, riderID int64, effortPct float64) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	effortMultiplier := effortPct / 100.0
	s.SetRiderEffort(riderID, effortMultiplier)
}

// SetSprintTrainOrder sets ordered rider IDs for sprint-train behavior.
func (a *App) SetSprintTrainOrder(sessionID string, riderIDs []int64) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	s.SetSprintTrainOrder(riderIDs)
}

// SetRiderFollow sets or clears a rider's follow target for sprint-train behavior.
// followRiderID <= 0 clears follow behavior for riderID.
func (a *App) SetRiderFollow(sessionID string, riderID int64, followRiderID int64) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	s.SetRiderFollow(riderID, followRiderID, followRiderID > 0)
}

// LaunchSprint marks a rider as launched (or clears launch when riderID <= 0).
func (a *App) LaunchSprint(sessionID string, riderID int64) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	s.LaunchSprint(riderID)
}

func (a *App) StopInteractiveSession(sessionID string) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	s.Stop()
}
