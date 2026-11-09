package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

// Race shape: how a race was won, as a check that races unfold like real ones
// (gap errors alone can't tell a bunch sprint from a breakaway win).

// Outcome is how a stage or race was won.
type Outcome string

const (
	OutcomeSprint    Outcome = "sprint"    // from a big group (15+ riders)
	OutcomeBreakaway Outcome = "breakaway" // by a rider ahead of the main group (see mainGroup) with breakCheckKm to go
	OutcomeSelection Outcome = "selection" // from a small group or solo formed out of the main field
	OutcomeTT        Outcome = "tt"
)

const (
	// breakCheckKm: a winner who was in a group ahead of the main field this
	// far from the finish won from the break (the day's break, or a move that
	// went clear mid-race).
	breakCheckKm = 30.0
	// sprintGroupSize: a winner's group at least this big at the line is a
	// sprint (bunch or reduced bunch).
	sprintGroupSize = 15
	// sameGroupS: riders finishing within this of the winner were with them.
	sameGroupS = 5.0
)

// RaceShape records what a race needs for its outcome.
type RaceShape struct {
	breakAt map[int64]bool // riders ahead of the main field at breakCheckKm
	seen    bool
}

// observe is called after each course segment with the distance left.
func (s *RaceShape) observe(state *RaceState, remainingKm float64) {
	if s == nil || s.seen || remainingKm > breakCheckKm || state == nil {
		return
	}
	s.seen = true
	s.breakAt = map[int64]bool{}
	main := mainGroup(state)
	for _, c := range state.Clusters[:main] {
		for _, rs := range c {
			s.breakAt[rs.Rider.ID] = true
		}
	}
}

// mainGroup is the road group the race revolves around: the race leader's in
// a stage race, the one with most favourites in a one-day race (not simply
// the biggest: on a mountain stage the gruppetto can be bigger than the
// favourites' group).
func mainGroup(state *RaceState) int {
	leader, best := int64(0), math.Inf(1)
	for id, gap := range state.GCGaps {
		if gap < best || (gap == best && id < leader) {
			leader, best = id, gap
		}
	}
	main, most := 0, -1
	for i, c := range state.Clusters {
		n := 0
		for _, rs := range c {
			if leader != 0 && rs.Rider.ID == leader {
				return i
			}
			if state.Favourites[rs.Rider.ID] {
				n++
			}
		}
		if n > most {
			main, most = i, n
		}
	}
	return main
}

// Classify returns how a race with these results was won.
func (s *RaceShape) Classify(results []model.RaceResult, tt bool) Outcome {
	if tt {
		return OutcomeTT
	}
	var winner *model.RaceResult
	for i := range results {
		if results[i].Position == 1 {
			winner = &results[i]
		}
	}
	if winner == nil {
		return ""
	}
	if s != nil && s.breakAt[winner.RiderID] {
		return OutcomeBreakaway
	}
	n := 0
	for _, r := range results {
		if r.Position > 0 && r.TimeGapSeconds <= sameGroupS {
			n++
		}
	}
	if n >= sprintGroupSize {
		return OutcomeSprint
	}
	return OutcomeSelection
}
