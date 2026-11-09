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

	// linedOutIntensity: when the workers ride at this fraction of threshold
	// or more, the group strings out single file and nobody gets the deep
	// shelter of a packed bunch.
	linedOutIntensity = 0.95
)

// Phase intensity: the fraction of sustainable power the front of a group
// rides at when riders are simply holding position. Attacks and chases are
// layered on top by TacticsPowerMultiplier. This is rider intent, not terrain.
// In the final phase riders race to the line instead (see FormGroups).
var phaseIntensity = map[RacePhase]float64{
	PhaseEarly: 0.82,
	PhaseMid:   0.90,
}

// IntensityForPhase returns the holding intensity for a race phase.
func IntensityForPhase(p RacePhase) float64 {
	if v, ok := phaseIntensity[p]; ok {
		return v
	}
	return 1.0
}

// lineShelterPosition: in a single-file line riders beyond this position get
// no extra shelter (CFD/wind-tunnel: ~35-40% saving 2nd wheel, ~45-50% further
// back); only a packed bunch gives the deep shelter of DraftFactor's tail.
const lineShelterPosition = 5

// groupDraft returns the CdA multiplier for a position in a group, given its
// formation.
func groupDraft(position int, linedOut bool, seg *model.Segment, speed float64) float64 {
	p := position
	if linedOut && p > lineShelterPosition {
		p = lineShelterPosition
	}
	return crosswindDraft(draftFor(p, seg.SurfaceType), position, seg, speed)
}

