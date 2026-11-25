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

	// References, when set, replace the 2024 ones (KnownRealStageRaces, and
	// KnownRealRaces with Classics): the races to fit (Nov 2026: every 2026
	// reference, one-day and GC).
	OneDay, Stage []simulation.RealRaceEntry
	// Weight scales a reference's evidence by display name (default 1):
	// races run on an approximate course count less.
	Weight map[string]float64
	// Prior (ridge) counts the base rating as this many races asking for no
	// change: a rider's correction is the sum of his races' steps over
	// (their weight + Prior), pulled back towards the base each pass, so a
	// rider seen once moves less than one seen in five races. 0 = off (the
	// plain mean of the old fit).
	Prior float64
	// Start: corrections to begin from (the installed fit), by rider ID. The
	// prior still pulls towards the base, so the target is unchanged; a fit
	// that starts near its answer just needs fewer passes.
	Start map[int64]float64
	// MinPasses before the fit may stop on a settled average (with a prior).
	MinPasses int
}

// settledAvgPct: with a prior the fit stops once the running average of the
// last half of the passes moves less than this per pass for the mean rider
// (and settledAvgMaxPct for any rider): noise keeps single passes swinging
// (some rider at the step limit every pass), the average settles.
const (
	settledAvgPct    = 0.05
	settledAvgMaxPct = 0.3
)

// DefaultOptions are a reasonable trade-off between noise and run time (runs
// within a pass use all CPUs). With 4 stage-race runs a pass, the Tour gap
// error swung 140-460 s between passes; 24 runs keep it steady.
func DefaultOptions() Options {
	// ±5%: with race-to-race form in the model (RaceFormFactor), a bigger
	// correction is more likely a rider's lucky (or unlucky) race than his
	// rating (Martínez's and Hirt's 2024 Giros pinned them at the old ±8%).
	return Options{Iterations: 12, Runs: 40, StageRuns: 24, MaxChange: 0.05}
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
	settledPct         = 0.1   // stop when the mean step falls below this...
	settledMaxPct      = 0.5   // ...and no rider still asks for more than this
	maxCorrectionPct   = 10.0  // one race's error, with a prior (see riderStep)
	damping            = 0.7   // fraction of the requested step taken (damps run-to-run noise)
	missingSimPos      = 500.0 // simulated position beyond this: rider never finished
	plausibleMarginPct = 2.0   // tolerance above the strongest rider (fit noise)
)

