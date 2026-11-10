package seed

import "time"

// Giro2024 is the stage race with the real 2024 Giro d'Italia route (a
// never-fitted GC reference; stage profiles from import-stages).
const Giro2024 = "Giro d'Italia 2024"

// giro2024Stages: routes, distances, types and rest days from the English
// Wikipedia stage table of the 2024 Giro d'Italia.
func giro2024Stages() []realStage {
	may := func(day int) time.Time { return time.Date(2024, 5, day, 0, 0, 0, 0, time.UTC) }
	return []realStage{
		{"Venaria Reale - Turin", 140, "Hilly Stage with Uphill Finish", false, false, may(4)},
		{"San Francesco al Campo - Santuario di Oropa", 161, "Summit Finish Stage", false, false, may(5)},
		{"Novara - Fossano", 166, "Flat Sprint Stage", false, false, may(6)},
		{"Acqui Terme - Andora", 190, "Flat Sprint Stage", false, false, may(7)},
		{"Genoa - Lucca", 178, "Rolling Stage", false, false, may(8)},
		{"Viareggio - Rapolano Terme", 180, "Rolling Stage", false, false, may(9)},
		{"Foligno - Perugia (ITT)", 40.6, "Long ITT", true, false, may(10)},
		{"Spoleto - Prati di Tivo", 152, "Summit Finish Stage", false, false, may(11)},
		{"Avezzano - Naples", 214, "Flat Sprint Stage (long)", false, false, may(12)},
		{"Pompei - Cusano Mutri", 142, "Medium Mountain Stage", false, true, may(14)},
		{"Foiano di Val Fortore - Francavilla al Mare", 207, "Flat Sprint Stage (long)", false, false, may(15)},
		{"Martinsicuro - Fano", 193, "Rolling Stage", false, false, may(16)},
		{"Riccione - Cento", 179, "Flat Sprint Stage", false, false, may(17)},
		{"Castiglione delle Stiviere - Desenzano del Garda (ITT)", 31.2, "Long ITT", true, false, may(18)},
		{"Manerba del Garda - Livigno", 222, "High Mountain Stage", false, false, may(19)},
		{"Livigno - Santa Cristina Valgardena", 118.7, "Dolomites Summit Finish", false, true, may(21)},
		{"Selva di Val Gardena - Passo del Brocon", 159, "High Mountain Stage", false, false, may(22)},
		{"Fiera di Primiero - Padua", 178, "Flat Sprint Stage", false, false, may(23)},
		{"Mortegliano - Sappada", 157, "Medium Mountain Stage", false, false, may(24)},
		{"Alpago - Bassano del Grappa", 184, "High Mountain Stage", false, false, may(25)},
		{"Rome - Rome", 125, "Flat Sprint Stage (short)", false, false, may(26)},
	}
}
