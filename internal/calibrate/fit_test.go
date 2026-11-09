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
		got, ok := riderStep(c.c, c.stage)
		if ok != c.ok || math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: step %.2f (%v), want %.2f (%v)", c.name, got, ok, c.want, c.ok)
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