// Adjustment is a fitted threshold correction for one rider.
type Adjustment struct {
	RiderID  int64
	Name     string
	Scale    float64 // multiplies the threshold from the earlier rating layers
	WKg      float64 // fitted threshold W/kg
	ClimbWKg float64 // fitted W/kg on a climb (climbing ability applied)
	Evidence []string
	// Spread: the scale's standard deviation over the averaged passes (how
	// firmly the data pin the rider down; 0 without averaging).
	Spread float64
	// Implausible is set when the fit pushes a rider more than
	// plausibleMarginPct above the strongest unfitted rider's climbing W/kg:
	// the fit is then compensating for something the model gets wrong
	// (tactics, role), not measuring the rider.
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
	for id, v := range opt.Start {
		scale[id] = v
	}
	evidence := map[int64][]string{}
	// history holds each pass's scales: with a prior the result is the
	// average of the last half of the passes (Polyak averaging), since each
	// pass judges riders on fresh random runs and the last one alone keeps
	// whichever swing came last (Nov 2026: a 20-pass fit ended still taking
	// 0.3% steps).
	var history []map[int64]float64
	for it := 1; it <= opt.Iterations; it++ {
		riders := applyScales(base, scale)
		var results []simulation.ValidationSummary
		if opt.Stage != nil || opt.OneDay != nil {
			results = simulation.RunRealStageRaceValidationFor(db, riders, opt.Stage, opt.StageRuns)
			results = append(results, simulation.RunRealRaceValidationFor(db, riders, opt.OneDay, opt.Runs)...)
		} else {
			results = simulation.RunRealStageRaceValidationFor(db, riders, simulation.KnownRealStageRaces(), opt.StageRuns)
			if opt.Classics {
				results = append(results, simulation.RunRealRaceValidationFor(db, riders, simulation.KnownRealRaces(), opt.Runs)...)
			}
		}
		steps := map[int64][]float64{}
		weights := map[int64][]float64{}
		ev := map[int64][]string{}
		var mae float64
		for _, res := range results {
			mae += res.MeanTimeGapErrorSecs
			stage := strings.Contains(res.Name, "(GC)")
			w := 1.0
			if v, ok := opt.Weight[res.Name]; ok {
				w = v
			}
			for _, c := range res.Riders {
				if s, ok := riderStep(c, stage, opt.Prior > 0); ok {
					steps[c.RiderID] = append(steps[c.RiderID], s)
					weights[c.RiderID] = append(weights[c.RiderID], w)
					ev[c.RiderID] = append(ev[c.RiderID], fmt.Sprintf("%s %+.1f%%", res.Name, s))
				}
			}
			for _, c := range res.Intruders {
				// Real place known (saved full results): judged like anyone.
				if c.RealPos > 0 && c.RealGap >= 0 {
					if s, ok := riderStep(c, stage, opt.Prior > 0); ok {
						steps[c.RiderID] = append(steps[c.RiderID], s)
						weights[c.RiderID] = append(weights[c.RiderID], w)
						ev[c.RiderID] = append(ev[c.RiderID], fmt.Sprintf("%s %dth %+.1f%%", res.Name, c.RealPos, s))
					}
					continue
				}
				s := -intruderPenaltyPct * c.Top10Pct / 100
				steps[c.RiderID] = append(steps[c.RiderID], s)
				weights[c.RiderID] = append(weights[c.RiderID], w)
				ev[c.RiderID] = append(ev[c.RiderID], fmt.Sprintf("%s intruder %+.1f%%", res.Name, s))
			}
		}
		var total, largest float64
		for id, ss := range steps {
			cur := scale[id]
			if cur == 0 {
				cur = 1
			}
			// Weighted mean step; with a prior, the base counts as Prior
			// races asking to return to it (ridge on the log scale).
			sum, wsum := 0.0, 0.0
			for i, s := range ss {
				sum += s * weights[id][i]
				wsum += weights[id][i]
			}
			mean := sum / wsum
			if opt.Prior > 0 {
				mean = (sum - opt.Prior*100*math.Log(cur)) / (wsum + opt.Prior)
			}
			mean = clamp(mean, -maxStepPct, maxStepPct) // rate limit per pass
			total += math.Abs(mean)
			// With a prior, the bound shrinks with the evidence as the prior
			// does (MaxChange × w/(w+Prior)): a rider whose error never
			// responds to his rating (he works for a leader; Cepeda's one
			// Romandie) would otherwise sit at the full bound on one race.
			bound := opt.MaxChange
			if opt.Prior > 0 {
				bound *= wsum / (wsum + opt.Prior)
			}
			next := clamp(cur*(1+damping*mean/100), 1-bound, 1+bound)
			if next == cur && math.Abs(next-1) >= bound-1e-9 {
				// Pinned at the bound and still asking: nothing left to fit
				// (the rider's error isn't his rating); don't wait on him.
				total -= math.Abs(mean)
			} else {
				largest = math.Max(largest, math.Abs(mean))
			}
			scale[id] = next
			evidence[id] = ev[id]
		}
		meanStep := 0.0
		if len(steps) > 0 {
			meanStep = total / float64(len(steps))
		}
		logf("pass %d: mean gap error %.0fs over %d cases, %d riders adjusted, mean step %.2f%%, largest %.2f%%\n",
			it, mae/float64(max(1, len(results))), len(results), len(steps), meanStep, largest)
		// A rider far off keeps asking for big steps while the many riders
		// near their rating pull the mean down: settle on both.
		snap := make(map[int64]float64, len(scale))
		for id, v := range scale {
			snap[id] = v
		}
		history = append(history, snap)
		if meanStep < settledPct && largest < settledMaxPct {
			break
		}
		if opt.Prior > 0 && opt.MinPasses > 0 && len(history) >= opt.MinPasses {
			now, before := tailMean(history), tailMean(history[:len(history)-1])
			var sum, most float64
			for id, v := range now {
				d := math.Abs(v-before[id]) * 100
				sum += d
				most = math.Max(most, d)
			}
			mean := sum / float64(max(1, len(now)))
			logf("  average moved %.3f%% (largest %.2f%%)\n", mean, most)
			if mean < settledAvgPct && most < settledAvgMaxPct {
				logf("average settled after %d passes\n", len(history))
				break
			}
		}
	}
	spread := map[int64]float64{}
	if opt.Prior > 0 && len(history) >= 4 {
		tail := history[len(history)/2:]
		avg := map[int64]float64{}
		for id := range scale {
			var sum, sq float64
			for _, h := range tail {
				v := h[id]
				if v == 0 {
					v = 1
				}
				sum += v
				sq += v * v
			}
			n := float64(len(tail))
			m := sum / n
			avg[id] = m
			spread[id] = math.Sqrt(math.Max(0, sq/n-m*m))
		}
		scale = avg
		logf("averaged the last %d passes (Polyak)\n", len(tail))
	}
	climb := func(r model.Rider, ftp float64) float64 {
		return ftp * simulation.ModifierFactor(r.ClimbingAbility) / r.WeightKg
	}
	byID := map[int64]model.Rider{}
	maxClimb := 0.0
	for _, r := range base {
		byID[r.ID] = r
		if r.WeightKg > 0 {
			maxClimb = math.Max(maxClimb, climb(r, float64(r.FTPWatts)))
		}
	}
	var out []Adjustment
	for id, s := range scale {
		if math.Abs(s-1) < 0.002 {
			continue
		}
		s = math.Round(s*1e4) / 1e4 // the precision Write stores, so seeding reproduces it
		r := byID[id]
		a := Adjustment{RiderID: id, Name: r.Name, Scale: s, Evidence: evidence[id], Spread: spread[id]}
		if r.WeightKg > 0 {
			a.WKg = float64(r.FTPWatts) * s / r.WeightKg
			a.ClimbWKg = climb(r, float64(r.FTPWatts)*s)
			a.Implausible = a.ClimbWKg > maxClimb*(1+plausibleMarginPct/100) && s > 1
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// riderStep is the threshold change (%) one race asks of a rider: from the
// gap error when the real gap is known, else from the position error.
//
// With a prior (wide) the race's error is kept whole (up to the fit's
// bounds): a per-race cap would cap the correction itself, since the prior
// balances the capped error; the rate limit then applies to the combined
// step.
func riderStep(c simulation.RiderCheck, stage, wide bool) (float64, bool) {
	if c.SimPos > missingSimPos {
		return 0, false // never finished in the simulation: no usable signal
	}
	if c.RealGap >= 0 {
		per := oneDaySecsPerPct
		if stage {
			per = stageSecsPerPct
		}
		lim := maxStepPct
		if wide {
			lim = maxCorrectionPct
		}
		step := clamp((c.SimGap-c.RealGap)/per, -lim, lim)
		// A one-day winner's real margin is one draw (Flanders 2026: 34 s,
		// history 0-62 s): winning by more in the simulation lowers him by
		// at most the per-race step, not the wide bound. Nov 2026: one
		// margin cut Pogačar's threshold 5.2%, which also weakened him on
		// the long climbs the single scale can't tell apart.
		if !stage && c.RealPos == 1 && step < -maxStepPct {
			step = -maxStepPct
		}
		return step, true
	}
	if c.RealPos > 0 {
		return clamp((c.SimPos-float64(c.RealPos))/placesPerPct, -1, 1), true
	}
	return 0, false
}

// tailMean is each rider's mean scale over the last half of the passes (1
// for a rider never adjusted).
func tailMean(history []map[int64]float64) map[int64]float64 {
	tail := history[len(history)/2:]
	ids := map[int64]bool{}
	for _, h := range tail {
		for id := range h {
			ids[id] = true
		}
	}
	out := make(map[int64]float64, len(ids))
	for id := range ids {
		sum := 0.0
		for _, h := range tail {
			v := h[id]
			if v == 0 {
				v = 1
			}
			sum += v
		}
		out[id] = sum / float64(len(tail))
	}
	return out
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

// Write saves adjustments in the rider-overrides format (FTP_scale column).
func Write(path string, adj []Adjustment) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "# Threshold corrections fitted by `cycling-sim fit-ratings` to reference results")
	fmt.Fprintln(f, "# (see the Source column; `fit-ratings --season=2026`: every 2026 reference, ridge")
	fmt.Fprintln(f, "# prior, bound shrinking with evidence, averaged over the last half of the passes).")
	fmt.Fprintln(f, "# Applied last at seed time, as a multiplier on the threshold from the earlier")
	fmt.Fprintln(f, "# layers and the WorldDB rebuild. Regenerated on each fit.")
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
