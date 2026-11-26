package stagefile

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"cycling_sim/internal/model"
)

// fixture builds a minimal stage file in the format this package reads: a
// header, a name, a profile and a sector list (Latin-1 name included).
func fixture(t *testing.T, profile [][2]float32, sectors []Sector) []byte {
	t.Helper()
	var b bytes.Buffer
	w := func(v any) { binary.Write(&b, binary.LittleEndian, v) }
	str := func(s []byte) { w(int32(len(s))); b.Write(s) }
	w(int32(1))
	str([]byte("CTStageData"))
	w([]byte{1, 0, 0, 0, 1, 0, 0, 0})
	str([]byte("Start - Finish;"))
	w(int32(7))
	w([]float32{12.5, 30})
	w(int32(len(profile)))
	for _, p := range profile {
		w(p)
	}
	w(int32(len(sectors)))
	for _, s := range sectors {
		name := []byte(s.Name)
		if s.Name == "Quérénaing" {
			name = []byte{'Q', 'u', 0xE9, 'r', 0xE9, 'n', 'a', 'i', 'n', 'g'} // Latin-1
		}
		str(name)
		w(int32(s.Stars))
		w([]float32{float32(s.Start), float32(s.End)})
	}
	return b.Bytes()
}

// A 10,000-unit stage (100 km): flat, then a 10 km climb at 8% from km 60,
// then flat; one cobbled sector on the flat. Altitude is in 7.5 m units.
func testStage(t *testing.T) []byte {
	var prof [][2]float32
	for d := 0; d <= 10000; d += 20 {
		alt := 10.0
		switch {
		case d > 6000 && d <= 7000:
			alt += 0.08 * float64(d-6000) * DistanceUnitM / AltitudeUnitM
		case d > 7000:
			alt += 80 * DistanceUnitM / AltitudeUnitM
		}
		prof = append(prof, [2]float32{float32(d), float32(alt)})
	}
	return fixture(t, prof, []Sector{{Name: "Quérénaing", Stars: 4, Start: 2000, End: 2300}})
}

func TestParse(t *testing.T) {
	st, err := Parse(testStage(t))
	if err != nil {
		t.Fatal(err)
	}
	if st.Name != "Start - Finish" {
		t.Errorf("name %q", st.Name)
	}
	if len(st.Profile) != 501 || st.Length() != 10000 {
		t.Errorf("profile %d points, length %.0f", len(st.Profile), st.Length())
	}
	if len(st.Sectors) != 1 || st.Sectors[0].Name != "Quérénaing" || st.Sectors[0].Stars != 4 {
		t.Errorf("sectors %+v", st.Sectors)
	}
	if _, err := Parse([]byte("not a stage file")); err == nil {
		t.Error("expected an error for a non-stage file")
	}
}

func TestSegmentsPreserveGradientAndDistance(t *testing.T) {
	st, _ := Parse(testStage(t))
	segs, err := Segments(st, 100) // 10,000 units → 100 km: 10 m per unit
	if err != nil {
		t.Fatal(err)
	}
	total, cob, steepest := 0.0, 0.0, 0.0
	for _, s := range segs {
		total += s.DistanceKm
		if s.SurfaceType == model.SurfaceCobbles {
			cob += s.DistanceKm
		}
		steepest = math.Max(steepest, s.GradientPct)
	}
	if math.Abs(total-100) > 0.01 {
		t.Errorf("segments sum to %.2f km, want 100", total)
	}
	if math.Abs(cob-3) > 0.01 {
		t.Errorf("cobbles %.2f km, want 3", cob)
	}
	if math.Abs(steepest-8) > 0.1 {
		t.Errorf("steepest %.1f%%, want ~8%%", steepest)
	}
	if c := ClimbingM(segs); math.Abs(c-800) > 10 {
		t.Errorf("climbing %.0f m, want ~800", c)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	st, _ := Parse(testStage(t))
	segs, _ := Segments(st, 100)
	p := Profile{Course: "Test Classic", Stage: st.Name}
	for _, s := range segs {
		p.Segments = append(p.Segments, SegmentJSON{Name: s.Name, DistanceKm: s.DistanceKm, GradientPct: s.GradientPct, Cobbles: s.SurfaceType == model.SurfaceCobbles})
	}
	path := filepath.Join(t.TempDir(), "local", "profiles.json")
	if err := Save(path, []Profile{p}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ms := got["Test Classic"].ModelSegments()
	if len(ms) != len(segs) || ms[0].SegmentOrder != 1 {
		t.Fatalf("round trip lost segments: %d vs %d", len(ms), len(segs))
	}
	if missing, err := Load(filepath.Join(t.TempDir(), "none.json")); err != nil || len(missing) != 0 {
		t.Error("a missing profiles file should load as empty")
	}
}

func TestLoadSourcesAltitudeScale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.csv")
	os.WriteFile(path, []byte("Course,File,Note\nA,a.cds,\nB,b.cds,too tall,,0.8\n"), 0o644)
	src, err := LoadSources(path)
	if err != nil || len(src) != 2 {
		t.Fatalf("%v %+v", err, src)
	}
	if src[0].AltScale != 0 || src[1].AltScale != 0.8 {
		t.Errorf("altitude scales %v, %v; want 0 (none) and 0.8", src[0].AltScale, src[1].AltScale)
	}
}

func TestCutEnd(t *testing.T) {
	st := &Stage{Profile: []Point{{0, 0}, {100, 10}, {200, 30}, {300, 0}}, Sectors: []Sector{{Name: "a", Start: 50, End: 120}, {Name: "b", Start: 260, End: 280}}}
	cutEnd(st, 150) // end at 150
	if st.Length() != 150 || st.Profile[len(st.Profile)-1].Alt != 20 {
		t.Errorf("profile %v, want ending at 150 (alt 20)", st.Profile)
	}
	if len(st.Sectors) != 1 || st.Sectors[0].End != 120 {
		t.Errorf("sectors %v, want only the first", st.Sectors)
	}
}
