package season

import (
	"fmt"
	"math"
	"sort"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/service"
	"cycling_sim/internal/simulation"
)

// LoadDecayDays: racing load fades with this time constant (days). A Grand
// Tour (21 race days) still weighs ~13 a week later and ~2 a month later.
const LoadDecayDays = 14.0

// loadFree is the load a rider carries without losing anything; above it,
// each race day of load costs loadCost of threshold, down to loadFloor.
const (
	loadFree  = 6.0
	loadCost  = 0.004
	loadFloor = 0.94
	// RestLoad: AI teams rest a rider carrying more than this load (straight
	// after a Grand Tour, or after two weeks of racing).
	RestLoad = 10.0
	// MaxGrandTours an AI team gives one rider in a year.
	MaxGrandTours = 2
	// RaceDayBudget: teams keep a domestique under about this many race days
	// (leaders race fewer, see riderBudget).
	RaceDayBudget = 75
)

// LoadFactor is the threshold a rider keeps with his racing load.
func LoadFactor(load float64) float64 {
	return math.Max(loadFloor, 1-loadCost*math.Max(0, load-loadFree))
}

// MoraleFactor: a happy rider finds ~1% more, an unhappy one loses ~1%.
func MoraleFactor(morale float64) float64 {
	return 1 + (morale-MoraleNeutral)*0.0003
}

// SquadSize is the number of riders a team starts an event with.
func SquadSize(e *model.SeasonEvent) int {
	if e.Category == model.CategoryGrandTour {
		return 8
	}
	return 7
}

// loadAt is a rider's load days after the season's date. Load decays
// exponentially, so a race starting before the season's date (one that
// overlaps the last one raced) gets the load back by a negative days.
func loadAt(sr model.SeasonRider, days float64) float64 {
	return sr.Load * math.Exp(-days/LoadDecayDays)
}

// raceRider is the rider as he starts the race: his threshold scaled by his
// load (at the start date) and morale.
func raceRider(sr model.SeasonRider, days float64) model.Rider {
	r := sr.Rider
	load := loadAt(sr, days)
	r.FTPWatts = int(math.Round(float64(r.FTPWatts) * LoadFactor(load) * MoraleFactor(sr.Morale)))
	return r
}

// Prepared is an event ready to race: the start list (season riders as they
// start), and the one-day config or stage-race plan.
type Prepared struct {
	Season *model.Season
	Event  *model.SeasonEvent
	Riders []model.Rider
	OneDay simulation.RaceConfig
	Stages *service.StageRacePlan
}

// Prepare builds an event's field from the season's plan (see planYear):
// each team's planned riders, less any retired or still racing an
// overlapping event; the player's squad is his picks (without picks: his
// planned riders), filled up by the team's own choice. Riders race with
// their load and morale, in their seasonal form.
func Prepare(db *database.DB, s *model.Season, eventID int64, picks []int64) (*Prepared, error) {
	e, err := Event(db, s, eventID)
	if err != nil {
		return nil, err
	}
	if e.Done {
		return nil, fmt.Errorf("%s has been raced", e.Name)
	}
	entries, err := db.ListSeasonEntries(s.ID)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		if err := planYear(db, s); err != nil {
			return nil, err
		}
		if entries, err = db.ListSeasonEntries(s.ID); err != nil {
			return nil, err
		}
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, err
	}
	busy, err := busyRiders(db, s, e)
	if err != nil {
		return nil, err
	}
	planned := idSet(entries[e.ID])
	picked := idSet(picks)
	days := e.Start.Sub(s.Date).Hours() / 24
	var field, mine, spare []model.Rider
	for _, sr := range srs {
		if sr.Retired || sr.Team == "" || busy[sr.ID] {
			continue
		}
		r := raceRider(sr, days)
		switch {
		case sr.Team != s.Team:
			if planned[sr.ID] {
				field = append(field, r)
			}
		case len(picks) > 0 && picked[sr.ID], len(picks) == 0 && planned[sr.ID]:
			mine = append(mine, r)
		default:
			spare = append(spare, r)
		}
	}
	squad := SquadSize(e)
	if len(mine) > squad {
		mine = mine[:squad]
	}
	if len(mine) < squad && len(spare) > 0 {
		fill, err := pickField(db, *e, spare, 1, squad-len(mine))
		if err != nil {
			return nil, err
		}
		mine = append(mine, fill...)
	}
	p := &Prepared{Season: s, Event: e, Riders: append(field, mine...)}
	if e.Kind == model.EventOneDay {
		race, err := db.GetRace(e.RefID)
		if err != nil {
			return nil, err
		}
		course, err := db.GetCourse(race.CourseID)
		if err != nil {
			return nil, err
		}
		race.Date = e.Start
		p.OneDay = simulation.RaceConfig{Race: race, Course: course, Riders: p.Riders, FormDate: e.Start}
		return p, nil
	}
	sr, err := db.GetStageRace(e.RefID)
	if err != nil {
		return nil, err
	}
	stages, infos, err := service.BuildStageConfigs(db, sr, false)
	if err != nil {
		return nil, err
	}
	for i := range stages {
		stages[i].SeasonalForm = true
		stages[i].Race.Date = inYear(stages[i].Race.Date, s.Year)
	}
	p.Stages = &service.StageRacePlan{StageRace: sr, Stages: stages, StageInfos: infos, Riders: p.Riders}
	return p, nil
}

