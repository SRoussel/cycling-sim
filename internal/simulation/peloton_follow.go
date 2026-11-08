package simulation

import (
	"math"
	"sort"

	"cycling_sim/internal/model"
)

// Group-dynamics constants. These are the only knobs of the group model; all
// gaps, drops, catches and regroups emerge from physics plus rider intent.
const (
	// contactSeconds: riders this close on the road are in contact and ride as
	// one road group (a short effort closes ~150 m).
	contactSeconds = 10.0

	// maxRotation: at most this many riders usefully share the front in a
	// rotating pace line; more riders add shelter but no extra speed.
	maxRotation = 8

	// cobbleDraftRetention: fraction of the normal drafting benefit available on
	// cobbles, where riders can't hold a wheel smoothly.
	cobbleDraftRetention = 0.4
)

// Phase intensity: the fraction of sustainable power the front of a group
// rides at when riders are simply holding position. Attacks and chases are
// layered on top by TacticsPowerMultiplier. This is rider intent, not terrain.
var phaseIntensity = map[RacePhase]float64{
	PhaseEarly: 0.82,
	PhaseMid:   0.90,
	PhaseFinal: 0.97,
}

// IntensityForPhase returns the holding intensity for a race phase.
func IntensityForPhase(p RacePhase) float64 {
	if v, ok := phaseIntensity[p]; ok {
		return v
	}
	return 1.0
}

// draftFor returns the CdA multiplier for a rider at a position in a group.
func draftFor(position int, surface model.SurfaceType) float64 {
	d := DraftFactor(position)
	if surface == model.SurfaceCobbles {
		d = 1 - (1-d)*cobbleDraftRetention
	}
	return d
}

// rotationFactor is the average CdA multiplier of a rider sharing the front
// in a k-rider pace line: one turn in the wind, k-1 turns on a wheel.
func rotationFactor(k int, surface model.SurfaceType) float64 {
	if k <= 1 {
		return 1.0
	}
	if k > maxRotation {
		k = maxRotation
	}
	wheel := draftFor(2, surface)
	return (1 + float64(k-1)*wheel) / float64(k)
}

// isSheltered reports whether a rider stays out of the rotation (protected
// leaders/sprinters and riders conserving).
func isSheltered(rp riderPower) bool {
	role := rp.state.TeamRole
	return role == RoleLeader || role == RoleSprinter || rp.action == ActionConserve
}

// paceLineSpeed is the speed a group sustains when its strongest workers
// rotate on the front. Sheltered riders only work when the group is too
// small to protect them.
func paceLineSpeed(rps []riderPower, segment *model.Segment) float64 {
	var workers []riderPower
	for _, rp := range rps {
		if !isSheltered(rp) {
			workers = append(workers, rp)
		}
	}
	if len(workers) < 2 {
		workers = append([]riderPower(nil), rps...)
	}
	sort.SliceStable(workers, func(i, j int) bool { return workers[i].power > workers[j].power })
	k := len(workers)
	if k > maxRotation {
		k = maxRotation
	}
	var p, m, cda, handling float64
	for _, w := range workers[:k] {
		p += w.power
		m += w.state.Rider.TotalMassKg()
		cda += w.state.Rider.CdA
		handling += float64(w.state.Rider.Handling)
	}
	n := float64(k)
	return SpeedFromPower(p/n, m/n, cda/n*rotationFactor(k, segment.SurfaceType),
		segment.GradientPct, segment.SurfaceType, int(handling/n))
}

// followSplit splits riders into road groups with one physical rule on all
// terrain. A group's pace comes from its rotating workers (and is never
// slower than its strongest rider alone); each follower must hold that pace
// with their positional draft plus spendable W'. Riders who can't are dropped
// and form the next group from those remaining. Drafting only reduces aero
// drag, so its benefit fades naturally as speed drops on climbs; descending
// skill caps speed on the way down. One rule for every gradient.
func followSplit(rps []riderPower, segment *model.Segment, firstID int) []*Group {
	if len(rps) == 0 {
		return nil
	}
	solo := make(map[*RiderState]float64, len(rps))
	for _, rp := range rps {
		r := rp.state.Rider
		solo[rp.state] = math.Min(SpeedCap(r, segment),
			SpeedFromPower(rp.power, r.TotalMassKg(), r.CdA, segment.GradientPct, segment.SurfaceType, r.Handling))
	}
	remaining := append([]riderPower(nil), rps...)
	sort.SliceStable(remaining, func(i, j int) bool { return solo[remaining[i].state] > solo[remaining[j].state] })

	var groups []*Group
	for len(remaining) > 0 {
		speed := paceLineSpeed(remaining, segment)
		if s := solo[remaining[0].state]; speed < s {
			speed = s
		}
		// Nobody in front goes faster than the front rider dares (descents).
		speed = math.Min(speed, SpeedCap(remaining[0].state.Rider, segment))
		g := &Group{ID: firstID + len(groups), Speed: speed}
		segTime := SegmentTime(speed, segment.DistanceKm)
		var members, rest []riderPower
		for _, rp := range remaining {
			r := rp.state.Rider
			required := powerForSpeed(speed, r.TotalMassKg(), r.CdA*draftFor(len(members)+1, segment.SurfaceType),
				segment.GradientPct, segment.SurfaceType, r.Handling)
			available := rp.power + rp.state.WPrime.AvailablePowerAboveFTP(segTime)
			canHold := available >= required && SpeedCap(r, segment) >= speed
			if len(members) == 0 || canHold {
				members = append(members, rp)
			} else {
				rest = append(rest, rp)
			}
		}
		if len(groups) > 0 {
			for _, m := range members {
				m.state.Dropped = true
			}
		}
		assignTeamPositions(members) // domestiques ride in the wind for their leaders
		for _, m := range members {
			m.state.GroupID = g.ID
			g.Riders = append(g.Riders, m.state)
		}
		groups = append(groups, g)
		remaining = rest
	}
	return groups
}
