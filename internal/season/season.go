// Package season is the season mode: a save game in which the player runs
// one team through a year of racing (and then the next). A season holds its
// own copy of the riders (model.SeasonRider), a calendar of the seeded races,
// every result and the points they earned. Races are prepared here with the
// season's riders (their racing load and morale applied) and recorded back;
// the CLI simulates them, the GUI can also direct them live.
package season

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/service"
)

// FirstYear is the year a new season starts in.
const FirstYear = 2026

// New starts a season with the player's team: every rider is copied into the
// season with a contract and salary, and the year's calendar is drawn up.
func New(db *database.DB, team, name string) (*model.Season, error) {
	riders, err := db.ListRiders()
	if err != nil {
		return nil, err
	}
	found := false
	for _, r := range riders {
		found = found || r.Team == team
	}
	if !found {
		return nil, fmt.Errorf("no team named %q", team)
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%s %d", team, FirstYear)
	}
	s := &model.Season{Name: name, Team: team, Year: FirstYear, StartYear: FirstYear, Phase: model.PhaseRacing,
		Date: time.Date(FirstYear, 1, 1, 0, 0, 0, 0, time.UTC)}
	if err := db.CreateSeason(s); err != nil {
		return nil, err
	}
	rng := rngFor(s, 0)
	values := Values(db, riders)
	srs := make([]model.SeasonRider, 0, len(riders))
	for _, r := range riders {
		srs = append(srs, model.SeasonRider{
			Rider:       r,
			ContractEnd: FirstYear + contractYears(r, values[r.ID]) - 1,
			Salary:      SalaryFor(values[r.ID], r.Age),
			Morale:      MoraleNeutral,
			Potential:   potential(rng),
		})
	}
	if err := db.SaveSeasonRiders(s.ID, srs); err != nil {
		return nil, err
	}
	// The budget: this year's wage bill with a little room to sign riders.
	s.Budget = int(math.Round(float64(WageBill(srs, team))*1.08/10) * 10)
	if err := db.UpdateSeason(s); err != nil {
		return nil, err
	}
	if err := buildCalendar(db, s); err != nil {
		return nil, err
	}
	if err := planYear(db, s); err != nil {
		return nil, err
	}
	if err := setObjectives(db, s); err != nil {
		return nil, err
	}
	if err := db.AddNews([]model.NewsItem{{SeasonID: s.ID, Year: s.Year, Date: s.Date, Mine: true,
		Text: fmt.Sprintf("You take charge of %s for the %d season. Budget: %s.", team, s.Year, Money(s.Budget))}}); err != nil {
		return nil, err
	}
	return s, nil
}

// contractYears: 1-3 years left at the start, drawn from the rider's ID so
// a new season is reproducible; young stars and veterans sign shorter.
func contractYears(r model.Rider, value float64) int {
	n := 1 + int(r.ID*7919%3)
	if r.Age >= 33 {
		n = 1 + int(r.ID%2)
	}
	return n
}

// Monuments and Grand Tours by name (the seeded calendar's generic races).
var (
	monuments  = []string{"Milan-San Remo", "Tour of Flanders", "Paris-Roubaix", "Liege-Bastogne-Liege", "Il Lombardia"}
	grandTours = []string{"Tour de France", "Giro d'Italia", "Vuelta a Espana"}
)

func isOneOf(name string, list []string) bool {
	for _, n := range list {
		if n == name {
			return true
		}
	}
	return false
}

// referenceEdition: races seeded for validation against a real edition
// ("Strade Bianche 2024") aren't on a season's calendar.
func referenceEdition(name string) bool {
	f := strings.Fields(name)
	if len(f) == 0 {
		return false
	}
	last := f[len(f)-1]
	return len(last) == 4 && strings.HasPrefix(last, "20")
}

// editionYear is the year a dated edition's name ends with (0: generic).
func editionYear(name string) int {
	if !referenceEdition(name) {
		return 0
	}
	f := strings.Fields(name)
	y, _ := strconv.Atoi(f[len(f)-1])
	return y
}

// baseName is a race's name without an edition year.
func baseName(name string) string {
	if !referenceEdition(name) {
		return name
	}
	f := strings.Fields(name)
	return strings.Join(f[:len(f)-1], " ")
}