func idSet(ids []int64) map[int64]bool {
	m := make(map[int64]bool, len(ids))
	for _, id := range ids {
		m[id] = true
	}
	return m
}

// busyRiders: riders already racing on the event's days (in an overlapping
// event raced before it).
func busyRiders(db *database.DB, s *model.Season, e *model.SeasonEvent) (map[int64]bool, error) {
	events, err := db.ListSeasonEvents(s.ID, s.Year)
	if err != nil {
		return nil, err
	}
	entries, err := db.ListSeasonEntries(s.ID)
	if err != nil {
		return nil, err
	}
	busy := map[int64]bool{}
	for _, ev := range events {
		if ev.ID == e.ID || !ev.Done || ev.End.Before(e.Start) || ev.Start.After(e.End) {
			continue
		}
		for _, id := range entries[ev.ID] {
			busy[id] = true
		}
	}
	return busy, nil
}

// SetPicks saves the player's squad for an event as the plan.
func SetPicks(db *database.DB, s *model.Season, eventID int64, picks []int64) error {
	entries, err := db.ListSeasonEntries(s.ID)
	if err != nil {
		return err
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return err
	}
	team := map[int64]bool{}
	for _, sr := range srs {
		team[sr.ID] = sr.Team == s.Team
	}
	var ids []int64
	for _, id := range entries[eventID] {
		if !team[id] {
			ids = append(ids, id)
		}
	}
	for _, id := range picks {
		if team[id] {
			ids = append(ids, id)
		}
	}
	return db.SaveEventEntries(s.ID, eventID, ids)
}

// AutoPicks is the player's squad for an event as planned (filled up by the
// team's choice where riders are missing).
func AutoPicks(db *database.DB, s *model.Season, eventID int64) ([]int64, error) {
	p, err := Prepare(db, s, eventID, nil)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, r := range p.Riders {
		if r.Team == s.Team {
			ids = append(ids, r.ID)
		}
	}
	return ids, nil
}

// Simulate races a prepared event without the player (seed 0: random) and
// records it.
func Simulate(db *database.DB, p *Prepared, seed int64) error {
	if p.Stages != nil {
		out := service.RunStageRace(p.Stages, seed, nil, nil)
		return RecordStageRace(db, p, out)
	}
	cfg := p.OneDay
	cfg.Seed = seed
	return RecordOneDay(db, p, service.RunRace(cfg, nil))
}

// RecordOneDay saves a one-day race's result, its points, and the riders'
// racing load, and moves the season on to the race's date.
func RecordOneDay(db *database.DB, p *Prepared, results []model.RaceResult) error {
	teams := teamsOf(p.Riders)
	var out []model.SeasonResult
	for _, r := range results {
		out = append(out, model.SeasonResult{
			SeasonID: p.Season.ID, EventID: p.Event.ID, Year: p.Season.Year, RiderID: r.RiderID, Team: teams[r.RiderID],
			Kind: model.ResultRace, Position: r.Position, GapS: r.TimeGapSeconds,
			Points: Points(p.Event.Category, model.ResultRace, r.Position),
		})
	}
	return finish(db, p, out)
}

