package simulation

import (
	"testing"

	"cycling_sim/internal/model"
)

func TestCarbShareAndBonk(t *testing.T) {
	if carbShare(0.6) != 0.4 || carbShare(1.2) != 1 || carbShare(0.75) != 0.55 {
		t.Fatalf("carb share table: %.2f %.2f %.2f", carbShare(0.6), carbShare(1.2), carbShare(0.75))
	}
	rs := stateFor(1, "A", 400, 68, RoleDomestique)
	// Sitting in at 70% of threshold for 6 h: no bonk.
	for i := 0; i < 360; i++ {
		rs.burnFuel(280, 60, 400)
	}
	if rs.FuelShare() < 0.3 {
		t.Errorf("6 h at 70%%: fuel %.2f, should not bonk", rs.FuelShare())
	}
	// Driving at 90% for 4 h: empty.
	rs2 := stateFor(2, "A", 400, 68, RoleDomestique)
	for i := 0; i < 240; i++ {
		rs2.burnFuel(360, 60, 400)
	}
	if rs2.FuelShare() > 0.05 || fuelFactorAt(rs2.FuelShare()) > 0.72 {
		t.Errorf("4 h at 90%%: fuel %.2f factor %.2f, should be empty", rs2.FuelShare(), fuelFactorAt(rs2.FuelShare()))
	}
}

func TestBunchSurgePower(t *testing.T) {
	g := &Group{}
	for i := 1; i <= 50; i++ {
		rs := stateFor(int64(i), "A", 400, 68, RoleDomestique)
		rs.GroupPosition = i
		g.Riders = append(g.Riders, rs)
	}
	flat := &model.Segment{GradientPct: 0}
	v := 46 / 3.6
	front, back := bunchSurgePower(g.Riders[0], g, flat, v), bunchSurgePower(g.Riders[49], g, flat, v)
	if front < 10 || front > 30 || back < 55 || back > 95 {
		t.Errorf("surge power front %.0f W (want ~20), back %.0f W (want ~75)", front, back)
	}
	if bunchSurgePower(g.Riders[49], g, &model.Segment{GradientPct: 6}, 5) != 0 {
		t.Error("no bunch surges on a climb")
	}
	g.LinedOut = true
	if bunchSurgePower(g.Riders[49], g, flat, v) != 0 {
		t.Error("a strung-out line doesn't surge like a bunch")
	}
}
