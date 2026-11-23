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
	// realPassTop10Target is the minimum % of known top-10 riders that, on
	// average per run, also finish in the simulated top-10.
	realPassTop10Target = 40.0

	// realPassGapMAE is the maximum acceptable mean-absolute-error (seconds)
	// between reference and simulated time gaps, over riders with a known gap.
	realPassGapMAE = 120.0

	// realMinMatchedRiders is the minimum number of known-ID riders before the
	// validation case is considered meaningful.
	realMinMatchedRiders = 3

	// realValidationRuns is the number of simulation runs (seeds) per race.
	realValidationRuns = 60
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
	// DidNotStart lists riders in the DB who didn't ride the real race; they
	// are removed from the simulated start list so the field is comparable.
	DidNotStart []int64
	// StartList, when set, is the real field: only DB riders on it start
	// (names matched case- and space-insensitively), each for the team they
	// rode for then (today's rosters differ).
	StartList []Starter
}

// Starter is a rider on a reference race's real start list.
type Starter struct {
	Name, Team string
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
			InternalName: "Strade Bianche 2024", // the 215 km 2024 route
			// 1 Pogačar  2 Skujiņš +2:44  3 Van Gils +2:47  4 Pidcock*  8 L.Martinez  9 Zana  10 Laporte
			Top10RiderIDs: []int64{452, 193, 312, 0, 0, 0, 0, 44, 347, 421},
			Top10GapSecs:  []float64{0, 164, 167, u, u, u, u, 288, 289, 317},
			// Not on the start list (CyclingStage rider list; Cycling Weekly for
			// van der Poel and van Aert).
			StartList: strade2024Field, // the real start list (validation_fields.go)
		},
		{
			DisplayName:  "Milan-San Remo 2024",
			InternalName: "Milan-San Remo",
			// Bunch sprint: 1 Philipsen  2 Matthews  3 Pogačar  4 Pedersen  7 Van Gils  8 Stuyven  10 van der Poel
			Top10RiderIDs: []int64{17, 366, 452, 190, 0, 0, 312, 336, 0, 28},
			Top10GapSecs:  []float64{0, 0, 0, 0, u, u, 0, 0, u, 0},
			StartList:     msr2024Field, // the real start list (validation_fields.go)
		},
		{
			DisplayName:  "Tour of Flanders 2024",
			InternalName: "Tour of Flanders",
			// 1 van der Poel solo; group at +1:02: 2 Mozzato*  3 Politt  4 Bjerg  5 Morgado  6 Sheffield  7 Naesen  9 Bettiol
			Top10RiderIDs: []int64{28, 0, 453, 436, 446, 163, 78, 0, 493, 0},
			Top10GapSecs:  []float64{0, 62, 62, 62, 62, 62, 62, 62, 62, 62},
			// Pogačar didn't ride; van Aert crashed out at Dwars door Vlaanderen;
			// the GC riders below weren't on the start list (CyclingStage).
			StartList: flanders2024Field, // the real start list (validation_fields.go)
		},
		{
			DisplayName:  "Paris-Roubaix 2024",
			InternalName: "Paris-Roubaix",
			// 1 van der Poel solo  2 Philipsen +3:00  3 Pedersen  4 Politt  6 G.Vermeersch +3:47  9 Wærenskjold  10 Mihkels
			Top10RiderIDs: []int64{28, 17, 190, 453, 0, 314, 0, 0, 491, 101},
			Top10GapSecs:  []float64{0, 180, 180, 180, u, 227, u, u, u, u},
			// Pogačar didn't ride; van Aert injured; the rest weren't on the
			// Cyclingnews start list.
			StartList: roubaix2024Field, // the real start list (validation_fields.go)
		},
		{
			DisplayName:  "Liege-Bastogne-Liege 2024",
			InternalName: "Liege-Bastogne-Liege",
			// 1 Pogačar  2 Bardet* +1:39  3 van der Poel +2:02  4 Van Gils  5 A.Paret-Peintre  6 Vansevenant  8 Lutsenko  9 Bilbao
			Top10RiderIDs: []int64{452, 0, 28, 312, 79, 344, 0, 274, 33, 0},
			Top10GapSecs:  []float64{0, 99, 122, 122, 122, 122, 122, 122, 122, 122},
			StartList:     liege2024Field,
		},
		{
			DisplayName:  "Il Lombardia 2024",
			InternalName: "Il Lombardia",
			// 1 Pogačar  2 Evenepoel +3:16  3 Ciccone +4:31  8 Powless +4:58  9 Gaudu +4:58
			Top10RiderIDs: []int64{452, 294, 177, 0, 0, 0, 0, 103, 125, 0},
			Top10GapSecs:  []float64{0, 196, 271, u, u, u, u, 298, 298, 298},
			StartList:     lombardia2024Field, // the real start list (validation_fields.go)
		},
	}
}

