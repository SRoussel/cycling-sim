package calibrate

import (
	"math"
	"path/filepath"
	"testing"

	"cycling_sim/internal/model"
	"cycling_sim/internal/seed"
	"cycling_sim/internal/simulation"
)

func TestRiderStep(t *testing.T) {
	cases := []struct {
		name  string
		c     simulation.RiderCheck
		stage bool
		want  float64
		ok    bool
	}{
		{"too slow one-day", simulation.RiderCheck{RealPos: 2, RealGap: 60, SimGap: 180, SimPos: 9}, false, 1, true},
		{"too fast stage", simulation.RiderCheck{RealPos: 3, RealGap: 900, SimGap: 600, SimPos: 2}, true, -1, true},
		{"capped", simulation.RiderCheck{RealPos: 2, RealGap: 0, SimGap: 3000, SimPos: 40}, false, maxStepPct, true},
		{"position only", simulation.RiderCheck{RealPos: 5, RealGap: -1, SimPos: 15}, false, 0.5, true},
		{"never finished", simulation.RiderCheck{RealPos: 5, RealGap: 30, SimPos: 522}, false, 0, false},
	}
	for _, c := range cases {
		got, ok := riderStep(c.c, c.stage, false)
		if ok != c.ok || math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: step %.2f (%v), want %.2f (%v)", c.name, got, ok, c.want, c.ok)
		}
	}
	// With the wide bound (a prior), a one-day winner's bigger margin still
	// lowers him by at most the per-race step; a loser's error is not capped.
	wide := []struct {
		name string
		c    simulation.RiderCheck
		want float64
	}{
		{"winner by far more", simulation.RiderCheck{RealPos: 1, RealGap: 0, SimGap: -600, SimPos: 1}, -maxStepPct},
		{"loser too fast", simulation.RiderCheck{RealPos: 2, RealGap: 600, SimGap: 0, SimPos: 1}, -5},
	}
	for _, c := range wide {
		got, ok := riderStep(c.c, false, true)
		if !ok || math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: step %.2f (%v), want %.2f", c.name, got, ok, c.want)
		}
	}
}

func TestUnfitAndWriteRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fitted.csv")
	adj := []Adjustment{{RiderID: 1, Name: "Tadej Pogačar", Scale: 1.05, Evidence: []string{"Tour +1.0%"}}}
	if err := Write(path, adj); err != nil {
		t.Fatal(err)
	}
	layer, err := seed.LoadOverrides(path)
	if err != nil {
		t.Fatal(err)
	}
	// Applying the layer scales the threshold.
	r := model.Rider{ID: 1, Name: "Tadej Pogačar", FTPWatts: 400}
	for _, o := range layer {
		if err := o.Apply(&r); err != nil {
			t.Fatal(err)
		}
	}
	if r.FTPWatts != 420 {
		t.Fatalf("fitted FTP %d, want 420", r.FTPWatts)
	}
}

// A known place far behind the sim counts no more than an intruder; one
// ahead of it counts fully.
func TestKnownPlaceStep(t *testing.T) {
	behind := simulation.RiderCheck{RealPos: 91, RealGap: 3600, SimPos: 8, SimGap: 30, Top10Pct: 60}
	s, ok := knownPlaceStep(behind, true, true)
	if !ok || s != -intruderPenaltyPct*0.6 {
		t.Errorf("far behind: got %.2f, want %.2f", s, -intruderPenaltyPct*0.6)
	}
	ahead := simulation.RiderCheck{RealPos: 12, RealGap: 20, SimPos: 30, SimGap: 200, Top10Pct: 5}
	want, _ := riderStep(ahead, false, true)
	if s, _ := knownPlaceStep(ahead, false, true); want <= 0 || s != want {
		t.Errorf("ahead: got %.2f, want riderStep %.2f > 0", s, want)
	}
}

// A rider's tail starts where his correction first turns back; one still
// travelling keeps the last half.
func TestRiderTail(t *testing.T) {
	seq := func(vs ...float64) []map[int64]float64 {
		var h []map[int64]float64
		for _, v := range vs {
			h = append(h, map[int64]float64{1: v})
		}
		return h
	}
	got := riderTail(seq(1.00, 0.985, 0.975, 0.978, 0.974, 0.977, 0.975, 0.976), 1)
	if len(got) != 6 || got[0] != 0.975 {
		t.Errorf("turning back at pass 3: tail %v, want from 0.975 (6 passes)", got)
	}
	got = riderTail(seq(1.00, 0.99, 0.98, 0.97, 0.96, 0.95), 1)
	if len(got) != 3 {
		t.Errorf("still travelling: tail %v, want the last half", got)
	}
	if _, _, se := tailStats([]float64{1, 1, 1}); se != 0 {
		t.Errorf("constant tail: se %v, want 0", se)
	}
}
