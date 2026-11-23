package simulation

import (
	"math"
	"sort"
)

// Calibration (Nov 2026): a real race is one draw from what could have
// happened, so the simulator is judged on whether its probabilities are
// honest, not on reproducing the result. Every starter's simulated chances
// (top 10, win) are set against what he did; over many riders and races a
// 30% chance should come true about 30% of the time (reliability), and the
// probability given to the real winner is a proper score (log score) that
// rewards being both sharp and honest.

// CalibPoint is one starter in one reference race.
type CalibPoint struct {
	P10, PWin float64 // simulated chance of a top 10 / of winning
	In10, Won bool    // what he did
	Runs      int     // simulations behind the chances
}

// calibPoints builds the field's points: every rider who started in the
// runs (seen), with his top-10 and win counts over runs, against the real
// top 10 (IDs in order; 0 = a rider not in the database).
func calibPoints(seen map[int64]int, top10, wins map[int64]int, runs int, real []int64) []CalibPoint {
	if runs <= 0 {
		return nil
	}
	inReal := map[int64]bool{}
	for _, id := range real {
		if id > 0 {
			inReal[id] = true
		}
	}
	winner := int64(0)
	if len(real) > 0 {
		winner = real[0]
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]CalibPoint, 0, len(ids))
	for _, id := range ids {
		out = append(out, CalibPoint{
			P10:  float64(top10[id]) / float64(runs),
			PWin: float64(wins[id]) / float64(runs),
			In10: inReal[id], Won: id == winner && winner > 0, Runs: runs,
		})
	}
	return out
}

// ReliabilityBin is one band of predicted chance and how often it came true.
type ReliabilityBin struct {
	Lo, Hi    float64
	N         int
	Predicted float64 // mean predicted chance in the band
	Actual    float64 // share that happened
}

// reliabilityEdges bands top-10 chances.
var reliabilityEdges = []float64{0, 0.02, 0.1, 0.25, 0.5, 0.75, 1.0001}

// Reliability bins the points' top-10 chances (or win chances with win).
func Reliability(points []CalibPoint, win bool) []ReliabilityBin {
	bins := make([]ReliabilityBin, len(reliabilityEdges)-1)
	for i := range bins {
		bins[i].Lo, bins[i].Hi = reliabilityEdges[i], math.Min(1, reliabilityEdges[i+1])
	}
	for _, p := range points {
		pr, hit := p.P10, p.In10
		if win {
			pr, hit = p.PWin, p.Won
		}
		for i := range bins {
			if pr >= reliabilityEdges[i] && pr < reliabilityEdges[i+1] {
				bins[i].N++
				bins[i].Predicted += pr
				if hit {
					bins[i].Actual++
				}
			}
		}
	}
	for i := range bins {
		if bins[i].N > 0 {
			bins[i].Predicted /= float64(bins[i].N)
			bins[i].Actual /= float64(bins[i].N)
		}
	}
	return bins
}

// WinnerLogScore is the mean log2 probability the simulation gave the real
// winners (smoothed: a winner the runs never produced gets half a run), and
// the same for a field-blind guess (1/field size) for scale.
func WinnerLogScore(races [][]CalibPoint) (score, blind float64, n int) {
	for _, pts := range races {
		for _, p := range pts {
			if !p.Won {
				continue
			}
			pr := (p.PWin*float64(p.Runs) + 0.5) / (float64(p.Runs) + 1)
			score += math.Log2(pr)
			blind += math.Log2(1 / float64(len(pts)))
			n++
		}
	}
	if n > 0 {
		score /= float64(n)
		blind /= float64(n)
	}
	return score, blind, n
}

// Brier10 is the mean squared error of top-10 chances over the points.
func Brier10(points []CalibPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	var s float64
	for _, p := range points {
		y := 0.0
		if p.In10 {
			y = 1
		}
		s += (p.P10 - y) * (p.P10 - y)
	}
	return s / float64(len(points))
}
