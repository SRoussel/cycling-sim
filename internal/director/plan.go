package director

import (
	"fmt"
	"math"
	"sort"

	"cycling_sim/internal/simulation"
)

// PlanPoint is a place on the route a race plan can target: a decisive
// climb or cobbled/gravel sector (the ones the race's moments are about).
type PlanPoint struct {
	Key      string  `json:"key"`
	Label    string  `json:"label"`
	Kind     string  `json:"kind"` // climb, cobbles, gravel
	StartKm  float64 `json:"startKm"`
	LengthKm float64 `json:"lengthKm"`
	ToGoKm   float64 `json:"toGoKm"` // at its start
}

// PlanPoints lists a route's decisive climbs and sectors in race order.
func PlanPoints(profile *simulation.RouteProfile) []PlanPoint {
	if profile == nil {
		return nil
	}
	d := New("")
	d.SetRoute(profile)
	var out []PlanPoint
	for _, c := range d.climbs {
		if !d.decisive[c.StartKm] {
			continue
		}
		name := c.Name
		if name == "" {
			name = fmt.Sprintf("Climb at %.0f km to go", profile.TotalKm-c.StartKm)
		}
		out = append(out, PlanPoint{Key: fmt.Sprintf("c%.1f", c.StartKm), Kind: "climb", StartKm: c.StartKm, LengthKm: c.LengthKm,
			ToGoKm: profile.TotalKm - c.StartKm,
			Label:  fmt.Sprintf("%s (%.1f km at %.1f%%, %.0f km to go)", name, c.LengthKm, c.AvgPct, profile.TotalKm-c.StartKm)})
	}
	for _, s := range d.sectors {
		if !d.decisiveSectors[s.StartKm] {
			continue
		}
		noun, _ := sectorWords(s.Surface)
		out = append(out, PlanPoint{Key: fmt.Sprintf("s%.1f", s.StartKm), Kind: noun, StartKm: s.StartKm, LengthKm: s.LengthKm,
			ToGoKm: profile.TotalKm - s.StartKm,
			Label:  fmt.Sprintf("%s (%.1f km of %d★ %s, %.0f km to go)", s.Name, s.LengthKm, s.Stars, noun, profile.TotalKm-s.StartKm)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartKm < out[j].StartKm })
	return out
}

// AttackAtKm is where a planned attack on the point goes, in km remaining:
// near the top of a climb, a third of the way into a sector.
func (p PlanPoint) AttackAtKm() float64 {
	if p.Kind == "climb" {
		return p.ToGoKm - 0.75*p.LengthKm
	}
	return p.ToGoKm - p.LengthKm/3
}

// HardFromKm and HardForKm: the team drives from 1 km before the point to its
// end (positioning into it, then the pace on it).
func (p PlanPoint) HardFromKm() float64 { return p.ToGoKm + 1 }
func (p PlanPoint) HardForKm() float64  { return math.Max(1, p.LengthKm+1) }
