package director

import (
	"fmt"
	"sort"

	"cycling_sim/internal/simulation"
)

// Goal is what the team is expected to achieve today, set from its riders'
// strength in this field: a team with a favourite rides to win, a mid-level
// team for a place, an outsider for the break.
type Goal struct {
	Kind      string `json:"kind"` // place (finish in the top Target) or break (a rider in the day's break)
	RiderID   int64  `json:"riderId"`
	RiderName string `json:"riderName"`
	Target    int    `json:"target"` // place: finish position to reach
	Text      string `json:"text"`
}

// The break goal: a rider of ours this far clear of the main field.
const breakGoalGapS = 60

// chooseGoal sets the goal from the first view of the race. On courses
// without decisive climbs or sectors (a sprint) the team's sprinter is
// judged by sprint rating against the field's sprinters; otherwise its best
// rider by time over the course (the favourites' measure).
func (d *Director) chooseGoal(w *view) *Goal {
	type cand struct {
		id   int64
		rank int
	}
	var best *cand
	sprintCourse := len(d.decisive) == 0 && len(d.decisiveSectors) == 0
	if sprintCourse {
		var sprints []int64
		for id, r := range w.Riders {
			if r.Role == simulation.RoleSprinter || r.Team == w.team {
				sprints = append(sprints, id)
			}
		}
		// Rank every team's sprinter and our riders by sprint rating.
		sort.Slice(sprints, func(i, j int) bool { return w.Riders[sprints[i]].Sprint > w.Riders[sprints[j]].Sprint })
		for i, id := range sprints {
			if w.mine(id) {
				best = &cand{id, i + 1}
				break
			}
		}
	} else {
		// The team's leader (the engine picks him by time over the hard
		// sections, a better one-day measure than time over the whole
		// course, which favours time triallists on flat routes); failing a
		// leader, our best rider by course time.
		for _, leaderOnly := range []bool{true, false} {
			for id, r := range w.Riders {
				if r.Team != w.team || r.Rank == 0 || (leaderOnly && r.Role != simulation.RoleLeader) {
					continue
				}
				if best == nil || r.Rank < best.rank {
					best = &cand{id, r.Rank}
				}
			}
			if best != nil {
				break
			}
		}
	}
	if best == nil {
		return nil
	}
	name := w.name(best.id)
	place := func(target int, text string) *Goal {
		return &Goal{Kind: "place", RiderID: best.id, RiderName: name, Target: target, Text: text}
	}
	switch {
	case best.rank <= 2:
		return place(1, fmt.Sprintf("Win with %s", name))
	case best.rank <= 5:
		return place(3, fmt.Sprintf("Podium with %s", name))
	case best.rank <= 10:
		return place(5, fmt.Sprintf("Top 5 with %s", name))
	case best.rank <= 20:
		return place(10, fmt.Sprintf("Top 10 with %s", name))
	}
	return &Goal{Kind: "break", Text: "Get a rider in the break"}
}

// Goal returns today's goal (nil until the race has started).
func (d *Director) Goal() *Goal { return d.goal }

// BreakMade reports whether a rider of ours was in a break during the race.
func (d *Director) BreakMade() bool { return d.breakMade }

// Watch follows the race for the goal (set once, the break tracked) without
// asking for decisions: the goal counts whatever the decision level.
func (d *Director) Watch(v *simulation.RaceView) {
	if v == nil || len(v.Road) == 0 {
		return
	}
	if d.totalKm == 0 {
		d.totalKm = v.KmTotal
	}
	d.observeGoal(newView(v, d.Team))
}

// observeGoal sets the goal once and tracks the break goal.
func (d *Director) observeGoal(w *view) {
	if d.goal == nil && !d.goalTried {
		d.goalTried = true
		d.goal = d.chooseGoal(w)
	}
	if d.breakMade || w.main == 0 || w.gap(w.main) < breakGoalGapS {
		return
	}
	for gi := 0; gi < w.main; gi++ {
		if len(w.myRiders(gi)) > 0 {
			d.breakMade = true
		}
	}
}
