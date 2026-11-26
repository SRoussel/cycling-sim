package simulation

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
)

// 2026 references (Nov 2026: the reference season moved to 2026, the year the
// rosters and ratings describe). Read at run time from two data files, both
// from Wikipedia: data/reference_2026.csv (one-day top 10s, GC top 10s with
// gaps) and data/startlists_2026.csv (every starter, with his team then).
// Each race runs on our seeded course of that name; where the 2026 route
// differs (most stage races, until their 2026 stage files are imported) the
// course is an approximation.

// race2026Course maps a reference race to the seeded race or stage race it
// runs on (same name unless listed).
var race2026Course = map[string]string{
	"Volta a Catalunya":          "Volta a Catalunya 2024",
	"Tour Auvergne-Rhone-Alpes":  "Criterium du Dauphine 2023", // stand-in routes (no 2026 file)
	"Paris-Nice":                 "Paris-Nice 2025",
	"Tour of the Basque Country": "Tour of the Basque Country 2023",
	// Tour de Suisse stays on its generic course: 2026 was a 5-stage race,
	// the newest file (2023) an 8-stage one with two big mountain days.
	"Tour de France":     "Tour de France 2023",
	"Giro d'Italia":      "Giro d'Italia 2026",
	"Tirreno-Adriatico":  "Tirreno-Adriatico 2026",
	"Tour de Romandie":   "Tour de Romandie 2026",
	"Strade Bianche":     "Strade Bianche 2026",
	"E3 Saxo Classic":    "E3 Saxo Classic 2026",
	"Gent-Wevelgem":      "Gent-Wevelgem 2026",
	"Tour of Flanders":   "Tour of Flanders 2026",
	"Paris-Roubaix":      "Paris-Roubaix 2026",
	"La Fleche Wallonne": "La Fleche Wallonne 2026",
}

