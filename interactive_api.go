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
		infos = append(infos, riderToInfo(r))
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

	stages, stageInfos, err := a.buildStageConfigs(sr, false)
	if err != nil {
		return nil
	}

	selConfig := selection.DefaultConfig(model.RaceTypeRoad, true, len(stages))
	selected := selection.SelectStageRaceStartList(allRiders, stageInfos, selConfig)
	infos := make([]RiderInfo, 0, len(selected))
	for _, r := range selected {
		infos = append(infos, riderToInfo(r))
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

	raceConfig, selected, err := a.loadRaceConfig(raceID, enableIncidents, nil)
	if err != nil || len(selected) == 0 {
		return InteractiveSessionInfo{}
	}
	if controlledTeam == "" {
		controlledTeam = selected[0].Team
	}

	sess := newInteractiveSession(controlledTeam)
	a.putSession(sess)
	raceConfig.ActionOverrides = sess

	allInfos := make([]RiderInfo, 0, len(selected))
	teamInfos := make([]RiderInfo, 0)
	for _, r := range selected {
		info := riderToInfo(r)
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
		if raceConfig.Race.RaceType == model.RaceTypeTT {
			results := simulation.SimulateTT(raceConfig)
			_ = a.db.SaveRaceResults(results)
			runtime.EventsEmit(a.ctx, "interactive_simulation_complete", InteractiveComplete{SessionID: sess.id, RaceID: raceID})
			return
		}
		results := simulation.SimulateRoadRaceWithProgress(raceConfig, a.makeRaceProgressCallback(raceID, sess.id, sess.Pace))
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
	if err := a.clearStageRaceData(stageRaceID); err != nil {
		return InteractiveSessionInfo{}
	}

	allRiders, err := a.db.ListRiders()
	if err != nil {
		return InteractiveSessionInfo{}
	}
	stages, stageInfos, err := a.buildStageConfigs(sr, true)
	if err != nil {
		return InteractiveSessionInfo{}
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
		info := riderToInfo(r)
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
		progressCallback := a.makeStageRaceProgressCallback(stageRaceID, len(stages), sess.id, sess.Pace)
		allResults, allStandings := simulation.SimulateStageRaceWithProgress(sr, stages, selected, 0, progressCallback, sess)
		pointsByStage, komByStage, youthByStage := simulation.ComputeStageRaceCompetitions(sr.ID, stages, selected, allResults)
		a.saveStageRaceData(stages, allResults, allStandings, pointsByStage, komByStage, youthByStage)
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
