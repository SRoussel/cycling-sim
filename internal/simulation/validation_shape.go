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
	CrossSeason  bool      // not the ratings' season (2024): informational
}

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
		Stages:       []Outcome{X, X, S, S, B, B, T, X, S, B, S, B, S, T, X, X, B, S, B, X, S},
	}, {
		// Tirreno 2024: opening ITT; sprints by Philipsen, Bauhaus and Milan
		// (twice); Vingegaard's two mountain wins.
		DisplayName:  "Tirreno-Adriatico 2024 (race shape)",
		InternalName: "Tirreno-Adriatico 2024",
		Stages:       []Outcome{T, S, S, S, X, X, S},
	}, {
		// Catalunya 2024: Schultz from a select group (stage 1), Pogačar's
		// four wins from selections, sprints on stages 4 and 5.
		DisplayName:  "Volta a Catalunya 2024 (race shape)",
		InternalName: "Volta a Catalunya 2024",
		Stages:       []Outcome{X, X, X, S, S, X, X},
	}, {
		DisplayName:  "Tour de France 2023 (race shape)",
		InternalName: "Tour de France 2023",
		Stages:       []Outcome{X, X, S, S, B, X, S, S, B, B, S, B, B, X, B, T, B, B, B, X, S},
		CrossSeason:  true,
	}}
}

// shapeRuns: full stage-race simulations per race-shape check.
const shapeRuns = 32

// ShapeResult is a stage race's simulated outcomes against the real ones.
type ShapeResult struct {
	Name      string
	Agreement float64             // share of (stage, run) outcomes matching the real one
	Real, Sim map[Outcome]float64 // outcomes per race (sim: mean over runs)
	PerStage  []map[Outcome]float64
	Stages    []Outcome
}

// RunShapeValidation simulates each known stage race and classifies how
// every stage was won.
func RunShapeValidation(db *database.DB, crossSeason bool) []ShapeResult {
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
	for _, ref := range KnownRealShapes() {
		if ref.CrossSeason && !crossSeason {
			continue
		}
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
			stages = append(stages, StageConfig{Race: race, Course: course, StageNumber: st.StageNumber, RestDayBefore: st.RestDayBefore, Prestige: st.Prestige})
			infos = append(infos, selection.StageInfo{Course: course, RaceType: race.RaceType})
		}
		start := selection.SelectStageRaceStartList(stageRaceField(ref.InternalName, riders), infos, selection.DefaultConfig(model.RaceTypeRoad, true, len(stages)))
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
		res := ShapeResult{Name: ref.DisplayName, Real: map[Outcome]float64{}, Sim: map[Outcome]float64{}, Stages: ref.Stages}
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
