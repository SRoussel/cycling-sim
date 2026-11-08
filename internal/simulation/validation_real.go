package simulation

// validation_real.go — Real-race validation using hardcoded reference results.
//
// Results are sourced from publicaly available 2024 season placings.
// Only riders present in the internal database (non-zero IDs) are used.
//
// Rider ID quick-reference (from data/riders.csv):
//
//	 28 = Mathieu van der Poel   429 = Wout van Aert      452 = Tadej Pogačar
//	190 = Mads Pedersen          336 = Jasper Stuyven      60 = Tiesj Benoot
//	338 = Dylan van Baarle        17 = Jasper Philipsen    14 = Kaden Groves
//	269 = Biniam Girmay          184 = Jonathan Milan     294 = Remco Evenepoel
//	306 = Primož Roglič          431 = Jonas Vingegaard   150 = Filippo Ganna
//	156 = Michał Kwiatkowski      47 = Matej Mohorič      492 = Davide Ballerini

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
	// realPassTop10Target is the minimum % of known top-10 riders that must also
	// appear (on average across runs) in the simulated top-10 for the case to pass.
	realPassTop10Target = 40.0

	// realPassGapMAE is the maximum acceptable mean-absolute-error (seconds)
	// between reference and simulated time gaps, over riders with a known gap.
	realPassGapMAE = 120.0

	// realMinMatchedRiders is the minimum number of known-ID riders before the
	// validation case is considered meaningful.
	realMinMatchedRiders = 3

	// realValidationRuns is the number of simulation runs (seeds) per race.
	realValidationRuns = 20
)

// gapUnknown marks a reference slot whose time gap is not known.
const gapUnknown = -1

// RealRaceEntry holds a hardcoded reference top-10 for a known race.
type RealRaceEntry struct {
	// DisplayName is the human-readable label shown in the UI.
	DisplayName string
	// InternalName must exactly match the race name as seeded in the database.
	InternalName string
	// Top10RiderIDs lists internal rider IDs in 1st-through-10th finishing order.
	// Use 0 for a rider not in the internal DB or a place not verified; those slots are skipped.
	Top10RiderIDs []int64
	// Top10GapSecs holds the real gap to the winner for each slot (gapUnknown if not known).
	Top10GapSecs []float64
}

// KnownRealRaces returns 2024 reference results for the six seeded classics.
//
// Verified November 2026 against ProCyclingStats, Cyclingnews and CyclingStage
// race reports. Places that sources disagreed on are left as 0 / gapUnknown.
// Riders not in data/riders.csv (e.g. Mozzato, Pidcock, Bardet, Teuns) are 0.
func KnownRealRaces() []RealRaceEntry {
	const u = gapUnknown
	return []RealRaceEntry{
		{
			DisplayName:  "Strade Bianche 2024",
			InternalName: "Strade Bianche",
			// 1 Pogačar  2 Skujiņš +2:44  3 Van Gils +2:47  4 Pidcock*  8 L.Martinez  9 Zana  10 Laporte
			Top10RiderIDs: []int64{452, 193, 312, 0, 0, 0, 0, 44, 347, 421},
			Top10GapSecs:  []float64{0, 164, 167, u, u, u, u, 288, 289, 317},
		},
		{
			DisplayName:  "Milan-San Remo 2024",
			InternalName: "Milan-San Remo",
			// Bunch sprint: 1 Philipsen  2 Matthews  3 Pogačar  4 Pedersen  7 Van Gils  8 Stuyven  10 van der Poel
			Top10RiderIDs: []int64{17, 366, 452, 190, 0, 0, 312, 336, 0, 28},
			Top10GapSecs:  []float64{0, 0, 0, 0, u, u, 0, 0, u, 0},
		},
		{
			DisplayName:  "Tour of Flanders 2024",
			InternalName: "Tour of Flanders",
			// 1 van der Poel solo; group at +1:02: 2 Mozzato*  3 Politt  4 Bjerg  5 Morgado  6 Sheffield  7 Naesen  9 Bettiol
			Top10RiderIDs: []int64{28, 0, 453, 436, 446, 163, 78, 0, 493, 0},
			Top10GapSecs:  []float64{0, 62, 62, 62, 62, 62, 62, 62, 62, 62},
		},
		{
			DisplayName:  "Paris-Roubaix 2024",
			InternalName: "Paris-Roubaix",
			// 1 van der Poel solo  2 Philipsen +3:00  3 Pedersen  4 Politt  6 G.Vermeersch +3:47  9 Wærenskjold  10 Mihkels
			Top10RiderIDs: []int64{28, 17, 190, 453, 0, 314, 0, 0, 491, 101},
			Top10GapSecs:  []float64{0, 180, 180, 180, u, 227, u, u, u, u},
		},
		{
			DisplayName:  "Liege-Bastogne-Liege 2024",
			InternalName: "Liege-Bastogne-Liege",
			// 1 Pogačar  2 Bardet* +1:39  3 van der Poel +2:02  4 Van Gils  5 A.Paret-Peintre  6 Vansevenant  8 Lutsenko  9 Bilbao
			Top10RiderIDs: []int64{452, 0, 28, 312, 79, 344, 0, 274, 33, 0},
			Top10GapSecs:  []float64{0, 99, 122, 122, 122, 122, 122, 122, 122, 122},
		},
		{
			DisplayName:  "Il Lombardia 2024",
			InternalName: "Il Lombardia",
			// 1 Pogačar  2 Evenepoel +3:16  3 Ciccone +4:31  8 Powless +4:58  9 Gaudu +4:58
			Top10RiderIDs: []int64{452, 294, 177, 0, 0, 0, 0, 103, 125, 0},
			Top10GapSecs:  []float64{0, 196, 271, u, u, u, u, 298, 298, 298},
		},
	}
}

