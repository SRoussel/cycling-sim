package simulation

import "sort"

// RaceView is the race as a team director sees it at the end of a step:
// every group on the road with its riders, who each rider is, and what
// happened during the step. The game layer (internal/director) reads it to
// find the moments that call for a decision; the engine never reads it.
type RaceView struct {
	KmRemaining float64
	KmTotal     float64
	StageRace   bool
	Road        []RoadGroup // front to back
	Riders      map[int64]RiderBrief
	Events      []TacticsEvent // events logged during this step
}

// RoadGroup is one group on the road: riders within contact of each other.
type RoadGroup struct {
	GapSeconds float64 // behind the front of the race
	IDs        []int64 // in road order
}

// RiderBrief is what a director knows about a rider mid-race.
type RiderBrief struct {
	Name      string
	Team      string
	Role      TeamRole
	Favourite bool    // one of the riders the bunch marks
	Rank      int     // time over the course at threshold, 1 = fastest (0: unknown)
	Sprint    int     // sprint rating
	HardRank  int     // time over the hard sections (climbs, cobbles), 1 = fastest
	ClimbWKg  float64 // climbing W/kg (how stage races pick GC leaders)
	// Stage races: GC before this stage, and live (with today's time so far).
	GCPos, LiveGCPos int // 1 = leader (0: no GC yet)
	GCGap, LiveGCGap float64
	WPrimePct        float64
	LegsPct          float64
	FuelPct          float64 // carbohydrate left, % of a full store
}

// roadClusters splits the active riders into road groups (contact range).
func roadClusters(states []*RiderState) [][]*RiderState {
	active := make([]*RiderState, 0, len(states))
	for _, rs := range states {
		if !rs.DNF {
			active = append(active, rs)
		}
	}
	if len(active) == 0 {
		return nil
	}
	sort.Slice(active, func(i, j int) bool { return active[i].TotalTime < active[j].TotalTime })
	clusters := [][]*RiderState{{active[0]}}
	for i := 1; i < len(active); i++ {
		if active[i].TotalTime-active[i-1].TotalTime <= contactSeconds {
			clusters[len(clusters)-1] = append(clusters[len(clusters)-1], active[i])
		} else {
			clusters = append(clusters, []*RiderState{active[i]})
		}
	}
	return clusters
}

func buildRaceView(states []*RiderState, state *RaceState, kmRemaining, kmTotal float64, events []TacticsEvent) *RaceView {
	clusters := roadClusters(states)
	if len(clusters) == 0 {
		return nil
	}
	view := &RaceView{
		KmRemaining: kmRemaining,
		KmTotal:     kmTotal,
		Riders:      make(map[int64]RiderBrief, len(states)),
		Events:      events,
	}
	if state != nil {
		view.StageRace = state.IsStageRace
	}
	front := clusters[0][0].TotalTime
	for _, c := range clusters {
		g := RoadGroup{GapSeconds: c[0].TotalTime - front, IDs: make([]int64, len(c))}
		for i, rs := range c {
			g.IDs[i] = rs.Rider.ID
			brief := RiderBrief{
				Name:      rs.Rider.Name,
				Team:      rs.Rider.Team,
				Role:      rs.TeamRole,
				WPrimePct: clampPercent(rs.WPrime.FractionRemaining() * 100),
				LegsPct:   legsPct(rs),
				FuelPct:   rs.FuelShare() * 100,
				Sprint:    rs.Rider.SprintAbility,
				ClimbWKg:  climbingWKg(rs.Rider),
			}
			if state != nil {
				// Read-only: no RaceState helper with per-step caches.
				brief.Favourite = state.Favourites[rs.Rider.ID]
				brief.Rank = state.CourseRank[rs.Rider.ID]
				brief.HardRank = state.HardRank[rs.Rider.ID]
			}
			view.Riders[rs.Rider.ID] = brief
		}
		view.Road = append(view.Road, g)
	}
	if state != nil && len(state.GCGaps) > 0 {
		addGC(view, states, state.GCGaps)
	}
	return view
}

// addGC fills the GC positions before the stage and live (GC time plus the
// stage so far: the classification if the race ended here).
func addGC(view *RaceView, states []*RiderState, gaps map[int64]float64) {
	type entry struct {
		id        int64
		gap, live float64
	}
	var es []entry
	for _, rs := range states {
		if g, ok := gaps[rs.Rider.ID]; ok && !rs.DNF {
			es = append(es, entry{rs.Rider.ID, g, g + rs.TotalTime})
		}
	}
	if len(es) == 0 {
		return
	}
	sort.Slice(es, func(i, j int) bool { return es[i].gap < es[j].gap })
	if es[len(es)-1].gap > es[0].gap { // before the first stage nobody has a GC time
		for i, e := range es {
			b := view.Riders[e.id]
			b.GCPos, b.GCGap = i+1, e.gap-es[0].gap
			view.Riders[e.id] = b
		}
	}
	sort.Slice(es, func(i, j int) bool { return es[i].live < es[j].live })
	for i, e := range es {
		b := view.Riders[e.id]
		b.LiveGCPos, b.LiveGCGap = i+1, e.live-es[0].live
		view.Riders[e.id] = b
	}
}
