package simulation

import (
	"sort"

	"cycling_sim/internal/model"
)

// followSplit splits riders into road groups using one physical rule on all
// terrain: the fastest rider sets the tempo at the power their tactical
// decision gives them (hold / attack / conserve multipliers), and each follower must produce
// the power needed to hold that speed in the draft for their position, using
// their power plus whatever W' they can spend over the segment. Riders who
// can't are dropped and start the next group at their own speed.
//
// Drafting only reduces aerodynamic drag, so its benefit in watts falls off
// naturally as speed drops on steeper climbs; no gradient thresholds needed.
func followSplit(rps []riderPower, segment *model.Segment, firstID int) []*Group {
	if len(rps) == 0 {
		return nil
	}
	solo := make(map[*RiderState]float64, len(rps))
	for _, rp := range rps {
		r := rp.state.Rider
		solo[rp.state] = SpeedFromPower(rp.power, r.TotalMassKg(), r.CdA, segment.GradientPct, segment.SurfaceType, r.Handling)
	}
	sort.SliceStable(rps, func(i, j int) bool { return solo[rps[i].state] > solo[rps[j].state] })

	var groups []*Group
	var members []riderPower
	var current *Group
	start := func(rp riderPower, speed float64, dropped bool) {
		if current != nil {
			groups = append(groups, current)
		}
		current = &Group{ID: firstID + len(groups), Speed: speed}
		members = members[:0]
		rp.state.Dropped = rp.state.Dropped || dropped
		current.Riders = append(current.Riders, rp.state)
		members = append(members, rp)
	}
	finish := func() {
		// Within a formed group, domestiques ride in the wind for their leaders.
		assignTeamPositions(append([]riderPower(nil), members...))
		for _, rs := range current.Riders {
			rs.GroupID = current.ID
		}
	}

	leader := rps[0]
	lr := leader.state.Rider
	start(leader, SpeedFromPower(leader.power, lr.TotalMassKg(), lr.CdA, segment.GradientPct, segment.SurfaceType, lr.Handling), false)

	for _, rp := range rps[1:] {
		r := rp.state.Rider
		position := len(current.Riders) + 1
		required := powerForSpeed(current.Speed, r.TotalMassKg(), r.CdA*DraftFactor(position), segment.GradientPct, segment.SurfaceType, r.Handling)
		segTime := SegmentTime(current.Speed, segment.DistanceKm)
		available := rp.power + rp.state.WPrime.AvailablePowerAboveFTP(segTime)
		if available >= required {
			current.Riders = append(current.Riders, rp.state)
			members = append(members, rp)
			continue
		}
		finish()
		start(rp, solo[rp.state], true)
	}
	finish()
	groups = append(groups, current)
	return groups
}
