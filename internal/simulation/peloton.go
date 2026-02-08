package simulation

import (
	"math"
	"math/rand"
	"sort"

	"cycling_sim/internal/model"
)

// RiderState tracks a rider's current state during a race simulation.
type RiderState struct {
	Rider         *model.Rider
	WPrime        *WPrimeState
	FormFactor    float64 // daily form multiplier (from randomness)
	TotalTime     float64 // accumulated time in seconds
	GroupID       int     // which group the rider is in
	GroupPosition int     // position within group
	Dropped       bool    // can't stay with group
	EffectiveFTP  int     // FTP after fatigue adjustment
}

// Group represents a cluster of riders traveling together.
type Group struct {
	ID      int
	Riders  []*RiderState
	Speed   float64 // group speed in m/s
	GapTime float64 // cumulative time gap from the race leader
}

// FormGroups analyzes rider powers on a segment and splits them into groups.
// On flat/rolling terrain, the peloton stays together because of drafting.
// On climbs (gradient > 4%), riders split based on power-to-weight.
func FormGroups(riders []*RiderState, segment *model.Segment, isTT bool, remainingDistance float64, rng *rand.Rand) []*Group {
	if isTT || len(riders) == 0 {
		// TT: everyone in their own group
		groups := make([]*Group, len(riders))
		for i, r := range riders {
			r.GroupID = i
			r.GroupPosition = 1
			groups[i] = &Group{ID: i, Riders: []*RiderState{r}}
		}
		return groups
	}

	// Calculate each rider's sustainable power for this segment
	type riderPower struct {
		state     *RiderState
		power     float64 // effective power for this segment
		wpw       float64 // watts per kg
		breakaway bool
	}

	rps := make([]riderPower, len(riders))
	for i, rs := range riders {
		basePower := float64(rs.EffectiveFTP) * rs.FormFactor
		power := EffectivePower(rs.Rider, basePower, segment, false)

		// W' depletion limits power
		if rs.WPrime.FractionRemaining() < 0.1 {
			// Can't sustain above FTP when W' is very low
			maxPower := float64(rs.EffectiveFTP) * rs.FormFactor
			if power > maxPower {
				power = maxPower
			}
		}

		rps[i] = riderPower{
			state:     rs,
			power:     power,
			wpw:       power / rs.Rider.WeightKg,
			breakaway: BreakawayAttempt(rs, segment, remainingDistance, rng),
		}
	}

	if segment.GradientPct <= 4.0 {
		// Flat/rolling: peloton stays together, speed determined by leaders
		// Sort by power descending to assign group positions
		sort.Slice(rps, func(i, j int) bool { return rps[i].power > rps[j].power })

		group := &Group{ID: 0, Riders: make([]*RiderState, len(rps))}
		for i, rp := range rps {
			rp.state.GroupID = 0
			rp.state.GroupPosition = i + 1
			group.Riders[i] = rp.state
		}

		// Group speed: leader's speed (they set the pace)
		leader := rps[0]
		leaderSpeed := SpeedFromPower(leader.power, leader.state.Rider.TotalMassKg(),
			leader.state.Rider.CdA, segment.GradientPct, segment.SurfaceType)
		group.Speed = leaderSpeed

		return []*Group{group}
	}

	// Climbing: split by W/kg — drafting benefit is minimal on climbs
	sort.Slice(rps, func(i, j int) bool { return rps[i].wpw > rps[j].wpw })

	var groups []*Group
	currentGroup := &Group{ID: 0}

	for i, rp := range rps {
		if i == 0 {
			rp.state.GroupID = 0
			rp.state.GroupPosition = 1
			currentGroup.Riders = append(currentGroup.Riders, rp.state)

			// Leader's speed on the climb
			speed := SpeedFromPower(rp.power, rp.state.Rider.TotalMassKg(),
				rp.state.Rider.CdA, segment.GradientPct, segment.SurfaceType)
			currentGroup.Speed = speed
			continue
		}

		// Can this rider stay with the current group?
		// On climbs, calculate their individual speed and compare
		riderSpeed := SpeedFromPower(rp.power, rp.state.Rider.TotalMassKg(),
			rp.state.Rider.CdA, segment.GradientPct, segment.SurfaceType)

		// Allow a small drafting benefit even on climbs (reduced)
		climbDraftFactor := 0.85 // small benefit on climbs
		requiredPower := powerForSpeed(currentGroup.Speed, rp.state.Rider.TotalMassKg(),
			rp.state.Rider.CdA*climbDraftFactor, segment.GradientPct, segment.SurfaceType)

		if rp.power >= requiredPower*0.99 {
			// Can stay with group
			rp.state.GroupID = currentGroup.ID
			rp.state.GroupPosition = len(currentGroup.Riders) + 1
			currentGroup.Riders = append(currentGroup.Riders, rp.state)
		} else {
			// Dropped — start new group
			groups = append(groups, currentGroup)
			newID := len(groups)
			currentGroup = &Group{ID: newID, Speed: riderSpeed}
			rp.state.GroupID = newID
			rp.state.GroupPosition = 1
			rp.state.Dropped = true
			currentGroup.Riders = append(currentGroup.Riders, rp.state)
		}
	}
	groups = append(groups, currentGroup)

	return groups
}

// powerForSpeed calculates the power needed to sustain a given speed.
func powerForSpeed(speedMPS float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType) float64 {
	fGrav := ForceGravity(totalMassKg, gradientPct)
	fRoll := ForceRolling(totalMassKg, gradientPct, surface)
	fAero := ForceAero(cda, speedMPS)
	return speedMPS * (fGrav + fRoll + fAero)
}

// SimulateSegment processes all riders through a segment, returning their times.
func SimulateSegment(riders []*RiderState, segment *model.Segment, isTT bool, disanceRemaining float64, rng *rand.Rand) {
	groups := FormGroups(riders, segment, isTT, disanceRemaining, rng)

	for _, group := range groups {
		var groupSpeed float64

		if segment.GradientPct < -2.0 {
			// Descent: each rider has individual speed capped by ability
			for _, rs := range group.Riders {
				basePower := float64(rs.EffectiveFTP) * rs.FormFactor * 0.5 // coast on descents
				speed := DescentSpeed(basePower, rs.Rider.TotalMassKg(),
					rs.Rider.CdA, segment.GradientPct, segment.SurfaceType,
					rs.Rider.DescendingAbility, segment.IsTechnical)
				segTime := SegmentTime(speed, segment.DistanceKm)
				rs.TotalTime += segTime

				// W' recovery during descent (easy effort)
				rs.WPrime.Update(basePower, rs.EffectiveFTP, segTime)
			}
			continue
		}

		// Normal segment: group travels together
		groupSpeed = group.Speed
		if groupSpeed < minSpeedMPS {
			groupSpeed = minSpeedMPS
		}
		segTime := SegmentTime(groupSpeed, segment.DistanceKm)

		for _, rs := range group.Riders {
			rs.TotalTime += segTime

			// Calculate actual power this rider is producing
			draftFactor := DraftFactor(rs.GroupPosition)
			if segment.GradientPct > 4.0 {
				draftFactor = math.Max(draftFactor, 0.85) // reduced draft on climbs
			}
			actualPower := powerForSpeed(groupSpeed, rs.Rider.TotalMassKg(),
				rs.Rider.CdA*draftFactor, segment.GradientPct, segment.SurfaceType)
			if actualPower < 0 {
				actualPower = 0
			}

			rs.WPrime.Update(actualPower, rs.EffectiveFTP, segTime)
		}
	}
}
