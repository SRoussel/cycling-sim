package simulation

import "testing"

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