// foldKey matches rider names across sources: accents dropped, case and
// hyphens ignored.
func foldKey(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == '-' {
			r = ' '
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	var rows [][]string
	for {
		row, err := r.Read()
		if err == io.EOF {
			return rows, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		rows = append(rows, row)
	}
}

// Load2026References builds the 2026 one-day and stage-race references from
// the two data files, matching riders by name (accents and case ignored).
func Load2026References(referencePath, startListPath string, riders []model.Rider) (oneDay, stage []RealRaceEntry, err error) {
	ref, err := readCSV(referencePath)
	if err != nil {
		return nil, nil, err
	}
	starts, err := readCSV(startListPath)
	if err != nil {
		return nil, nil, err
	}
	resolve := nameResolver(riders)
	fields := map[string][]Starter{}
	for _, row := range starts[1:] { // Race,Quality,Rider,Team
		if len(row) < 4 {
			continue
		}
		if r, ok := resolve(row[2]); ok {
			fields[row[0]] = append(fields[row[0]], Starter{Name: r.Name, Team: row[3]})
		}
	}
	type acc struct {
		kind  string
		ids   []int64
		gaps  []float64
		order int
	}
	races := map[string]*acc{}
	var names []string
	for _, row := range ref[1:] { // Kind,Race,Date,Position,Rider,GapSeconds
		if len(row) < 6 {
			continue
		}
		a := races[row[1]]
		if a == nil {
			a = &acc{kind: row[0]}
			races[row[1]] = a
			names = append(names, row[1])
		}
		gap, _ := strconv.ParseFloat(row[5], 64)
		var id int64
		if r, ok := resolve(row[4]); ok {
			id = r.ID
		}
		a.ids = append(a.ids, id)
		a.gaps = append(a.gaps, gap)
	}
	for _, name := range names {
		a := races[name]
		course := name
		if c, ok := race2026Course[name]; ok {
			course = c
		}
		e := RealRaceEntry{DisplayName: name + " 2026", InternalName: course, Top10RiderIDs: a.ids, Top10GapSecs: a.gaps, StartList: fields[name]}
		if a.kind == "gc" {
			e.DisplayName = name + " 2026 (GC)"
			stage = append(stage, e)
		} else {
			oneDay = append(oneDay, e)
		}
	}
	// Riders who abandoned (optional file beside the start lists).
	if ab, err := readCSV(filepath.Join(filepath.Dir(startListPath), "abandons_2026.csv")); err == nil && len(ab) > 1 {
		ids := map[string]int64{}
		for _, r := range riders {
			ids[fieldName(r.Name)] = r.ID
		}
		for _, row := range ab[1:] { // Race,Rider,Mark
			if len(row) < 2 {
				continue
			}
			id, ok := ids[fieldName(row[1])]
			if !ok {
				continue
			}
			for i := range stage {
				if strings.HasPrefix(stage[i].DisplayName, row[0]) {
					stage[i].Abandoned = append(stage[i].Abandoned, id)
				}
			}
		}
	}
	loadSavedResults(filepath.Join(filepath.Dir(startListPath), "local", "results_2026.csv"), resolve, oneDay, stage)
	return oneDay, stage, nil
}

// savedRaceNames maps ProCyclingStats race titles (pages the user saved) to
// the reference names, by a distinctive part of the title.
var savedRaceNames = []struct{ part, race string }{
	{"Strade Bianche", "Strade Bianche"}, {"Milano", "Milan-San Remo"}, {"E3 Saxo", "E3 Saxo Classic"},
	{"Wevelgem", "Gent-Wevelgem"}, {"Vlaanderen", "Tour of Flanders"}, {"Roubaix", "Paris-Roubaix"},
	{"Amstel", "Amstel Gold Race"}, {"Wallon", "La Fleche Wallonne"}, {"Bastogne", "Liege-Bastogne-Liege"},
	{"San Sebastian", "Clasica San Sebastian"}, {"Lombardia", "Il Lombardia"}, {"Paris-Nice", "Paris-Nice"},
	{"Tirreno", "Tirreno-Adriatico"}, {"Catalunya", "Volta a Catalunya"}, {"Itzulia", "Tour of the Basque Country"},
	{"Basque", "Tour of the Basque Country"}, {"Romandie", "Tour de Romandie"}, {"Giro", "Giro d'Italia"},
	{"Dauphin", "Tour Auvergne-Rhone-Alpes"}, {"Auvergne", "Tour Auvergne-Rhone-Alpes"}, {"Suisse", "Tour de Suisse"},
	{"Tour de France", "Tour de France"}, {"Vuelta", "Vuelta a Espana"},
}

// loadSavedResults reads the full results and start lists the user saved
// from ProCyclingStats (tools/pcs_saved_pages.py; optional): every rider's
// real place and gap, who didn't finish, and the field where the references
// have no start list (the start list page, else everyone in the result).
func loadSavedResults(path string, resolve func(string) (model.Rider, bool), oneDay, stage []RealRaceEntry) {
	rows, err := readCSV(path)
	if err != nil || len(rows) < 2 {
		return
	}
	entryFor := func(title string) *RealRaceEntry {
		for _, n := range savedRaceNames {
			if !strings.Contains(title, n.part) {
				continue
			}
			for i := range oneDay {
				if oneDay[i].DisplayName == n.race+" 2026" {
					return &oneDay[i]
				}
			}
			for i := range stage {
				if stage[i].DisplayName == n.race+" 2026 (GC)" {
					return &stage[i]
				}
			}
		}
		return nil
	}
	starts := map[*RealRaceEntry][]Starter{}
	finishers := map[*RealRaceEntry][]Starter{}
	for _, row := range rows[1:] { // Race,Kind,Pos,Rider,Team,GapS,Status
		if len(row) < 7 {
			continue
		}
		e := entryFor(row[0])
		if e == nil {
			continue
		}
		r, ok := resolve(row[3])
		if !ok {
			continue
		}
		switch row[1] {
		case "start":
			starts[e] = append(starts[e], Starter{Name: r.Name, Team: row[4]})
		case "result", "gc":
			finishers[e] = append(finishers[e], Starter{Name: r.Name, Team: row[4]})
			if row[6] != "" {
				e.Abandoned = append(e.Abandoned, r.ID)
				continue
			}
			pos, _ := strconv.Atoi(row[2])
			gap, err := strconv.ParseFloat(row[5], 64)
			if pos <= 0 || err != nil {
				continue
			}
			if e.Full == nil {
				e.Full = map[int64]FullPlace{}
			}
			e.Full[r.ID] = FullPlace{Pos: pos, Gap: gap}
		}
	}
	for e, list := range starts {
		if len(e.StartList) == 0 {
			e.StartList = list
		}
	}
	for e, list := range finishers {
		// A one-day result lists every starter; a final GC only finishers
		// plus those it marks as out, so it's no start list.
		if len(e.StartList) == 0 && !strings.HasSuffix(e.DisplayName, "(GC)") {
			e.StartList = list
		}
	}
}

// RunSeedSetsFor scores given references over several seed sets (as
// RunSeedSets does for the 2024 ones).
// The race shapes of shapeRefs come per seed set from the same stage-race
// runs (a ref no GC reference runs on is simulated on its own).
func RunSeedSetsFor(db *database.DB, sets int, oneDay, stage []RealRaceEntry, shapeRefs []RealShape) ([]SeedSetResult, [][]ShapeResult) {
	riders, err := db.ListRiders()
	if err != nil || len(riders) == 0 {
		return nil, nil
	}
	return RunSeedSetsWith(db, riders, sets, oneDay, stage, shapeRefs)
}

// RunSeedSetsWith is RunSeedSetsFor with given riders (ratings other than
// the database's: cross-validation folds).
func RunSeedSetsWith(db *database.DB, riders []model.Rider, sets int, oneDay, stage []RealRaceEntry, shapeRefs []RealShape) ([]SeedSetResult, [][]ShapeResult) {
	defer func(b int64) { ValidationSeedBase = b }(ValidationSeedBase)
	var out []SeedSetResult
	var shapes [][]ShapeResult
	index := map[string]int{}
	for set := 0; set < sets; set++ {
		ValidationSeedBase = int64(set) * 1000
		rows := RunRealRaceValidationFor(db, riders, oneDay, realValidationRuns)
		stageRows := RunRealStageRaceValidationFor(db, riders, stage, realStageRaceRuns)
		rows = append(rows, stageRows...)
		done, missing := ShapesFromSummaries(shapeRefs, stageRows)
		if len(missing) > 0 {
			done = append(done, RunShapeValidationFor(db, missing, FieldOf(stage))...)
		}
		shapes = append(shapes, done)
		for _, r := range rows {
			i, ok := index[r.Name]
			if !ok {
				i = len(out)
				index[r.Name] = i
				out = append(out, SeedSetResult{Name: r.Name})
			}
			out[i].MAE = append(out[i].MAE, r.MeanTimeGapErrorSecs)
			out[i].MedianMAE = append(out[i].MedianMAE, r.MedianGapErrorSecs)
			if r.Spread != nil {
				out[i].Spread = append(out[i].Spread, *r.Spread)
			}
			if r.Finish != nil {
				out[i].Finish = append(out[i].Finish, *r.Finish)
			}
			out[i].Calib = append(out[i].Calib, r.Calib)
			for _, c := range r.Riders {
				if c.RealPos == 1 {
					out[i].RealWinnerPc += c.WinPct / float64(sets)
				}
			}
		}
	}
	return out, shapes
}

// nameResolver finds a database rider for a name as another source spells
// it: the same name (accents, case, hyphens ignored), else the one rider
// whose name holds all of its words or whose words it holds ("Mikkel
// Norsgaard Bjerg" is Mikkel Bjerg, "Albert Philipsen" is Albert Withen
// Philipsen). Ambiguous names resolve to nobody.
func nameResolver(riders []model.Rider) func(string) (model.Rider, bool) {
	byKey := make(map[string]model.Rider, len(riders))
	words := make([][]string, len(riders))
	for i, r := range riders {
		k := foldKey(r.Name)
		byKey[k] = r
		words[i] = strings.Fields(k)
	}
	contains := func(big, small []string) bool {
		for _, w := range small {
			found := false
			for _, b := range big {
				found = found || b == w
			}
			if !found {
				return false
			}
		}
		return true
	}
	return func(name string) (model.Rider, bool) {
		k := foldKey(name)
		if r, ok := byKey[k]; ok {
			return r, true
		}
		w := strings.Fields(k)
		if len(w) < 2 {
			return model.Rider{}, false
		}
		hit := -1
		for i, rw := range words {
			if len(rw) >= 2 && (contains(rw, w) || contains(w, rw)) && rw[len(rw)-1] == w[len(w)-1] {
				if hit >= 0 {
					return model.Rider{}, false
				}
				hit = i
			}
		}
		if hit < 0 {
			return model.Rider{}, false
		}
		return riders[hit], true
	}
}

// StageRaceField is the field of a 2024/2023 reference stage race.
func StageRaceField(stageRace string, riders []model.Rider) []model.Rider {
	return stageRaceField(stageRace, riders)
}

// FieldOf gives a stage race the field of the reference among entries that
// runs on it (everyone if none does).
func FieldOf(entries []RealRaceEntry) func(string, []model.Rider) []model.Rider {
	return func(stageRace string, riders []model.Rider) []model.Rider {
		for _, e := range entries {
			if e.InternalName == stageRace {
				return e.pool(riders)
			}
		}
		return riders
	}
}
