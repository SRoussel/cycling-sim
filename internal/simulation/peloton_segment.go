package simulation

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"cycling_sim/internal/model"
)

// powerForSpeed calculates the power needed to sustain a given speed.
func powerForSpeed(speedMPS float64, totalMassKg float64, cda float64, gradientPct float64, surface model.SurfaceType, handling int) float64 {
	fGrav := ForceGravity(totalMassKg, gradientPct)
	fRoll := ForceRolling(totalMassKg, gradientPct, surface, handling)
	fAero := ForceAero(cda, speedMPS)
	return speedMPS * (fGrav + fRoll + fAero)
}

// partitionByTimeGap splits riders into road clusters based on accumulated time gaps.
// Riders who are too far apart on the road cannot regroup. Returns clusters sorted
// by leader time (fastest cluster first).
func partitionByTimeGap(riders []*RiderState, segment *model.Segment) [][]*RiderState {
	if len(riders) <= 1 {
		return [][]*RiderState{riders}
	}

	// Sort by accumulated time (road position)
	sorted := make([]*RiderState, len(riders))
	copy(sorted, riders)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TotalTime < sorted[j].TotalTime
	})

	// Gap threshold: how close riders must be to potentially regroup.
	// On flat terrain, riders within ~15s can draft back on.
	// On climbs, gaps are sticky — only very close riders regroup.
	// On descents, some natural regrouping occurs.
	gapThreshold := 45.0 // flat road: peloton draft can close moderate gaps
	if segment.SurfaceType == model.SurfaceCobbles {
		gapThreshold = 10.0 // cobbles shatter groups; gaps stick
	} else if segment.GradientPct > 4.0 {
		gapThreshold = 8.0 // gaps are hard to close on climbs, but small regrouping happens
	} else if segment.GradientPct < -2.0 {
		gapThreshold = 60.0 // descents allow significant regrouping
	}

	var clusters [][]*RiderState
	cluster := []*RiderState{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		gap := sorted[i].TotalTime - sorted[i-1].TotalTime
		if gap <= gapThreshold {
			cluster = append(cluster, sorted[i])
		} else {
			clusters = append(clusters, cluster)
			cluster = []*RiderState{sorted[i]}
		}
	}
	clusters = append(clusters, cluster)
	return clusters
}

