package simulation

import (
	"fmt"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
)

// RealShape is how each stage of a real stage race was won.
type RealShape struct {
	DisplayName  string
	InternalName string
	Stages       []Outcome // by stage number - 1
	Kind         string    // ShapeGrandTour or ShapeWeekLong: averages are per kind
}

// Race-shape kinds: breakaway rates differ between three-week and
// week-long races, so outcomes are averaged per kind.
const (
	ShapeGrandTour = "Grand Tours"
	ShapeWeekLong  = "Week-long races"
)

// KnownRealShapes: the 2023 Tour on its real route. From the English
// Wikipedia race overview and stage articles (read Nov 2026); stages 12, 13,
// 15 and 17-19 were incomplete there and are classed from race reports:
// Izagirre (12), Kwiatkowski (13), Poels (15) and Gall (17) won from the
// day's break, Asgreen (18) from a four-rider break that held off the
// sprinters by metres, Mohoric (19) from a move that went clear mid-race.
func KnownRealShapes() []RealShape {
	S, B, X, T := OutcomeSprint, OutcomeBreakaway, OutcomeSelection, OutcomeTT
	return []RealShape{{
		// Giro 2024 (Wikipedia stage table and race reports): sprints by
		// Merlier, Milan and Kooij; breaks won by B. Thomas, P. Sánchez,
		// V. Paret-Peintre, Alaphilippe, Steinhauser and Vendrame; Narváez
		// from a three-rider move on stage 1 and Pogačar's five stage wins
		// from selections; two time trials.
		DisplayName:  "Giro d'Italia 2024 (race shape)",
		InternalName: "Giro d'Italia 2024",
		Kind:         ShapeGrandTour,
		Stages:       []Outcome{X, X, S, S, B, B, T, X, S, B, S, B, S, T, X, X, B, S, B, X, S},
	}, {
		// Tirreno 2024: opening ITT; sprints by Philipsen, Bauhaus and Milan
		// (twice); Vingegaard's two mountain wins.
		DisplayName:  "Tirreno-Adriatico 2024 (race shape)",
		InternalName: "Tirreno-Adriatico 2024",
		Kind:         ShapeWeekLong,
		Stages:       []Outcome{T, S, S, S, X, X, S},
	}, {
		// Catalunya 2024: Schultz from a select group (stage 1), Pogačar's
		// four wins from selections, sprints on stages 4 and 5.
		DisplayName:  "Volta a Catalunya 2024 (race shape)",
		InternalName: "Volta a Catalunya 2024",
		Kind:         ShapeWeekLong,
		Stages:       []Outcome{X, X, X, S, S, X, X},
	}, {
		DisplayName:  "Tour de France 2023 (race shape)",
		InternalName: "Tour de France 2023",
		Stages:       []Outcome{X, X, S, S, B, X, S, S, B, B, S, B, B, X, B, T, B, B, B, X, S},
		// How stages are won is race character, not riders' form, so the
		// 2023 Tour counts towards the Grand Tour averages.
		Kind: ShapeGrandTour,
	}}
}

// KnownRealShapes2026: the 2026 stage races on their real routes (English
// Wikipedia stage tables and stage articles, read Nov 2026). Judgement calls
// where the page doesn't say: Giro stage 5 (Arrieta, "rainy day raid"), 12
// (Segaert's long-range attack, favourites behind) and 19 (Kuss, while his
// leader Vingegaard led the race) are breakaways; Romandie stage 1 (Pogačar
// out-sprinting Lipowitz and two others) is a selection.
func KnownRealShapes2026() []RealShape {
	S, B, X, T := OutcomeSprint, OutcomeBreakaway, OutcomeSelection, OutcomeTT
	return []RealShape{{
		// Sprints: Magnier (3), Thomas Silva, Narváez, Ballerini, Milan;
		// Vingegaard's five wins from selections; breaks: Arrieta, Narváez
		// (8, 11), Segaert, Bettiol, Dversnes, Valgren, Kuss.
		DisplayName:  "Giro d'Italia 2026 (race shape)",
		InternalName: "Giro d'Italia 2026",
		Kind:         ShapeGrandTour,
		Stages:       []Outcome{S, S, S, S, B, S, X, B, X, T, B, B, B, X, B, X, B, S, B, X, S},
	}, {
		// Ganna's ITT; Van der Poel from a group of favourites (2) and a
		// reduced sprint (4); Valgren from the break; del Toro at Camerino.
		DisplayName:  "Tirreno-Adriatico 2026 (race shape)",
		InternalName: "Tirreno-Adriatico 2026",
		Kind:         ShapeWeekLong,
		Stages:       []Outcome{T, X, S, S, B, X, S},
	}, {
		// Godon's prologue; Pogačar's four wins and Godon's sprint of a
		// small group (3) all from selections.
		DisplayName:  "Tour de Romandie 2026 (race shape)",
		InternalName: "Tour de Romandie 2026",
		Kind:         ShapeWeekLong,
		Stages:       []Outcome{T, X, X, X, X, X},
	}, tour2023Shape()}
}

// tour2023Shape: the 2023 Tour on its real route, the 2026 Tour's stand-in
// (no 2026 file); how stages are won is race character, not form.
func tour2023Shape() RealShape {
	for _, r := range KnownRealShapes() {
		if r.InternalName == "Tour de France 2023" {
			return r
		}
	}
	return RealShape{}
}

// shapeRuns: full stage-race simulations per race-shape check.
const shapeRuns = 32

