package simulation

import (
	"strings"
	"testing"
)

// The reserve's trend and its main cause come from the recent window.
func TestReserveWhy(t *testing.T) {
	rs := &RiderState{WPrime: &WPrimeState{WPrimeMax: 20000, WPrimeCurrent: 20000}}
	for i := 0; i < 10; i++ { // a minute on a climb, a few surges
		rs.noteReserve(causeClimb, -300, 6)
		rs.noteReserve(causeSurges, -50, 0)
	}
	perMin, why := rs.reserveWhy()
	if perMin >= -0.5 || !strings.Contains(why, "climb") {
		t.Errorf("draining on the climb: %.1f%%/min %q", perMin, why)
	}
	for i := 0; i < 60; i++ { // six minutes refilling on the descent
		rs.noteReserve(causeWheel, 120, 6)
	}
	if perMin, why := rs.reserveWhy(); perMin <= 0 || why != "" {
		t.Errorf("refilling: %.1f%%/min %q", perMin, why)
	}
}
