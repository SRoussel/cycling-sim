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
func partitionByTimeGap(riders []*RiderState, segment *model.Segment, distanceRemaining float64) [][]*RiderState {
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
	// Priority order:
	//  1. Cobbles always shatter groups (10 s).
	//  2. Final 5 km - sprint zone; gaps lock in (10 s).
	//  3. Steep climbs in the final 60 km (>6%) are decisive (5 s).
	//  4. Any climb >4%: sticky gaps (8 s).
	//  5. Steep descents (<-2%) outside the final 5 km allow regrouping (60 s).
	//  6. Final 15 km on flat/rolling: gaps carry through (15 s).
	//  7. Final 60 km on flat/rolling: gaps from preceding climbs persist (20 s).
	//  8. Default flat road (>60 km to go): peloton draft closes gaps (45 s).
	gapThreshold := 45.0
	if segment.SurfaceType == model.SurfaceCobbles {
		gapThreshold = 10.0 // cobbles shatter groups; gaps stick
	} else if distanceRemaining < 5 {
		// Sprint zone: gaps are locked in regardless of terrain.
		gapThreshold = 10.0
	} else if segment.GradientPct > 6.0 && distanceRemaining < 60 {
		// Decisive final climbs: only riders within 5 s can stay connected.
		gapThreshold = 5.0
	} else if segment.GradientPct > 4.0 {
		gapThreshold = 8.0 // moderate climbs: gaps are hard to close
	} else if segment.GradientPct < -2.0 && distanceRemaining >= 5 {
		// Steep descents outside sprint zone: significant regrouping possible.
		// This covers Cipressa descent (8.7km remaining), Poggio descent, etc.
		gapThreshold = 60.0
	} else if distanceRemaining < 15 {
		// Final 15 km (flat, rolling, modest descent): gaps from last climb hold.
		gapThreshold = 15.0
	} else if distanceRemaining < 60 {
		// Final 60 km flat/rolling: chase groups cannot close 20+ s gaps.
		gapThreshold = 20.0
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
	clusters := partitionByTimeGap(activeRiders, segment, distanceRemaining)

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

		// When the power analysis says the peloton CAN match the break (breakawayGap==0),
		// prevent physical speed differences from creating spurious time gaps on flat cobbles.
		// On flat/rolling cobbles, gradient multipliers (EffectivePower) don't strongly
		// favour any rider, so the small weight/CdA advantage of lighter riders should not
		// open a decisive gap. On steep cobbled climbs (>6%), genuine climbing ability
		// differences ARE meaningful and should be allowed to produce gaps.
		if breakawayGap == 0 && segment.SurfaceType == model.SurfaceCobbles &&
			segment.GradientPct < 6.0 &&
			len(pelotonGroup.Riders) > 0 && pelotonGroup.Speed < breakGroup.Speed {
			pelotonGroup.Speed = breakGroup.Speed
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
