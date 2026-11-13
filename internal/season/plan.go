package season

import (
	"fmt"
	"math"
	"sort"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
	"cycling_sim/internal/service"
)

// Prestige ranks events for the teams' planning: the biggest races get their
// squads first, so a star's race days go to his targets.
func Prestige(e model.SeasonEvent) float64 {
	switch {
	case e.Name == "Tour de France":
		return 100
	case e.Category == model.CategoryGrandTour && e.Name == "Giro d'Italia":
		return 85
	case e.Category == model.CategoryGrandTour:
		return 75
	case e.Category == model.CategoryMonument:
		return 65
	case e.Category == model.CategoryWeekLong:
		return 45
	}
	return 35
}

// schedule is one rider's planned events.
type schedule []model.SeasonEvent

// loadAtStart is the racing load a schedule gives the rider at a date (race
// days of events finished before it, decayed; an event in progress counts
// its days so far).
func (sc schedule) loadAtStart(startUnix int64) float64 {
	load := 0.0
	for _, e := range sc {
		if e.Start.Unix() >= startUnix {
			continue
		}
		end := e.End.Unix()
		days := float64(e.Days())
		if end >= startUnix {
			days = float64(startUnix-e.Start.Unix()) / 86400
			end = startUnix
		}
		load += days * math.Exp(-float64(startUnix-end)/86400/LoadDecayDays)
	}
	return load
}

// fits: whether adding an event keeps a rider's schedule realistic: not
// overlapping another event, rested at the start of every event (load at
// most RestLoad), at most MaxGrandTours and within his race-day budget.
func (sc schedule) fits(e model.SeasonEvent, already, budget int) bool {
	days, gts := e.Days()+already, 0
	if e.Category == model.CategoryGrandTour {
		gts++
	}
	for _, x := range sc {
		if !x.End.Before(e.Start) && !x.Start.After(e.End) {
			return false
		}
		days += x.Days()
		if x.Category == model.CategoryGrandTour {
			gts++
		}
	}
	if days > budget || gts > MaxGrandTours {
		return false
	}
	with := append(append(schedule{}, sc...), e)
	for _, x := range with {
		if with.loadAtStart(x.Start.Unix()) > RestLoad {
			return false
		}
	}
	return true
}

// Planning a year (planYear): riders first pick their targets, then teams
// fill their squads around them.
const (
	// wantFrom: a rider targets an event once he is this many standard
	// deviations above the field on it; the want grows to full over wantSpan.
	wantFrom = 1.0
	wantSpan = 1.5
	// leaderDays: the best riders race this many fewer days than the budget
	// (real stars ~50, domestiques ~70-80).
	leaderDays = 22.0
	// leaderValue: riders this good (Values) race their own targets only;
	// teams fill squads around them with the others.
	leaderValue = 1.5
	// targetShare: a rider targets only events he suits at least this share
	// as well as his best (Van der Poel targets the cobbles and San Remo,
	// not the Ardennes); between targets he trains. A share, not a band:
	// an outlier like Pogačar stands out on many kinds of race.
	targetShare = 0.65
	// leadersPerEvent: a team takes at most this many leaders to one race
	// and spreads the others over its other targets (an Almeida rides the
	// Giro or the Vuelta for himself, not the Tour for Pogačar).
	leadersPerEvent = 2
	// secondGrandTour: a rider's second Grand Tour is worth this share of
	// its prestige to him (most leaders target one); the Vuelta after the
	// Tour (the usual double, a second chance at the end of the summer)
	// keeps vueltaDouble of it.
	secondGrandTour = 0.5
	vueltaDouble    = 0.6
)

// riderBudget is a rider's race days for the year: fewer for the best riders,
// who race their targets and rest between them.
func riderBudget(value float64) int {
	return RaceDayBudget - int(leaderDays*math.Max(0, math.Min(1, value/3)))
}

