package stagefile

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cycling_sim/internal/model"
)

// Source maps a seeded course to a stage file name (in the user's stage dir).
type Source struct {
	Course string
	File   string
	Note   string
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
	Cobbles     bool    `json:"cobbles,omitempty"`
}

// ModelSegments returns the profile's segments as model segments.
func (p Profile) ModelSegments() []model.Segment {
	out := make([]model.Segment, len(p.Segments))
	for i, s := range p.Segments {
		surface := model.SurfaceRoad
		if s.Cobbles {
			surface = model.SurfaceCobbles
		}
		out[i] = model.Segment{SegmentOrder: i + 1, Name: s.Name, DistanceKm: s.DistanceKm,
			GradientPct: s.GradientPct, SurfaceType: surface, IsTechnical: s.Cobbles}
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
		segs, err := Segments(st, km)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", s.Course, err))
			continue
		}
		p := Profile{Course: s.Course, Stage: st.Name, File: s.File}
		for _, sg := range segs {
			p.Segments = append(p.Segments, SegmentJSON{Name: sg.Name, DistanceKm: sg.DistanceKm,
				GradientPct: sg.GradientPct, Cobbles: sg.SurfaceType == model.SurfaceCobbles})
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
