package seed

import "time"

// Stand-in routes (Nov 2026): the WorldDB 2026 mod has no 2026 stage files
// for these races, so their 2026 references run on the newest real route it
// has (route names and lengths from those stage files; profiles from
// import-stages). Dated as the 2026 edition (form, season). The Tour uses the
// real 2023 route (Tour2023); the Vuelta has no file in any year.
const (
	ParisNice2025 = "Paris-Nice 2025"
	Basque2023    = "Tour of the Basque Country 2023"
	Suisse2023    = "Tour de Suisse 2023"
	Dauphine2023  = "Criterium du Dauphine 2023"
)

// standInStages lays a route's stages on consecutive days from start; a
// stage shorter than 60 km is a time trial.
func standInStages(start time.Time, stages []struct {
	route string
	km    float64
}) []realStage {
	var out []realStage
	for i, s := range stages {
		tmpl, tt := "Rolling Stage", false
		if s.km < 60 {
			tmpl, tt = "Short ITT", true
		}
		out = append(out, realStage{s.route, s.km, tmpl, tt, false, start.AddDate(0, 0, i)})
	}
	return out
}

type routeStage = struct {
	route string
	km    float64
}

func parisNice2025Stages() []realStage {
	return standInStages(time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC), []routeStage{
		{"Le Perray-en-Yvelines - Le Perray-en-Yvelines", 158.1},
		{"Montesson - Bellegarde", 184.4},
		{"Nevers Magny-Cours - Nevers (TTT)", 39.7},
		{"Saint-Pourcain-sur-Sioule - La Loge des Gardes", 195.3},
		{"Saint-Just-en-Chevalet - La Cote-Saint-Andre", 188.6},
		{"Pernes-les-Fontaines - Berre-l'Etang", 177.2},
		{"Nice - Auron", 112.4},
		{"Nice - Nice", 126.5},
	})
}

func basque2023Stages() []realStage {
	return standInStages(time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC), []routeStage{
		{"Vitoria-Gasteiz - Labastida", 172.6},
		{"Viana - Leitza", 189.7},
		{"Errenteria - Amasa-Villabona", 160.2},
		{"Santurtzi - Santurtzi", 175.0},
		{"Amorebieta-Etxano - Amorebieta-Etxano", 167.4},
		{"Eibar - Eibar", 138.0},
	})
}

func suisse2023Stages() []realStage {
	return standInStages(time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC), []routeStage{
		{"Einsiedeln - Einsiedeln (ITT)", 16.3},
		{"Beromunster - Nottwil", 175.0},
		{"Tafers - Villars-sur-Ollon", 146.7},
		{"Monthey - Leukerbad", 153.0},
		{"Fiesch - La Punt", 214.4},
		{"La Punt - Oberwil-Lieli", 214.3},
		{"Tubach - Weinfelden", 180.1},
		{"St. Gallen - Abtwil (ITT)", 29.8},
	})
}

func dauphine2023Stages() []realStage {
	return standInStages(time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC), []routeStage{
		{"Chambon-sur-Lac - Chambon-sur-Lac", 162.8},
		{"Brassac-les-Mines - La Chaise-Dieu", 172.7},
		{"Monistrol-sur-Loire - Le Coteau", 202.2},
		{"Cours - Belmont-de-la-Loire (ITT)", 36.8},
		{"Cormoranche-sur-Saone - Salins-les-Bains", 194.0},
		{"Nantua - Crest-Voland", 179.1},
		{"Porte-de-Savoie - Col de la Croix de Fer", 160.2},
		{"Le Pont-de-Claix - La Bastille", 168.8},
	})
}
