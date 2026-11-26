package seed

import (
	"time"

	"cycling_sim/internal/model"
)

// The rest of the 2026 WorldTour calendar, for the season mode (Nov 2026:
// the season had 21 races and nothing before March). Routes are the newest
// the WorldDB mod has (data/stage_sources.csv; import-stages supplies the
// profiles); distances and stage routes from those files, dates from the
// 2026 UCI WorldTour calendar (English Wikipedia). Not validation
// references. GP de Montréal is missing: the mod has no file for it.

// worldTourClassic is a one-day race seeded under its plain name (the
// season leaves dated names out), on a template course stretched to the
// stage file's length until import-stages supplies the profile.
type worldTourClassic struct {
	name, template string
	km             float64
	date           time.Time
	wind           float64
}

func worldTourClassics() []worldTourClassic {
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, time.UTC) }
	return []worldTourClassic{
		{"Cadel Evans Great Ocean Road Race", "Amstel Gold Race", 193.5, d(2, 1), 18},
		{"Omloop Het Nieuwsblad", "Tour of Flanders", 200.1, d(2, 28), 18},
		{"Tour of Bruges", "Gent-Wevelgem", 208.1, d(3, 25), 22}, // was Classic Brugge-De Panne
		{"Dwars door Vlaanderen", "E3 Saxo Classic", 192.3, d(4, 1), 15},
		{"Eschborn-Frankfurt", "Amstel Gold Race", 216.5, d(5, 1), 10},
		{"Copenhagen Sprint", "Flat Sprint Stage (long)", 245.7, d(6, 14), 18},
		{"Hamburg Cyclassics", "Flat Sprint Stage (long)", 210.8, d(8, 16), 15},
		{"Bretagne Classic", "Clasica San Sebastian", 263.1, d(8, 30), 15},
		{"Grand Prix Cycliste de Quebec", "Amstel Gold Race", 220.6, d(9, 11), 10},
	}
}

func worldTourClassicCourses(templates []courseData) []courseData {
	byName := make(map[string][]model.Segment, len(templates))
	for _, c := range templates {
		byName[c.course.Name] = c.segments
	}
	var out []courseData
	for _, c := range worldTourClassics() {
		out = append(out, cd(c.name, c.km, scaleSegments(byName[c.template], c.km)))
	}
	return out
}

func worldTourClassicRaces(cIDs map[string]int64) []model.Race {
	var out []model.Race
	for _, c := range worldTourClassics() {
		out = append(out, model.Race{Name: c.name, CourseID: cIDs[c.name], RaceType: model.RaceTypeRoad,
			Date: c.date, Temperature: 15, WindSpeedKmh: c.wind})
	}
	return out
}

// WorldTour stage races on the mod's newest routes; a name's year is its
// route's, the dates are 2026's (the season keeps the day and month).
const (
	TourDownUnder2026 = "Tour Down Under 2026"
	UAETour2026       = "UAE Tour 2026"
	RenewiTour2024    = "Renewi Tour 2024"
	Pologne2024       = "Tour de Pologne 2024"
	Guangxi2023       = "Tour of Guangxi 2023"
)

// consecutive dates stage i (from 0) on start + i days.
func consecutive(year int, m time.Month, day int) func(i int) time.Time {
	return func(i int) time.Time { return time.Date(year, m, day+i, 0, 0, 0, 0, time.UTC) }
}

func tourDownUnder2026Stages() []realStage {
	d := consecutive(2026, 1, 20)
	return []realStage{
		{"Adelaide (Prologue)", 4.9, "Short Prologue TT", true, false, d(0)},
		{"Tanunda - Tanunda", 125.3, "Rolling Stage", false, false, d(1)},
		{"Norwood - Uraidla", 155.3, "Hilly Stage with Uphill Finish", false, false, d(2)},
		{"Henley Beach - Nairne", 157.3, "Rolling Stage", false, false, d(3)},
		{"Brighton - Willunga Hill", 181.6, "Hilly Stage with Uphill Finish", false, false, d(4)},
		{"Stirling - Stirling", 168.5, "Medium Mountain Stage", false, false, d(5)},
	}
}

func uaeTour2026Stages() []realStage {
	d := consecutive(2026, 2, 16)
	return []realStage{
		{"Madinat Zayed - Liwa Palace", 150.3, "Rolling Stage", false, false, d(0)},
		{"Hudayriat Island (ITT)", 17.7, "Short ITT", true, false, d(1)},
		{"Umm Al Quwain - Jebel Mobrah", 180.8, "Summit Finish Stage", false, false, d(2)},
		{"Fujairah - Fujairah", 179.2, "Rolling Stage", false, false, d(3)},
		{"Dubai - Hamdan Bin Mohammed Smart University", 173.4, "Flat Sprint Stage", false, false, d(4)},
		{"Al Ain - Jebel Hafeet", 172.6, "Summit Finish Stage", false, false, d(5)},
		{"Abu Dhabi - Abu Dhabi Breakwater", 148.4, "Flat Sprint Stage (short)", false, false, d(6)},
	}
}

func renewiTour2024Stages() []realStage {
	d := consecutive(2024, 8, 19)
	return []realStage{
		{"Riemst - Bilzen", 181.0, "Rolling Stage", false, false, d(0)},
		{"Tessenderlo (ITT)", 21.7, "Short ITT", true, false, d(1)},
		{"Blankenberge - Ardooie", 189.0, "Flat Sprint Stage", false, false, d(2)},
		{"Oostburg - Aalter", 178.6, "Flat Sprint Stage", false, false, d(3)},
		{"Menen - Geraardsbergen", 218.7, "Rolling Stage", false, false, d(4)},
	}
}

func pologne2024Stages() []realStage {
	d := consecutive(2024, 8, 3)
	return []realStage{
		{"Wroclaw - Karpacz", 164.2, "Hilly Stage with Uphill Finish", false, false, d(0)},
		{"Myslakowice - Karpacz (ITT)", 22.4, "Mountain TT", true, false, d(1)},
		{"Walbrzych - Duszniki-Zdroj", 164.5, "Medium Mountain Stage", false, false, d(2)},
		{"Kudowa-Zdroj - Prudnik", 194.5, "Rolling Stage", false, false, d(3)},
		{"Katowice - Katowice", 210.8, "Flat Sprint Stage", false, false, d(4)},
		{"Wadowice - Bukowina Resort", 189.4, "Medium Mountain Stage", false, false, d(5)},
		{"Wieliczka - Krakow", 158.8, "Rolling Stage", false, false, d(6)},
	}
}

func guangxi2023Stages() []realStage {
	d := consecutive(2023, 10, 13)
	return []realStage{
		{"Beihai - Beihai", 143.0, "Flat Sprint Stage (short)", false, false, d(0)},
		{"Beihai - Qinzhou", 158.2, "Flat Sprint Stage", false, false, d(1)},
		{"Nanning - Nanning", 124.7, "Rolling Stage", false, false, d(2)},
		{"Nanning - Nongla Scenic Area", 166.2, "Hilly Stage with Uphill Finish", false, false, d(3)},
		{"Liuzhou - Guilin", 218.4, "Flat Sprint Stage (long)", false, false, d(4)},
		{"Guilin - Guilin", 177.0, "Flat Sprint Stage", false, false, d(5)},
	}
}
