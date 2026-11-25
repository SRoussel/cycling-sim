package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"cycling_sim/internal/director"
	"cycling_sim/internal/model"
	"cycling_sim/internal/season"
	"cycling_sim/internal/service"
	"cycling_sim/internal/simulation"
)

// Season mode for the frontend: one hub per season (calendar, objectives,
// standings, news), the squad for the next race, the roster, the transfer
// market and the year's end; races run simulated or live (the race screen,
// recorded into the season).

type SeasonInfo struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Team      string `json:"team"`
	Year      int    `json:"year"`
	StartYear int    `json:"startYear"`
	Phase     string `json:"phase"`
	Date      string `json:"date"`
	Budget    int    `json:"budget"`
}

type SeasonEventInfo struct {
	ID       int64    `json:"id"`
	Kind     string   `json:"kind"`
	RefID    int64    `json:"refId"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Start    string   `json:"start"`
	End      string   `json:"end"`
	Days     int      `json:"days"`
	Done     bool     `json:"done"`
	Podium   []string `json:"podium"`
	Mine     string   `json:"mine"`   // our best result ("3rd, Pedersen")
	Points   int      `json:"points"` // our points from it
}

type StandingRow struct {
	Pos     int    `json:"pos"`
	RiderID int64  `json:"riderId"`
	Name    string `json:"name"`
	Team    string `json:"team"`
	Points  int    `json:"points"`
	Wins    int    `json:"wins"`
	Mine    bool   `json:"mine"`
}

type ObjectiveInfo struct {
	Text    string `json:"text"`
	Kind    string `json:"kind"`
	Target  int    `json:"target"`
	Current int    `json:"current"`
	Met     bool   `json:"met"`
	Decided bool   `json:"decided"`
}

type NewsInfo struct {
	Date string `json:"date"`
	Text string `json:"text"`
	Mine bool   `json:"mine"`
}

type SeasonHub struct {
	Season        SeasonInfo        `json:"season"`
	Wages         int               `json:"wages"`
	NextCommitted int               `json:"nextCommitted"`
	NextRiders    int               `json:"nextRiders"`
	Roster        int               `json:"roster"`
	Confidence    float64           `json:"confidence"`
	Objectives    []ObjectiveInfo   `json:"objectives"`
	Events        []SeasonEventInfo `json:"events"`
	Next          *SeasonEventInfo  `json:"next"`
	Riders        []StandingRow     `json:"riders"`
	Teams         []StandingRow     `json:"teams"`
	News          []NewsInfo        `json:"news"`
	YearOver      bool              `json:"yearOver"`
}

type SeasonRiderInfo struct {
	RiderInfo
	Age         int      `json:"age"`
	Specialty   string   `json:"specialty"`
	WKg         float64  `json:"wkg"`
	SprintW     int      `json:"sprintW"`
	Value       float64  `json:"value"`
	Salary      int      `json:"salary"`
	ContractEnd int      `json:"contractEnd"`
	NextTeam    string   `json:"nextTeam"`
	NextSalary  int      `json:"nextSalary"`
	Morale      float64  `json:"morale"`
	Load        float64  `json:"load"`
	RaceDays    int      `json:"raceDays"`
	Points      int      `json:"points"`
	Wins        int      `json:"wins"`
	Programme   []string `json:"programme"` // events still to come he's planned for
	Ask         int      `json:"ask"`       // asking salary (renewal or signing)
}

type SquadRider struct {
	SeasonRiderInfo
	Fit       float64 `json:"fit"`
	TeamRank  int     `json:"teamRank"`
	LoadPct   float64 `json:"loadPct"`
	FormPct   float64 `json:"formPct"`
	Planned   bool    `json:"planned"`
	Available bool    `json:"available"`
	Why       string  `json:"why"`
}

type SquadInfo struct {
	Event  SeasonEventInfo      `json:"event"`
	Size   int                  `json:"size"`
	Riders []SquadRider         `json:"riders"`
	Points []director.PlanPoint `json:"points"` // one-day races: the plan's decisive points
	Race   *RaceInfo            `json:"race"`   // one-day races: the race (for the race screen)
}

type OfferReply struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func seasonInfo(s *model.Season) SeasonInfo {
	return SeasonInfo{ID: s.ID, Name: s.Name, Team: s.Team, Year: s.Year, StartYear: s.StartYear, Phase: s.Phase,
		Date: s.Date.Format("2 Jan 2006"), Budget: s.Budget}
}

func (a *App) loadSeason(id int64) (*model.Season, error) {
	if a.db == nil {
		return nil, fmt.Errorf("no database")
	}
	return a.db.GetSeason(id)
}

// ListSeasons returns the saved seasons, newest first.
func (a *App) ListSeasons() []SeasonInfo {
	out := []SeasonInfo{}
	if a.db == nil {
		return out
	}
	ss, _ := a.db.ListSeasons()
	for i := range ss {
		out = append(out, seasonInfo(&ss[i]))
	}
	return out
}

// GetTeamNames lists the teams a season can be started with (biggest
// squads first: WorldTeams, then ProTeams).
func (a *App) GetTeamNames() []string {
	out := []string{}
	if a.db == nil {
		return out
	}
	riders, _ := a.db.ListRiders()
	count := map[string]int{}
	for _, r := range riders {
		count[r.Team]++
	}
	for t, n := range count {
		if n >= 15 {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// NewSeason starts a season with a team.
func (a *App) NewSeason(team, name string) (SeasonInfo, error) {
	a.seasonMu.Lock()
	defer a.seasonMu.Unlock()
	if a.db == nil {
		return SeasonInfo{}, fmt.Errorf("no database")
	}
	s, err := season.New(a.db, team, name)
	if err != nil {
		return SeasonInfo{}, err
	}
	return seasonInfo(s), nil
}

// DeleteSeason removes a season.
func (a *App) DeleteSeason(id int64) error {
	a.seasonMu.Lock()
	defer a.seasonMu.Unlock()
	if a.db == nil {
		return fmt.Errorf("no database")
	}
	return a.db.DeleteSeason(id)
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}

func seasonNames(srs []model.SeasonRider) map[int64]model.SeasonRider {
	m := make(map[int64]model.SeasonRider, len(srs))
	for _, sr := range srs {
		m[sr.ID] = sr
	}
	return m
}

func eventInfo(e model.SeasonEvent) SeasonEventInfo {
	return SeasonEventInfo{ID: e.ID, Kind: e.Kind, RefID: e.RefID, Name: e.Name, Category: e.Category,
		Start: e.Start.Format("2 Jan"), End: e.End.Format("2 Jan"), Days: e.Days(), Done: e.Done, Podium: []string{}}
}

// GetSeasonHub is everything the season's main screen shows.
func (a *App) GetSeasonHub(id int64) (SeasonHub, error) {
	s, err := a.loadSeason(id)
	if err != nil {
		return SeasonHub{}, err
	}
	srs, err := a.db.ListSeasonRiders(s.ID)
	if err != nil {
		return SeasonHub{}, err
	}
	byID := seasonNames(srs)
	hub := SeasonHub{Season: seasonInfo(s), Wages: season.WageBill(srs, s.Team), Objectives: []ObjectiveInfo{},
		Events: []SeasonEventInfo{}, Riders: []StandingRow{}, Teams: []StandingRow{}, News: []NewsInfo{}}
	hub.NextCommitted, hub.NextRiders = season.NextWageBill(srs, s.Team, s.Year)
	for _, sr := range srs {
		if sr.Team == s.Team && !sr.Retired {
			hub.Roster++
		}
	}
	if s.Phase == model.PhaseRacing {
		ps, err := season.ObjectivesProgress(a.db, s)
		if err != nil {
			return hub, err
		}
		for _, p := range ps {
			hub.Objectives = append(hub.Objectives, ObjectiveInfo{Text: p.Text, Kind: p.Kind, Target: p.Target, Current: p.Current, Met: p.Met, Decided: p.Decided})
		}
		hub.Confidence = season.BoardConfidence(ps)
	}
	// In the off-season the calendar is the year just raced (the next one
	// is drawn up when the year starts).
	shownYear := s.Year
	if s.Phase == model.PhaseOffseason {
		shownYear--
	}
	events, _ := a.db.ListSeasonEvents(s.ID, shownYear)
	results, _ := a.db.ListSeasonResults(s.ID, shownYear, 0)
	byEvent := map[int64][]model.SeasonResult{}
	for _, r := range results {
		byEvent[r.EventID] = append(byEvent[r.EventID], r)
	}
	for _, e := range events {
		info := eventInfo(e)
		var best *model.SeasonResult
		for i, r := range byEvent[e.ID] {
			final := r.Kind == model.ResultRace || r.Kind == model.ResultGC
			if final && r.Position <= 3 {
				info.Podium = append(info.Podium, byID[r.RiderID].Name)
			}
			if r.Team == s.Team {
				info.Points += r.Points
				if final && (best == nil || r.Position < best.Position) {
					best = &byEvent[e.ID][i]
				}
			}
		}
		if best != nil {
			info.Mine = fmt.Sprintf("%s, %s", ordinal(best.Position), byID[best.RiderID].Name)
		}
		hub.Events = append(hub.Events, info)
		if !e.Done && hub.Next == nil && s.Phase == model.PhaseRacing {
			next := info
			hub.Next = &next
		}
	}
	hub.YearOver = s.Phase == model.PhaseRacing && hub.Next == nil
	year := s.Year
	if s.Phase == model.PhaseOffseason {
		year-- // the standings of the year just raced
	}
	riders, teams, _ := season.Standings(a.db, s, year)
	for i, r := range riders {
		if i < 20 || r.Team == s.Team && i < 100 {
			hub.Riders = append(hub.Riders, StandingRow{Pos: i + 1, RiderID: r.RiderID, Name: byID[r.RiderID].Name, Team: r.Team, Points: r.Points, Wins: r.Wins, Mine: r.Team == s.Team})
		}
	}
	for i, t := range teams {
		hub.Teams = append(hub.Teams, StandingRow{Pos: i + 1, Team: t.Team, Points: t.Points, Wins: t.Wins, Mine: t.Team == s.Team})
	}
	news, _ := a.db.ListNews(s.ID, 25)
	for _, n := range news {
		hub.News = append(hub.News, NewsInfo{Date: n.Date.Format("2 Jan 2006"), Text: n.Text, Mine: n.Mine})
	}
	return hub, nil
}

// seasonRiderInfos describes riders with their season's points and
// programme; values rate them against the active field.
func (a *App) seasonRiderInfos(s *model.Season, srs []model.SeasonRider, pick func(model.SeasonRider) bool) []SeasonRiderInfo {
	var active []model.Rider
	for _, sr := range srs {
		if !sr.Retired && sr.Team != "" {
			active = append(active, sr.Rider)
		}
	}
	values := season.Values(a.db, active)
	year := s.Year
	if s.Phase == model.PhaseOffseason {
		year--
	}
	standings, _, _ := season.Standings(a.db, s, year)
	pts := map[int64]season.Standing{}
	for _, r := range standings {
		pts[r.RiderID] = r
	}
	events, _ := a.db.ListSeasonEvents(s.ID, s.Year)
	entries, _ := a.db.ListSeasonEntries(s.ID)
	programme := map[int64][]string{}
	for _, e := range events {
		if e.Done {
			continue
		}
		for _, id := range entries[e.ID] {
			programme[id] = append(programme[id], e.Name)
		}
	}
	out := []SeasonRiderInfo{}
	for _, sr := range srs {
		if !pick(sr) {
			continue
		}
		v, ok := values[sr.ID]
		if !ok {
			v = season.Values(a.db, append(append([]model.Rider{}, active...), sr.Rider))[sr.ID]
		}
		r := sr.Rider
		info := SeasonRiderInfo{
			RiderInfo: riderToInfo(r), Age: r.Age, Specialty: simulation.SpecialtyName(&r), WKg: float64(r.FTPWatts) / r.WeightKg,
			SprintW: r.MaxSprintWatts, Value: v, Salary: sr.Salary, ContractEnd: sr.ContractEnd, NextTeam: sr.NextTeam,
			NextSalary: sr.NextSalary, Morale: sr.Morale, Load: sr.Load, RaceDays: sr.RaceDays,
			Points: pts[sr.ID].Points, Wins: pts[sr.ID].Wins, Programme: programme[sr.ID],
			Ask: season.AskingSalary(sr, v, sr.Team != s.Team && sr.Team != ""),
		}
		if info.Programme == nil {
			info.Programme = []string{}
		}
		out = append(out, info)
	}
	return out
}

// GetSeasonRoster is the player's riders.
func (a *App) GetSeasonRoster(id int64) ([]SeasonRiderInfo, error) {
	s, err := a.loadSeason(id)
	if err != nil {
		return nil, err
	}
	srs, err := a.db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, err
	}
	out := a.seasonRiderInfos(s, srs, func(sr model.SeasonRider) bool { return sr.Team == s.Team && !sr.Retired })
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	return out, nil
}

// GetSeasonMarket lists riders the player can sign: in the racing season,
// other teams' riders whose contracts end this year (they join next year);
// in the off-season, the free agents (they join now).
func (a *App) GetSeasonMarket(id int64) ([]SeasonRiderInfo, error) {
	s, err := a.loadSeason(id)
	if err != nil {
		return nil, err
	}
	srs, err := a.db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, err
	}
	out := a.seasonRiderInfos(s, srs, func(sr model.SeasonRider) bool {
		if sr.Retired || sr.Team == s.Team || sr.NextTeam != "" {
			return false
		}
		if s.Phase == model.PhaseOffseason {
			return sr.Team == ""
		}
		return sr.Team != "" && sr.ContractEnd <= s.Year
	})
	sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	if len(out) > 150 {
		out = out[:150]
	}
	return out, nil
}

// OfferContract offers a rider a contract (salary in thousand euro a year).
func (a *App) OfferContract(id, riderID int64, salary, years int) (OfferReply, error) {
	a.seasonMu.Lock()
	defer a.seasonMu.Unlock()
	s, err := a.loadSeason(id)
	if err != nil {
		return OfferReply{}, err
	}
	ok, msg, err := season.Offer(a.db, s, riderID, salary, years)
	return OfferReply{OK: ok, Message: msg}, err
}

// AutoRenewContracts renews the player's expiring contracts (best riders
// first) up to 26 riders for next year, as far as the budget goes.
func (a *App) AutoRenewContracts(id int64) ([]string, error) {
	a.seasonMu.Lock()
	defer a.seasonMu.Unlock()
	s, err := a.loadSeason(id)
	if err != nil {
		return nil, err
	}
	out, err := season.AutoRenew(a.db, s, 26)
	if out == nil {
		out = []string{}
	}
	return out, err
}

// GetSeasonSquad is the player's choice of riders for an event.
func (a *App) GetSeasonSquad(id, eventID int64) (SquadInfo, error) {
	s, err := a.loadSeason(id)
	if err != nil {
		return SquadInfo{}, err
	}
	e, err := season.Event(a.db, s, eventID)
	if err != nil {
		return SquadInfo{}, err
	}
	opts, err := season.Squad(a.db, s, eventID)
	if err != nil {
		return SquadInfo{}, err
	}
	srs, _ := a.db.ListSeasonRiders(s.ID)
	infos := a.seasonRiderInfos(s, srs, func(sr model.SeasonRider) bool { return sr.Team == s.Team && !sr.Retired })
	byID := map[int64]SeasonRiderInfo{}
	for _, in := range infos {
		byID[in.ID] = in
	}
	out := SquadInfo{Event: eventInfo(*e), Size: season.SquadSize(e), Riders: []SquadRider{}, Points: []director.PlanPoint{}}
	for _, o := range opts {
		out.Riders = append(out.Riders, SquadRider{SeasonRiderInfo: byID[o.ID], Fit: o.Fit, TeamRank: o.TeamRank,
			LoadPct: o.LoadPct, FormPct: o.FormPct, Planned: o.Planned, Available: o.Available, Why: o.Why})
	}
	if e.Kind == model.EventOneDay {
		race, err := a.db.GetRace(e.RefID)
		if err == nil {
			if course, err := a.db.GetCourse(race.CourseID); err == nil {
				out.Points = director.PlanPoints(simulation.BuildRouteProfile(course))
				if out.Points == nil {
					out.Points = []director.PlanPoint{}
				}
			}
			for _, ri := range a.GetRaces() {
				if ri.ID == race.ID {
					r := ri
					out.Race = &r
				}
			}
		}
	}
	return out, nil
}

// SetSeasonSquad saves the player's squad for an event.
func (a *App) SetSeasonSquad(id, eventID int64, riderIDs []int64) error {
	a.seasonMu.Lock()
	defer a.seasonMu.Unlock()
	s, err := a.loadSeason(id)
	if err != nil {
		return err
	}
	return season.SetPicks(a.db, s, eventID, riderIDs)
}

// SimulateSeasonEvent races the next event without the player (his squad as
// planned) and records it. Returns the event with its result.
func (a *App) SimulateSeasonEvent(id, eventID int64) (SeasonEventInfo, error) {
	a.seasonMu.Lock()
	defer a.seasonMu.Unlock()
	s, err := a.loadSeason(id)
	if err != nil {
		return SeasonEventInfo{}, err
	}
	if err := a.checkNext(s, eventID); err != nil {
		return SeasonEventInfo{}, err
	}
	p, err := season.Prepare(a.db, s, eventID, nil)
	if err != nil {
		return SeasonEventInfo{}, err
	}
	if err := season.Simulate(a.db, p, 0); err != nil {
		return SeasonEventInfo{}, err
	}
	hub, err := a.GetSeasonHub(id)
	if err != nil {
		return SeasonEventInfo{}, err
	}
	for _, e := range hub.Events {
		if e.ID == eventID {
			return e, nil
		}
	}
	return SeasonEventInfo{}, nil
}

// checkNext: events are raced in calendar order.
func (a *App) checkNext(s *model.Season, eventID int64) error {
	if s.Phase != model.PhaseRacing {
		return fmt.Errorf("the season is in its off-season")
	}
	next, err := season.NextEvent(a.db, s)
	if err != nil {
		return err
	}
	if next == nil || next.ID != eventID {
		return fmt.Errorf("that isn't the next race")
	}
	return nil
}

// StartSeasonRace races a one-day event live: the race screen with the
// player's squad, recorded into the season when it finishes.
func (a *App) StartSeasonRace(id, eventID int64, plan RacePlan) (InteractiveSessionInfo, error) {
	a.seasonMu.Lock()
	s, err := a.loadSeason(id)
	if err == nil {
		err = a.checkNext(s, eventID)
	}
	var p *season.Prepared
	if err == nil {
		p, err = season.Prepare(a.db, s, eventID, nil)
	}
	a.seasonMu.Unlock()
	if err != nil {
		return InteractiveSessionInfo{}, err
	}
	if p.Stages != nil {
		return InteractiveSessionInfo{}, fmt.Errorf("%s is a stage race", p.Event.Name)
	}
	cfg := p.OneDay
	sess := newInteractiveSession(s.Team)
	sess.SetPlan(plan, director.PlanPoints(simulation.BuildRouteProfile(cfg.Course)))
	a.putSession(sess)
	cfg.ActionOverrides = sess
	allInfos, teamInfos := splitTeam(cfg.Riders, s.Team)
	raceID := cfg.Race.ID
	go func() {
		defer a.deleteSession(sess.id)
		if a.ctx == nil {
			return
		}
		results := simulation.SimulateRoadRaceWithProgress(cfg, a.makeRaceProgressCallback(raceID, sess))
		if sess.IsStopped() {
			return
		}
		a.seasonMu.Lock()
		err := season.RecordOneDay(a.db, p, results)
		a.seasonMu.Unlock()
		if err != nil {
			runtime.LogErrorf(a.ctx, "recording %s: %v", p.Event.Name, err)
		}
		goal, breakMade := sess.Goal()
		runtime.EventsEmit(a.ctx, "interactive_simulation_complete", InteractiveComplete{SessionID: sess.id, RaceID: raceID, Goal: goal, BreakMade: breakMade})
	}()
	return InteractiveSessionInfo{SessionID: sess.id, RaceID: raceID, ControlledTeam: s.Team, AllRiders: allInfos, TeamRiders: teamInfos}, nil
}

// StartSeasonStageRace races a stage race live, recorded into the season.
func (a *App) StartSeasonStageRace(id, eventID int64) (InteractiveSessionInfo, error) {
	a.seasonMu.Lock()
	s, err := a.loadSeason(id)
	if err == nil {
		err = a.checkNext(s, eventID)
	}
	var p *season.Prepared
	if err == nil {
		p, err = season.Prepare(a.db, s, eventID, nil)
	}
	a.seasonMu.Unlock()
	if err != nil {
		return InteractiveSessionInfo{}, err
	}
	if p.Stages == nil {
		return InteractiveSessionInfo{}, fmt.Errorf("%s is a one-day race", p.Event.Name)
	}
	plan := p.Stages
	sess := newInteractiveSession(s.Team)
	a.putSession(sess)
	allInfos, teamInfos := splitTeam(plan.Riders, s.Team)
	srID := plan.StageRace.ID
	go func() {
		defer a.deleteSession(sess.id)
		if a.ctx == nil {
			return
		}
		progress := a.makeStageRaceProgressCallback(srID, len(plan.Stages), sess)
		out := service.RunStageRace(plan, 0, progress, sess)
		if sess.IsStopped() {
			return
		}
		a.seasonMu.Lock()
		err := season.RecordStageRace(a.db, p, out)
		a.seasonMu.Unlock()
		if err != nil {
			runtime.LogErrorf(a.ctx, "recording %s: %v", p.Event.Name, err)
		}
		runtime.EventsEmit(a.ctx, "interactive_stage_race_complete", InteractiveComplete{SessionID: sess.id, StageRaceID: srID, Goal: sess.GCGoal()})
	}()
	return InteractiveSessionInfo{SessionID: sess.id, StageRaceID: srID, ControlledTeam: s.Team, AllRiders: allInfos, TeamRiders: teamInfos}, nil
}

// SeasonResultRow is one line of an event's results.
type SeasonResultRow struct {
	Kind     string  `json:"kind"`
	Stage    int     `json:"stage"`
	Position int     `json:"position"`
	RiderID  int64   `json:"riderId"`
	Name     string  `json:"name"`
	Team     string  `json:"team"`
	Gap      float64 `json:"gap"`
	Points   int     `json:"points"`
	Mine     bool    `json:"mine"`
}

// GetSeasonEventResults lists an event's results: the race (one-day) or the
// final GC and each stage's top 10 (stage races), our riders always.
func (a *App) GetSeasonEventResults(id, eventID int64) ([]SeasonResultRow, error) {
	s, err := a.loadSeason(id)
	if err != nil {
		return nil, err
	}
	srs, _ := a.db.ListSeasonRiders(s.ID)
	byID := seasonNames(srs)
	year := s.Year
	events, _ := a.db.ListSeasonEvents(s.ID, year)
	found := false
	for _, e := range events {
		found = found || e.ID == eventID
	}
	if !found {
		year--
	}
	rs, err := a.db.ListSeasonResults(s.ID, year, eventID)
	if err != nil {
		return nil, err
	}
	out := []SeasonResultRow{}
	for _, r := range rs {
		mine := r.Team == s.Team
		limit := 30
		if r.Kind == model.ResultStage {
			limit = 10
		}
		if r.Position > limit && !mine {
			continue
		}
		out = append(out, SeasonResultRow{Kind: r.Kind, Stage: r.Stage, Position: r.Position, RiderID: r.RiderID,
			Name: byID[r.RiderID].Name, Team: r.Team, Gap: r.GapS, Points: r.Points, Mine: mine})
	}
	return out, nil
}

// GetSeasonRaceResults is a season one-day race's result in the race
// screen's shape.
func (a *App) GetSeasonRaceResults(id, eventID int64) (RaceResults, error) {
	rows, err := a.GetSeasonEventResults(id, eventID)
	if err != nil {
		return RaceResults{}, err
	}
	s, _ := a.loadSeason(id)
	e, _ := season.Event(a.db, s, eventID)
	out := RaceResults{Results: []SimulationResult{}}
	if e != nil {
		out.RaceID, out.RaceName = e.RefID, e.Name
	}
	for _, r := range rows {
		if r.Kind == model.ResultRace {
			out.Results = append(out.Results, SimulationResult{Position: r.Position, RiderID: r.RiderID, RiderName: r.Name, Team: r.Team, Gap: r.Gap})
		}
	}
	return out, nil
}

// EndSeasonYear closes the year: the board's verdict, contracts, ageing,
// retirements, new riders; the transfer window opens.
func (a *App) EndSeasonYear(id int64) ([]NewsInfo, error) {
	a.seasonMu.Lock()
	defer a.seasonMu.Unlock()
	s, err := a.loadSeason(id)
	if err != nil {
		return nil, err
	}
	news, err := season.EndYear(a.db, s)
	return newsInfos(news), err
}

// StartSeasonYear closes the transfer window and starts the next year.
func (a *App) StartSeasonYear(id int64) ([]NewsInfo, error) {
	a.seasonMu.Lock()
	defer a.seasonMu.Unlock()
	s, err := a.loadSeason(id)
	if err != nil {
		return nil, err
	}
	news, err := season.StartYear(a.db, s)
	return newsInfos(news), err
}

func newsInfos(items []model.NewsItem) []NewsInfo {
	out := []NewsInfo{}
	for _, n := range items {
		if strings.TrimSpace(n.Text) != "" {
			out = append(out, NewsInfo{Date: n.Date.Format("2 Jan 2006"), Text: n.Text, Mine: n.Mine})
		}
	}
	return out
}
