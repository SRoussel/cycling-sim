package simulation

import "testing"

func TestWilson95(t *testing.T) {
	// 4 of 10: roughly 17-69%; 0 of 10 still allows up to ~28%.
	lo, hi := Wilson95(4, 10)
	if lo < 0.15 || lo > 0.19 || hi < 0.66 || hi > 0.71 {
		t.Errorf("4 of 10: %.2f-%.2f, want ~0.17-0.69", lo, hi)
	}
	if lo, hi := Wilson95(0, 10); lo != 0 || hi < 0.26 || hi > 0.30 {
		t.Errorf("0 of 10: %.2f-%.2f, want 0-~0.28", lo, hi)
	}
}
