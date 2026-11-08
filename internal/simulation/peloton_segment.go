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

// partitionByTimeGap splits riders into road clusters: riders within
// contactSeconds of the rider ahead are in contact and ride as one road group.
// Returns clusters sorted by leader time (fastest cluster first).
func partitionByTimeGap(riders []*RiderState) [][]*RiderState {
	if len(riders) <= 1 {
		return [][]*RiderState{riders}
	}
	sorted := make([]*RiderState, len(riders))
	copy(sorted, riders)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TotalTime < sorted[j].TotalTime })

	var clusters [][]*RiderState
	cluster := []*RiderState{sorted[0]}
	for i := 1; i < len(sorted); i++ {
		if sorted[i].TotalTime-sorted[i-1].TotalTime <= contactSeconds {
			cluster = append(cluster, sorted[i])
		} else {
			clusters = append(clusters, cluster)
			cluster = []*RiderState{sorted[i]}
		}
	}
	return append(clusters, cluster)
}

// updateBreakawayState derives the breakaway situation from the road: the
// leading road group is a breakaway when it is smaller than the main group.
// BreakawayGap is the actual time gap from the break to the main group.
func updateBreakawayState(state *RaceState, clusters [][]*RiderState) {
	if state == nil {
		return
	}
	main := 0
	for i, c := range clusters {
		if len(c) > len(clusters[main]) {
			main = i
		}
	}
	isBreak := main > 0
	if isBreak && !state.BreakawayActive {
		state.LogEvent("breakaway", breakawayComposition(clusters[0], 6))
	} else if !isBreak && state.BreakawayActive {
		state.LogEvent("breakaway", "breakaway caught")
	}
	state.HasBreakaway = isBreak
	state.BreakawayActive = isBreak
	state.BreakawayGap = 0
	state.BreakawayHasDangerous = false
	if !isBreak {
		return
	}
	state.BreakawayGap = clusters[main][0].TotalTime - clusters[0][0].TotalTime
	for _, c := range clusters[:main] {
		for _, rs := range c {
			if state.Rival.DangerousRiders[rs.Rider.ID] {
				state.BreakawayHasDangerous = true
			}
		}
	}
}

// resolveCatches enforces that groups can't pass through each other on the
// road: a group that would arrive before the group ahead of it caught that
// group during the segment, and finishes with it.
func resolveCatches(groups []*Group, startTimes map[*RiderState]float64) {
	type span struct {
		g            *Group
		start, front float64
	}
	var spans []span
	for _, g := range groups {
		if len(g.Riders) == 0 {
			continue
		}
		sp := span{g: g, start: startTimes[g.Riders[0]], front: g.Riders[0].TotalTime}
		for _, rs := range g.Riders {
			sp.start = math.Min(sp.start, startTimes[rs])
			sp.front = math.Min(sp.front, rs.TotalTime)
		}
		spans = append(spans, sp)
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	floor := math.Inf(-1)
	for _, sp := range spans {
		if sp.front < floor {
			for _, rs := range sp.g.Riders {
				if rs.TotalTime < floor {
					rs.TotalTime = floor
				}
			}
			sp.front = floor
		}
		floor = sp.front
	}
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

	// Partition riders into road clusters: dropped riders can't teleport back.
	clusters := partitionByTimeGap(activeRiders)
	updateBreakawayState(state, clusters)

	// Form groups within each cluster independently
	var groups []*Group
	groupIDOffset := 0
	for _, cluster := range clusters {
		clusterGroups := FormGroups(cluster, segment, isTT, distanceRemaining, rng, state, localParams)
		for _, g := range clusterGroups {
			g.ID += groupIDOffset
			for _, rs := range g.Riders {
				rs.GroupID += groupIDOffset
			}
		}
		groupIDOffset += len(clusterGroups)
		groups = append(groups, clusterGroups...)
	}

	startTimes := make(map[*RiderState]float64, len(activeRiders))
	for _, rs := range activeRiders {
		startTimes[rs] = rs.TotalTime
	}

	for _, group := range groups {
		var groupSpeed float64

		// Every group travels together at its speed.
		groupSpeed = group.Speed
		if groupSpeed < minSpeedMPS {
			groupSpeed = minSpeedMPS
		}
		segTime := SegmentTime(groupSpeed, segment.DistanceKm)

		for _, rs := range group.Riders {
			rs.TotalTime += segTime

			// Power this rider actually produces at the group's speed.
			draftFactor := draftFor(rs.GroupPosition, segment.SurfaceType)
			actualPower := powerForSpeed(groupSpeed, rs.Rider.TotalMassKg(),
				rs.Rider.CdA*draftFactor, segment.GradientPct, segment.SurfaceType, rs.Rider.Handling)
			if actualPower < 0 {
				actualPower = 0
			}

			rs.WPrime.UpdateWithHandling(actualPower, rs.currentFTPInt(), segTime, rs.Rider.Handling, segment.SurfaceType == model.SurfaceCobbles)
			rs.addWork(actualPower, segTime)
		}
	}
	if !isTT {
		resolveCatches(groups, startTimes)
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
