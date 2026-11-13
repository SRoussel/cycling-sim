package simulation

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
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
	"Volta a Catalunya":         "Volta a Catalunya 2024",
	"Tour Auvergne-Rhone-Alpes": "Criterium du Dauphine",
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
	return oneDay, stage, nil
}

// RunSeedSetsFor scores given references over several seed sets (as
// RunSeedSets does for the 2024 ones).
func RunSeedSetsFor(db *database.DB, sets int, oneDay, stage []RealRaceEntry) []SeedSetResult {
	riders, err := db.ListRiders()
	if err != nil || len(riders) == 0 {
		return nil
	}
	defer func(b int64) { ValidationSeedBase = b }(ValidationSeedBase)
	var out []SeedSetResult
	index := map[string]int{}
	for set := 0; set < sets; set++ {
		ValidationSeedBase = int64(set) * 1000
		rows := RunRealRaceValidationFor(db, riders, oneDay, realValidationRuns)
		rows = append(rows, RunRealStageRaceValidationFor(db, riders, stage, realStageRaceRuns)...)
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
			for _, c := range r.Riders {
				if c.RealPos == 1 {
					out[i].RealWinnerPc += c.WinPct / float64(sets)
				}
			}
		}
	}
	return out
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