// planYear plans every rider's programme for the year's events not yet
// raced. Riders first take their targets: (rider, event) pairs in order of
// prestige × how far the rider stands out on it, while his schedule allows
// (no overlaps, rested, Grand Tours, race days) and his team has a place.
// Then each event, biggest first, gets its full field: the teams already in
// it fill their squads and the teams that suit it best take the other places
// (the player's team always rides). Saved as the season's entries.
func planYear(db *database.DB, s *model.Season) error {
	events, err := db.ListSeasonEvents(s.ID, s.Year)
	if err != nil {
		return err
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return err
	}
	var riders []model.Rider
	raced := map[int64]int{}
	for _, sr := range srs {
		if !sr.Retired && sr.Team != "" {
			riders = append(riders, sr.Rider)
			raced[sr.ID] = sr.RaceDays
		}
	}
	values := Values(db, riders)
	var todo []model.SeasonEvent
	for _, e := range events {
		if !e.Done {
			todo = append(todo, e)
		}
	}
	sort.SliceStable(todo, func(i, j int) bool { return Prestige(todo[i]) > Prestige(todo[j]) })

	plans := map[int64]schedule{}
	entries := map[int64][]int64{}
	squads := map[int64]map[string]int{} // event -> team -> riders
	free := func(r model.Rider, e model.SeasonEvent) bool {
		return plans[r.ID].fits(e, raced[r.ID], riderBudget(values[r.ID]))
	}
	add := func(r model.Rider, e model.SeasonEvent) {
		plans[r.ID] = append(plans[r.ID], e)
		entries[e.ID] = append(entries[e.ID], r.ID)
		if squads[e.ID] == nil {
			squads[e.ID] = map[string]int{}
		}
		squads[e.ID][r.Team]++
	}
	aiTeams := func(e model.SeasonEvent) int {
		n := selection.DefaultConfig(model.RaceTypeRoad, e.Kind == model.EventStageRace, e.Days()).NumTeams - 1
		for team := range squads[e.ID] {
			if team != s.Team {
				n--
			}
		}
		return n
	}

	// 1. Targets.
	type pair struct {
		r    model.Rider
		e    model.SeasonEvent
		want float64
	}
	fitOf := map[int64]map[int64]float64{} // event -> rider -> fit
	best := map[int64]float64{}
	for _, e := range todo {
		profiles, err := eventProfiles(db, e)
		if err != nil {
			return err
		}
		fitOf[e.ID] = fits(profiles, riders)
		for id, f := range fitOf[e.ID] {
			best[id] = math.Max(best[id], f)
		}
	}
	var pairs []pair
	for _, e := range todo {
		for id, f := range fitOf[e.ID] {
			if f < best[id]*targetShare {
				continue
			}
			if w := math.Min(1, (f-wantFrom)/wantSpan); w > 0 {
				pairs = append(pairs, pair{e: e, want: Prestige(e) * w, r: riderByID(riders, id)})
			}
		}
	}
	// A rider's second Grand Tour is worth less to him: those pairs go back
	// in the queue at their reduced want once he has one.
	gts := map[int64]int{}
	leaders := map[string]int{} // team + event -> leaders in it
	for len(pairs) > 0 {
		sort.SliceStable(pairs, func(i, j int) bool {
			if pairs[i].want != pairs[j].want {
				return pairs[i].want > pairs[j].want
			}
			return pairs[i].r.ID < pairs[j].r.ID
		})
		p := pairs[0]
		pairs = pairs[1:]
		in := squads[p.e.ID][p.r.Team]
		if in >= SquadSize(&p.e) || !free(p.r, p.e) {
			continue
		}
		if in == 0 && p.r.Team != s.Team && aiTeams(p.e) <= 0 {
			continue
		}
		if values[p.r.ID] >= leaderValue {
			key := p.r.Team + "\x00" + fmt.Sprint(p.e.ID)
			if leaders[key] >= leadersPerEvent {
				continue
			}
			leaders[key]++
		}
		add(p.r, p.e)
		if p.e.Category != model.CategoryGrandTour {
			continue
		}
		if gts[p.r.ID]++; gts[p.r.ID] == 1 {
			for i := range pairs {
				if pairs[i].r.ID == p.r.ID && pairs[i].e.Category == model.CategoryGrandTour {
					if pairs[i].e.Name == "Vuelta a Espana" || p.e.Name == "Vuelta a Espana" {
						pairs[i].want *= vueltaDouble
					} else {
						pairs[i].want *= secondGrandTour
					}
				}
			}
		}
	}

	// 2. Full fields.
	for _, e := range todo {
		squad := SquadSize(&e)
		// Leaders ride their targets; squads fill with the others, and with
		// leaders who got few targets (a team's third classics leader rides
		// as a helper rather than not at all).
		byTeam := map[string][]model.Rider{}
		for _, r := range riders {
			if free(r, e) && (values[r.ID] < leaderValue || plannedDays(plans[r.ID]) < leaderFillDays) {
				byTeam[r.Team] = append(byTeam[r.Team], r)
			}
		}
		fill := func(team string) error {
			need := squad - squads[e.ID][team]
			if need <= 0 || len(byTeam[team]) == 0 {
				return nil
			}
			picked, err := pickField(db, e, byTeam[team], 1, need)
			if err != nil {
				return err
			}
			for _, r := range picked {
				add(r, e)
			}
			return nil
		}
		teams := []string{s.Team}
		for team := range squads[e.ID] {
			if team != s.Team {
				teams = append(teams, team)
			}
		}
		sort.Strings(teams[1:])
		for _, team := range teams {
			if err := fill(team); err != nil {
				return err
			}
		}
		if n := aiTeams(e); n > 0 {
			var pool []model.Rider
			for team, rs := range byTeam {
				if _, in := squads[e.ID][team]; !in && team != s.Team && len(rs) >= 3 {
					pool = append(pool, rs...)
				}
			}
			sort.SliceStable(pool, func(i, j int) bool { return pool[i].ID < pool[j].ID })
			picked, err := pickField(db, e, pool, n, 1)
			if err != nil {
				return err
			}
			for _, r := range picked {
				if err := fill(r.Team); err != nil {
					return err
				}
			}
		}
	}
	for _, e := range todo {
		if err := db.SaveEventEntries(s.ID, e.ID, entries[e.ID]); err != nil {
			return err
		}
	}
	return nil
}

