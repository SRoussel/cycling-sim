package simulation

// validation_fc.go — Sprint 4 validation: wire FirstCycling reference data as ground truth.
//
// How it works:
//  1. KnownFCRaces() maps FC race IDs (from firstcycling.com) to the internal race
//     name exactly as seeded by internal/seed/seed.go.
//  2. RunFCValidationSuite(db) scans what FC data is in the DB, finds matching internal
//     races, runs N simulations, and reports top-10 overlap and time-gap MAE metrics.
//  3. The FC cases are informational — they don't block the synthetic release gate, but a
//     "release ready" banner in the UI requires all synthetic tests AND any available FC
//     tests to pass.
//
// Pass thresholds (generous because field composition differs from real life):
//   Top-10 overlap  ≥ FCPassTop10Target  (a low bar; ≥1 matched rider in sim top-10)
//   Time-gap MAE    ≤ FCPassGapMAE       seconds
//   Matched riders  ≥ FCMinMatchedRiders (skip gracefully if too few matched)

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
)

const (
	// FCPassTop10Target is the minimum percentage of matched FC top-10 riders
	// that must also appear in the simulated top-10 for the test to pass.
	FCPassTop10Target = 20.0 // 20% — 2 out of 10 matched riders

	// FCPassGapMAE is the maximum acceptable mean-absolute-error (in seconds)
	// between FC reference time-gaps and simulated time-gaps for matched riders.
	FCPassGapMAE = 600.0 // 10 minutes

	// FCMinMatchedRiders is the minimum number of matched riders before a validation
	// case is considered meaningful. Cases below this emit a "too few matches" summary.
	FCMinMatchedRiders = 3

	// FCValidationRuns is the number of simulation runs used for FC validation.
	FCValidationRuns = 10
)

// FCRaceInfo is a single entry in the well-known FC race lookup table.
type FCRaceInfo struct {
	// FCRaceID is the firstcycling.com race identifier (the "r" param in the URL).
	FCRaceID int
	// InternalName is the exact internal race name as seeded by seed.go.
	InternalName string
	// DisplayName is the human-readable name shown in the UI.
	DisplayName string
}

// KnownFCRaces returns a lookup table of well-known one-day classic FC race IDs
// and their corresponding internal race names. Racing stage IDs (stage>0) are handled
// separately and not included here since internal stage numbers don't map to FC stages.
//
// FC race IDs sourced from firstcycling.com URL: race.php?r=<ID>
func KnownFCRaces() []FCRaceInfo {
	return []FCRaceInfo{
		{FCRaceID: 71, InternalName: "Paris-Roubaix", DisplayName: "Paris-Roubaix"},
		{FCRaceID: 74, InternalName: "Milan-San Remo", DisplayName: "Milan-San Remo"},
		{FCRaceID: 9, InternalName: "Tour of Flanders", DisplayName: "Tour of Flanders"},
		{FCRaceID: 7, InternalName: "Liege-Bastogne-Liege", DisplayName: "Liège-Bastogne-Liège"},
		{FCRaceID: 22, InternalName: "Il Lombardia", DisplayName: "Il Lombardia"},
		{FCRaceID: 1363, InternalName: "Strade Bianche", DisplayName: "Strade Bianche"},
	}
}