// riderCdAFactor is the CdA multiplier a group member actually rides at:
// workers average their turns in the rotation, others sit at their position.
func riderCdAFactor(rs *RiderState, g *Group, seg *model.Segment) float64 {
	if g.Workers[rs] && g.Rotation > 1 {
		return rotationFactor(g.Rotation, seg.SurfaceType)
	}
	return groupDraft(rs.GroupPosition, g.LinedOut, seg, g.Speed)
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

// isSheltered reports whether a rider stays out of the rotation.
func isSheltered(rp riderPower) bool { return rp.sheltered }

// shelteredFor decides who sits in: riders conserving, sprinters (saving it
// for the sprint), and team leaders until the final phase, when they race, or
// until they chase in a stage race.
func shelteredFor(rs *RiderState, action Action, state *RaceState) bool {
	phase := phaseOf(state)
	switch {
	case action == ActionConserve:
		return true
	case rs.TeamRole == RoleSprinter:
		return true
	case rs.TeamRole == RoleLeader:
		// In a stage race a chasing GC leader takes turns (time lost now is
		// lost for good); in a one-day race they wait for the finale.
		stageChase := state != nil && state.IsStageRace && action == ActionChase
		return phase != PhaseFinal && !stageChase
	}
	return false
}

// paceLineSpeed is the speed a group sustains when its strongest workers
// rotate on the front. Sheltered riders only work when the group is too
// small to protect them.
func paceLineSpeed(rps []riderPower, segment *model.Segment, cooperative bool) (speed, intensity float64, workerSet map[*RiderState]bool, k int) {
	// Workers are riders actively pulling: chasing, attacking or driving.
	// In a big group everyone else is a passenger; a small group (no bigger
	// than a rotation) shares the work. With nobody working, the
	// non-sheltered riders set a tempo.
	var workers []riderPower
	for _, rp := range rps {
		if !isSheltered(rp) && (rp.action == ActionChase || rp.action == ActionAttack || len(rps) <= maxRotation) {
			workers = append(workers, rp)
		}
	}
	if len(workers) < 2 {
		workers = workers[:0]
		for _, rp := range rps {
			if !isSheltered(rp) {
				workers = append(workers, rp)
			}
		}
	}
	if len(workers) < 2 {
		workers = append([]riderPower(nil), rps...)
	}
	sort.SliceStable(workers, func(i, j int) bool { return workers[i].power > workers[j].power })
	k = len(workers)
	if k > maxRotation {
		k = maxRotation
	}
	// With N workers sharing k slots, each is on the front only k/N of the
	// time, so the rotation can spend W' N/k times faster while there:
	// P_rot = FTP + (N/k)·W'/T. This is why big groups catch small breaks.
	// It needs cooperation: teams pooling legs to chase a break. In a lead-out
	// teams compete for the front instead, so there's no pooling.
	depth := 1.0
	if cooperative {
		depth = float64(len(workers)) / float64(k)
	}
	workerSet = make(map[*RiderState]bool, k)
	var p, m, cda, handling, ftp float64
	for _, w := range workers[:k] {
		workerSet[w.state] = true
		threshold := EffectivePower(w.state.Rider, w.state.CurrentFTP(), segment, false)
		ftp += threshold
		power := w.power
		if power > threshold { // the W'-funded part scales with the pool depth
			power = threshold + (power-threshold)*depth
		}
		p += power
		m += w.state.Rider.TotalMassKg()
		cda += w.state.Rider.CdA
		handling += float64(w.state.Rider.Handling)
	}
	n := float64(k)
	speed = segmentSpeed(p/n, m/n, cda/n*rotationFactor(k, segment.SurfaceType), segment, int(handling/n))
	if ftp > 0 {
		intensity = p / ftp
	}
	return speed, intensity, workerSet, k
}

// followSplit splits riders into road groups with one physical rule on all
// terrain. A group's pace comes from its rotating workers (and is never
// slower than its strongest worker alone); each follower must hold that pace
// with their positional draft plus spendable W'. Riders who can't are dropped
// and form the next group from those remaining. Drafting only reduces aero
// drag, so its benefit fades naturally as speed drops on climbs; descending
// skill caps speed on the way down. One rule for every gradient.
func followSplit(rps []riderPower, segment *model.Segment, firstID int, cooperative, racing bool) []*Group {
	if len(rps) == 0 {
		return nil
	}
	solo := make(map[*RiderState]float64, len(rps))
	for _, rp := range rps {
		r := rp.state.Rider
		solo[rp.state] = math.Min(SpeedCap(r, segment),
			segmentSpeed(rp.power, r.TotalMassKg(), r.CdA, segment, r.Handling))
	}
	remaining := append([]riderPower(nil), rps...)
	sort.SliceStable(remaining, func(i, j int) bool { return solo[remaining[i].state] > solo[remaining[j].state] })

	var groups []*Group
	for len(remaining) > 0 {
		// The pace comes from the riders doing the work: their rotation, and
		// never slower than the strongest worker would ride on the front alone.
		// Sheltered leaders follow; they don't set the tempo.
		// A big group in which nobody is working (no chase, attack or team
		// drive) holds a collective tempo: riders on the front ride at a pace
		// the bulk of the bunch can follow, so it is set by the middle of the
		// group, not its strongest rider. In the finale everyone races.
		holding := !racing && len(remaining) > maxRotation && !anyWorking(remaining)
		pacers := remaining
		if holding {
			pacers = tempoRiders(remaining)
		}
		speed, intensity, workerSet, k := paceLineSpeed(pacers, segment, cooperative)
		linedOut := intensity >= linedOutIntensity
		if s := strongestWorkerSpeed(remaining, solo); !holding && speed < s {
			speed = s
		}
		// An attacker in the group drives the pace at their own burst speed.
		for _, rp := range remaining {
			if rp.action == ActionAttack && solo[rp.state] > speed {
				speed = solo[rp.state]
			}
		}
		// Nobody in front goes faster than the front rider dares (descents).
		speed = math.Min(speed, SpeedCap(remaining[0].state.Rider, segment))
		g := &Group{ID: firstID + len(groups), Speed: speed, LinedOut: linedOut, Workers: workerSet, Rotation: k}
		segTime := SegmentTime(speed, segment.DistanceKm)
		var members, rest []riderPower
		for _, rp := range remaining {
			r := rp.state.Rider
			available := rp.power + rp.state.WPrime.AvailablePowerAboveFTP(segTime)
			needs := func(cdaFactor float64) float64 {
				return segmentPower(speed, r.TotalMassKg(), r.CdA*cdaFactor, segment, r.Handling)
			}
			canHold := false
			if SpeedCap(r, segment) >= speed {
				if workerSet[rp.state] && k > 1 && available >= needs(rotationFactor(k, segment.SurfaceType)) {
					canHold = true // takes turns on the front
				} else if available >= needs(groupDraft(len(members)+1, linedOut, segment, speed)) {
					// Can't (or won't) take turns but can sit on: a passenger.
					delete(workerSet, rp.state)
					canHold = true
				}
			}
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
		assignTeamPositions(members, gutterExposure(segment, speed) > 0.3) // domestiques ride in the wind for their leaders
		for _, m := range members {
			m.state.GroupID = g.ID
			g.Riders = append(g.Riders, m.state)
		}
		groups = append(groups, g)
		remaining = rest
	}
	return groups
}

// anyWorking reports whether a rider in the group is chasing or attacking
// (team drives count as chasing).
func anyWorking(rps []riderPower) bool {
	for _, rp := range rps {
		if rp.action == ActionChase || rp.action == ActionAttack {
			return true
		}
	}
	return false
}

// tempoRiders returns the riders who set a holding group's tempo: a
// rotation's worth of non-sheltered riders around the group's median power.
func tempoRiders(rps []riderPower) []riderPower {
	var pool []riderPower
	for _, rp := range rps {
		if !isSheltered(rp) {
			pool = append(pool, rp)
		}
	}
	if len(pool) <= maxRotation {
		return rps
	}
	sort.SliceStable(pool, func(i, j int) bool { return pool[i].power > pool[j].power })
	lo := len(pool)/2 - maxRotation/2
	return pool[lo : lo+maxRotation]
}

// strongestWorkerSpeed is the solo speed of the fastest non-sheltered rider
// (or of the fastest rider when everyone is sheltered).
func strongestWorkerSpeed(rps []riderPower, solo map[*RiderState]float64) float64 {
	best, bestAny := 0.0, 0.0
	for _, rp := range rps {
		v := solo[rp.state]
		bestAny = math.Max(bestAny, v)
		if !isSheltered(rp) {
			best = math.Max(best, v)
		}
	}
	if best == 0 {
		return bestAny
	}
	return best
}