// SimulateSegment processes all riders through a segment, returning their times.
func SimulateSegment(riders []*RiderState, segment *model.Segment, isTT bool, distanceRemaining float64, rng *rand.Rand, state *RaceState, params *TacticsParams, incidentsEnabled ...bool) {
	localParams := params
	if localParams == nil {
		defaultParams := DefaultTacticsParams()
		localParams = &defaultParams
	}
	if state != nil {
		state.KmRemaining = distanceRemaining
		state.Phase = PhaseFromKm(distanceRemaining)
		state.Segment = segment
		// Sprint trains only really matter on flat/rolling finishes.
		state.SprintChaseActive = state.Phase == PhaseFinal && state.Rival.SprintTeamsChasing && segment.GradientPct < 3.0 && segment.SurfaceType != model.SurfaceCobbles
		if state.SprintChaseActive && !state.LastSprintChase {
			state.LogEvent("sprint-chase", "sprinter teams drive the chase")
		}
		state.LastSprintChase = state.SprintChaseActive
		adjusted := ApplyEventModifiers(*localParams, state)
		localParams = &adjusted
	}

	// Apply random incidents (crashes/punctures) first, if enabled
	enableIncidents := len(incidentsEnabled) > 0 && incidentsEnabled[0]
	if enableIncidents {
		ApplyIncidents(riders, segment, rng)
	}

	// Remove DNF riders from active participation
	var activeRiders []*RiderState
	for _, r := range riders {
		if !r.DNF {
			activeRiders = append(activeRiders, r)
		}
	}

	// Partition riders into road clusters by accumulated time gaps.
	// Dropped riders can't teleport back to the front group.
	clusters := partitionByTimeGap(activeRiders, segment)

	// Form groups within each cluster independently
	var groups []*Group
	groupIDOffset := 0
	for _, cluster := range clusters {
		clusterGroups := FormGroups(cluster, segment, isTT, distanceRemaining, rng, state, localParams)
		// Offset group IDs to avoid collisions between clusters
		for _, g := range clusterGroups {
			g.ID += groupIDOffset
			for _, rs := range g.Riders {
				rs.GroupID += groupIDOffset
			}
		}
		groupIDOffset += len(clusterGroups)
		groups = append(groups, clusterGroups...)
	}

	// Calculate breakaway time gap if there's a breakaway group on flat terrain
	var breakawayGap float64
	if len(groups) >= 2 && groups[0].IsBreakaway {
		breakGroup := groups[0]
		pelotonGroup := groups[1]
		if state != nil {
			state.BreakawayHasDangerous = false
			for _, rs := range breakGroup.Riders {
				if state.Rival.DangerousRiders[rs.Rider.ID] {
					state.BreakawayHasDangerous = true
					break
				}
			}
		}

		// Average power of breakaway riders
		var breakPowerSum float64
		for _, rs := range breakGroup.Riders {
			if rs.LastSegmentPower > 0 {
				breakPowerSum += rs.LastSegmentPower
				continue
			}
			basePower := float64(rs.EffectiveFTP) * rs.FormFactor
			breakPowerSum += EffectivePower(rs.Rider, basePower, segment, false)
		}
		breakAvgPower := breakPowerSum / float64(len(breakGroup.Riders))

		// Peloton leader power (strongest rider sets chase pace)
		var pelotonLeaderPower float64
		if len(pelotonGroup.Riders) > 0 {
			rs := pelotonGroup.Riders[0]
			if rs.LastSegmentPower > 0 {
				pelotonLeaderPower = rs.LastSegmentPower
			} else {
				basePower := float64(rs.EffectiveFTP) * rs.FormFactor
				pelotonLeaderPower = EffectivePower(rs.Rider, basePower, segment, false)
			}
		}

		cohesion := BreakawayCohesion(breakGroup.Riders)
		breakawayGap = EvaluateBreakawayWithCohesion(breakAvgPower, pelotonLeaderPower,
			segment.DistanceKm, distanceRemaining, len(breakGroup.Riders), cohesion, *localParams)
		if state != nil {
			if state.BreakawayHasDangerous {
				breakawayGap /= localParams.GCChaseAggression
			}
			if state.SprintChaseActive {
				breakawayGap /= localParams.SprintTrainEfficiency
			}
		}
		if state != nil {
			if !state.BreakawayActive {
				composition := breakawayComposition(breakGroup.Riders, 6)
				state.LogEvent("breakaway", composition)
			}
			state.HasBreakaway = true
			state.BreakawayGap = breakawayGap
			state.BreakawayActive = true
		}
	} else if state != nil {
		if state.BreakawayActive {
			state.LogEvent("breakaway", "breakaway caught")
		}
		state.HasBreakaway = false
		state.BreakawayGap = 0
		state.BreakawayHasDangerous = false
		state.BreakawayActive = false
	}

	for _, group := range groups {
		var groupSpeed float64

		if segment.GradientPct < -2.0 {
			// Descent: each rider has individual speed capped by ability
			for _, rs := range group.Riders {
				basePower := float64(rs.EffectiveFTP) * rs.FormFactor * 0.5 // coast on descents
				speed := DescentSpeed(basePower, rs.Rider.TotalMassKg(),
					rs.Rider.CdA, segment.GradientPct, segment.SurfaceType,
					rs.Rider.Handling, rs.Rider.DescendingAbility, segment.IsTechnical)
				segTime := SegmentTime(speed, segment.DistanceKm)
				rs.TotalTime += segTime

				// W' recovery during descent (easy effort)
				// Use handling-based fatigue on technical terrain
				if segment.SurfaceType == model.SurfaceCobbles {
					rs.WPrime.UpdateWithHandling(basePower, rs.EffectiveFTP, segTime, rs.Rider.Handling, true)
				} else {
					rs.WPrime.Update(basePower, rs.EffectiveFTP, segTime)
				}
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
			adjustedTime := segTime
			// Breakaway riders get a time bonus from the gap they've built
			if group.IsBreakaway && breakawayGap > 0 {
				adjustedTime -= breakawayGap
				if adjustedTime < segTime*0.9 {
					adjustedTime = segTime * 0.9 // cap gap gain at 10% of segment time
				}
			}
			rs.TotalTime += adjustedTime

			// Calculate actual power this rider is producing
			draftFactor := DraftFactor(rs.GroupPosition)
			if group.IsBreakaway {
				// Small breakaway group gets less draft benefit
				draftFactor = math.Max(draftFactor, 0.65)
			}
			if segment.GradientPct > 4.0 {
				draftFactor = math.Max(draftFactor, 0.85) // reduced draft on climbs
			}
			actualPower := powerForSpeed(groupSpeed, rs.Rider.TotalMassKg(),
				rs.Rider.CdA*draftFactor, segment.GradientPct, segment.SurfaceType, rs.Rider.Handling)
			if actualPower < 0 {
				actualPower = 0
			}

			rs.WPrime.UpdateWithHandling(actualPower, rs.EffectiveFTP, segTime, rs.Rider.Handling, segment.SurfaceType == model.SurfaceCobbles)
		}
	}
}

func breakawayComposition(riders []*RiderState, limit int) string {
	if len(riders) == 0 {
		return "breakaway formed"
	}
	if limit <= 0 {
		limit = 1
	}
	if len(riders) < limit {
		limit = len(riders)
	}
	parts := make([]string, 0, limit+1)
	for i := 0; i < limit; i++ {
		parts = append(parts, riders[i].Rider.Name)
	}
	if len(riders) > limit {
		parts = append(parts, fmt.Sprintf("+%d more", len(riders)-limit))
	}
	return "breakaway: " + strings.Join(parts, ", ")
}
