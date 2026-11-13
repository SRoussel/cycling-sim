package simulation

import "math"

// Short-effort ceiling (Nov 2026). Above threshold, a rider could spend his
// whole W' over any effort (threshold + W'/t), so over 30 s a climber with a
// big threshold and reserve out-produced a sprinter (Vingegaard 19.3 W/kg
// against Philipsen's 16.7). Real maximal power falls from the sprint towards
// maximal aerobic power within a minute or two (the phosphocreatine and
// glycolytic part fades), so efforts are also capped by
//
//	MAP + (sprint - MAP)·exp(-(t - 5 s)/τ),  MAP = threshold·(1 + punch gap),
//
// the peak sprint being a ~5 s figure, with τ = 40 s: a sprinter (1900 W
// peak) gets ~1260 W over 30 s and ~860 W over a minute, a climber (1250 W)
// ~910 W and ~680 W, and at 5 min and
// beyond the ceiling is about MAP (~8 W/kg for the best climbers), where W'
// spread over the effort is the limit anyway.
const (
	sprintDecaySeconds = 40.0
	peakSprintSeconds  = 5.0 // how long the peak sprint figure lasts
)

// powerCeiling is the most a rider can produce for an effort of seconds
// from his threshold on this terrain, peak sprint and punch gap.
func powerCeiling(threshold, sprint, gap, seconds float64) float64 {
	mapW := threshold * (1 + gap)
	if sprint < mapW {
		sprint = mapW
	}
	return mapW + (sprint-mapW)*math.Exp(-math.Max(0, seconds-peakSprintSeconds)/sprintDecaySeconds)
}

// maxPowerFor is what a rider can ride for seconds starting from base (at
// least his threshold on this terrain): base plus his W' left spread over the
// effort, but no more than his ceiling for that duration (never below base).
func (rs *RiderState) maxPowerFor(threshold, base, seconds float64) float64 {
	p := base + rs.WPrime.AvailablePowerAboveFTP(seconds)
	c := powerCeiling(threshold, FatiguedSprintPower(rs), punchGap(rs.Rider), seconds)
	return math.Min(p, math.Max(base, c))
}
