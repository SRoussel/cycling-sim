package simulation

import (
	"testing"

	"cycling_sim/internal/database"
)

// TestRunFCValidationSuiteNilDB ensures a nil database is handled gracefully.
func TestRunFCValidationSuiteNilDB(t *testing.T) {
	results := RunFCValidationSuite(nil)
	if results != nil {
		t.Errorf("expected nil when db is nil, got %v", results)
	}
}

// TestRunFCValidationSuiteEmptyDB ensures an empty DB returns nil (no FC data).
func TestRunFCValidationSuiteEmptyDB(t *testing.T) {
	db, err := database.OpenMemory()
	if err != nil {
		t.Fatalf("failed to open in-memory DB: %v", err)
	}
	results := RunFCValidationSuite(db)
	if results != nil {
		t.Errorf("expected nil for empty DB, got %d results", len(results))
	}
}

// TestKnownFCRacesNotEmpty verifies the static lookup table is populated.
func TestKnownFCRacesNotEmpty(t *testing.T) {
	races := KnownFCRaces()
	if len(races) == 0 {
		t.Fatal("KnownFCRaces() returned empty slice")
	}
	for _, r := range races {
		if r.FCRaceID <= 0 {
			t.Errorf("race %q has invalid FCRaceID %d", r.InternalName, r.FCRaceID)
		}
		if r.InternalName == "" {
			t.Errorf("race with FCRaceID %d has empty InternalName", r.FCRaceID)
		}
	}
}

// TestFCPassThresholdsReasonable checks that the threshold constants are within
// sensible bounds so an accidental edit is caught immediately.
func TestFCPassThresholdsReasonable(t *testing.T) {
	if FCPassTop10Target < 5 || FCPassTop10Target > 80 {
		t.Errorf("FCPassTop10Target=%v outside reasonable range [5, 80]", FCPassTop10Target)
	}
	if FCPassGapMAE < 60 || FCPassGapMAE > 7200 {
		t.Errorf("FCPassGapMAE=%v outside reasonable range [60, 7200]", FCPassGapMAE)
	}
	if FCMinMatchedRiders < 1 || FCMinMatchedRiders > 20 {
		t.Errorf("FCMinMatchedRiders=%d outside reasonable range [1, 20]", FCMinMatchedRiders)
	}
}
