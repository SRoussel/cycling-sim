// Package calibrate fits rider thresholds to real results by running the
// simulator itself: riders who finish too far behind their real result get a
// little more threshold, riders who finish too far ahead (or crowd a top 10
// they weren't in) a little less, and the loop repeats until it settles.
//
// It fits only 2024 references (by default the Tour GC); the 2023 hold-out
// shows whether the fitted ratings generalise or merely memorise.
package calibrate

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/simulation"
)

// Options controls a fit.
type Options struct {
	Iterations int     // passes of simulate → adjust
	Runs       int     // simulations per one-day race per pass
	StageRuns  int     // simulations per stage race per pass
	MaxChange  float64 // largest total correction, e.g. 0.08 = ±8%
	// Classics also fits to the one-day references. Off by default: with one
	// or two results per rider it fits 2024 but makes the 2023 hold-out worse.
	Classics bool
	Log      func(format string, args ...any)
}

// DefaultOptions are a reasonable trade-off between noise and run time.
func DefaultOptions() Options {
	return Options{Iterations: 12, Runs: 20, StageRuns: 4, MaxChange: 0.08}
}

// Step sizes: how many seconds of gap error ask for a 1% threshold change.
// A one-day race is decided over ~30-60 min of hard racing (1% ≈ 20-40 s); a
// Grand Tour over many hours of climbing (1% ≈ a few minutes).
const (
	oneDaySecsPerPct   = 120.0
	stageSecsPerPct    = 300.0
	placesPerPct       = 20.0  // when only the position is known
	maxStepPct         = 1.5   // per pass and race
	intruderPenaltyPct = 0.5   // per pass, scaled by how often they intrude
	settledPct         = 0.1   // stop when the mean step falls below this
	damping            = 0.7   // fraction of the requested step taken (damps run-to-run noise)
	missingSimPos      = 500.0 // simulated position beyond this: rider never finished
)

// Adjustment is a fitted threshold correction for one rider.
type Adjustment struct {
	RiderID  int64
	Name     string
	Scale    float64 // multiplies the threshold from the earlier rating layers
	WKg      float64 // fitted threshold W/kg
	Evidence []string
	// Implausible is set when the fit pushes a rider above the strongest
	// unfitted rider's W/kg: the fit is then compensating for something the
	// model gets wrong (tactics, role), not measuring the rider.
	Implausible bool
}

