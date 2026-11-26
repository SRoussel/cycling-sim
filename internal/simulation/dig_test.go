package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

// On a climb a rider a little weaker than the pace holds the wheel at any
// cost, lets it go on his own pace, and with a reserve to keep holds only
// while he has more than it.
func TestDigLevels(t *testing.T) {
	seg := &model.Segment{DistanceKm: 1, GradientPct: 7, SurfaceType: model.SurfaceRoad}
	split := func(dig Dig, reserve float64) int {
		strong := testRider()
		strong.ID, strong.FTPWatts = 1, 430
		weak := testRider()
		weak.ID, weak.FTPWatts = 2, 400
		a := &RiderState{Rider: strong, WPrime: NewWPrimeState(strong), FormFactor: 1, EffectiveFTP: 430}
		b := &RiderState{Rider: weak, WPrime: NewWPrimeState(weak), FormFactor: 1, EffectiveFTP: 400}
		b.WPrime.WPrimeCurrent = reserve * b.WPrime.WPrimeMax
		rps := []riderPower{
			{state: a, power: 430, action: ActionChase},
			{state: b, power: 400, dig: dig},
		}
		return len(followSplit(rps, seg, 1, true, true))
	}
	if n := split(DigAnyCost, 1); n != 1 {
		t.Fatalf("at any cost he holds the wheel: %d groups", n)
	}
	if n := split(DigOwnPace, 1); n != 2 {
		t.Errorf("on his own pace he lets it go: %d groups", n)
	}
	if n := split(DigKeepReserve, 1); n != 1 {
		t.Errorf("keeping a reserve, a full one still lets him hold on: %d groups", n)
	}
	if n := split(DigKeepReserve, 0.32); n != 2 {
		t.Errorf("keeping a reserve, near his floor he lets it go: %d groups", n)
	}
}