// KnownRealRaces2023 are hold-out references: the same classics in 2023,
// never used for fitting ratings, so they show whether fitted ratings
// generalise. Read from the English Wikipedia result tables (Nov 2026).
// Riders not in data/riders.csv are 0 (Pidcock, Rui Costa, Turgis, Küng,
// F. Wright, Trentin, Hirschi, S. Yates). Start lists aren't curated, so
// only matched-rider metrics are meaningful.
func KnownRealRaces2023() []RealRaceEntry {
	return []RealRaceEntry{
		{
			DisplayName: "Strade Bianche 2023", InternalName: "Strade Bianche",
			// 1 Pidcock* 2 Madouas 3 Benoot 4 Costa* 5 Valter 6 Mohorič 7 Bilbao 8 Grégoire 9 Formolo 10 Kron
			Top10RiderIDs: []int64{0, 134, 60, 0, 55, 47, 33, 128, 240, 479},
			Top10GapSecs:  []float64{0, 20, 22, 23, 23, 34, 64, 78, 83, 95},
		},
		{
			DisplayName: "Milan-San Remo 2023", InternalName: "Milan-San Remo",
			// 1 van der Poel 2 Ganna 3 van Aert 4 Pogačar 5 Kragh Andersen 6 Pedersen 7 Powless 8 Mohorič 9 Turgis* 10 Stuyven
			Top10RiderIDs: []int64{28, 150, 429, 452, 183, 190, 103, 47, 0, 336},
			Top10GapSecs:  []float64{0, 15, 15, 15, 26, 26, 26, 26, 26, 26},
		},
		{
			DisplayName: "Tour of Flanders 2023", InternalName: "Tour of Flanders",
			// 1 Pogačar 2 van der Poel 3 Pedersen 4 van Aert 5 Powless 6 Küng* 7 Asgreen 8 Wright* 9 Jorgenson 10 Trentin*
			Top10RiderIDs: []int64{452, 28, 190, 429, 103, 0, 89, 0, 416, 0},
			Top10GapSecs:  []float64{0, 16, 72, 72, 72, 72, 72, 72, 79, 169},
		},
		{
			DisplayName: "Paris-Roubaix 2023", InternalName: "Paris-Roubaix",
			// 1 van der Poel 2 Philipsen 3 van Aert 4 Pedersen 5 Küng* 6 Ganna 7 Degenkolb 8 Walscheid 9 Rex 10 Laporte
			Top10RiderIDs: []int64{28, 17, 429, 190, 0, 150, 382, 201, 334, 421},
			Top10GapSecs:  []float64{0, 46, 46, 50, 50, 50, 155, 211, 215, 251},
		},
		{
			DisplayName: "Liege-Bastogne-Liege 2023", InternalName: "Liege-Bastogne-Liege",
			// 1 Evenepoel 2 Pidcock* 3 Buitrago 4 Healy 5 Madouas 6 G.Martin 7 Benoot 8 Konrad 9 Skjelmose 10 Hirschi*
			Top10RiderIDs: []int64{294, 0, 36, 95, 134, 135, 60, 182, 192, 0},
			Top10GapSecs:  []float64{0, 66, 66, 68, 84, 85, 97, 108, 108, 108},
		},
		{
			DisplayName: "Il Lombardia 2023", InternalName: "Il Lombardia 2023", // Como-Bergamo
			// 1 Pogačar 2 Bagioli 3 Roglič 4 Vlasov 5 S.Yates* 6 A.Yates 7 C.Rodríguez 8 Carapaz 9 Evenepoel 10 Kron
			Top10RiderIDs: []int64{452, 175, 306, 315, 0, 461, 161, 93, 294, 479},
			Top10GapSecs:  []float64{0, 52, 52, 52, 52, 52, 52, 66, 86, 86},
		},
	}
}

