package simulation

import (
	"fmt"
	"math"
	"sort"
	"strings"
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
	RiderID   int64
	// Gaps: his time behind the winner (GC leader) in each run he finished
	// (Runs - len(Gaps) runs he abandoned); for the misses diagnostic.
	Gaps []float64 `json:"-"`
}

// withGaps attaches each rider's per-run gaps to his point.
func withGaps(pts []CalibPoint, gaps map[int64][]float64) []CalibPoint {
	for i := range pts {
		pts[i].Gaps = gaps[pts[i].RiderID]
	}
	return pts
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
			In10: inReal[id], Won: id == winner && winner > 0, Runs: runs, RiderID: id,
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

// Race history (Nov 2026): race character is judged against a race's long
// run (data/history_results.csv, about ten editions), not one edition.

// HistoryRace is one race's past editions.
type HistoryRace struct {
	Name        string
	Gap2        []float64 // gap of 2nd place per edition (s): the winning margin, or GC 2nd
	Gap10       []float64 // gap of 10th place per edition (s)
	Solo, Bunch int       // one-day editions won solo (2nd 5 s+ down) / with the whole top 10 together
}

// LoadHistory reads data/history_results.csv by race name.
func LoadHistory(path string) (map[string]*HistoryRace, error) {
	rows, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	out := map[string]*HistoryRace{}
	for _, row := range rows[1:] { // Kind,Race,Year,Winner,Gap2,Within5,Gap10
		if len(row) < 7 {
			continue
		}
		h := out[row[1]]
		if h == nil {
			h = &HistoryRace{Name: row[1]}
			out[row[1]] = h
		}
		var g2, g10, within float64
		fmt.Sscan(row[4], &g2)
		fmt.Sscan(row[6], &g10)
		h.Gap2 = append(h.Gap2, g2)
		h.Gap10 = append(h.Gap10, g10)
		if row[0] == "oneday" {
			fmt.Sscan(row[5], &within)
			if g2 >= finishSoloS {
				h.Solo++
			} else if within >= 10 {
				h.Bunch++
			}
		}
	}
	return out, nil
}

// HistoryName is the race name a reference result goes by in the history
// ("Tour of Flanders 2026" and "Giro d'Italia 2026 (GC)" are "Tour of
// Flanders" and "Giro d'Italia").
func HistoryName(display string) string {
	s := strings.TrimSuffix(display, " (GC)")
	f := strings.Fields(s)
	if len(f) > 1 && len(f[len(f)-1]) == 4 && strings.HasPrefix(f[len(f)-1], "20") {
		s = strings.Join(f[:len(f)-1], " ")
	}
	return s
}

// Range is the median and the min-max of xs.
func Range(xs []float64) (med, lo, hi float64) {
	if len(xs) == 0 {
		return 0, 0, 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return medianOf(s), s[0], s[len(s)-1]
}
