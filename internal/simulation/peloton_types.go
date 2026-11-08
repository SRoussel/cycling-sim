package simulation

import "cycling_sim/internal/model"

// TeamRole represents a rider's role within their team for a race.
type TeamRole int

const (
	RoleDomestique TeamRole = iota
	RoleLeader              // best GC rider on the team
	RoleSprinter            // designated sprint finisher
)

// RiderState tracks a rider's current state during a race simulation.
type RiderState struct {
	Rider            *model.Rider
	WPrime           *WPrimeState
	FormFactor       float64  // daily form multiplier (from randomness)
	TotalTime        float64  // accumulated time in seconds
	GroupID          int      // which group the rider is in
	GroupPosition    int      // position within group
	Dropped          bool     // can't stay with group
	EffectiveFTP     int      // FTP after fatigue adjustment
	LastSegmentPower float64  // power used for the current segment (after tactics)
	DNF              bool     // did not finish (e.g., crash)
	TotalPenalty     float64  // cumulative penalty time from incidents
	TeamRole         TeamRole // role within team (leader, sprinter, domestique)
	WorkKJ           float64  // work done so far this race (for durability)
}

// Calculate each rider's sustainable power for this segment.
type riderPower struct {
	state     *RiderState
	power     float64 // effective power for this segment
	wpw       float64 // watts per kg
	breakaway bool
	action    Action
	sheltered bool // sits in the group instead of taking turns
}

// Group represents a cluster of riders traveling together.
type Group struct {
	ID          int
	Riders      []*RiderState
	Speed       float64              // group speed in m/s
	GapTime     float64              // cumulative time gap from the race leader
	IsBreakaway bool                 // whether this is a breakaway group
	LinedOut    bool                 // riding single file (hard pace): no deep bunch shelter
	Workers     map[*RiderState]bool // riders sharing the front in rotation
	Rotation    int                  // how many riders rotate (≤ maxRotation)
}