// KnownRealStageRaces2023 is a cross-season check, never fitted: the 2023
// Tour on its real route. Ratings are 2024 form, so part of its error is
// riders' form changing between seasons; it isn't in the judging total.
func KnownRealStageRaces2023() []RealRaceEntry {
	const u = gapUnknown
	return []RealRaceEntry{
		{
			DisplayName:  "Tour de France 2023 (GC)",
			InternalName: "Tour de France 2023", // the real 2023 route
			// 1 Vingegaard 2 Pogačar +7:29 3 A.Yates +10:56 4 S.Yates* +12:23
			// 5 C.Rodríguez +13:17 6 Bilbao +13:27 7 Hindley +14:44
			// 8 Gall +16:09 9 Gaudu +23:08 10 G.Martin +26:30
			// (Wikipedia, 2023 Tour de France; * not in the DB). Only the
			// podium is scored until the reference uses 2023 rosters: today's
			// teams make Bilbao, Gaudu and Martin domestiques (+2 h).
			Top10RiderIDs: []int64{431, 452, 461, 0, 0, 0, 0, 0, 0, 0},
			Top10GapSecs:  []float64{0, 449, 656, u, u, u, u, u, u, u},
			// Didn't start (Giro/Vuelta or injured): Evenepoel, Roglič,
			// Almeida, Ayuso, Buitrago, Geoghegan Hart, Vlasov, Caruso, Tiberi,
			// Uijtdebroeks, L.Martinez, Arensman, Van Wilder (Wikipedia, List of
			// teams and cyclists in the 2023 Tour de France). Riders who
			// abandoned (Mas, Carapaz, Jorgenson, van Aert) ride.
			DidNotStart: []int64{294, 306, 433, 174, 36, 179, 315, 37, 54, 259, 44, 145, 342},
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
	return RunRealRaceValidationFor(db, allRiders, KnownRealRaces(), realValidationRuns)
}

// RunRealRaceValidationFor validates one-day reference races with a given
// rider set (e.g. candidate ratings) and number of runs per race.
func RunRealRaceValidationFor(db *database.DB, allRiders []model.Rider, entries []RealRaceEntry, runs int) []ValidationSummary {
	if db == nil || len(allRiders) == 0 || runs <= 0 {
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
	for _, entry := range entries {
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
		top10Hits := make(map[int64]int) // runs in which a rider finished top-10

		pool := entry.pool(allRiders)
		var refIDs []int64
		for _, m := range matched {
			refIDs = append(refIDs, m.riderID)
		}
		allRuns := parallelRuns(runs, func(run int) []model.RaceResult {
			riders := withStarters(selection.SelectStartList(pool, course, race, selCfg), pool, refIDs)
			cfg := RaceConfig{
				Race:   race,
				Course: course,
				Riders: riders,
				Seed:   ValidationSeedBase + int64(run+1),
			}
			// A real edition: riders had their form of the day.
			cfg.FormDate = race.Date
			if race.RaceType == model.RaceTypeTT {
				return SimulateTT(cfg)
			}
			return SimulateRoadRace(cfg)
		})
		var finish FinishShape
		var margins []float64
		for _, simResults := range allRuns {
			gaps := make([]float64, 0, 20)
			for p := 1; p <= 20; p++ {
				for _, res := range simResults {
					if res.Position == p {
						gaps = append(gaps, res.TimeGapSeconds)
					}
				}
			}
			switch finishKind(gaps) {
			case "solo":
				finish.Solo++
			case "bunch":
				finish.Bunch++
			default:
				finish.Small++
			}
			if len(gaps) > 1 {
				margins = append(margins, gaps[1])
			}
			for _, res := range simResults {
				if res.Position == 1 {
					wins[res.RiderID]++
				}
				if res.Position >= 1 && res.Position <= 10 {
					top10Hits[res.RiderID]++
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

		// ── Metric 2: Top-10 overlap (per run, averaged over runs) ─────────────
		// The share of the real top-10 riders who also finish top-10 in a run.
		var top10Pct float64
		for _, m := range matched {
			top10Pct += float64(top10Hits[m.riderID])
		}
		top10Pct = top10Pct / float64(len(matched)*runs) * 100

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
		if w := entry.Top10RiderIDs[0]; w > 0 && runs > 0 {
			winnerPct = float64(wins[w]) / float64(runs) * 100
		}

		passed := top10Pct >= realPassTop10Target && (gapN == 0 || gapMAE <= realPassGapMAE)

		details := fmt.Sprintf(
			"Matched %d | Corr %.2f | Top-10 overlap %.0f%% (≥%.0f%%) | Gap MAE %.0fs over %d (≤%.0fs) | Real winner wins %.0f%%",
			len(matched), corr, top10Pct, realPassTop10Target, gapMAE, gapN, realPassGapMAE, winnerPct,
		)

		var checks []RiderCheck
		for _, m := range matched {
			checks = append(checks, RiderCheck{
				RiderID: m.riderID, RealPos: m.refPos, RealGap: entry.Top10GapSecs[m.refPos-1],
				SimPos: avgPos(m.riderID), SimGap: avgGap(m.riderID),
				Top10Pct: float64(top10Hits[m.riderID]) / float64(runs) * 100,
				WinPct:   float64(wins[m.riderID]) / float64(runs) * 100,
			})
		}
		intruders := findIntruders(entry, top10Hits, wins, runs, avgPos, avgGap)
		if n := float64(len(allRuns)); n > 0 {
			finish.Solo, finish.Small, finish.Bunch = finish.Solo/n, finish.Small/n, finish.Bunch/n
		}
		if len(margins) > 0 {
			finish.Margin = medianOf(margins)
		}
		finish.RealMargin = entry.Top10GapSecs[1]
		finish.RealKind = finishKind(entry.Top10GapSecs)

		results = append(results, ValidationSummary{
			Name:                 entry.DisplayName,
			Passed:               passed,
			Correlation:          corr,
			Top10Accuracy:        top10Pct,
			MeanTimeGapErrorSecs: gapMAE,
			MatchedRiders:        len(matched),
			IsRealRace:           true,
			Details:              details,
			Riders:               checks,
			Intruders:            intruders,
			Finish:               &finish,
			Calib:                calibPoints(posCnt, top10Hits, wins, runs, entry.Top10RiderIDs),
		})
	}
	return results
}

// realStageRaceRuns is the number of full stage-race simulations per case.
const realStageRaceRuns = 32

// realStageRaceGapMAE is the pass bar for GC gap error (seconds); Grand Tour
// gaps are an order of magnitude larger than one-day gaps.
const realStageRaceGapMAE = 300.0

// KnownRealStageRaces returns final GC references for seeded stage races.
// The seeded courses are generic, so only the GC shape is comparable.
func KnownRealStageRaces() []RealRaceEntry {
	const u = gapUnknown
	return []RealRaceEntry{
		{
			DisplayName:  "Tour de France 2024 (GC)",
			InternalName: "Tour de France",
			// 1 Pogačar  2 Vingegaard +6:17  3 Evenepoel +9:18  4 Almeida
			// 5 Landa  6 A.Yates  7 C.Rodríguez +25:04  8 Jorgenson +26:34
			// 9 Gee* +27:21  10 Buitrago +29:03 (7, 8 and 10 cross-checked
			// against the young rider classification; gaps for 4-6 unverified).
			Top10RiderIDs: []int64{452, 431, 294, 433, 326, 461, 161, 416, 0, 36},
			Top10GapSecs:  []float64{0, 377, 558, u, u, u, 1504, 1594, 1641, 1743},
			// O'Connor, Tiberi, Uijtdebroeks and Arensman didn't start
			// (Giro/Vuelta instead; Wikipedia, List of teams and cyclists in the
			// 2024 Tour de France). Riders who started and abandoned (Roglič,
			// Ayuso, Vlasov) ride: abandons are chance the sim can't know, and
			// the gap error only scores reference riders.
			DidNotStart: []int64{370, 54, 259, 145},
		},
		{
			DisplayName:  "Giro d'Italia 2024 (GC)",
			InternalName: "Giro d'Italia 2024", // real route (import-stages)
			// 1 Pogačar 2 D.Martínez +9:56 3 G.Thomas* +10:24 4 O'Connor +12:07
			// 5 Tiberi +12:49 6 Arensman +14:31 7 Rubio +15:52 8 Hirt +18:05
			// 9 Bardet* +20:32 10 Storer* +21:11 (Wikipedia, 2024 Giro d'Italia;
			// * not in the DB).
			Top10RiderIDs: []int64{452, 301, 0, 370, 54, 145, 254, 270, 0, 0},
			Top10GapSecs:  []float64{0, 596, 624, 727, 769, 871, 952, 1085, 1232, 1271},
			StartList:     giro2024Field,
		},
	}
}

// RunRealStageRaceValidation simulates each known stage race several times
// and compares the final GC with the reference.
func RunRealStageRaceValidation(db *database.DB) []ValidationSummary {
	if db == nil {
		return nil
	}
	allRiders, err := db.ListRiders()
	if err != nil || len(allRiders) == 0 {
		return nil
	}
	return RunRealStageRaceValidationFor(db, allRiders, append(KnownRealStageRaces(), KnownSameSeasonStageRaces()...), realStageRaceRuns)
}

// RunRealStageRaceValidationFor validates stage-race references with a given
// rider set and number of runs.
func RunRealStageRaceValidationFor(db *database.DB, allRiders []model.Rider, entries []RealRaceEntry, runs int) []ValidationSummary {
	if db == nil || len(allRiders) == 0 || runs <= 0 {
		return nil
	}
	srs, err := db.ListStageRaces()
	if err != nil {
		return nil
	}
	var out []ValidationSummary
	for _, entry := range entries {
		var sr *model.StageRace
		for i := range srs {
			if srs[i].Name == entry.InternalName {
				sr, _ = db.GetStageRace(srs[i].ID)
				break
			}
		}
		if sr == nil {
			out = append(out, ValidationSummary{Name: entry.DisplayName, IsRealRace: true,
				Details: fmt.Sprintf("Stage race %q not found. Run 'seed' first.", entry.InternalName)})
			continue
		}
		stages := make([]StageConfig, 0, len(sr.Stages))
		infos := make([]selection.StageInfo, 0, len(sr.Stages))
		for _, st := range sr.Stages {
			race, err1 := db.GetRace(st.RaceID)
			if err1 != nil {
				continue
			}
			course, err2 := db.GetCourse(race.CourseID)
			if err2 != nil {
				continue
			}
			stages = append(stages, StageConfig{Race: race, Course: course, StageNumber: st.StageNumber, RestDayBefore: st.RestDayBefore, Prestige: st.Prestige, SeasonalForm: true})
			infos = append(infos, selection.StageInfo{Course: course, RaceType: race.RaceType})
		}
		pool := entry.pool(allRiders)
		var refIDs []int64
		for _, id := range entry.Top10RiderIDs {
			if id > 0 {
				refIDs = append(refIDs, id)
			}
		}
		riders := withStarters(selection.SelectStageRaceStartList(pool, infos, selection.DefaultConfig(model.RaceTypeRoad, true, len(stages))), pool, refIDs)

		wins := map[int64]int{}
		top10 := map[int64]int{}
		gapSum := map[int64]float64{}
		gapN := map[int64]int{}
		gapAll := map[int64][]float64{} // every run's gap, for medians
		posSum := map[int64]float64{}
		allRuns := parallelRuns(runs, func(run int) [][]model.GCStanding {
			_, standings := SimulateStageRace(sr, stages, riders, ValidationSeedBase+int64(run+1))
			return standings
		})
		for _, standings := range allRuns {
			if len(standings) == 0 || len(standings[len(standings)-1]) == 0 {
				continue
			}
			final := standings[len(standings)-1]
			lead := final[0].TotalTimeSeconds
			for _, g := range final {
				if g.GCPosition == 1 {
					wins[g.RiderID]++
				}
				if g.GCPosition <= 10 {
					top10[g.RiderID]++
				}
				gapSum[g.RiderID] += g.TotalTimeSeconds - lead
				gapAll[g.RiderID] = append(gapAll[g.RiderID], g.TotalTimeSeconds-lead)
				gapN[g.RiderID]++
				posSum[g.RiderID] += float64(g.GCPosition)
			}
		}
		var at2, at10 []float64 // GC gap of 2nd and 10th place per run
		for _, standings := range allRuns {
			if len(standings) == 0 || len(standings[len(standings)-1]) < 10 {
				continue
			}
			final := standings[len(standings)-1]
			at2 = append(at2, final[1].TotalTimeSeconds-final[0].TotalTimeSeconds)
			at10 = append(at10, final[9].TotalTimeSeconds-final[0].TotalTimeSeconds)
		}
		var spread *GCSpread
		if len(at2) > 0 && entry.Top10GapSecs[1] != gapUnknown && entry.Top10GapSecs[9] != gapUnknown {
			spread = &GCSpread{Sim2: medianOf(at2), Real2: entry.Top10GapSecs[1], Sim10: medianOf(at10), Real10: entry.Top10GapSecs[9]}
		}
		var matched []int64
		var refPos []int
		for i, id := range entry.Top10RiderIDs {
			if id > 0 {
				matched = append(matched, id)
				refPos = append(refPos, i+1)
			}
		}
		overlap, mae, medMAE, n := 0.0, 0.0, 0.0, 0
		avgPos := map[int64]float64{}
		for i, id := range matched {
			overlap += float64(top10[id])
			if gapN[id] > 0 {
				avgPos[id] = posSum[id] / float64(gapN[id])
			} else {
				avgPos[id] = float64(len(riders) + 1)
			}
			if ref := entry.Top10GapSecs[refPos[i]-1]; ref != gapUnknown {
				avg := 4 * 3600.0 // eliminated or missing: count as hours down
				if gapN[id] > 0 {
					avg = gapSum[id] / float64(gapN[id])
				}
				mae += math.Abs(avg - ref)
				med := avg
				if g := gapAll[id]; len(g) > 0 {
					sorted := append([]float64(nil), g...)
					sort.Float64s(sorted)
					med = sorted[len(sorted)/2]
				}
				medMAE += math.Abs(med - ref)
				n++
			}
		}
		overlap = overlap / float64(len(matched)*runs) * 100
		if n > 0 {
			mae /= float64(n)
			medMAE /= float64(n)
		}
		corr := validationSpearman(matched, avgPos)
		winPct := float64(wins[entry.Top10RiderIDs[0]]) / float64(runs) * 100
		simPos := func(id int64) float64 {
			if gapN[id] == 0 {
				return float64(len(riders) + 1)
			}
			return posSum[id] / float64(gapN[id])
		}
		simGap := func(id int64) float64 {
			if gapN[id] == 0 {
				return 4 * 3600
			}
			return gapSum[id] / float64(gapN[id])
		}
		var checks []RiderCheck
		for i, id := range matched {
			checks = append(checks, RiderCheck{
				RiderID: id, RealPos: refPos[i], RealGap: entry.Top10GapSecs[refPos[i]-1],
				SimPos: simPos(id), SimGap: simGap(id),
				Top10Pct: float64(top10[id]) / float64(runs) * 100,
				WinPct:   float64(wins[id]) / float64(runs) * 100,
			})
		}
		intruders := findIntruders(entry, top10, wins, runs, simPos, simGap)
		passed := overlap >= realPassTop10Target && mae <= realStageRaceGapMAE
		out = append(out, ValidationSummary{
			Name: entry.DisplayName, IsRealRace: true, Passed: passed,
			Correlation: corr, Top10Accuracy: overlap, MeanTimeGapErrorSecs: mae, MedianGapErrorSecs: medMAE, MatchedRiders: len(matched),
			Details: fmt.Sprintf("Matched %d | Corr %.2f | GC top-10 overlap %.0f%% (≥%.0f%%) | GC gap MAE %.0fs over %d (≤%.0fs), median-gap MAE %.0fs | Real winner wins %.0f%%",
				len(matched), corr, overlap, realPassTop10Target, mae, n, realStageRaceGapMAE, medMAE, winPct),
			Riders: checks, Intruders: intruders, Spread: spread,
			Calib: calibPoints(gapN, top10, wins, runs, entry.Top10RiderIDs),
		})
	}
	return out
}

func medianOf(xs []float64) float64 {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}

// withStarters adds riders who really started (the reference top 10) to a
// selected start list when selection left them out.
func withStarters(list, pool []model.Rider, ids []int64) []model.Rider {
	in := make(map[int64]bool, len(list))
	for _, r := range list {
		in[r.ID] = true
	}
	for _, id := range ids {
		if in[id] {
			continue
		}
		for _, r := range pool {
			if r.ID == id {
				list = append(list, r)
				in[id] = true
				break
			}
		}
	}
	return list
}

// intruderTop10Share: a rider not in the real top 10 who finishes top 10 in
// at least this share of runs is reported as an intruder.
const intruderTop10Share = 0.5

// findIntruders lists riders who often make the simulated top 10 but were not
// in the real top 10 (riders the reference doesn't name can't be judged:
// unknown slots may be theirs).
func findIntruders(entry RealRaceEntry, top10 map[int64]int, wins map[int64]int, runs int,
	pos func(int64) float64, gap func(int64) float64) []RiderCheck {
	known := map[int64]bool{}
	unknownSlots := 0
	for _, id := range entry.Top10RiderIDs {
		if id > 0 {
			known[id] = true
		} else {
			unknownSlots++
		}
	}
	var out []RiderCheck
	for id, n := range top10 {
		if known[id] || float64(n) < intruderTop10Share*float64(runs) {
			continue
		}
		out = append(out, RiderCheck{
			RiderID: id, RealGap: gapUnknown, SimPos: pos(id), SimGap: gap(id),
			Top10Pct: float64(n) / float64(runs) * 100, WinPct: float64(wins[id]) / float64(runs) * 100,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SimPos < out[j].SimPos })
	// Unnamed reference places (riders not in the DB, or not verified) could
	// belong to the most frequent intruders, so they aren't judged.
	if unknownSlots >= len(out) {
		return nil
	}
	return out[unknownSlots:]
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

// RunHoldoutValidation validates the cross-season (2023) references.
func RunHoldoutValidation(db *database.DB) []ValidationSummary {
	if db == nil {
		return nil
	}
	allRiders, err := db.ListRiders()
	if err != nil || len(allRiders) == 0 {
		return nil
	}
	return append(RunRealRaceValidationFor(db, allRiders, KnownRealRaces2023(), realValidationRuns),
		RunRealStageRaceValidationFor(db, allRiders, KnownRealStageRaces2023(), realStageRaceRuns)...)
}

// field maps the entry's real start list to each rider's team then (nil:
// no list).
func (e RealRaceEntry) field() map[string]string {
	if len(e.StartList) == 0 {
		return nil
	}
	out := make(map[string]string, len(e.StartList))
	for _, s := range e.StartList {
		out[fieldName(s.Name)] = s.Team
	}
	return out
}

// fieldName matches rider names case- and space-insensitively.
func fieldName(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// KnownSameSeasonStageRaces are 2024 stage races the ratings are never fitted
// to (fit-ratings uses KnownRealStageRaces: the Tour and Giro GC): the test
// of the model in the season the ratings describe.
func KnownSameSeasonStageRaces() []RealRaceEntry {
	const u = gapUnknown
	return []RealRaceEntry{
		{
			DisplayName:  "Tirreno-Adriatico 2024 (GC)",
			InternalName: "Tirreno-Adriatico 2024", // real route; approximate field
			// 1 Vingegaard 2 Ayuso +1:24 3 Hindley +1:52 4 del Toro +2:20
			// 5 O'Connor +2:24 6 Arensman +2:25 7 Uijtdebroeks +3:10
			// 8 Kämna +4:02 9 Pidcock* +4:05 10 Vauquelin +4:24 (Wikipedia).
			Top10RiderIDs: []int64{431, 174, 298, 439, 370, 145, 259, 181, 0, 170},
			Top10GapSecs:  []float64{0, 84, 112, 140, 144, 145, 190, 242, 245, 264},
		},
		{
			DisplayName:  "Volta a Catalunya 2024 (GC)",
			InternalName: "Volta a Catalunya 2024", // real route; approximate field
			// 1 Pogačar 2 Landa +3:41 3 Bernal +5:03 4 Vlasov +5:56 5 Mas +6:01
			// 6 Harper* +6:01 7 L.Martinez +6:02 8 Tiberi +6:33 9 Almeida +6:33
			// 10 Fortunato +7:27 (Wikipedia).
			Top10RiderIDs: []int64{452, 326, 147, 315, 245, 0, 44, 54, 433, 497},
			Top10GapSecs:  []float64{0, 221, 303, 356, 361, 361, 362, 393, 393, 447},
		},
	}
}

// stageRaceField is the field of a reference stage race: the rider pool
// without its verified non-starters, restricted to its real start list (each
// rider for the team they rode for then) when it has one.
func stageRaceField(stageRace string, riders []model.Rider) []model.Rider {
	for _, e := range append(append(KnownRealStageRaces(), KnownSameSeasonStageRaces()...), KnownRealStageRaces2023()...) {
		if e.InternalName == stageRace {
			return e.pool(riders)
		}
	}
	return riders
}

// pool is the riders who can start this reference race (see stageRaceField).
func (e RealRaceEntry) pool(riders []model.Rider) []model.Rider {
	dns := make(map[int64]bool, len(e.DidNotStart))
	for _, id := range e.DidNotStart {
		dns[id] = true
	}
	field := e.field()
	out := make([]model.Rider, 0, len(riders))
	for _, r := range riders {
		if dns[r.ID] {
			continue
		}
		if field == nil {
			out = append(out, r)
		} else if team, ok := field[fieldName(r.Name)]; ok {
			r.Team = team
			out = append(out, r)
		}
	}
	return out
}
