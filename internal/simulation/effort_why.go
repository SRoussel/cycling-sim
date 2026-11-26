package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

// Why a rider's anaerobic reserve (W') is going down: each step's change is
// put down to what he was doing (on the front, holding a wheel on a climb,
// the bunch's surges...) and kept as a running window of the last ~90 s of
// racing, for the live race's rider panel. Bookkeeping only: nothing here
// changes the race.

// Reserve causes.
const (
	causeFront   = iota // taking turns on the front of his group
	causeAlone          // riding alone (a solo attack or chase, or dropped)
	causeAttack         // attacking
	causeClimb          // holding the group's pace on a climb
	causeCobbles        // holding the pace over cobbles or gravel
	causeWheel          // holding the group's pace elsewhere
	causeSurges         // the bunch's accelerations deep in the group
	nCauses
)

var causeText = [nCauses]string{
	causeFront:   "taking turns on the front",
	causeAlone:   "riding alone",
	causeAttack:  "attacking",
	causeClimb:   "holding the wheel on the climb",
	causeCobbles: "holding the pace on the rough road",
	causeWheel:   "holding the group's pace",
	causeSurges:  "surges deep in the bunch",
}

// reserveWindowS: how much racing the "why" looks back over.
const reserveWindowS = 90.0

// reserveCause is what rider rs is doing in group g on segment.
func reserveCause(rs *RiderState, g *Group, segment *model.Segment, state *RaceState) int {
	if state != nil && state.LastActions != nil && state.LastActions[rs.Rider.ID] == ActionAttack {
		return causeAttack
	}
	if len(g.Riders) == 1 {
		return causeAlone
	}
	if g.Workers[rs] {
		return causeFront
	}
	switch {
	case segment.SurfaceType.Unpaved():
		return causeCobbles
	case climbOf(segment) >= terrainClimb:
		return causeClimb
	}
	return causeWheel
}

// noteReserve adds a step's W' change (J) to cause c's running window.
func (rs *RiderState) noteReserve(c int, delta, seconds float64) {
	keep := math.Exp(-seconds / reserveWindowS)
	for i := range rs.reserveRecent {
		rs.reserveRecent[i] *= keep
	}
	rs.reserveRecent[c] += delta
}

// reserveWhy is the rider's recent reserve trend (% of his W' per minute,
// negative when draining) and its main cause ("" when it isn't draining).
func (rs *RiderState) reserveWhy() (perMin float64, why string) {
	if rs.WPrime == nil || rs.WPrime.WPrimeMax <= 0 {
		return 0, ""
	}
	sum, worst, worstC := 0.0, 0.0, -1
	for c, v := range rs.reserveRecent {
		sum += v
		if v < worst {
			worst, worstC = v, c
		}
	}
	perMin = sum / rs.WPrime.WPrimeMax * 100 * 60 / reserveWindowS
	if perMin < -0.5 && worstC >= 0 {
		why = causeText[worstC]
	}
	return perMin, why
}