// RecordStageRace saves every stage result and the final GC with their points.
func RecordStageRace(db *database.DB, p *Prepared, out service.StageRaceOutput) error {
	teams := teamsOf(p.Riders)
	var rs []model.SeasonResult
	for i, stage := range out.Results {
		for _, r := range stage {
			rs = append(rs, model.SeasonResult{
				SeasonID: p.Season.ID, EventID: p.Event.ID, Year: p.Season.Year, RiderID: r.RiderID, Team: teams[r.RiderID],
				Kind: model.ResultStage, Stage: i + 1, Position: r.Position, GapS: r.TimeGapSeconds,
				Points: Points(p.Event.Category, model.ResultStage, r.Position),
			})
		}
	}
	if n := len(out.GC); n > 0 {
		gc := append([]model.GCStanding(nil), out.GC[n-1]...)
		sort.SliceStable(gc, func(i, j int) bool { return gc[i].GCPosition < gc[j].GCPosition })
		for _, g := range gc {
			rs = append(rs, model.SeasonResult{
				SeasonID: p.Season.ID, EventID: p.Event.ID, Year: p.Season.Year, RiderID: g.RiderID, Team: teams[g.RiderID],
				Kind: model.ResultGC, Position: g.GCPosition, GapS: g.TotalTimeSeconds - gc[0].TotalTimeSeconds,
				Points: Points(p.Event.Category, model.ResultGC, g.GCPosition),
			})
		}
	}
	return finish(db, p, rs)
}

func teamsOf(riders []model.Rider) map[int64]string {
	m := make(map[int64]string, len(riders))
	for _, r := range riders {
		m[r.ID] = r.Team
	}
	return m
}

// finish saves the results, marks the event raced, decays every rider's load
// to the event's end and adds the race days to those who rode it.
func finish(db *database.DB, p *Prepared, results []model.SeasonResult) error {
	if err := db.SaveSeasonResults(p.Event.ID, results); err != nil {
		return err
	}
	var starters []int64
	for _, r := range p.Riders {
		starters = append(starters, r.ID)
	}
	if err := db.SaveEventEntries(p.Season.ID, p.Event.ID, starters); err != nil {
		return err
	}
	srs, err := db.ListSeasonRiders(p.Season.ID)
	if err != nil {
		return err
	}
	rode := map[int64]bool{}
	for _, r := range p.Riders {
		rode[r.ID] = true
	}
	end := p.Event.End
	if end.Before(p.Season.Date) {
		end = p.Season.Date
	}
	days := end.Sub(p.Season.Date).Hours() / 24
	for i := range srs {
		srs[i].Load *= math.Exp(-days / LoadDecayDays)
		if rode[srs[i].ID] {
			srs[i].Load += float64(p.Event.Days())
			srs[i].RaceDays += p.Event.Days()
		}
	}
	applyMorale(srs, p, results)
	if err := db.SaveSeasonRiders(p.Season.ID, srs); err != nil {
		return err
	}
	p.Event.Done = true
	if err := db.SaveSeasonEvents([]model.SeasonEvent{*p.Event}); err != nil {
		return err
	}
	p.Season.Date = end
	return db.UpdateSeason(p.Season)
}

// applyMorale: results lift a rider, being left at home when he's fresh and
// the race suits him dents it a little; everything drifts back to neutral.
func applyMorale(srs []model.SeasonRider, p *Prepared, results []model.SeasonResult) {
	points := map[int64]int{}
	for _, r := range results {
		points[r.RiderID] += r.Points
	}
	rode := map[int64]bool{}
	for _, r := range p.Riders {
		rode[r.ID] = true
	}
	for i := range srs {
		sr := &srs[i]
		sr.Morale += (MoraleNeutral - sr.Morale) * 0.1
		if pts := points[sr.ID]; pts > 0 {
			sr.Morale += math.Min(15, 2*math.Sqrt(float64(pts)/10))
		}
		if rode[sr.ID] && p.Event.Category == model.CategoryGrandTour {
			sr.Morale += 2 // picked for a Grand Tour
		}
		sr.Morale = math.Max(0, math.Min(100, sr.Morale))
	}
}
