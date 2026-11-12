package season

import (
	"sort"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/simulation"
)

// SquadOption is one of the player's riders as a choice for an event.
type SquadOption struct {
	model.SeasonRider
	Fit       float64 // standard deviations above the field on this event
	TeamRank  int     // 1 = the team's best fit for it
	Load      float64 // racing load at the start
	LoadPct   float64 // threshold kept with that load (100 = fresh)
	FormPct   float64 // seasonal form at the start (100 = at his peak)
	Planned   bool    // in the team's plan (the squad as it stands)
	Available bool
	Why       string // why he isn't available
}

// Squad lists the player's riders for an event: how each suits it, his load
// and form at the start, whether he's planned and whether he can ride.
func Squad(db *database.DB, s *model.Season, eventID int64) ([]SquadOption, error) {
	e, err := Event(db, s, eventID)
	if err != nil {
		return nil, err
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, err
	}
	entries, err := db.ListSeasonEntries(s.ID)
	if err != nil {
		return nil, err
	}
	busy, err := busyRiders(db, s, e)
	if err != nil {
		return nil, err
	}
	planned := idSet(entries[e.ID])
	profiles, err := eventProfiles(db, *e)
	if err != nil {
		return nil, err
	}
	fit := fits(profiles, activeRiders(srs))
	days := e.Start.Sub(s.Date).Hours() / 24
	var out []SquadOption
	for _, sr := range srs {
		if sr.Team != s.Team || sr.Retired {
			continue
		}
		load := loadAt(sr, days)
		o := SquadOption{
			SeasonRider: sr, Fit: fit[sr.ID], Load: load, LoadPct: 100 * LoadFactor(load),
			FormPct: 100 * simulation.SeasonForm(&sr.Rider, e.Start), Planned: planned[sr.ID], Available: true,
		}
		if busy[sr.ID] {
			o.Available, o.Why = false, "racing elsewhere"
		}
		out = append(out, o)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Fit > out[j].Fit })
	for i := range out {
		out[i].TeamRank = i + 1
	}
	return out, nil
}