// ShapeResult is a stage race's simulated outcomes against the real ones.
type ShapeResult struct {
	Name      string
	Kind      string
	Agreement float64             // share of (stage, run) outcomes matching the real one
	Real, Sim map[Outcome]float64 // outcomes per race (sim: mean over runs)
	PerStage  []map[Outcome]float64
	Stages    []Outcome
}

// RunShapeValidation simulates each known stage race and classifies how
// every stage was won.
func RunShapeValidation(db *database.DB) []ShapeResult {
	return RunShapeValidationFor(db, KnownRealShapes(), stageRaceField)
}

// RunShapeValidationFor runs the given shape references, each on the field
// field(stage race name, riders) gives it.
func RunShapeValidationFor(db *database.DB, refs []RealShape, field func(string, []model.Rider) []model.Rider) []ShapeResult {
	if db == nil {
		return nil
	}
	riders, err := db.ListRiders()
	if err != nil || len(riders) == 0 {
		return nil
	}
	srs, err := db.ListStageRaces()
	if err != nil {
		return nil
	}
	var out []ShapeResult
	for _, ref := range refs {
		var sr *model.StageRace
		for i := range srs {
			if srs[i].Name == ref.InternalName {
				sr, _ = db.GetStageRace(srs[i].ID)
			}
		}
		if sr == nil || len(sr.Stages) != len(ref.Stages) {
			continue
		}
		var stages []StageConfig
		var infos []selection.StageInfo
		for _, st := range sr.Stages {
			race, err1 := db.GetRace(st.RaceID)
			course, err2 := db.GetCourse(race.CourseID)
			if err1 != nil || err2 != nil {
				continue
			}
			stages = append(stages, StageConfig{Race: race, Course: course, StageNumber: st.StageNumber, RestDayBefore: st.RestDayBefore, Prestige: st.Prestige, SeasonalForm: true})
			infos = append(infos, selection.StageInfo{Course: course, RaceType: race.RaceType})
		}
		start := selection.SelectStageRaceStartList(field(ref.InternalName, riders), infos, selection.DefaultConfig(model.RaceTypeRoad, true, len(stages)))
		runs := parallelRuns(shapeRuns, func(run int) []Outcome {
			own := make([]StageConfig, len(stages))
			copy(own, stages)
			for i := range own {
				own[i].Shape = &RaceShape{}
			}
			results, _ := SimulateStageRace(sr, own, start, ValidationSeedBase+int64(run+1))
			o := make([]Outcome, len(own))
			for i := range own {
				if i < len(results) {
					o[i] = own[i].Shape.Classify(results[i], own[i].Race.RaceType == model.RaceTypeTT)
				}
			}
			return o
		})
		res := ShapeResult{Name: ref.DisplayName, Kind: ref.Kind, Real: map[Outcome]float64{}, Sim: map[Outcome]float64{}, Stages: ref.Stages}
		match := 0.0
		for i, want := range ref.Stages {
			res.Real[want]++
			per := map[Outcome]float64{}
			for _, o := range runs {
				per[o[i]] += 1 / float64(len(runs))
				res.Sim[o[i]] += 1 / float64(len(runs))
				if o[i] == want {
					match++
				}
			}
			res.PerStage = append(res.PerStage, per)
		}
		res.Agreement = match / float64(len(runs)*len(ref.Stages))
		out = append(out, res)
	}
	return out
}

// Summary is a one-line description of a shape result.
func (r ShapeResult) Summary() string {
	f := func(m map[Outcome]float64, o Outcome) string { return fmt.Sprintf("%.1f", m[o]) }
	return fmt.Sprintf("Stage outcomes match %.0f%% | sprints %s (real %s), breakaways %s (real %s), selections %s (real %s)",
		r.Agreement*100, f(r.Sim, OutcomeSprint), f(r.Real, OutcomeSprint), f(r.Sim, OutcomeBreakaway), f(r.Real, OutcomeBreakaway),
		f(r.Sim, OutcomeSelection), f(r.Real, OutcomeSelection))
}

// ShapeAverage is the mean outcomes per race of one kind, sim and real.
type ShapeAverage struct {
	Kind      string
	Races     int
	Real, Sim map[Outcome]float64
}

// ShapeAverages averages race shapes per kind (per race).
func ShapeAverages(results []ShapeResult) []ShapeAverage {
	var out []ShapeAverage
	for _, kind := range []string{ShapeGrandTour, ShapeWeekLong} {
		a := ShapeAverage{Kind: kind, Real: map[Outcome]float64{}, Sim: map[Outcome]float64{}}
		for _, r := range results {
			if r.Kind != kind {
				continue
			}
			a.Races++
			for o, v := range r.Real {
				a.Real[o] += v
			}
			for o, v := range r.Sim {
				a.Sim[o] += v
			}
		}
		if a.Races == 0 {
			continue
		}
		for o := range a.Real {
			a.Real[o] /= float64(a.Races)
		}
		for o := range a.Sim {
			a.Sim[o] /= float64(a.Races)
		}
		out = append(out, a)
	}
	return out
}

// Summary compares a kind's average outcomes per race.
func (a ShapeAverage) Summary() string {
	f := func(m map[Outcome]float64, o Outcome) string { return fmt.Sprintf("%.1f", m[o]) }
	return fmt.Sprintf("per race: sprints %s (real %s), breakaways %s (real %s), selections %s (real %s)",
		f(a.Sim, OutcomeSprint), f(a.Real, OutcomeSprint), f(a.Sim, OutcomeBreakaway), f(a.Real, OutcomeBreakaway),
		f(a.Sim, OutcomeSelection), f(a.Real, OutcomeSelection))
}
