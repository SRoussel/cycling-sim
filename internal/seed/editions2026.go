package seed

import (
	"time"

	"cycling_sim/internal/model"
)

// 2026 editions on their real routes (Nov 2026: 2026 is the validation
// season). Stage tables from English Wikipedia; route profiles from the
// WorldDB 2026 stage files via import-stages (data/stage_sources.csv). Races
// whose 2026 file the mod lacks run on the generic course of the same name.
const (
	Giro2026     = "Giro d'Italia 2026"
	Tirreno2026  = "Tirreno-Adriatico 2026"
	Romandie2026 = "Tour de Romandie 2026"
)

func giro2026Stages() []realStage {
	may := func(day int) time.Time { return time.Date(2026, 5, day, 0, 0, 0, 0, time.UTC) }
	return []realStage{
		{"Nesebar - Burgas", 147, "Flat Sprint Stage", false, false, may(8)},
		{"Burgas - Veliko Tarnovo", 221, "Rolling Stage", false, false, may(9)},
		{"Plovdiv - Sofia", 175, "Flat Sprint Stage", false, false, may(10)},
		{"Catanzaro - Cosenza", 138, "Flat Sprint Stage (short)", false, true, may(12)},
		{"Praia a Mare - Potenza", 203, "Hilly Stage with Uphill Finish", false, false, may(13)},
		{"Paestum - Naples", 142, "Flat Sprint Stage (short)", false, false, may(14)},
		{"Formia - Blockhaus", 244, "Summit Finish Stage", false, false, may(15)},
		{"Chieti - Fermo", 156, "Hilly Stage with Uphill Finish", false, false, may(16)},
		{"Cervia - Corno alle Scale", 184, "Summit Finish Stage", false, false, may(17)},
		{"Viareggio - Massa (ITT)", 42, "Long ITT", true, true, may(19)},
		{"Porcari - Chiavari", 195, "Rolling Stage", false, false, may(20)},
		{"Imperia - Novi Ligure", 175, "Flat Sprint Stage", false, false, may(21)},
		{"Alessandria - Verbania", 189, "Rolling Stage", false, false, may(22)},
		{"Aosta - Pila", 133, "Summit Finish Stage", false, false, may(23)},
		{"Voghera - Milan", 157, "Flat Sprint Stage", false, false, may(24)},
		{"Bellinzona - Cari", 113, "Summit Finish Stage", false, true, may(26)},
		{"Cassano d'Adda - Andalo", 202, "Medium Mountain Stage", false, false, may(27)},
		{"Fai della Paganella - Pieve di Soligo", 171, "Rolling Stage", false, false, may(28)},
		{"Feltre - Alleghe", 151, "High Mountain Stage", false, false, may(29)},
		{"Gemona del Friuli - Piancavallo", 200, "Summit Finish Stage", false, false, may(30)},
		{"Rome - Rome", 131, "Flat Sprint Stage (short)", false, false, may(31)},
	}
}

func tirreno2026Stages() []realStage {
	mar := func(day int) time.Time { return time.Date(2026, 3, day, 0, 0, 0, 0, time.UTC) }
	return []realStage{
		{"Lido di Camaiore - Lido di Camaiore (ITT)", 11.5, "Short ITT", true, false, mar(9)},
		{"Camaiore - San Gimignano", 206, "Rolling Stage", false, false, mar(10)},
		{"Cortona - Magliano de' Marsi", 221, "Flat Sprint Stage (long)", false, false, mar(11)},
		{"Tagliacozzo - Martinsicuro", 213, "Rolling Stage", false, false, mar(12)},
		{"Marotta-Mondolfo - Mombaroccio", 184, "Hilly Stage with Uphill Finish", false, false, mar(13)},
		{"San Severino Marche - Camerino", 188, "Medium Mountain Stage", false, false, mar(14)},
		{"Civitanova Marche - San Benedetto del Tronto", 142, "Flat Sprint Stage (short)", false, false, mar(15)},
	}
}

func romandie2026Stages() []realStage {
	apr := func(day int) time.Time { return time.Date(2026, 4, day, 0, 0, 0, 0, time.UTC) }
	may := func(day int) time.Time { return time.Date(2026, 5, day, 0, 0, 0, 0, time.UTC) }
	return []realStage{
		{"Villars-sur-Glane (Prologue)", 3.2, "Short ITT", true, false, apr(28)},
		{"Martigny - Martigny", 171.2, "Medium Mountain Stage", false, false, apr(29)},
		{"Rue - Vucherens", 173.1, "Rolling Stage", false, false, apr(30)},
		{"Orbe - Orbe", 176.6, "Medium Mountain Stage", false, false, may(1)},
		{"Broc - Charmey", 149.6, "Medium Mountain Stage", false, false, may(2)},
		{"Lucens - Leysin", 178.2, "Summit Finish Stage", false, false, may(3)},
	}
}

// classic2026 is a 2026 one-day edition seeded as its own course (copying
// the generic course's segments until import-stages supplies the profile).
type classic2026 struct {
	name, base string
	km         float64 // length of the 2026 stage file
	date       time.Time
	wind       float64
}

func classics2026() []classic2026 {
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, time.UTC) }
	return []classic2026{
		{"Strade Bianche 2026", "Strade Bianche 2024", 200, d(3, 7), 10},
		{"E3 Saxo Classic 2026", "E3 Saxo Classic", 209, d(3, 27), 15},
		{"Gent-Wevelgem 2026", "Gent-Wevelgem", 241, d(3, 29), 22},
		{"Tour of Flanders 2026", "Tour of Flanders", 275, d(4, 5), 18},
		{"Paris-Roubaix 2026", "Paris-Roubaix", 264, d(4, 12), 20},
		{"La Fleche Wallonne 2026", "La Fleche Wallonne", 208, d(4, 22), 12},
	}
}

func classic2026Courses(templates []courseData) []courseData {
	byName := make(map[string][]model.Segment, len(templates))
	for _, c := range templates {
		byName[c.course.Name] = c.segments
	}
	var out []courseData
	for _, c := range classics2026() {
		out = append(out, cd(c.name, c.km, scaleSegments(byName[c.base], c.km)))
	}
	return out
}

func classic2026Races(cIDs map[string]int64) []model.Race {
	var out []model.Race
	for _, c := range classics2026() {
		out = append(out, model.Race{Name: c.name, CourseID: cIDs[c.name], RaceType: model.RaceTypeRoad,
			Date: c.date, Temperature: 12, WindSpeedKmh: c.wind})
	}
	return out
}
