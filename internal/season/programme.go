package season

import (
	"fmt"
	"math"
	"strings"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/simulation"
)

// A rider's programme for the rest of the year, as the player plans it:
// which races he rides (the events' entries, which planYear fills and the
// player can change) and which he peaks for (his seasonal form targets,
// stored as the days of the year the peaks are centred on).

// PlanEvent is one race still to come in a rider's plan.
type PlanEvent struct {
	Event   model.SeasonEvent
	Planned bool    // he's in the team's squad for it
	Peak    bool    // he peaks for it (the player's choice)
	FormPct float64 // his seasonal form there, % of his peak, with the current targets
	Squad   int     // riders the team has planned for it
	Size    int     // squad size
}

// eventDay is the day of the year a peak for e is centred on (its middle).
func eventDay(e model.SeasonEvent) float64 {
	return float64(e.Start.YearDay()) + float64(e.Days()-1)/2
}

// RiderPlan is the player's rider's races to come.
func RiderPlan(db *database.DB, s *model.Season, riderID int64) (*model.SeasonRider, []PlanEvent, error) {
	sr, err := ourRider(db, s, riderID)
	if err != nil {
		return nil, nil, err
	}
	events, err := db.ListSeasonEvents(s.ID, s.Year)
	if err != nil {
		return nil, nil, err
	}
	entries, err := db.ListSeasonEntries(s.ID)
	if err != nil {
		return nil, nil, err
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, nil, err
	}
	ours := map[int64]bool{}
	for _, x := range srs {
		ours[x.ID] = x.Team == s.Team && !x.Retired
	}
	var out []PlanEvent
	for _, e := range events {
		if e.Done {
			continue
		}
		mid := e.Start.AddDate(0, 0, (e.Days()-1)/2) // a stage race's form: at its middle
		p := PlanEvent{Event: e, Size: SquadSize(&e), FormPct: 100 * simulation.SeasonForm(&sr.Rider, mid)}
		for _, id := range entries[e.ID] {
			if ours[id] {
				p.Squad++
			}
			if id == riderID {
				p.Planned = true
			}
		}
		for _, d := range sr.Rider.Peaks() {
			if math.Abs(d-eventDay(e)) < 0.5 {
				p.Peak = true
			}
		}
		out = append(out, p)
	}
	return sr, out, nil
}

// SetRiderPlan sets which of the races to come the player's rider rides
// (planned) and which he peaks for (peaks, at most model.MaxPeaks; none:
// the peaks of his specialty). A race whose squad is full can't take him.
func SetRiderPlan(db *database.DB, s *model.Season, riderID int64, planned, peaks []int64) error {
	sr, err := ourRider(db, s, riderID)
	if err != nil {
		return err
	}
	if len(peaks) > model.MaxPeaks {
		return fmt.Errorf("a rider can peak for %d races at most", model.MaxPeaks)
	}
	events, err := db.ListSeasonEvents(s.ID, s.Year)
	if err != nil {
		return err
	}
	entries, err := db.ListSeasonEntries(s.ID)
	if err != nil {
		return err
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return err
	}
	ours := map[int64]bool{}
	names := map[int64]string{}
	for _, x := range srs {
		ours[x.ID] = x.Team == s.Team && !x.Retired
		names[x.ID] = x.Name
	}
	want, peak := map[int64]bool{}, map[int64]bool{}
	for _, id := range planned {
		want[id] = true
	}
	for _, id := range peaks {
		peak[id] = true
	}
	var days []float64
	for _, e := range events {
		if peak[e.ID] {
			days = append(days, eventDay(e))
		}
	}
	for _, e := range events {
		if e.Done {
			continue
		}
		in, squad := false, 0
		var ids []int64
		for _, id := range entries[e.ID] {
			if id == riderID {
				in = true
				continue
			}
			if ours[id] {
				squad++
			}
			ids = append(ids, id)
		}
		if want[e.ID] == in {
			continue
		}
		if want[e.ID] {
			if squad >= SquadSize(&e) {
				var who []string
				for _, id := range ids {
					if ours[id] {
						who = append(who, names[id])
					}
				}
				return fmt.Errorf("the %s squad is full (%s): take one of them out of it first", e.Name, strings.Join(who, ", "))
			}
			ids = append(ids, riderID)
		}
		if err := db.SaveEventEntries(s.ID, e.ID, ids); err != nil {
			return err
		}
	}
	sr.Rider.PeakDays = [model.MaxPeaks]float64{}
	copy(sr.Rider.PeakDays[:], days)
	return db.SaveSeasonRiders(s.ID, []model.SeasonRider{*sr})
}

// ourRider loads one of the player's riders.
func ourRider(db *database.DB, s *model.Season, riderID int64) (*model.SeasonRider, error) {
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, err
	}
	for i := range srs {
		if srs[i].ID == riderID {
			if srs[i].Team != s.Team || srs[i].Retired {
				return nil, fmt.Errorf("%s doesn't ride for %s", srs[i].Name, s.Team)
			}
			return &srs[i], nil
		}
	}
	return nil, fmt.Errorf("no rider %d", riderID)
}
