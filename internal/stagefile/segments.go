package stagefile

import (
	"fmt"
	"math"
	"sort"

	"cycling_sim/internal/model"
)

// Conversion parameters.
const (
	// ToleranceM: the simplified profile stays within this many metres of
	// the original altitude everywhere (keeps short steep climbs).
	ToleranceM = 10.0
	// MinSegmentKm: shorter road pieces are merged into a neighbour (sectors
	// are always kept, however short).
	MinSegmentKm = 0.5
	// maxGradient keeps segments within the model's valid range.
	maxGradient = 25.0
)

type node struct {
	d, alt float64 // metres
}

// Segments converts a stage into course segments for a course of totalKm.
// Distance and altitude are scaled together to the course's real length, so
// gradients are preserved. Sectors become cobbled (technical) segments.
func Segments(st *Stage, totalKm float64) ([]model.Segment, error) {
	L := st.Length()
	if L <= 0 || len(st.Profile) < 2 {
		return nil, fmt.Errorf("stage %q has no profile", st.Name)
	}
	// Metres per distance unit, stretching the file's route to the stated
	// distance; altitude is stretched by the same factor so gradients keep
	// their file values.
	scale := totalKm * 1000 / L
	altScale := scale * AltitudeUnitM / DistanceUnitM
	pts := make([]node, len(st.Profile))
	for i, p := range st.Profile {
		pts[i] = node{p.D * scale, p.Alt * altScale}
	}
	keep := simplify(pts, ToleranceM)

	// Breakpoints: simplified profile vertices plus sector boundaries.
	cuts := map[float64]bool{}
	for _, i := range keep {
		cuts[pts[i].d] = true
	}
	for _, s := range st.Sectors {
		cuts[s.Start*scale] = true
		cuts[math.Min(s.End*scale, pts[len(pts)-1].d)] = true
	}
	ds := make([]float64, 0, len(cuts))
	for d := range cuts {
		ds = append(ds, d)
	}
	sort.Float64s(ds)

	type piece struct {
		from, to node
		sector   *Sector
	}
	var pieces []piece
	for i := 0; i+1 < len(ds); i++ {
		a, b := ds[i], ds[i+1]
		if b-a < 1 { // under a metre: rounding at a shared boundary
			continue
		}
		var sec *Sector
		mid := (a + b) / 2 / scale
		for k := range st.Sectors {
			if mid >= st.Sectors[k].Start && mid <= st.Sectors[k].End {
				sec = &st.Sectors[k]
				break
			}
		}
		pieces = append(pieces, piece{node{a, altAt(pts, a)}, node{b, altAt(pts, b)}, sec})
	}

	// Merge short road pieces into the previous piece of the same surface.
	var merged []piece
	for _, p := range pieces {
		n := len(merged)
		short := (p.to.d-p.from.d)/1000 < MinSegmentKm
		if n > 0 && p.sector == nil && merged[n-1].sector == nil &&
			(short || (merged[n-1].to.d-merged[n-1].from.d)/1000 < MinSegmentKm) {
			merged[n-1].to = p.to
			continue
		}
		merged = append(merged, p)
	}

	segs := make([]model.Segment, 0, len(merged))
	for i, p := range merged {
		km := (p.to.d - p.from.d) / 1000
		grad := (p.to.alt - p.from.alt) / (p.to.d - p.from.d) * 100
		grad = math.Max(-maxGradient, math.Min(maxGradient, grad))
		s := model.Segment{
			SegmentOrder: i + 1,
			DistanceKm:   km,
			GradientPct:  math.Round(grad*100) / 100,
			SurfaceType:  model.SurfaceRoad,
		}
		at := p.from.d / 1000
		switch {
		case p.sector != nil:
			s.SurfaceType = model.SurfaceCobbles
			s.IsTechnical = true
			s.Name = fmt.Sprintf("%s (%d★)", p.sector.Name, p.sector.Stars)
		case grad >= 3:
			s.Name = fmt.Sprintf("Climb at km %.0f", at)
		case grad <= -3:
			s.Name = fmt.Sprintf("Descent at km %.0f", at)
		default:
			s.Name = fmt.Sprintf("Road at km %.0f", at)
		}
		segs = append(segs, s)
	}
	return segs, nil
}

// altAt interpolates the profile altitude at distance d (metres).
func altAt(pts []node, d float64) float64 {
	i := sort.Search(len(pts), func(i int) bool { return pts[i].d >= d })
	switch {
	case i <= 0:
		return pts[0].alt
	case i >= len(pts):
		return pts[len(pts)-1].alt
	}
	a, b := pts[i-1], pts[i]
	return a.alt + (b.alt-a.alt)*(d-a.d)/(b.d-a.d)
}

// simplify returns the indices kept by Douglas-Peucker with a vertical
// tolerance (metres): every dropped point lies within tol of the line.
func simplify(pts []node, tol float64) []int {
	keep := make([]bool, len(pts))
	keep[0], keep[len(pts)-1] = true, true
	var rec func(lo, hi int)
	rec = func(lo, hi int) {
		worst, at := 0.0, -1
		for i := lo + 1; i < hi; i++ {
			line := pts[lo].alt + (pts[hi].alt-pts[lo].alt)*(pts[i].d-pts[lo].d)/(pts[hi].d-pts[lo].d)
			if e := math.Abs(pts[i].alt - line); e > worst {
				worst, at = e, i
			}
		}
		if worst > tol {
			keep[at] = true
			rec(lo, at)
			rec(at, hi)
		}
	}
	rec(0, len(pts)-1)
	var out []int
	for i, k := range keep {
		if k {
			out = append(out, i)
		}
	}
	return out
}

// ClimbingM is the total ascent of a segment list in metres.
func ClimbingM(segs []model.Segment) float64 {
	m := 0.0
	for _, s := range segs {
		if s.GradientPct > 0 {
			m += s.DistanceKm * 10 * s.GradientPct
		}
	}
	return m
}
