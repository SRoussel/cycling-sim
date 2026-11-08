package simulation

import (
	"math/rand"
	"testing"

	"cycling_sim/internal/model"
)

// A pure sprinter should beat a puncheur with equal FTP in most 1v1 sprints,
// but not every time — sprint outcomes must retain randomness.
func TestSprintFinish_SpecialistBeatsPuncheurMostly(t *testing.T) {
	sprinter := &model.Rider{
		ID: 1, Name: "Sprinter",
		WeightKg: 76, HeightCm: 181, FTPWatts: 385, MaxSprintWatts: 1850,
		SprintAbility: 97, ClimbingAbility: 45, TTAbility: 60,
		DescendingAbility: 70, Endurance: 72, Handling: 70, CdA: 0.34,
	}
	puncheur := &model.Rider{
		ID: 2, Name: "Puncheur",
		WeightKg: 69, HeightCm: 178, FTPWatts: 385, MaxSprintWatts: 1700,
		SprintAbility: 84, ClimbingAbility: 82, TTAbility: 75,
		DescendingAbility: 70, Endurance: 85, Handling: 78, CdA: 0.31,
	}
	if SprintPower(sprinter) <= SprintPower(puncheur) {
		t.Fatalf("sprinter power %.0fW should exceed puncheur %.0fW", SprintPower(sprinter), SprintPower(puncheur))
	}

	base := func(r *model.Rider) RiderState {
		return RiderState{Rider: r, EffectiveFTP: 385, FormFactor: 1.0, WPrime: NewWPrimeState(r)}
	}
	rng := rand.New(rand.NewSource(42))
	const n = 400
	wins := 0
	for i := 0; i < n; i++ {
		s, p := base(sprinter), base(puncheur)
		SprintFinish([]*RiderState{&s, &p}, rng)
		if s.TotalTime < p.TotalTime {
			wins++
		}
	}
	rate := float64(wins) / n
	t.Logf("sprinter 1v1 win rate: %.0f%%", rate*100)
	if rate < 0.55 || rate > 0.95 {
		t.Errorf("sprinter win rate %.0f%% outside expected 55-95%%", rate*100)
	}
}
