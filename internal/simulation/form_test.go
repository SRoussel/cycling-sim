package simulation

import (
	"testing"
	"time"

	"cycling_sim/internal/model"
)

func TestSeasonForm(t *testing.T) {
	climber := model.Rider{Name: "Climber", FTPWatts: 430, WeightKg: 62, ClimbingAbility: 88, SprintAbility: 55, Handling: 60}
	puncheur := model.Rider{Name: "Puncheur", FTPWatts: 395, WeightKg: 63, ClimbingAbility: 84, SprintAbility: 70, Handling: 72}
	if specialtyOf(&climber) != specClimber || specialtyOf(&puncheur) != specPuncheur {
		t.Fatalf("specialties: climber %v puncheur %v", specialtyOf(&climber), specialtyOf(&puncheur))
	}
	liege := time.Date(2024, 4, 21, 0, 0, 0, 0, time.UTC)
	tour := time.Date(2024, 7, 14, 0, 0, 0, 0, time.UTC)

	if f := SeasonForm(&climber, liege); f > 0.975 || f < 1-offPeakLoss {
		t.Errorf("a Grand Tour climber at Liège: form %.3f, want well below peak", f)
	}
	if f := SeasonForm(&climber, tour); f < 0.999 {
		t.Errorf("a Grand Tour climber at the Tour: form %.3f, want peak", f)
	}
	if f := SeasonForm(&puncheur, liege); f < 0.999 {
		t.Errorf("a puncheur at Liège: form %.3f, want peak", f)
	}
	if f := SeasonForm(&climber, time.Time{}); f != 1 {
		t.Errorf("no date (one-off race): form %.3f, want 1", f)
	}
}