// leaderFillDays: a leader with fewer race days planned from his targets
// also fills squads.
const leaderFillDays = 25

func plannedDays(sc schedule) int {
	n := 0
	for _, e := range sc {
		n += e.Days()
	}
	return n
}

func riderByID(riders []model.Rider, id int64) model.Rider {
	i := sort.Search(len(riders), func(i int) bool { return riders[i].ID >= id })
	return riders[i]
}

// pickField is the selector's field for an event from a pool of riders:
// numTeams teams (0: the usual number, -1: one fewer, leaving the player's
// team its place) of the event's squad size.
func pickField(db *database.DB, e model.SeasonEvent, pool []model.Rider, numTeams, perTeam int) ([]model.Rider, error) {
	squad := SquadSize(&e)
	if perTeam == 0 {
		perTeam = squad
	}
	if e.Kind == model.EventOneDay {
		race, err := db.GetRace(e.RefID)
		if err != nil {
			return nil, err
		}
		course, err := db.GetCourse(race.CourseID)
		if err != nil {
			return nil, err
		}
		cfg := selection.DefaultConfig(race.RaceType, false, 0)
		cfg.NumTeams = teamsFor(cfg.NumTeams, numTeams)
		cfg.RidersPerTeam = perTeam
		return selection.SelectStartList(pool, course, race, cfg), nil
	}
	sr, err := db.GetStageRace(e.RefID)
	if err != nil {
		return nil, err
	}
	_, infos, err := service.BuildStageConfigs(db, sr, false)
	if err != nil {
		return nil, err
	}
	cfg := selection.DefaultConfig(model.RaceTypeRoad, true, len(infos))
	cfg.NumTeams = teamsFor(cfg.NumTeams, numTeams)
	cfg.RidersPerTeam = perTeam
	return selection.SelectStageRaceStartList(pool, infos, cfg), nil
}

func teamsFor(usual, n int) int {
	switch {
	case n > 0:
		return n
	case n < 0:
		return usual + n
	}
	return usual
}
