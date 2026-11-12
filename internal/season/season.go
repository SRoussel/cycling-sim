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
	"strings"
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
	s := &model.Season{Name: name, Team: team, Year: FirstYear, StartYear: FirstYear, Date: time.Date(FirstYear, 1, 1, 0, 0, 0, 0, time.UTC)}
	if err := db.CreateSeason(s); err != nil {
		return nil, err
	}
	values := Values(riders)
	srs := make([]model.SeasonRider, 0, len(riders))
	for _, r := range riders {
		srs = append(srs, model.SeasonRider{
			Rider:       r,
			ContractEnd: FirstYear + contractYears(r, values[r.ID]) - 1,
			Salary:      SalaryFor(values[r.ID], r.Age),
			Morale:      MoraleNeutral,
		})
	}
	if err := db.SaveSeasonRiders(s.ID, srs); err != nil {
		return nil, err
	}
	if err := buildCalendar(db, s); err != nil {
		return nil, err
	}
	if err := planYear(db, s); err != nil {
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

// inYear moves a seeded date into the season's year (same day and month).
func inYear(t time.Time, year int) time.Time {
	return time.Date(year, t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// buildCalendar lists the seeded one-day road races and stage races (not the
// reference editions or stand-alone time trials) for the season's year.
func buildCalendar(db *database.DB, s *model.Season) error {
	var events []model.SeasonEvent
	oneDay, err := service.OneDayRaces(db)
	if err != nil {
		return err
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
		return err
	}
	for _, sr := range srs {
		if referenceEdition(sr.Name) {
			continue
		}
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
		cat := model.CategoryWeekLong
		if isOneOf(sr.Name, grandTours) {
			cat = model.CategoryGrandTour
		}
		events = append(events, model.SeasonEvent{SeasonID: s.ID, Year: s.Year, Kind: model.EventStageRace, RefID: sr.ID, Name: sr.Name, Category: cat, Start: inYear(first, s.Year), End: inYear(last, s.Year)})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Start.Before(events[j].Start) })
	return db.SaveSeasonEvents(events)
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

// Values rates every rider: his best specialty (climbing W/kg, flat power
// against drag, sprint power) as a z-score in the field, so a rider is worth
// what he is best at relative to his peers.
func Values(riders []model.Rider) map[int64]float64 {
	type metric func(r model.Rider) float64
	metrics := []metric{
		func(r model.Rider) float64 { // climbing: W/kg with the climbing ability
			return float64(r.FTPWatts) / r.WeightKg * (1 + float64(r.ClimbingAbility-70)*0.002)
		},
		func(r model.Rider) float64 { // flat/TT: watts per drag area
			return float64(r.FTPWatts) / r.CdA * (1 + float64(r.TTAbility-70)*0.002)
		},
		func(r model.Rider) float64 { // sprint: peak watts with the sprint ability
			return float64(r.MaxSprintWatts) * (1 + float64(r.SprintAbility-70)*0.002)
		},
		func(r model.Rider) float64 { // classics: threshold watts with handling (cobbles)
			return float64(r.FTPWatts) * (1 + float64(r.Handling-70)*0.002) * (1 + float64(r.Endurance-70)*0.002)
		},
	}
	out := make(map[int64]float64, len(riders))
	for i, m := range metrics {
		var sum, sq float64
		for _, r := range riders {
			v := m(r)
			sum += v
			sq += v * v
		}
		n := float64(len(riders))
		mean := sum / n
		sd := math.Sqrt(math.Max(sq/n-mean*mean, 1e-9))
		for _, r := range riders {
			z := (m(r) - mean) / sd
			if i == 0 || z > out[r.ID] {
				out[r.ID] = z
			}
		}
	}
	return out
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
