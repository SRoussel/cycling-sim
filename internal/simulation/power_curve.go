package simulation

import (
	"math"

	"cycling_sim/internal/model"
)

// PowerCurve is a rider's fresh maximal power (W) for each duration (s) as
// the race model sees it, on flat roads: threshold (CP) plus the anaerobic
// reserve W' spent evenly over the effort (Morton's two-parameter model,
// CP + W'/t), capped by the short-effort ceiling (powerCeiling: the sprint
// fading towards maximal aerobic power); over long efforts threshold and W'
// fade with the work done (DurabilityFactor, Endurance), solved as a fixed
// point. Terrain abilities (climbing, time trial, cobbles), seasonal form
// and fuel aren't applied.
func PowerCurve(r *model.Rider, seconds []float64) []float64 {
	sprint := SprintPower(r)
	cp := float64(r.FTPWatts)
	wPrime := NewWPrimeState(r).WPrimeMax
	out := make([]float64, len(seconds))
	for i, t := range seconds {
		if t <= 0 {
			continue
		}
		p := cp + wPrime/t
		for k := 0; k < 30; k++ {
			d := DurabilityFactor(p*t/1000, r.Endurance)
			next := (cp + wPrime/t) * d
			if math.Abs(next-p) < 0.01 {
				p = next
				break
			}
			p = next
		}
		if sprint > 0 {
			p = math.Min(p, math.Max(cp, powerCeiling(cp, sprint, punchGap(r), t)))
		}
		out[i] = p
	}
	return out
}
