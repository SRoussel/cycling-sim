package seed

import (
	"fmt"
	"time"

	"cycling_sim/internal/model"
)

// Tour2023 is the name of the stage race with the real 2023 Tour de France
// route (the stage-race hold-out). The generic "Tour de France" stays as is.
const Tour2023 = "Tour de France 2023"

// realStage is one stage of a real stage race: route and distance from the
// English Wikipedia stage table, and a generic course whose shape stands in
// for the real profile until one is imported (cycling-sim import-stages).
type realStage struct {
	route    string
	km       float64
	template string
	tt       bool
	rest     bool // rest day before
	date     time.Time
}

// realStageRace is a stage race seeded on its real route.
type realStageRace struct {
	name   string // stage race name
	prefix string // stage race names start with it (model.StageRaceNamePrefixes)
	stages []realStage
}

func (r realStageRace) stageName(n int) string {
	return fmt.Sprintf("%s Stage %d: %s", r.prefix, n, r.stages[n-1].route)
}

// realStageRaces are the stage races seeded on their real routes.
func realStageRaces() []realStageRace {
	return []realStageRace{
		{Tour2023, "TdF 2023", tdf2023Stages()},
		{Giro2024, "Giro 2024", giro2024Stages()},
		{Tirreno2024, "Tirreno 2024", tirreno2024Stages()},
		{Catalunya2024, "Catalunya 2024", catalunya2024Stages()},
		{Giro2026, "Giro 2026", giro2026Stages()},
		{Tirreno2026, "Tirreno 2026", tirreno2026Stages()},
		{Romandie2026, "Romandie 2026", romandie2026Stages()},
		{ParisNice2025, "Paris-Nice 2025", parisNice2025Stages()},
		{Basque2023, "Basque 2023", basque2023Stages()},
		{Suisse2023, "Suisse 2023", suisse2023Stages()},
		{Dauphine2023, "Dauphine 2023", dauphine2023Stages()},
		{TourDownUnder2026, "TDU 2026", tourDownUnder2026Stages()},
		{UAETour2026, "UAE Tour 2026", uaeTour2026Stages()},
		{RenewiTour2024, "Renewi 2024", renewiTour2024Stages()},
		{Pologne2024, "Pologne 2024", pologne2024Stages()},
		{Guangxi2023, "Guangxi 2023", guangxi2023Stages()},
	}
}

func tdf2023Stages() []realStage {
	jul := func(day int) time.Time { return time.Date(2023, 7, day, 0, 0, 0, 0, time.UTC) }
	return []realStage{
		{"Bilbao - Bilbao", 182, "Medium Mountain Stage", false, false, jul(1)},
		{"Vitoria-Gasteiz - San Sebastian", 208.9, "Rolling Stage", false, false, jul(2)},
		{"Amorebieta-Etxano - Bayonne", 193.5, "Flat Sprint Stage", false, false, jul(3)},
		{"Dax - Nogaro", 181.8, "Flat Sprint Stage", false, false, jul(4)},
		{"Pau - Laruns", 162.7, "High Mountain Stage", false, false, jul(5)},
		{"Tarbes - Cauterets-Cambasque", 144.9, "Pyrenees Summit Finish", false, false, jul(6)},
		{"Mont-de-Marsan - Bordeaux", 169.9, "Flat Sprint Stage (short)", false, false, jul(7)},
		{"Libourne - Limoges", 200.7, "Rolling Stage", false, false, jul(8)},
		{"Saint-Leonard-de-Noblat - Puy de Dome", 182.4, "Summit Finish Stage", false, false, jul(9)},
		{"Vulcania - Issoire", 167.2, "Medium Mountain Stage", false, true, jul(11)},
		{"Clermont-Ferrand - Moulins", 179.8, "Flat Sprint Stage", false, false, jul(12)},
		{"Roanne - Belleville-en-Beaujolais", 168.8, "Medium Mountain Stage", false, false, jul(13)},
		{"Chatillon-sur-Chalaronne - Grand Colombier", 137.8, "Summit Finish Stage", false, false, jul(14)},
		{"Annemasse - Morzine", 151.8, "High Mountain Stage", false, false, jul(15)},
		{"Les Gets - Saint-Gervais Mont-Blanc", 179, "Summit Finish Stage", false, false, jul(16)},
		{"Passy - Combloux (ITT)", 22.4, "Short ITT", true, true, jul(18)},
		{"Saint-Gervais Mont-Blanc - Courchevel", 165.7, "High Mountain Stage", false, false, jul(19)},
		{"Moutiers - Bourg-en-Bresse", 184.9, "Flat Sprint Stage", false, false, jul(20)},
		{"Moirans-en-Montagne - Poligny", 172.8, "Rolling Stage", false, false, jul(21)},
		{"Belfort - Le Markstein", 133.5, "Medium Mountain Stage", false, false, jul(22)},
		{"Saint-Quentin-en-Yvelines - Paris", 115.1, "Flat Sprint Stage (short)", false, false, jul(23)},
	}
}

// realStageCourses builds the real-route stage courses from their
// templates' segments.
func realStageCourses(templates []courseData) []courseData {
	byName := make(map[string][]model.Segment, len(templates))
	for _, c := range templates {
		byName[c.course.Name] = c.segments
	}
	var out []courseData
	for _, r := range realStageRaces() {
		for i, s := range r.stages {
			out = append(out, cd(r.stageName(i+1), s.km, scaleSegments(byName[s.template], s.km)))
		}
	}
	return out
}

// scaleSegments stretches a template's segments to km (gradients kept).
func scaleSegments(segs []model.Segment, km float64) []model.Segment {
	sum := 0.0
	for _, s := range segs {
		sum += s.DistanceKm
	}
	out := append([]model.Segment(nil), segs...)
	for i := range out {
		out[i].DistanceKm *= km / sum
	}
	return out
}

func realStageRaceRaces(cIDs map[string]int64) []model.Race {
	var out []model.Race
	for _, r := range realStageRaces() {
		for i, s := range r.stages {
			rt := model.RaceTypeRoad
			if s.tt {
				rt = model.RaceTypeTT
			}
			n := r.stageName(i + 1)
			out = append(out, model.Race{Name: n, CourseID: cIDs[n], RaceType: rt,
				Date: s.date, Temperature: 25, WindSpeedKmh: 10})
		}
	}
	return out
}