// RunFCValidationSuite scans the DB for available FC reference data and runs validation
// for every known race that has both FC results stored and a matching internal race.
// If db is nil or the fc_race_results table is empty, it returns an empty slice.
func RunFCValidationSuite(db *database.DB) []ValidationSummary {
	if db == nil {
		return nil
	}

	// Discover which (fc_race_id, year, stage) combinations are already fetched.
	fetched, err := db.ListFCFetched()
	if err != nil {
		return nil
	}
	if len(fetched) == 0 {
		return nil // no FC data at all; caller can note this in the UI
	}

	// Build a quick lookup set of what FC data is present keyed by fc_race_id.
	// We pick the most recent year available for each race ID.
	type fcKey struct {
		fcRaceID int
		year     int
		stage    int
	}
	latestByRace := make(map[int]fcKey) // fc_race_id → best (year, stage=0 preferred)
	for _, s := range fetched {
		if s.Stage != 0 {
			// Skip stage-level results; we only compare one-day race finishes.
			continue
		}
		if prev, ok := latestByRace[s.FCRaceID]; !ok || s.Year > prev.year {
			latestByRace[s.FCRaceID] = fcKey{s.FCRaceID, s.Year, s.Stage}
		}
	}

	// Load all internal races once.
	internalRaces, err := db.ListRaces()
	if err != nil {
		return nil
	}

	var results []ValidationSummary
	for _, info := range KnownFCRaces() {
		key, hasFCData := latestByRace[info.FCRaceID]
		caseName := fmt.Sprintf("%s %d (FC)", info.DisplayName, 0)
		if hasFCData {
			caseName = fmt.Sprintf("%s %d (FC)", info.DisplayName, key.year)
		}

		if !hasFCData {
			// Emit a "data not fetched" informational entry.
			results = append(results, ValidationSummary{
				Name:       caseName,
				IsRealRace: true,
				Passed:     false,
				Details: fmt.Sprintf(
					"FC data not fetched. Run: fc-fetch --race %d --year <year>",
					info.FCRaceID,
				),
			})
			continue
		}

		// Find the matching internal race.
		internalRace := findInternalRaceExact(internalRaces, info.InternalName)
		if internalRace == nil {
			results = append(results, ValidationSummary{
				Name:       caseName,
				IsRealRace: true,
				Passed:     false,
				Details: fmt.Sprintf(
					"No internal race named %q found. Seed the database first.",
					info.InternalName,
				),
			})
			continue
		}

		summary, err := runOneFCValidation(db, internalRace, key.fcRaceID, key.year, key.stage, caseName)
		if err != nil {
			results = append(results, ValidationSummary{
				Name:       caseName,
				IsRealRace: true,
				Passed:     false,
				Details:    fmt.Sprintf("Validation failed: %v", err),
			})
			continue
		}
		results = append(results, summary)
	}
	return results
}

// findInternalRaceExact finds an internal race by exact name, then falls back to
// case-insensitive substring matching.
func findInternalRaceExact(races []model.Race, name string) *model.Race {
	lowerName := strings.ToLower(name)
	// Exact match first.
	for i, r := range races {
		if r.Name == name {
			return &races[i]
		}
	}
	// Case-insensitive substring fallback.
	for i, r := range races {
		if strings.Contains(strings.ToLower(r.Name), lowerName) {
			return &races[i]
		}
	}
	return nil
}

