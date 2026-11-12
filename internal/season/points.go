package season

import (
	"sort"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
)

// Points tables (simplified from the UCI's): top places of a Grand Tour GC,
// a Monument, other one-day races and week-long GCs; stage places.
var (
	gtGC      = []int{1300, 1040, 880, 720, 640, 560, 480, 400, 320, 280, 240, 200, 160, 140, 120}
	monument  = []int{800, 640, 520, 440, 360, 320, 280, 240, 200, 160, 140, 120, 100, 80, 60}
	classic   = []int{500, 400, 325, 275, 225, 175, 150, 125, 100, 85, 70, 60, 50, 40, 35}
	weekGC    = []int{500, 400, 325, 275, 225, 175, 150, 125, 100, 85, 70, 60, 50, 40, 35}
	gtStage   = []int{210, 150, 110, 90, 60, 50, 40, 30, 20, 10}
	weekStage = []int{60, 25, 10}
)

// Points a place earns in an event of a category.
func Points(category, kind string, position int) int {
	var table []int
	switch kind {
	case model.ResultStage:
		table = weekStage
		if category == model.CategoryGrandTour {
			table = gtStage
		}
	case model.ResultGC:
		table = weekGC
		if category == model.CategoryGrandTour {
			table = gtGC
		}
	default:
		table = classic
		if category == model.CategoryMonument {
			table = monument
		}
	}
	if position < 1 || position > len(table) {
		return 0
	}
	return table[position-1]
}

// Standing is a rider's or a team's points total for a year.
type Standing struct {
	RiderID int64
	Team    string
	Points  int
	Wins    int
}

// Standings totals a year's points per rider and per team, best first.
func Standings(db *database.DB, s *model.Season, year int) (riders, teams []Standing, err error) {
	results, err := db.ListSeasonResults(s.ID, year, 0)
	if err != nil {
		return nil, nil, err
	}
	byRider := map[int64]*Standing{}
	byTeam := map[string]*Standing{}
	for _, r := range results {
		if r.Points == 0 && r.Position != 1 {
			continue
		}
		rs := byRider[r.RiderID]
		if rs == nil {
			rs = &Standing{RiderID: r.RiderID, Team: r.Team}
			byRider[r.RiderID] = rs
		}
		ts := byTeam[r.Team]
		if ts == nil {
			ts = &Standing{Team: r.Team}
			byTeam[r.Team] = ts
		}
		rs.Points += r.Points
		ts.Points += r.Points
		if r.Position == 1 {
			rs.Wins++
			ts.Wins++
		}
	}
	for _, v := range byRider {
		riders = append(riders, *v)
	}
	for _, v := range byTeam {
		teams = append(teams, *v)
	}
	less := func(a, b Standing) bool {
		if a.Points != b.Points {
			return a.Points > b.Points
		}
		return a.Wins > b.Wins
	}
	sort.SliceStable(riders, func(i, j int) bool { return less(riders[i], riders[j]) })
	sort.SliceStable(teams, func(i, j int) bool { return less(teams[i], teams[j]) })
	return riders, teams, nil
}
