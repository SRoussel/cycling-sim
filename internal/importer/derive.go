package importer

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cycling_sim/internal/model"
)

// Profiles are the result categories, one sub-folder each under the results
// directory (e.g. data/results/mountain/tour-2024-stage-14.html).
var Profiles = []string{"sprint", "hilly", "mountain", "cobbles", "tt"}

// Derivation parameters.
const (
	// pointsDecay: placings score 100·exp(-(pos-1)/pointsDecay)
	// (1st 100, 5th 61, 10th 32, 20th 9).
	pointsDecay = 8.0
	// shrinkRaces: scores are averaged as sum/(n+shrinkRaces), so one lucky
	// result counts for less than a consistent record.
	shrinkRaces = 2.0
	// minResults: a rider needs this many scored results in a profile before
	// that profile's ratings are derived.
	minResults = 2
)

// Library holds every imported result, keyed by profile.
type Library struct {
	Races   map[string]int              // profile → number of race files
	Results map[string]map[string][]int // profile → name key → positions
	Names   map[string]string           // name key → name as first seen
	Files   []string                    // files read
	Skipped map[string]string           // file → reason
}

// LoadDir reads every results file under dir/<profile>/.
func LoadDir(dir string) (*Library, error) {
	lib := &Library{
		Races:   map[string]int{},
		Results: map[string]map[string][]int{},
		Names:   map[string]string{},
		Skipped: map[string]string{},
	}
	for _, profile := range Profiles {
		files, _ := filepath.Glob(filepath.Join(dir, profile, "*"))
		sort.Strings(files)
		for _, f := range files {
			ext := strings.ToLower(filepath.Ext(f))
			if ext != ".html" && ext != ".htm" && ext != ".csv" {
				continue
			}
			results, err := ParseFile(f)
			if err != nil {
				lib.Skipped[f] = err.Error()
				continue
			}
			if len(results) == 0 {
				lib.Skipped[f] = "no result rows found"
				continue
			}
			lib.Files = append(lib.Files, f)
			lib.Races[profile]++
			if lib.Results[profile] == nil {
				lib.Results[profile] = map[string][]int{}
			}
			for _, r := range results {
				k := NameKey(r.Name)
				lib.Results[profile][k] = append(lib.Results[profile][k], r.Position)
				if _, ok := lib.Names[k]; !ok {
					lib.Names[k] = r.Name
				}
			}
		}
	}
	return lib, nil
}

// Rating is a derived attribute set for one rider (zero = not derived).
type Rating struct {
	Name     string // DB name
	Fields   map[string]int
	Evidence []string // e.g. "mountain p92 (4 results)"
}

// Derive turns imported results into ratings for riders in the database.
// Within each profile, riders are ranked by score and their percentile p
// (0 worst … 1 best among riders with results) maps onto attributes:
//
//	mountain → Climbing 70+27p, threshold 5.7+1.2p W/kg (FTP from weight)
//	sprint   → Sprint 70+27p, Sprint_watts 1550+400p
//	tt       → TT 70+27p
//	cobbles  → Handling 70+29p
//	hilly    → Climbing 68+22p (only when no mountain rating)
func Derive(lib *Library, riders []model.Rider) []Rating {
	byKey := map[string]model.Rider{}
	for _, r := range riders {
		byKey[NameKey(r.Name)] = r
	}
	ratings := map[string]*Rating{}
	get := func(r model.Rider) *Rating {
		if ratings[r.Name] == nil {
			ratings[r.Name] = &Rating{Name: r.Name, Fields: map[string]int{}}
		}
		return ratings[r.Name]
	}

	for _, profile := range Profiles {
		type scored struct {
			rider model.Rider
			score float64
			n     int
		}
		var list []scored
		for key, positions := range lib.Results[profile] {
			r, ok := byKey[key]
			if !ok || len(positions) < minResults {
				continue
			}
			sum := 0.0
			for _, p := range positions {
				sum += 100 * math.Exp(-float64(p-1)/pointsDecay)
			}
			list = append(list, scored{r, sum / (float64(len(positions)) + shrinkRaces), len(positions)})
		}
		if len(list) == 0 {
			continue
		}
		sort.Slice(list, func(i, j int) bool { return list[i].score < list[j].score })
		for i, s := range list {
			p := 1.0
			if len(list) > 1 {
				p = float64(i) / float64(len(list)-1)
			}
			rt := get(s.rider)
			rt.Evidence = append(rt.Evidence, fmt.Sprintf("%s p%.0f (%d results)", profile, p*100, s.n))
			switch profile {
			case "mountain":
				rt.Fields["Climbing"] = int(math.Round(70 + 27*p))
				rt.Fields["FTP_watts"] = int(math.Round((5.7 + 1.2*p) * s.rider.WeightKg))
			case "sprint":
				rt.Fields["Sprint"] = int(math.Round(70 + 27*p))
				rt.Fields["Sprint_watts"] = int(math.Round(1550 + 400*p))
			case "tt":
				rt.Fields["TT"] = int(math.Round(70 + 27*p))
			case "cobbles":
				rt.Fields["Handling"] = int(math.Round(70 + 29*p))
			case "hilly":
				if _, hasMountain := rt.Fields["Climbing"]; !hasMountain {
					rt.Fields["Climbing"] = int(math.Round(68 + 22*p))
				}
			}
		}
	}

	out := make([]Rating, 0, len(ratings))
	for _, r := range ratings {
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// derivedColumns is the column order of the derived ratings file.
var derivedColumns = []string{"FTP_watts", "Sprint_watts", "Climbing", "Sprint", "TT", "Handling"}

// WriteDerived writes ratings in the rider-overrides CSV format.
func WriteDerived(path string, ratings []Rating, sourceNote string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "# Derived by `cycling-sim import-results` from %s.\n", sourceNote)
	fmt.Fprintln(f, "# Applied after riders.csv and before rider_overrides.csv (curated rows win).")
	fmt.Fprintln(f, "# Regenerated on each import: edit rider_overrides.csv instead of this file.")
	w := csv.NewWriter(f)
	header := append([]string{"Name"}, derivedColumns...)
	header = append(header, "Source")
	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range ratings {
		row := []string{r.Name}
		for _, c := range derivedColumns {
			if v, ok := r.Fields[c]; ok {
				row = append(row, fmt.Sprint(v))
			} else {
				row = append(row, "")
			}
		}
		row = append(row, "derived: "+strings.Join(r.Evidence, "; "))
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