// runOneFCValidation runs FCValidationRuns simulations of a single race, compares the
// averaged results to FC reference data, and returns a ValidationSummary.
func runOneFCValidation(
	db *database.DB,
	race *model.Race,
	fcRaceID, year, stage int,
	caseName string,
) (ValidationSummary, error) {
	// Load FC reference results (joined with fc_rider_map for InternalRiderID).
	fcRows, err := db.ListFCResults(fcRaceID, year, stage)
	if err != nil {
		return ValidationSummary{}, fmt.Errorf("ListFCResults(%d/%d/%d): %w", fcRaceID, year, stage, err)
	}
	if len(fcRows) == 0 {
		return ValidationSummary{
			Name:       caseName,
			IsRealRace: true,
			Details:    "FC data present but row count is 0",
		}, nil
	}

	// Collect only riders with a confirmed internal match.
	type matchedEntry struct {
		internalID int64
		fcPosition int
		fcGapSecs  float64
	}
	var matched []matchedEntry
	for _, r := range fcRows {
		if r.InternalRiderID > 0 {
			matched = append(matched, matchedEntry{
				internalID: r.InternalRiderID,
				fcPosition: r.Position,
				fcGapSecs:  float64(r.GapSecs),
			})
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return matched[i].fcPosition < matched[j].fcPosition
	})

	if len(matched) < FCMinMatchedRiders {
		return ValidationSummary{
			Name:          caseName,
			IsRealRace:    true,
			MatchedRiders: len(matched),
			Details: fmt.Sprintf(
				"Only %d riders matched (need ≥%d). Run: fc-match to link FC riders.",
				len(matched), FCMinMatchedRiders,
			),
		}, nil
	}

	// Load internal course + full rider pool.
	course, err := db.GetCourse(race.CourseID)
	if err != nil {
		return ValidationSummary{}, fmt.Errorf("GetCourse(%d): %w", race.CourseID, err)
	}
	allRiders, err := db.ListRiders()
	if err != nil {
		return ValidationSummary{}, fmt.Errorf("ListRiders: %w", err)
	}

	// Run the simulation FCValidationRuns times, accumulating position and gap sums.
	posSum := make(map[int64]float64)
	posCnt := make(map[int64]int)
	gapSum := make(map[int64]float64)
	gapCnt := make(map[int64]int)

	selCfg := selection.DefaultConfig(race.RaceType, false, 0)
	for run := 0; run < FCValidationRuns; run++ {
		riders := selection.SelectStartList(allRiders, course, race, selCfg)
		cfg := RaceConfig{
			Race:   race,
			Course: course,
			Riders: riders,
			Seed:   int64(run + 1),
		}
		var simResults []model.RaceResult
		if race.RaceType == model.RaceTypeTT {
			simResults = SimulateTT(cfg)
		} else {
			simResults = SimulateRoadRace(cfg)
		}
		for _, res := range simResults {
			posSum[res.RiderID] += float64(res.Position)
			posCnt[res.RiderID]++
			gapSum[res.RiderID] += res.TimeGapSeconds
			gapCnt[res.RiderID]++
		}
	}

	avgPos := func(id int64) float64 {
		if posCnt[id] == 0 {
			return float64(len(allRiders) + 1)
		}
		return posSum[id] / float64(posCnt[id])
	}
	avgGap := func(id int64) float64 {
		if gapCnt[id] == 0 {
			return 0
		}
		return gapSum[id] / float64(gapCnt[id])
	}

	// ── Metric 1: Spearman correlation (matched riders only) ─────────────────
	expectedOrder := make([]int64, len(matched))
	avgPosMap := make(map[int64]float64, len(matched))
	for i, m := range matched {
		expectedOrder[i] = m.internalID
		avgPosMap[m.internalID] = avgPos(m.internalID)
	}
	corr := validationSpearman(expectedOrder, avgPosMap)

	// ── Metric 2: Top-10 overlap ─────────────────────────────────────────────
	// Riders who finished in FC top-10.
	fcTop10 := make(map[int64]bool)
	for _, m := range matched {
		if m.fcPosition <= 10 {
			fcTop10[m.internalID] = true
		}
	}
	overlapCount := 0
	for id := range fcTop10 {
		if avgPos(id) <= 10 {
			overlapCount++
		}
	}
	var top10Pct float64
	if len(fcTop10) > 0 {
		top10Pct = float64(overlapCount) / float64(len(fcTop10)) * 100
	}

	// ── Metric 3: Time-gap MAE ────────────────────────────────────────────────
	var gapMAESum float64
	for _, m := range matched {
		gapMAESum += math.Abs(m.fcGapSecs - avgGap(m.internalID))
	}
	gapMAE := gapMAESum / float64(len(matched))

	// ── Pass/fail ────────────────────────────────────────────────────────────
	passed := top10Pct >= FCPassTop10Target && gapMAE <= FCPassGapMAE

	details := fmt.Sprintf(
		"Matched %d riders | Corr %.2f | Top-10 overlap %.0f%% (≥%.0f%%) | Gap MAE %.0fs (≤%.0fs)",
		len(matched), corr, top10Pct, FCPassTop10Target, gapMAE, FCPassGapMAE,
	)

	return ValidationSummary{
		Name:                 caseName,
		Passed:               passed,
		Correlation:          corr,
		Top10Accuracy:        top10Pct,
		MeanTimeGapErrorSecs: gapMAE,
		MatchedRiders:        len(matched),
		IsRealRace:           true,
		Details:              details,
	}, nil
}