// RunRealRaceValidation runs simulations for each known real race and compares
// outcomes against hardcoded reference finishing orders.
// Returns nil if db is nil or the database has no riders.
func RunRealRaceValidation(db *database.DB) []ValidationSummary {
	if db == nil {
		return nil
	}

	allRiders, err := db.ListRiders()
	if err != nil || len(allRiders) == 0 {
		return nil
	}

	// Build a quick ID→exists set.
	riderExists := make(map[int64]bool, len(allRiders))
	for _, r := range allRiders {
		riderExists[r.ID] = true
	}

	internalRaces, err := db.ListRaces()
	if err != nil {
		return nil
	}

	var results []ValidationSummary
	for _, entry := range KnownRealRaces() {
		race := findRaceByName(internalRaces, entry.InternalName)
		if race == nil {
			results = append(results, ValidationSummary{
				Name:       entry.DisplayName,
				IsRealRace: true,
				Passed:     false,
				Details:    fmt.Sprintf("Race %q not found. Run 'seed' first.", entry.InternalName),
			})
			continue
		}

		// Collect matched rider IDs (those that exist in the DB).
		type matchedEntry struct {
			riderID int64
			refPos  int
		}
		var matched []matchedEntry
		for i, id := range entry.Top10RiderIDs {
			if id > 0 && riderExists[id] {
				matched = append(matched, matchedEntry{riderID: id, refPos: i + 1})
			}
		}

		if len(matched) < realMinMatchedRiders {
			results = append(results, ValidationSummary{
				Name:          entry.DisplayName,
				IsRealRace:    true,
				Passed:        false,
				MatchedRiders: len(matched),
				Details: fmt.Sprintf(
					"Only %d known riders in DB (need ≥%d).",
					len(matched), realMinMatchedRiders,
				),
			})
			continue
		}

		course, err := db.GetCourse(race.CourseID)
		if err != nil {
			results = append(results, ValidationSummary{
				Name:       entry.DisplayName,
				IsRealRace: true,
				Passed:     false,
				Details:    fmt.Sprintf("GetCourse: %v", err),
			})
			continue
		}

		// Run simulations and accumulate average positions.
		selCfg := selection.DefaultConfig(race.RaceType, false, 0)
		posSum := make(map[int64]float64)
		posCnt := make(map[int64]int)
		gapSum := make(map[int64]float64)
		gapCnt := make(map[int64]int)
		wins := make(map[int64]int)

		for run := 0; run < realValidationRuns; run++ {
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
				if res.Position == 1 {
					wins[res.RiderID]++
				}
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

		// ── Metric 1: Spearman correlation on matched riders ─────────────────
		sort.Slice(matched, func(i, j int) bool {
			return matched[i].refPos < matched[j].refPos
		})
		expectedOrder := make([]int64, len(matched))
		avgPosMap := make(map[int64]float64, len(matched))
		for i, m := range matched {
			expectedOrder[i] = m.riderID
			avgPosMap[m.riderID] = avgPos(m.riderID)
		}
		corr := validationSpearman(expectedOrder, avgPosMap)

		// ── Metric 2: Top-10 overlap ─────────────────────────────────────────
		refTop10 := make(map[int64]bool)
		for _, m := range matched {
			if m.refPos <= 10 {
				refTop10[m.riderID] = true
			}
		}
		overlapCount := 0
		for id := range refTop10 {
			if avgPos(id) <= 10 {
				overlapCount++
			}
		}
		var top10Pct float64
		if len(refTop10) > 0 {
			top10Pct = float64(overlapCount) / float64(len(refTop10)) * 100
		}

		// ── Metric 3: Time-gap MAE vs real gaps ──────────────────────────────
		var gapMAESum float64
		gapN := 0
		for _, m := range matched {
			ref := entry.Top10GapSecs[m.refPos-1]
			if ref == gapUnknown {
				continue
			}
			gapMAESum += math.Abs(avgGap(m.riderID) - ref)
			gapN++
		}
		gapMAE := 0.0
		if gapN > 0 {
			gapMAE = gapMAESum / float64(gapN)
		}

		// ── Metric 4: Winner match rate ──────────────────────────────────────
		winnerPct := 0.0
		if w := entry.Top10RiderIDs[0]; w > 0 && realValidationRuns > 0 {
			winnerPct = float64(wins[w]) / float64(realValidationRuns) * 100
		}

		passed := top10Pct >= realPassTop10Target && (gapN == 0 || gapMAE <= realPassGapMAE)

		details := fmt.Sprintf(
			"Matched %d | Corr %.2f | Top-10 overlap %.0f%% (≥%.0f%%) | Gap MAE %.0fs over %d (≤%.0fs) | Real winner wins %.0f%%",
			len(matched), corr, top10Pct, realPassTop10Target, gapMAE, gapN, realPassGapMAE, winnerPct,
		)

		results = append(results, ValidationSummary{
			Name:                 entry.DisplayName,
			Passed:               passed,
			Correlation:          corr,
			Top10Accuracy:        top10Pct,
			MeanTimeGapErrorSecs: gapMAE,
			MatchedRiders:        len(matched),
			IsRealRace:           true,
			Details:              details,
		})
	}
	return results
}

// findRaceByName finds an internal race by exact name, then falls back to
// case-insensitive substring matching.
func findRaceByName(races []model.Race, name string) *model.Race {
	lower := strings.ToLower(name)
	for i, r := range races {
		if r.Name == name {
			return &races[i]
		}
	}
	for i, r := range races {
		if strings.Contains(strings.ToLower(r.Name), lower) {
			return &races[i]
		}
	}
	return nil
}
