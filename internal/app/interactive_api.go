package app

import (
	"cycling_sim/internal/director"
	"sort"
	"strings"

	"cycling_sim/internal/model"
	"cycling_sim/internal/service"
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
	// One-day races: today's goal and whether a rider of ours made the break.
	Goal      *director.Goal `json:"goal,omitempty"`
	BreakMade bool           `json:"breakMade"`
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
	cfg, err := service.LoadRaceConfig(a.db, raceID, false, nil)
	if err != nil {
		return nil
	}
	return ridersToSortedInfos(cfg.Riders)
}

// PreviewStageRaceStartList returns the selected start list for a stage race.
func (a *App) PreviewStageRaceStartList(stageRaceID int64) []RiderInfo {
	if a.db == nil {
		return nil
	}
	plan, err := service.PrepareStageRace(a.db, stageRaceID, false)
	if err != nil {
		return nil
	}
	return ridersToSortedInfos(plan.Riders)
}

func ridersToSortedInfos(riders []model.Rider) []RiderInfo {
	infos := make([]RiderInfo, 0, len(riders))
	for _, r := range riders {
		infos = append(infos, riderToInfo(r))
	}
	return sortRidersByTeamThenName(infos)
}

// splitTeam returns sorted infos for the whole field and for one team.
func splitTeam(riders []model.Rider, team string) (all, mine []RiderInfo) {
	var teamRiders []model.Rider
	for _, r := range riders {
		if r.Team == team {
			teamRiders = append(teamRiders, r)
		}
	}
	return ridersToSortedInfos(riders), ridersToSortedInfos(teamRiders)
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
func (a *App) StartInteractiveSimulation(raceID int64, enableIncidents bool, controlledTeam string, plan RacePlan) InteractiveSessionInfo {
	if a.db == nil {
		return InteractiveSessionInfo{}
	}
	controlledTeam = strings.TrimSpace(controlledTeam)

	raceConfig, err := service.LoadRaceConfig(a.db, raceID, enableIncidents, nil)
	if err != nil || len(raceConfig.Riders) == 0 {
		return InteractiveSessionInfo{}
	}
	if controlledTeam == "" {
		controlledTeam = raceConfig.Riders[0].Team
	}

	sess := newInteractiveSession(controlledTeam)
	sess.SetPlan(plan, director.PlanPoints(simulation.BuildRouteProfile(raceConfig.Course)))
	a.putSession(sess)
	raceConfig.ActionOverrides = sess
	allInfos, teamInfos := splitTeam(raceConfig.Riders, controlledTeam)

	go func() {
		defer a.deleteSession(sess.id)
		if a.ctx == nil {
			return
		}
		results := service.RunRace(raceConfig, a.makeRaceProgressCallback(raceID, sess))
		if sess.IsStopped() {
			return // abandoned by the user: don't overwrite saved results
		}
		_ = a.db.SaveRaceResults(results)
		goal, breakMade := sess.Goal()
		runtime.EventsEmit(a.ctx, "interactive_simulation_complete", InteractiveComplete{SessionID: sess.id, RaceID: raceID, Goal: goal, BreakMade: breakMade})
	}()

	return InteractiveSessionInfo{
		SessionID:      sess.id,
		RaceID:         raceID,
		ControlledTeam: controlledTeam,
		AllRiders:      allInfos,
		TeamRiders:     teamInfos,
	}
}

// StartInteractiveStageRace starts a stage race simulation in interactive mode.
func (a *App) StartInteractiveStageRace(stageRaceID int64, controlledTeam string) InteractiveSessionInfo {
	if a.db == nil {
		return InteractiveSessionInfo{}
	}
	controlledTeam = strings.TrimSpace(controlledTeam)

	plan, err := service.PrepareStageRace(a.db, stageRaceID, true)
	if err != nil {
		return InteractiveSessionInfo{}
	}
	if controlledTeam == "" {
		controlledTeam = plan.Riders[0].Team
	}

	sess := newInteractiveSession(controlledTeam)
	a.putSession(sess)
	allInfos, teamInfos := splitTeam(plan.Riders, controlledTeam)

	go func() {
		defer a.deleteSession(sess.id)
		if a.ctx == nil {
			return
		}
		progress := a.makeStageRaceProgressCallback(stageRaceID, len(plan.Stages), sess)
		out := service.RunStageRace(plan, 0, progress, sess)
		if sess.IsStopped() {
			return // abandoned by the user: don't overwrite saved results
		}
		_ = service.SaveStageRaceOutput(a.db, out)
		runtime.EventsEmit(a.ctx, "interactive_stage_race_complete", InteractiveComplete{SessionID: sess.id, StageRaceID: stageRaceID, Goal: sess.GCGoal()})
	}()

	return InteractiveSessionInfo{
		SessionID:      sess.id,
		StageRaceID:    stageRaceID,
		ControlledTeam: controlledTeam,
		AllRiders:      allInfos,
		TeamRiders:     teamInfos,
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

// SetRiderStyle sets how deep a rider digs to hold his group ("any",
// "reserve", "own") and his pace when he does the work ("auto", "tempo",
// "threshold", "allout").
func (a *App) SetRiderStyle(sessionID string, riderID int64, dig, pace string) {
	if s := a.getSession(sessionID); s != nil {
		s.SetRiderStyle(riderID, dig, pace)
	}
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

// SetSprintTrainOrder sets who the lead-out rides for: the first rider ID
// (none: the team's fastest finisher in each group).
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

// SetTeamOrder sets one of the player's team orders. kind: "chase" (help
// chase breaks and moves), "break" (get riders in the break), "pace" (set
// the pace for the leader), "protect" (wait for and pace back a dropped
// leader), "leadout" (the sprint train) or "ambition" (stage races: yes =
// ride for the stage, no = ride for GC). value: "auto" (the team decides),
// "yes" or "no".
func (a *App) SetTeamOrder(sessionID string, kind string, value string) {
	sess := a.getSession(sessionID)
	if sess == nil {
		return
	}
	order := simulation.OrderAuto
	switch value {
	case "yes":
		order = simulation.OrderYes
	case "no":
		order = simulation.OrderNo
	}
	sess.SetTeamOrder(kind, order)
}

// SetStagePlan sets the player's plan for the coming stage of a stage race
// and starts the stage.
func (a *App) SetStagePlan(sessionID string, plan RacePlan) {
	if s := a.getSession(sessionID); s != nil {
		s.SetStagePlan(plan)
	}
}

// PreviewRacePlan lists the decisive climbs and sectors of a race's course,
// the places a race plan can target.
func (a *App) PreviewRacePlan(raceID int64) []director.PlanPoint {
	if a.db == nil {
		return nil
	}
	race, err := a.db.GetRace(raceID)
	if err != nil || race == nil {
		return nil
	}
	course, err := a.db.GetCourse(race.CourseID)
	if err != nil || course == nil {
		return nil
	}
	return director.PlanPoints(simulation.BuildRouteProfile(course))
}

// PreviewRaceProfile returns a race's course profile (elevation, sectors)
// for the setup page, before any session exists.
func (a *App) PreviewRaceProfile(raceID int64) *simulation.RouteProfile {
	if a.db == nil {
		return nil
	}
	race, err := a.db.GetRace(raceID)
	if err != nil || race == nil {
		return nil
	}
	course, err := a.db.GetCourse(race.CourseID)
	if err != nil || course == nil {
		return nil
	}
	return simulation.BuildRouteProfile(course)
}

// ResolveMoment applies the player's choice for a decision moment and resumes
// the race. It returns the chosen option's label ("" if the moment is gone).
func (a *App) ResolveMoment(sessionID string, momentID string, optionID string) string {
	s := a.getSession(sessionID)
	if s == nil {
		return ""
	}
	label, err := s.Resolve(momentID, optionID)
	if err != nil {
		return ""
	}
	return label
}

// SetDecisionLevel sets how often a session stops for decisions: "off",
// "key" or "all".
func (a *App) SetDecisionLevel(sessionID string, level string) {
	if s := a.getSession(sessionID); s != nil {
		s.SetDecisionLevel(level)
	}
}

func (a *App) StopInteractiveSession(sessionID string) {
	s := a.getSession(sessionID)
	if s == nil {
		return
	}
	s.Stop()
}