// inYear moves a seeded date into the season's year (same day and month).
func inYear(t time.Time, year int) time.Time {
	return time.Date(year, t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// buildCalendar saves the season's calendar for its year.
func buildCalendar(db *database.DB, s *model.Season) error {
	events, err := calendarEvents(db, s)
	if err != nil {
		return err
	}
	return db.SaveSeasonEvents(events)
}

// calendarEvents lists the seeded one-day road races and stage races (not the
// reference editions or stand-alone time trials) in the season's year.
func calendarEvents(db *database.DB, s *model.Season) ([]model.SeasonEvent, error) {
	var events []model.SeasonEvent
	oneDay, err := service.OneDayRaces(db)
	if err != nil {
		return nil, err
	}
	for _, r := range oneDay {
		if referenceEdition(r.Name) || r.RaceType != model.RaceTypeRoad {
			continue
		}
		cat := model.CategoryClassic
		if isOneOf(r.Name, monuments) {
			cat = model.CategoryMonument
		}
		d := inYear(r.Date, s.Year)
		events = append(events, model.SeasonEvent{SeasonID: s.ID, Year: s.Year, Kind: model.EventOneDay, RefID: r.ID, Name: r.Name, Category: cat, Start: d, End: d})
	}
	srs, err := db.ListStageRaces()
	if err != nil {
		return nil, err
	}
	// Stage races ride their newest real route: the dated editions seeded
	// from real stage files ("Giro d'Italia 2026", "Volta a Catalunya
	// 2024") over the generic stages, under the race's own name. Nov 2026
	// playtest: the season's Basque Country had stages named "Rolling" on
	// straight-ramp profiles while the real routes sat unused.
	newest := map[string]model.StageRace{}
	for _, sr := range srs {
		base := baseName(sr.Name)
		cur, ok := newest[base]
		if !ok || editionYear(sr.Name) > editionYear(cur.Name) {
			newest[base] = sr
		}
	}
	srs = srs[:0]
	for _, sr := range newest {
		srs = append(srs, sr)
	}
	sort.Slice(srs, func(i, j int) bool { return srs[i].ID < srs[j].ID })
	for _, sr := range srs {
		full, err := db.GetStageRace(sr.ID)
		if err != nil || len(full.Stages) == 0 {
			continue
		}
		var first, last time.Time
		for _, st := range full.Stages {
			race, err := db.GetRace(st.RaceID)
			if err != nil {
				continue
			}
			if first.IsZero() || race.Date.Before(first) {
				first = race.Date
			}
			if race.Date.After(last) {
				last = race.Date
			}
		}
		name := baseName(sr.Name)
		cat := model.CategoryWeekLong
		if isOneOf(name, grandTours) {
			cat = model.CategoryGrandTour
		}
		events = append(events, model.SeasonEvent{SeasonID: s.ID, Year: s.Year, Kind: model.EventStageRace, RefID: sr.ID, Name: name, Category: cat, Start: inYear(first, s.Year), End: inYear(last, s.Year)})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Start.Before(events[j].Start) })
	return events, nil
}

// NextEvent is the first event of the year not yet raced (nil: the year is over).
func NextEvent(db *database.DB, s *model.Season) (*model.SeasonEvent, error) {
	events, err := db.ListSeasonEvents(s.ID, s.Year)
	if err != nil {
		return nil, err
	}
	for i := range events {
		if !events[i].Done {
			return &events[i], nil
		}
	}
	return nil, nil
}

// Event finds one event of a season.
func Event(db *database.DB, s *model.Season, eventID int64) (*model.SeasonEvent, error) {
	events, err := db.ListSeasonEvents(s.ID, s.Year)
	if err != nil {
		return nil, err
	}
	for i := range events {
		if events[i].ID == eventID {
			return &events[i], nil
		}
	}
	return nil, fmt.Errorf("event %d is not on the %d calendar", eventID, s.Year)
}

// MoraleNeutral is a rider's morale with nothing to be happy or sad about.
const MoraleNeutral = 60.0

// Values rates every rider by how well he suits the races on the calendar:
// the mean of his three best fits (standard deviations above the field, see
// fits), the bigger races counting a little more. A Grand Tour contender, a
// cobbles star and a top sprinter all rate high; a domestique near zero.
func Values(db *database.DB, riders []model.Rider) map[int64]float64 {
	out := make(map[int64]float64, len(riders))
	if len(riders) == 0 {
		return out
	}
	profs, err := calendarProfiles(db)
	if err != nil || len(profs) == 0 {
		return out
	}
	best := make(map[int64][]float64, len(riders))
	for _, ep := range profs {
		weight := 0.7 + 0.3*Prestige(ep.event)/100
		for id, f := range fits(ep.profiles, riders) {
			best[id] = append(best[id], f*weight)
		}
	}
	for id, fs := range best {
		sort.Sort(sort.Reverse(sort.Float64Slice(fs)))
		n := min(3, len(fs))
		sum := 0.0
		for _, f := range fs[:n] {
			sum += f
		}
		out[id] = sum / float64(n)
	}
	return out
}

type eventProfile struct {
	event    model.SeasonEvent
	profiles *eventCourses
}

var (
	profileMu    sync.Mutex
	profileCache = map[*database.DB][]eventProfile{}
)

// calendarProfiles: the course profiles of every race a season's calendar
// holds (read once per database).
func calendarProfiles(db *database.DB) ([]eventProfile, error) {
	profileMu.Lock()
	defer profileMu.Unlock()
	if p, ok := profileCache[db]; ok {
		return p, nil
	}
	tmp := &model.Season{Year: FirstYear}
	events, err := calendarEvents(db, tmp)
	if err != nil {
		return nil, err
	}
	var out []eventProfile
	for _, e := range events {
		ps, err := eventProfiles(db, e)
		if err != nil {
			return nil, err
		}
		out = append(out, eventProfile{e, ps})
	}
	profileCache[db] = out
	return out, nil
}

// SalaryFor is a rider's yearly salary (thousand euro) for his value: ~250k
// for an average pro, millions for the stars; youngsters earn less.
func SalaryFor(value float64, age int) int {
	s := 250 * math.Exp(1.1*value)
	if age <= 23 {
		s *= 0.6
	}
	return int(math.Round(math.Min(math.Max(s, 120), 8000)/10) * 10)
}
