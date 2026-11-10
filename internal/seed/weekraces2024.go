package seed

import "time"

// Week-long 2024 stage races on their real routes (stage tables from English
// Wikipedia; profiles from import-stages). Approximate fields: the simulator
// picks riders from today's rosters.
const (
	Tirreno2024   = "Tirreno-Adriatico 2024"
	Catalunya2024 = "Volta a Catalunya 2024"
)

func tirreno2024Stages() []realStage {
	mar := func(day int) time.Time { return time.Date(2024, 3, day, 0, 0, 0, 0, time.UTC) }
	return []realStage{
		{"Lido di Camaiore - Lido di Camaiore (ITT)", 10, "Short ITT", true, false, mar(4)},
		{"Camaiore - Follonica", 198, "Rolling Stage", false, false, mar(5)},
		{"Volterra - Gualdo Tadino", 220, "Medium Mountain Stage", false, false, mar(6)},
		{"Arrone - Giulianova", 207, "Medium Mountain Stage", false, false, mar(7)},
		{"Torricella Sicura - Valle Castellana", 146, "Summit Finish Stage", false, false, mar(8)},
		{"Sassoferrato - Cagli", 180, "High Mountain Stage", false, false, mar(9)},
		{"San Benedetto del Tronto - San Benedetto del Tronto", 154, "Flat Sprint Stage (short)", false, false, mar(10)},
	}
}

func catalunya2024Stages() []realStage {
	mar := func(day int) time.Time { return time.Date(2024, 3, day, 0, 0, 0, 0, time.UTC) }
	return []realStage{
		{"Sant Feliu de Guixols - Sant Feliu de Guixols", 174, "Medium Mountain Stage", false, false, mar(18)},
		{"Mataro - Vallter", 186.5, "Summit Finish Stage", false, false, mar(19)},
		{"Sant Joan de les Abadesses - Port Aine", 176.5, "Summit Finish Stage", false, false, mar(20)},
		{"Sort - Lleida", 169, "Flat Sprint Stage", false, false, mar(21)},
		{"Altafulla - Viladecans", 167.5, "Flat Sprint Stage (short)", false, false, mar(22)},
		{"Berga - Queralt", 154.5, "Summit Finish Stage", false, false, mar(23)},
		{"Barcelona - Barcelona", 145.5, "Hilly Stage with Uphill Finish", false, false, mar(24)},
	}
}