// Fit fits threshold scales for riders in the 2024 references. base must be
// the riders before any fitted correction (see Unfit).
func Fit(db *database.DB, base []model.Rider, opt Options) ([]Adjustment, error) {
	if len(base) == 0 {
		return nil, fmt.Errorf("no riders: run 'cycling-sim seed' first")
	}
	logf := opt.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	scale := map[int64]float64{}
	evidence := map[int64][]string{}
	for it := 1; it <= opt.Iterations; it++ {
		riders := applyScales(base, scale)
		results := simulation.RunRealStageRaceValidationFor(db, riders, simulation.KnownRealStageRaces(), opt.StageRuns)
		if opt.Classics {
			results = append(results, simulation.RunRealRaceValidationFor(db, riders, simulation.KnownRealRaces(), opt.Runs)...)
		}
		steps := map[int64][]float64{}
		ev := map[int64][]string{}
		var mae float64
		for _, res := range results {
			mae += res.MeanTimeGapErrorSecs
			stage := strings.Contains(res.Name, "(GC)")
			for _, c := range res.Riders {
				if s, ok := riderStep(c, stage); ok {
					steps[c.RiderID] = append(steps[c.RiderID], s)
					ev[c.RiderID] = append(ev[c.RiderID], fmt.Sprintf("%s %+.1f%%", res.Name, s))
				}
			}
			for _, c := range res.Intruders {
				s := -intruderPenaltyPct * c.Top10Pct / 100
				steps[c.RiderID] = append(steps[c.RiderID], s)
				ev[c.RiderID] = append(ev[c.RiderID], fmt.Sprintf("%s intruder %+.1f%%", res.Name, s))
			}
		}
		var total float64
		for id, ss := range steps {
			mean := 0.0
			for _, s := range ss {
				mean += s / float64(len(ss))
			}
			total += math.Abs(mean)
			cur := scale[id]
			if cur == 0 {
				cur = 1
			}
			scale[id] = clamp(cur*(1+damping*mean/100), 1-opt.MaxChange, 1+opt.MaxChange)
			evidence[id] = ev[id]
		}
		meanStep := 0.0
		if len(steps) > 0 {
			meanStep = total / float64(len(steps))
		}
		logf("pass %d: mean gap error %.0fs over %d cases, %d riders adjusted, mean step %.2f%%\n",
			it, mae/float64(max(1, len(results))), len(results), len(steps), meanStep)
		if meanStep < settledPct {
			break
		}
	}
	byID := map[int64]model.Rider{}
	maxWKg := 0.0
	for _, r := range base {
		byID[r.ID] = r
		if r.WeightKg > 0 {
			maxWKg = math.Max(maxWKg, float64(r.FTPWatts)/r.WeightKg)
		}
	}
	var out []Adjustment
	for id, s := range scale {
		if math.Abs(s-1) < 0.002 {
			continue
		}
		r := byID[id]
		a := Adjustment{RiderID: id, Name: r.Name, Scale: s, Evidence: evidence[id]}
		if r.WeightKg > 0 {
			a.WKg = float64(r.FTPWatts) * s / r.WeightKg
			a.Implausible = a.WKg > maxWKg && s > 1
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// riderStep is the threshold change (%) one race asks of a rider: from the
// gap error when the real gap is known, else from the position error.
func riderStep(c simulation.RiderCheck, stage bool) (float64, bool) {
	if c.SimPos > missingSimPos {
		return 0, false // never finished in the simulation: no usable signal
	}
	if c.RealGap >= 0 {
		per := oneDaySecsPerPct
		if stage {
			per = stageSecsPerPct
		}
		return clamp((c.SimGap-c.RealGap)/per, -maxStepPct, maxStepPct), true
	}
	if c.RealPos > 0 {
		return clamp((c.SimPos-float64(c.RealPos))/placesPerPct, -1, 1), true
	}
	return 0, false
}

func applyScales(base []model.Rider, scale map[int64]float64) []model.Rider {
	out := make([]model.Rider, len(base))
	copy(out, base)
	for i := range out {
		if s, ok := scale[out[i].ID]; ok {
			out[i].FTPWatts = int(math.Round(float64(out[i].FTPWatts) * s))
		}
	}
	return out
}

// Unfit removes previously applied fitted scales (name → scale) from riders,
// so a new fit starts from the curated/derived ratings, not compounding.
func Unfit(riders []model.Rider, previous map[string]float64) []model.Rider {
	out := make([]model.Rider, len(riders))
	copy(out, riders)
	for i := range out {
		if s, ok := previous[strings.ToLower(strings.Join(strings.Fields(out[i].Name), " "))]; ok && s > 0 {
			out[i].FTPWatts = int(math.Round(float64(out[i].FTPWatts) / s))
		}
	}
	return out
}

// Write saves adjustments in the rider-overrides format (FTP_scale column).
func Write(path string, adj []Adjustment) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "# Threshold corrections fitted by `cycling-sim fit-ratings` to 2024 reference")
	fmt.Fprintln(f, "# results (see the Source column). Applied last at seed time, as a multiplier on the")
	fmt.Fprintln(f, "# threshold from riders.csv / rider_derived.csv / rider_overrides.csv.")
	fmt.Fprintln(f, "# Regenerated on each fit; the 2023 hold-out in `cycling-sim validate` checks it.")
	w := csv.NewWriter(f)
	if err := w.Write([]string{"Name", "FTP_scale", "Source"}); err != nil {
		return err
	}
	for _, a := range adj {
		src := "fitted: " + strings.Join(a.Evidence, "; ")
		if err := w.Write([]string{a.Name, fmt.Sprintf("%.4f", a.Scale), src}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
