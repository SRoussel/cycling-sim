package stagefile

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cycling_sim/internal/model"
)

// Source maps a seeded course to a stage file name (in the user's stage dir).
type Source struct {
	Course string
	File   string
	Note   string
	// Surface of the file's sectors: "cobbles" (default) or "gravel". Stage
	// files don't say what a sector is made of.
	Surface model.SurfaceType
	// AltScale (optional 5th column, 0 = none) corrects a file whose climbs
	// are taller than the real ones: altitudes are multiplied by it (set from
	// the real summit heights).
	AltScale float64
	// CutKm (optional 6th column) drops this much of the file's end: some
	// files run on past the finish line (Flèche Wallonne's profiles go on
	// 2-12 km beyond the top of the Mur, where the race finishes).
	CutKm float64
}

// LoadSources reads the course → stage file mapping (Course,File[,Note];
// '#' comments).
func LoadSources(path string) ([]Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	var out []Source
	for i, row := range rows {
		if i == 0 && len(row) > 0 && row[0] == "Course" {
			continue
		}
		if len(row) < 2 {
			continue
		}
		s := Source{Course: strings.TrimSpace(row[0]), File: strings.TrimSpace(row[1])}
		if len(row) > 2 {
			s.Note = strings.TrimSpace(row[2])
		}
		s.Surface = model.SurfaceCobbles
		if len(row) > 3 && strings.EqualFold(strings.TrimSpace(row[3]), string(model.SurfaceGravel)) {
			s.Surface = model.SurfaceGravel
		}
		if len(row) > 4 && strings.TrimSpace(row[4]) != "" {
			v, err := strconv.ParseFloat(strings.TrimSpace(row[4]), 64)
			if err != nil || v <= 0 {
				return nil, fmt.Errorf("%s: bad altitude scale %q", s.Course, row[4])
			}
			s.AltScale = v
		}
		if len(row) > 5 && strings.TrimSpace(row[5]) != "" {
			v, err := strconv.ParseFloat(strings.TrimSpace(row[5]), 64)
			if err != nil || v < 0 {
				return nil, fmt.Errorf("%s: bad cut %q", s.Course, row[5])
			}
			s.CutKm = v
		}
		out = append(out, s)
	}
	return out, nil
}

// Profile is a converted course: the stage it came from and its segments.
type Profile struct {
	Course   string        `json:"course"`
	Stage    string        `json:"stage"`
	File     string        `json:"file"`
	Segments []SegmentJSON `json:"segments"`
}

// SegmentJSON is the stored form of a segment.
type SegmentJSON struct {
	Name        string  `json:"name"`
	DistanceKm  float64 `json:"km"`
	GradientPct float64 `json:"gradient"`
	Cobbles     bool    `json:"cobbles,omitempty"` // an unpaved sector (cobbles, or gravel with Gravel)
	Gravel      bool    `json:"gravel,omitempty"`
	Stars       int     `json:"stars,omitempty"` // sector difficulty, 1-5
}

// ModelSegments returns the profile's segments as model segments.
func (p Profile) ModelSegments() []model.Segment {
	out := make([]model.Segment, len(p.Segments))
	for i, s := range p.Segments {
		surface := model.SurfaceRoad
		if s.Cobbles {
			surface = model.SurfaceCobbles
			if s.Gravel {
				surface = model.SurfaceGravel
			}
		}
		out[i] = model.Segment{SegmentOrder: i + 1, Name: s.Name, DistanceKm: s.DistanceKm,
			GradientPct: s.GradientPct, SurfaceType: surface, IsTechnical: s.Cobbles, CobbleStars: s.Stars}
	}
	return out
}

// Build converts the mapped stage files in dir for courses with the given
// stated distances. Missing or unreadable files are reported, not fatal.
func Build(dir string, sources []Source, distanceKm map[string]float64) ([]Profile, []string) {
	var out []Profile
	var problems []string
	for _, s := range sources {
		km, ok := distanceKm[s.Course]
		if !ok {
			problems = append(problems, fmt.Sprintf("%s: no such course", s.Course))
			continue
		}
		st, err := ParseFile(filepath.Join(dir, s.File))
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", s.Course, err))
			continue
		}
		if s.AltScale > 0 {
			for i := range st.Profile {
				st.Profile[i].Alt *= s.AltScale
			}
		}
		if s.CutKm > 0 {
			cutEnd(st, s.CutKm*1000/DistanceUnitM)
		}
		segs, err := Segments(st, km)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", s.Course, err))
			continue
		}
		p := Profile{Course: s.Course, Stage: st.Name, File: s.File}
		for _, sg := range segs {
			p.Segments = append(p.Segments, SegmentJSON{Name: sg.Name, DistanceKm: sg.DistanceKm,
				GradientPct: sg.GradientPct, Cobbles: sg.SurfaceType.Unpaved(), Gravel: sg.SurfaceType.Unpaved() && s.Surface == model.SurfaceGravel,
				Stars: sg.CobbleStars})
		}
		out = append(out, p)
	}
	return out, problems
}

// Save writes profiles as JSON.
func Save(path string, profiles []Profile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(profiles, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Load reads profiles saved by Save, keyed by course name. A missing file
// yields an empty map.
func Load(path string) (map[string]Profile, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Profile
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make(map[string]Profile, len(list))
	for _, p := range list {
		out[p.Course] = p
	}
	return out, nil
}

// cutEnd drops the profile beyond Length()-cut (profile units), ending it at
// an interpolated point, and trims sectors to it.
func cutEnd(st *Stage, cut float64) {
	end := st.Length() - cut
	if end <= 0 {
		return
	}
	var pts []Point
	for i, p := range st.Profile {
		if p.D <= end {
			pts = append(pts, p)
			continue
		}
		if i > 0 && st.Profile[i-1].D < end {
			a := st.Profile[i-1]
			f := (end - a.D) / (p.D - a.D)
			pts = append(pts, Point{D: end, Alt: a.Alt + f*(p.Alt-a.Alt)})
		}
		break
	}
	st.Profile = pts
	var secs []Sector
	for _, sc := range st.Sectors {
		if sc.Start < end {
			sc.End = min(sc.End, end)
			secs = append(secs, sc)
		}
	}
	st.Sectors = secs
}
