package season

import "math/rand"

// Name pools for young riders turning pro: common given names and surnames
// per nationality (combined at random, so they name nobody in particular).
var namePools = []struct {
	nat         string
	first, last []string
}{
	{"BEL", []string{"Arne", "Jens", "Lander", "Wout", "Seppe", "Ruben", "Thibau", "Milan", "Lennert", "Jarne"},
		[]string{"Claes", "Wouters", "Peeters", "Van Damme", "Maes", "De Smet", "Willems", "Goossens", "Vermeulen", "Hendrickx"}},
	{"FRA", []string{"Hugo", "Théo", "Lucas", "Mathis", "Enzo", "Baptiste", "Clément", "Louis", "Noé", "Axel"},
		[]string{"Moreau", "Girard", "Lefèvre", "Roux", "Fournier", "Bonnet", "Lambert", "Mercier", "Faure", "Chevalier"}},
	{"ITA", []string{"Lorenzo", "Matteo", "Riccardo", "Davide", "Federico", "Tommaso", "Gabriele", "Luca", "Pietro", "Simone"},
		[]string{"Ferrara", "Colombo", "Ricci", "Marini", "Greco", "Bruno", "Gallo", "Conti", "Mancini", "Costa"}},
	{"ESP", []string{"Pablo", "Álvaro", "Iker", "Unai", "Javier", "Sergio", "Mikel", "Raúl", "Hugo", "Adrián"},
		[]string{"García", "Fernández", "Ruiz", "Moreno", "Navarro", "Iglesias", "Ortega", "Delgado", "Castro", "Molina"}},
	{"NED", []string{"Daan", "Sem", "Thijs", "Bram", "Jesse", "Luuk", "Stijn", "Ruben", "Koen", "Niels"},
		[]string{"de Vries", "Bakker", "Visser", "Smit", "Meijer", "de Boer", "Mulder", "Bos", "Dekker", "Brouwer"}},
	{"GBR", []string{"Oliver", "Harry", "Jack", "Charlie", "Alfie", "Finley", "Archie", "Joseph", "Max", "Ethan"},
		[]string{"Walker", "Wright", "Hughes", "Clarke", "Wood", "Hall", "Turner", "Harrison", "Cooper", "Ward"}},
	{"DEN", []string{"Magnus", "Emil", "Frederik", "Oscar", "Mikkel", "Rasmus", "Anton", "Jonas", "Victor", "Lucas"},
		[]string{"Nielsen", "Jensen", "Hansen", "Larsen", "Sørensen", "Rasmussen", "Kristensen", "Poulsen", "Møller", "Lund"}},
	{"NOR", []string{"Henrik", "Sindre", "Eirik", "Ola", "Kristian", "Tobias", "Martin", "Even", "Sander", "Vegard"},
		[]string{"Berg", "Haugen", "Lie", "Moen", "Dahl", "Strand", "Solberg", "Bakken", "Lunde", "Eide"}},
	{"GER", []string{"Felix", "Jonas", "Leon", "Paul", "Niklas", "Moritz", "Tim", "Lukas", "Julian", "Jannik"},
		[]string{"Krüger", "Hoffmann", "Schäfer", "Koch", "Richter", "Wolf", "Neumann", "Schwarz", "Zimmermann", "Braun"}},
	{"AUS", []string{"Lachlan", "Cooper", "Riley", "Mitchell", "Callum", "Jayden", "Hamish", "Liam", "Kai", "Blake"},
		[]string{"Thompson", "Mitchell", "Kelly", "Ryan", "Campbell", "Stewart", "Robertson", "Murray", "Bennett", "Ross"}},
	{"USA", []string{"Tyler", "Owen", "Caleb", "Logan", "Wyatt", "Dylan", "Austin", "Cole", "Gavin", "Nolan"},
		[]string{"Parker", "Hayes", "Brooks", "Foster", "Reed", "Bryant", "Sullivan", "Coleman", "Porter", "Wells"}},
	{"COL", []string{"Santiago", "Sebastián", "Andrés", "Camilo", "Julián", "Brayan", "Esteban", "Nicolás", "Daniel", "Juan José"},
		[]string{"Rodríguez", "Gómez", "Rojas", "Vargas", "Ospina", "Cárdenas", "Pineda", "Restrepo", "Zapata", "Mejía"}},
	{"SLO", []string{"Luka", "Jan", "Žiga", "Nejc", "Matic", "Gašper", "Anže", "Tilen", "Rok", "Jure"},
		[]string{"Novak", "Kranjc", "Zupan", "Kovač", "Potočnik", "Golob", "Kos", "Vidmar", "Žagar", "Hribar"}},
	{"SUI", []string{"Noah", "Elia", "Nico", "Fabio", "Silvan", "Robin", "Jonas", "Yannick", "Dario", "Cyrill"},
		[]string{"Müller", "Meier", "Keller", "Huber", "Brunner", "Baumann", "Frei", "Gerber", "Widmer", "Steiner"}},
	{"POR", []string{"Diogo", "Tiago", "Rodrigo", "Gonçalo", "Afonso", "Duarte", "Rafael", "Martim", "Tomás", "Guilherme"},
		[]string{"Silva", "Santos", "Ferreira", "Pereira", "Oliveira", "Martins", "Sousa", "Lopes", "Marques", "Carvalho"}},
}

// newName draws a nationality and an unused name for a young rider.
func newName(rng *rand.Rand, taken map[string]bool) (nat, name string) {
	for tries := 0; ; tries++ {
		p := namePools[rng.Intn(len(namePools))]
		name = p.first[rng.Intn(len(p.first))] + " " + p.last[rng.Intn(len(p.last))]
		if !taken[name] || tries > 50 {
			taken[name] = true
			return p.nat, name
		}
	}
}
