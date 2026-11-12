package seed

import (
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/simulation"
	"cycling_sim/internal/stagefile"
)

type Result struct {
	Riders     int
	Courses    int
	Races      int
	StageRaces int
	Warnings   int
}

func Seed(db *database.DB) (Result, error) {
	var res Result
	if db == nil {
		return res, fmt.Errorf("db is nil")
	}

	// Riders are seeded into an empty database only (later rating changes go
	// through ApplyRatings). Courses, races and stage races are added by name
	// when missing, so new seed content reaches an existing database without
	// clearing its saved results.
	// Riders missing by name are added (a new riders.csv row or a WorldDB
	// import), so they reach an existing database too.
	existing, _ := db.ListRiders()
	have := make(map[string]bool, len(existing))
	for _, r := range existing {
		have[normalizeName(r.Name)] = true
	}
	built, err := BuildRiders()
	if err != nil {
		return res, err
	}
	for i := range built {
		if have[normalizeName(built[i].Name)] {
			continue
		}
		if err := db.CreateRider(&built[i]); err != nil {
			continue
		}
		res.Riders++
	}

	courseIDs, created, warnings := seedCourses(db)
	res.Courses = created
	res.Warnings += warnings

	raceIDs, created, warnings := seedRaces(db, courseIDs)
	res.Races = created
	res.Warnings += warnings

	created, warnings = seedStageRaces(db, raceIDs)
	res.StageRaces = created
	res.Warnings += warnings
	res.Warnings += ApplyStagePrestige(db)

	return res, nil
}

func ClearDBFile(dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("dbPath is empty")
	}
	if err := os.Remove(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	return nil
}

// DataPath returns where a data file lives (next to riders.csv).
func DataPath(name string) string {
	return filepath.Join(filepath.Dir(findDataFile("riders.csv")), name)
}

// FittedFile holds threshold corrections fitted to real results.
const FittedFile = "rider_fitted.csv"

// findDataFile locates a data file relative to the binary or source directory.
func findDataFile(name string) string {
	candidates := []string{
		filepath.Join("data", name),
		filepath.Join("..", "..", "data", name),
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		srcDir := filepath.Dir(thisFile)
		candidates = append(candidates, filepath.Join(srcDir, "..", "..", "data", name))
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return filepath.Join("data", name)
}

// RatingLayers are the rider-rating files applied on top of riders.csv, in
// order (later layers win): LLM-drafted profiles, ratings derived from
// imported results, curated overrides, and fitted threshold corrections.
var RatingLayers = []string{"rider_llm.csv", "rider_derived.csv", "rider_overrides.csv", FittedFile}

// BuildRiders reads riders.csv and applies every rating layer; with a
// WorldDB import (WorldDBFile) it also sets real ages and adds the WorldTeam
// and ProTeam riders riders.csv lacks (see worlddb.go).
func BuildRiders() ([]model.Rider, error) {
	var layers []map[string]RiderOverride
	for _, name := range RatingLayers {
		l, err := LoadOverrides(findDataFile(name))
		if err != nil {
			return nil, err
		}
		layers = append(layers, l)
	}
	riders, err := readRidersCSV(findDataFile("riders.csv"), layers...)
	if err != nil {
		return nil, err
	}
	wdb, err := loadWorldDB()
	if err != nil || wdb == nil {
		return scaleThresholdsWithMass(riders), err
	}
	// Fits are trained on ratings without the fitted layer (a correction
	// for particular races, not a property of the rating scale).
	train, err := readRidersCSV(findDataFile("riders.csv"), layers[:len(layers)-1]...)
	if err != nil {
		return nil, err
	}
	return scaleThresholdsWithMass(withWorldDB(riders, train, wdb)), nil
}

// UnfittedFTPs returns each rider's threshold from every rating layer except
// the fitted one (keyed by normalised name): the base a new fit starts from,
// whatever the database or an earlier fit holds.
func UnfittedFTPs() (map[string]int, error) {
	var layers []map[string]RiderOverride
	for _, name := range RatingLayers {
		if name == FittedFile {
			continue
		}
		l, err := LoadOverrides(findDataFile(name))
		if err != nil {
			return nil, err
		}
		layers = append(layers, l)
	}
	riders, err := readRidersCSV(findDataFile("riders.csv"), layers...)
	if err != nil {
		return nil, err
	}
	if wdb, err := loadWorldDB(); err != nil {
		return nil, err
	} else if wdb != nil {
		riders = withWorldDB(riders, riders, wdb)
	}
	riders = scaleThresholdsWithMass(riders)
	out := make(map[string]int, len(riders))
	for _, r := range riders {
		out[normalizeName(r.Name)] = r.FTPWatts
	}
	return out, nil
}

// readRidersCSV reads riders from a CSV file and applies rating layers.
func readRidersCSV(path string, layers ...map[string]RiderOverride) ([]model.Rider, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV has no data rows")
	}

	var out []model.Rider
	for _, row := range records[1:] {
		if len(row) < 19 {
			continue
		}
		weightKg, _ := strconv.ParseFloat(row[4], 64)
		heightCm, _ := strconv.ParseFloat(row[5], 64)
		ftpWatts, _ := strconv.Atoi(row[6])
		sprintWatts, _ := strconv.Atoi(row[7])
		vo2maxWatts, _ := strconv.Atoi(row[8])
		cda, _ := strconv.ParseFloat(row[9], 64)
		climbing, _ := strconv.Atoi(row[10])
		sprint, _ := strconv.Atoi(row[11])
		tt, _ := strconv.Atoi(row[12])
		classics, _ := strconv.Atoi(row[13])
		endurance, _ := strconv.Atoi(row[14])
		recovery, _ := strconv.Atoi(row[15])
		acceleration, _ := strconv.Atoi(row[16])
		consistency, _ := strconv.Atoi(row[17])
		handling, _ := strconv.Atoi(row[18])

		rider := model.Rider{
			Name:              row[1],
			Team:              row[2],
			Nationality:       row[3],
			Age:               simulation.DefaultAgeFromName(row[1]),
			WeightKg:          weightKg,
			HeightCm:          heightCm,
			FTPWatts:          ftpWatts,
			MaxSprintWatts:    sprintWatts,
			VO2MaxWatts:       vo2maxWatts,
			CdA:               cda,
			ClimbingAbility:   climbing,
			SprintAbility:     sprint,
			TTAbility:         tt,
			DescendingAbility: classics,
			Endurance:         endurance,
			Recovery:          recovery,
			Consistency:       consistency,
			TacticalAwareness: acceleration,
			Handling:          handling,
		}
		baseKg, baseCm, baseCdA := rider.WeightKg, rider.HeightCm, rider.CdA
		for _, overrides := range layers {
			if o, ok := overrides[normalizeName(rider.Name)]; ok {
				if err := o.Apply(&rider); err != nil {
					return nil, err
				}
			}
		}
		// A layer that corrects a rider's body (weight, height) without
		// setting CdA leaves the template body's aerodynamics: scale CdA by
		// the change in frontal area.
		if rider.CdA == baseCdA && (rider.WeightKg != baseKg || rider.HeightCm != baseCm) {
			rider.CdA = math.Round(baseCdA*frontalArea(rider.HeightCm, rider.WeightKg)/frontalArea(baseCm, baseKg)*1000) / 1000
		}
		out = append(out, rider)
	}
	return out, nil
}

// powerMassExponent: across trained cyclists, absolute aerobic power grows
// with body mass to about the 0.75th power (published allometric exponents
// run ~0.67-0.8): heavier riders have more watts but fewer per kilogram.
const powerMassExponent = 0.75

// ratingsMassExponent is how threshold scaled with mass in the full rated
// field (922 riders, Nov 2026): mass^0.63. The ratings were drafted largely
// as W/kg, leaving heavy riders short of watts; with drag following body
// size that made big engines no faster on the flat than riders 10 kg
// lighter. massRefKg is the field's geometric mean mass.
const (
	ratingsMassExponent = 0.63
	massRefKg           = 68.1
)

// scaleThresholdsWithMass corrects every threshold to powerMassExponent:
// × (m/massRefKg)^(0.75-0.63), +2.4% at 83 kg, -2.5% at 55 kg, none at the
// reference mass. Applied once to the finished field (after the WorldDB
// merge), identically for the fitted and unfitted builds.
func scaleThresholdsWithMass(riders []model.Rider) []model.Rider {
	for i := range riders {
		r := &riders[i]
		if r.WeightKg <= 0 || r.FTPWatts <= 0 {
			continue
		}
		f := math.Pow(r.WeightKg/massRefKg, powerMassExponent-ratingsMassExponent)
		r.FTPWatts = int(math.Round(float64(r.FTPWatts) * f))
	}
	return riders
}

// ApplyRatings recomputes every rider from riders.csv and the rating layers
// and updates the riders already in the database (matched by name) in place,
// so saved results survive. It returns how many riders were updated.
func ApplyRatings(db *database.DB) (int, error) {
	built, err := BuildRiders()
	if err != nil {
		return 0, err
	}
	existing, err := db.ListRiders()
	if err != nil {
		return 0, err
	}
	ids := map[string]int64{}
	for _, r := range existing {
		ids[normalizeName(r.Name)] = r.ID
	}
	var updates []model.Rider
	for _, r := range built {
		if id, ok := ids[normalizeName(r.Name)]; ok {
			r.ID = id
			updates = append(updates, r)
		}
	}
	return len(updates), db.UpdateRiderRatings(updates)
}

// ---- Seed courses/races/stage races (copied from CLI seed logic, kept deterministic) ----

type courseData struct {
	course   model.Course
	segments []model.Segment
}

// cd defines a course. Many course definitions only list the decisive
// sections, so the segments would otherwise sum to less than the real race
// distance. fitToDistance pads (or trims) the longest flat road segment so
// the simulated distance matches distKm.
func cd(name string, distKm float64, segments []model.Segment) courseData {
	return courseData{course: model.Course{Name: name, TotalDistanceKm: distKm}, segments: CourseSegments(name, distKm, segments)}
}

// LocalProfilesFile holds course profiles derived from locally installed
// stage files (never committed: the source files belong to their authors).
const LocalProfilesFile = "local/course_profiles.json"

// CourseSegments finalises a course's segments: fitted to its distance, with
// stable synthetic headings.
func CourseSegments(name string, distKm float64, segments []model.Segment) []model.Segment {
	return withHeadings(name, fitToDistance(segments, distKm))
}

// withHeadings assigns synthetic but stable segment headings: a random walk
// seeded by the course name, turning up to 90° between segments, so wind
// exposure varies along the route. Replace with real course geometry when
// available.
func withHeadings(course string, segments []model.Segment) []model.Segment {
	out := append([]model.Segment(nil), segments...)
	heading := float64(hashString(course) % 360)
	for i := range out {
		if i > 0 {
			turn := float64(hashString(fmt.Sprintf("%s#%d", course, i))%181) - 90
			heading = math.Mod(heading+turn+360, 360)
		}
		out[i].HeadingDeg = heading
	}
	return out
}

// syntheticWindDirection gives a race a stable wind direction (compass
// degrees the wind blows from) when the seed data doesn't specify one.
func syntheticWindDirection(race string) float64 {
	return float64(hashString("wind:"+race) % 360)
}

func hashString(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func fitToDistance(segments []model.Segment, distKm float64) []model.Segment {
	sum := 0.0
	for _, s := range segments {
		sum += s.DistanceKm
	}
	delta := distKm - sum
	if math.Abs(delta) < 0.05 {
		return segments
	}
	best := -1
	for i, s := range segments {
		if s.SurfaceType == model.SurfaceRoad && math.Abs(s.GradientPct) < 1.0 &&
			(best < 0 || s.DistanceKm > segments[best].DistanceKm) {
			best = i
		}
	}
	if best < 0 || segments[best].DistanceKm+delta < 1 {
		return segments
	}
	out := append([]model.Segment(nil), segments...)
	out[best].DistanceKm = math.Round((out[best].DistanceKm+delta)*10) / 10
	return out
}

func seg(order int, name string, distKm, gradPct float64, surface model.SurfaceType, technical bool) model.Segment {
	return model.Segment{SegmentOrder: order, Name: name, DistanceKm: distKm, GradientPct: gradPct, SurfaceType: surface, IsTechnical: technical}
}

func road(order int, name string, distKm, gradPct float64) model.Segment {
	return seg(order, name, distKm, gradPct, model.SurfaceRoad, false)
}

func cobbles(order int, name string, distKm, gradPct float64) model.Segment {
	return seg(order, name, distKm, gradPct, model.SurfaceCobbles, true)
}

func seedCourses(db *database.DB) (map[string]int64, int, int) {
	courses := []courseData{
		cd("Milan-San Remo", 290, []model.Segment{road(1, "Flat approach", 230, 0), road(2, "Cipressa", 5.6, 4.1), road(3, "Cipressa descent", 6, -4.0), road(4, "Poggio", 3.7, 3.7), road(5, "Poggio descent", 4, -5.0), road(6, "Via Roma finish", 1, 0)}),
		cd("Tour of Flanders", 270, []model.Segment{road(1, "Flat approach", 140, 0), cobbles(2, "Oude Kwaremont", 2.2, 4.0), road(3, "Road section", 20, 0.3), cobbles(4, "Koppenberg", 0.6, 11.6), road(5, "Road section", 30, 0.3), cobbles(6, "Paterberg", 0.4, 12.9), road(7, "Road section", 15, 0.3), cobbles(8, "Oude Kwaremont (2nd)", 2.2, 4.0), road(9, "Road section", 10, 0.3), cobbles(10, "Paterberg (2nd)", 0.4, 12.9), road(11, "Run-in to Oudenaarde", 18, 0)}),
		cd("Paris-Roubaix", 257, []model.Segment{road(1, "Road to Cancanson", 94, 0.2), cobbles(2, "Cancanson-Vieux-Reng", 1.8, 0.8), road(3, "Road section", 18, 0.1), cobbles(4, "Saint-Python-Viesly", 2.1, 1.2), road(5, "Road section", 25, 0.3), cobbles(6, "Trouee d'Arenberg", 2.3, 0), road(7, "Road section", 30, 0.2), cobbles(8, "Mons-en-Pevele", 3.0, 1.5), road(9, "Road section", 20, 0.1), cobbles(10, "Bouvines", 2.1, 0.3), road(11, "Road section", 18, 0.2), cobbles(12, "Saint-Leger-les-Dooms", 2.3, 0.6), road(13, "Road section", 35, 0.1), cobbles(14, "Modules sector", 3.2, 0.8), road(15, "Road section", 12, 0.2), cobbles(16, "Petite Forets", 3.4, 1.1), road(17, "Road to Carrefour", 7, 0.1), cobbles(18, "Carrefour de l'Arbre", 2.1, 0), road(19, "Road section", 5, 0.1), cobbles(20, "Gruson-Roubaix", 1.8, 0.5), road(21, "Velodrome finish", 0.5, 0)}),
		cd("Liege-Bastogne-Liege", 258, []model.Segment{road(1, "Liege to Bastogne", 60, 0.3), road(2, "Early Ardennes approach", 40, 0.5), road(3, "Cote de Wanne", 5, 7.5), road(4, "Descent to Stavelot", 7, -3.5), road(5, "Cote de Stockeu", 1, 12.0), road(6, "Valley road", 22, 0.2), road(7, "Cote de Haute Levee", 4, 6.0), road(8, "Road to Redoute", 12, 0.2), road(9, "Cote de la Redoute", 2.0, 8.8), road(10, "Valley between climbs", 8, -1.0), road(11, "Roche aux Faucons", 1.5, 11.0), road(12, "Road section", 8, -0.5), road(13, "Cote de Saint-Nicolas", 1.2, 8.6), road(14, "Descent to Liege", 3, -4.0), road(15, "Liege finish", 2, 0)}),
		cd("Il Lombardia", 243, []model.Segment{road(1, "Bergamo start", 20, 0.1), road(2, "Lake Como approach", 50, 0.3), road(3, "Madonna del Ghisallo", 10, 5.5), road(4, "Ghisallo descent", 8, -5.5), road(5, "Valley road to Civiglio", 20, 0.3), road(6, "Civiglio", 4.2, 9.7), road(7, "Civiglio descent", 5, -6.0), road(8, "Valley to San Fermo", 20, 0.3), road(9, "San Fermo della Battaglia", 2.7, 7.2), road(10, "Descent and road", 20, -0.8), road(11, "Run-in to Como", 10, 0)}),
		cd("Strade Bianche", 184, []model.Segment{road(1, "Road section", 50, 0.3), seg(2, "Sterrato sector 1", 3, 2.0, model.SurfaceCobbles, true), road(3, "Road section", 30, 0.5), seg(4, "Sterrato sector 2", 4, 3.0, model.SurfaceCobbles, true), road(5, "Road section", 25, 0.5), seg(6, "Sterrato sector 3", 5, 2.5, model.SurfaceCobbles, true), road(7, "Road to Siena", 20, 1.0), road(8, "Piazza del Campo climb", 3, 7.0)}),
		cd("Strade Bianche 2024", 215, []model.Segment{road(1, "Road section", 50, 0.3), seg(2, "Sterrato sector 1", 3, 2.0, model.SurfaceCobbles, true), road(3, "Road section", 30, 0.5), seg(4, "Sterrato sector 2", 4, 3.0, model.SurfaceCobbles, true), road(5, "Road section", 25, 0.5), seg(6, "Sterrato sector 3", 5, 2.5, model.SurfaceCobbles, true), road(7, "Road to Siena", 20, 1.0), road(8, "Piazza del Campo climb", 3, 7.0)}),                                                         // 2024 edition: 215 km (real profile via import-stages)
		cd("Il Lombardia 2023", 238, []model.Segment{road(1, "Bergamo start", 20, 0.1), road(2, "Lake Como approach", 50, 0.3), road(3, "Madonna del Ghisallo", 10, 5.5), road(4, "Ghisallo descent", 8, -5.5), road(5, "Valley road to Civiglio", 20, 0.3), road(6, "Civiglio", 4.2, 9.7), road(7, "Civiglio descent", 5, -6.0), road(8, "Valley to San Fermo", 20, 0.3), road(9, "San Fermo della Battaglia", 2.7, 7.2), road(10, "Descent and road", 20, -0.8), road(11, "Run-in to Como", 10, 0)}), // Como-Bergamo editions (2021, 2023); template until import-stages
		cd("E3 Saxo Classic", 204, []model.Segment{road(1, "Flat approach", 100, 0), cobbles(2, "Paterberg", 0.4, 12.9), road(3, "Road section", 25, 0.3), cobbles(4, "Oude Kwaremont", 2.2, 4.0), road(5, "Road section", 20, 0.3), cobbles(6, "Tiegemberg", 0.8, 6.8), road(7, "Road to finish", 30, 0)}),
		cd("Gent-Wevelgem", 261, []model.Segment{road(1, "Flat approach", 140, 0), road(2, "Kemmelberg", 0.8, 9.3), road(3, "Descent", 2, -5.0), road(4, "Road section", 30, 0), road(5, "Kemmelberg (2nd)", 0.8, 9.3), road(6, "Descent", 2, -5.0), road(7, "Flat run-in", 50, 0), road(8, "Sprint finish", 5, 0)}),
		cd("Amstel Gold Race", 254, []model.Segment{road(1, "Rolling approach", 80, 0.5), road(2, "Cauberg (1st)", 1.2, 5.8), road(3, "Road section", 40, 0.5), road(4, "Keutenberg", 0.7, 9.4), road(5, "Road section", 40, 0.5), road(6, "Gulperberg", 0.8, 9.3), road(7, "Road section", 30, 0.5), road(8, "Kruisberg", 1.0, 8.0), road(9, "Road to Cauberg", 20, 0.5), road(10, "Cauberg finish", 1.2, 5.8)}),
		cd("La Fleche Wallonne", 202, []model.Segment{road(1, "Road section", 100, 0.3), road(2, "Cote de Cherave", 1.3, 8.1), road(3, "Road section", 35, 0.3), road(4, "Mur de Huy (1st)", 1.3, 9.6), road(5, "Loop road", 40, 0.3), road(6, "Cote de Cherave (2nd)", 1.3, 8.1), road(7, "Road to Huy", 10, 0), road(8, "Mur de Huy finish", 1.3, 9.6)}),
		cd("Clasica San Sebastian", 228, []model.Segment{road(1, "Coastal road", 60, 0.3), road(2, "Jaizkibel", 8.1, 5.4), road(3, "Descent", 8, -5.0), road(4, "Road section", 50, 0.3), road(5, "Jaizkibel (2nd)", 8.1, 5.4), road(6, "Descent", 8, -5.0), road(7, "Road section", 30, 0.3), road(8, "Murgil Tontorra", 2.8, 8.4), road(9, "Descent and finish", 12, -2.0)}),
		cd("Flat Sprint Stage", 200, []model.Segment{road(1, "Flat road", 160, 0), road(2, "Gentle rolling", 30, 0.5), road(3, "Sprint finish", 10, 0)}),
		cd("Flat Sprint Stage (short)", 170, []model.Segment{road(1, "Flat road", 130, 0), road(2, "Gentle rolling", 30, 0.3), road(3, "Sprint finish", 10, 0)}),
		cd("Rolling Stage", 185, []model.Segment{road(1, "Road section", 60, 0.3), road(2, "Cat 3 climb", 4, 5.0), road(3, "Descent", 5, -4.0), road(4, "Road section", 40, 0.5), road(5, "Cat 3 climb", 5, 4.5), road(6, "Descent", 5, -4.0), road(7, "Road section", 30, 0.3), road(8, "Cat 4 climb", 2.5, 4.0), road(9, "Descent and finish", 8, -1.0)}),
		cd("Medium Mountain Stage", 175, []model.Segment{road(1, "Road section", 50, 0.3), road(2, "Cat 2 climb", 8, 5.8), road(3, "Descent", 10, -5.0), road(4, "Valley road", 30, 0.3), road(5, "Cat 2 climb", 10, 5.5), road(6, "Descent", 12, -5.0), road(7, "Valley road", 20, 0.3), road(8, "Cat 3 climb", 4, 5.0), road(9, "Descent to finish", 6, -3.0)}),
		cd("High Mountain Stage", 165, []model.Segment{road(1, "Valley approach", 40, 0.5), road(2, "HC climb", 15, 7.0), road(3, "Descent", 18, -6.0), road(4, "Valley road", 20, 0.3), road(5, "Cat 1 climb", 12, 6.5), road(6, "Descent", 14, -5.5), road(7, "Valley road", 15, 0.3), road(8, "Cat 1 climb", 10, 6.8), road(9, "Descent to finish", 6, -3.0)}),
		cd("Summit Finish Stage", 155, []model.Segment{road(1, "Valley approach", 40, 0.3), road(2, "Cat 1 climb", 12, 6.5), road(3, "Descent", 15, -6.0), road(4, "Valley road", 20, 0.3), road(5, "Cat 1 climb", 14, 6.8), road(6, "Descent", 16, -5.5), road(7, "Short valley", 8, 0.3), road(8, "HC summit finish", 14, 7.8)}),
		cd("Alpine Summit Finish", 170, []model.Segment{road(1, "Valley approach", 50, 0.3), road(2, "Col du Galibier", 18, 6.9), road(3, "Galibier descent", 20, -6.0), road(4, "Valley road", 20, 0.5), road(5, "Alpe d'Huez", 13.8, 8.1)}),
		cd("Dolomites Summit Finish", 160, []model.Segment{road(1, "Valley approach", 35, 0.3), road(2, "Passo Fedaia", 14, 7.6), road(3, "Descent", 12, -6.5), road(4, "Valley road", 15, 0.3), road(5, "Passo Pordoi", 11.8, 6.8), road(6, "Descent", 10, -5.5), road(7, "Passo Giau", 10, 9.3)}),
		cd("Pyrenees Summit Finish", 150, []model.Segment{road(1, "Valley approach", 30, 0.3), road(2, "Col du Tourmalet", 17.1, 7.3), road(3, "Descent", 18, -6.5), road(4, "Valley road", 15, 0.3), road(5, "Hautacam", 13.6, 7.8)}),
		cd("Prologue TT", 8, []model.Segment{road(1, "Urban circuit", 8, 0)}),
		cd("Short ITT", 25, []model.Segment{road(1, "Flat section", 10, 0), road(2, "Rolling section", 8, 1.5), road(3, "Flat return", 7, -0.5)}),
		cd("Long ITT", 40, []model.Segment{road(1, "Flat section 1", 12, 0), road(2, "Rolling hills", 8, 2.0), road(3, "Flat section 2", 10, 0), road(4, "False flat return", 10, -0.5)}),
		cd("Mountain TT", 30, []model.Segment{road(1, "Valley start", 8, 0.5), road(2, "Mountain climb", 12, 6.5), road(3, "False flat", 4, 2.0), road(4, "Final ramp", 6, 7.0)}),
		cd("Flat Sprint Stage (long)", 220, []model.Segment{road(1, "Flat road", 180, 0), road(2, "Gentle rolling", 30, 0.3), road(3, "Sprint finish", 10, 0)}),
		cd("Hilly Stage with Uphill Finish", 180, []model.Segment{road(1, "Road section", 60, 0.3), road(2, "Cat 3 climb", 5, 5.0), road(3, "Descent", 6, -4.0), road(4, "Road section", 50, 0.5), road(5, "Cat 3 climb", 4, 5.5), road(6, "Descent", 5, -4.0), road(7, "Road section", 20, 0.3), road(8, "Uphill finish", 3, 6.5)}),
		cd("Basque Country Mountain Stage", 165, []model.Segment{road(1, "Road section", 40, 0.5), road(2, "Cat 2 climb", 6, 7.0), road(3, "Descent", 8, -5.5), road(4, "Valley road", 30, 0.5), road(5, "Cat 2 climb", 7, 6.5), road(6, "Descent", 8, -5.5), road(7, "Valley road", 15, 0.3), road(8, "Cat 1 summit finish", 10, 7.5)}),
		cd("Swiss Mountain Stage", 175, []model.Segment{road(1, "Lake road", 50, 0.3), road(2, "Cat 1 alpine pass", 14, 6.5), road(3, "Descent", 16, -5.5), road(4, "Valley road", 25, 0.3), road(5, "Cat 1 alpine pass", 12, 7.0), road(6, "Descent", 10, -6.0), road(7, "Run-in", 8, 0)}),
		cd("Romandie Rolling Stage", 165, []model.Segment{road(1, "Lake road", 50, 0.3), road(2, "Cat 3 climb", 4, 5.0), road(3, "Descent", 5, -4.0), road(4, "Road section", 40, 0.5), road(5, "Cat 3 climb", 5, 4.5), road(6, "Descent", 5, -4.0), road(7, "Run-in", 20, 0.3)}),
		cd("Vuelta Mountain Stage", 175, []model.Segment{road(1, "Road section", 40, 0.5), road(2, "Puerto climb", 12, 6.2), road(3, "Descent", 14, -5.5), road(4, "Valley road", 25, 0.3), road(5, "Cat 1 climb", 14, 7.0), road(6, "Descent", 12, -5.5), road(7, "Valley road", 15, 0.3), road(8, "Cat 1 summit finish", 12, 7.5)}),
		cd("Vuelta Steep Summit", 155, []model.Segment{road(1, "Valley approach", 35, 0.3), road(2, "Cat 1 climb", 10, 7.2), road(3, "Descent", 12, -6.0), road(4, "Valley road", 20, 0.3), road(5, "Cat 1 climb", 12, 7.5), road(6, "Descent", 14, -5.5), road(7, "Short valley", 5, 0.3), road(8, "Angliru / Lagos finish", 12.5, 9.8)}),
		cd("Short Prologue TT", 5, []model.Segment{road(1, "Urban circuit", 5, 0.3)}),
	}
	courses = append(courses, realStageCourses(courses)...)
	// Real route profiles imported from locally installed stage files
	// (cycling-sim import-stages) replace the hand-written segments.
	if local, err := stagefile.Load(DataPath(LocalProfilesFile)); err == nil {
		for k := range courses {
			if p, ok := local[courses[k].course.Name]; ok {
				courses[k].segments = CourseSegments(courses[k].course.Name, courses[k].course.TotalDistanceKm, p.ModelSegments())
			}
		}
	}

	ids := make(map[string]int64)
	if existing, err := db.ListCourses(); err == nil {
		for _, c := range existing {
			ids[c.Name] = c.ID
		}
	}
	created, warnings := 0, 0
	for _, cdef := range courses {
		c := cdef.course
		if _, ok := ids[c.Name]; ok {
			continue
		}
		if err := db.CreateCourse(&c); err != nil {
			warnings++
			continue
		}
		ids[c.Name] = c.ID
		created++
		for _, s := range cdef.segments {
			s.CourseID = c.ID
			if err := db.CreateSegment(&s); err != nil {
				warnings++
			}
		}
	}
	return ids, created, warnings
}

func seedRaces(db *database.DB, cIDs map[string]int64) (map[string]int64, int, int) {
	d := func(y, m, day int) time.Time { return time.Date(y, time.Month(m), day, 0, 0, 0, 0, time.UTC) }

	races := []model.Race{
		{Name: "Milan-San Remo", CourseID: cIDs["Milan-San Remo"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 22), Temperature: 15, WindSpeedKmh: 10},
		{Name: "Tour of Flanders", CourseID: cIDs["Tour of Flanders"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 6), Temperature: 12, WindSpeedKmh: 18},
		{Name: "Paris-Roubaix", CourseID: cIDs["Paris-Roubaix"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 13), Temperature: 12, WindSpeedKmh: 20},
		{Name: "Liege-Bastogne-Liege", CourseID: cIDs["Liege-Bastogne-Liege"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 27), Temperature: 14, WindSpeedKmh: 12},
		{Name: "Il Lombardia", CourseID: cIDs["Il Lombardia"], RaceType: model.RaceTypeRoad, Date: d(2025, 10, 11), Temperature: 16, WindSpeedKmh: 8},

		{Name: "Strade Bianche", CourseID: cIDs["Strade Bianche"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 8), Temperature: 12, WindSpeedKmh: 10},
		// Editions whose route differs from the course above (validation references).
		{Name: "Strade Bianche 2024", CourseID: cIDs["Strade Bianche 2024"], RaceType: model.RaceTypeRoad, Date: d(2024, 3, 2), Temperature: 12, WindSpeedKmh: 10},
		{Name: "Il Lombardia 2023", CourseID: cIDs["Il Lombardia 2023"], RaceType: model.RaceTypeRoad, Date: d(2023, 10, 7), Temperature: 16, WindSpeedKmh: 8},
		{Name: "E3 Saxo Classic", CourseID: cIDs["E3 Saxo Classic"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 28), Temperature: 10, WindSpeedKmh: 15},
		{Name: "Gent-Wevelgem", CourseID: cIDs["Gent-Wevelgem"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 30), Temperature: 10, WindSpeedKmh: 22},
		{Name: "Amstel Gold Race", CourseID: cIDs["Amstel Gold Race"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 20), Temperature: 14, WindSpeedKmh: 12},
		{Name: "La Fleche Wallonne", CourseID: cIDs["La Fleche Wallonne"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 23), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Clasica San Sebastian", CourseID: cIDs["Clasica San Sebastian"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 2), Temperature: 25, WindSpeedKmh: 8},

		{Name: "TdF Stage 1 - Prologue TT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 7, 5), Temperature: 22, WindSpeedKmh: 8},
		{Name: "TdF Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 6), Temperature: 24, WindSpeedKmh: 10},
		{Name: "TdF Stage 3 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 7), Temperature: 25, WindSpeedKmh: 8},
		{Name: "TdF Stage 4 - Hilly", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 8), Temperature: 26, WindSpeedKmh: 10},
		{Name: "TdF Stage 5 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (long)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 9), Temperature: 27, WindSpeedKmh: 12},
		{Name: "TdF Stage 6 - Cobbles", CourseID: cIDs["E3 Saxo Classic"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 10), Temperature: 22, WindSpeedKmh: 15},
		{Name: "TdF Stage 7 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 11), Temperature: 24, WindSpeedKmh: 8},
		{Name: "TdF Stage 8 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 12), Temperature: 22, WindSpeedKmh: 8},
		{Name: "TdF Stage 9 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 13), Temperature: 28, WindSpeedKmh: 10},
		{Name: "TdF Stage 10 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 15), Temperature: 20, WindSpeedKmh: 5},
		{Name: "TdF Stage 11 - Pyrenees Summit", CourseID: cIDs["Pyrenees Summit Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 16), Temperature: 18, WindSpeedKmh: 8},
		{Name: "TdF Stage 12 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 17), Temperature: 30, WindSpeedKmh: 5},
		{Name: "TdF Stage 13 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 18), Temperature: 28, WindSpeedKmh: 10},
		{Name: "TdF Stage 14 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 19), Temperature: 24, WindSpeedKmh: 8},
		{Name: "TdF Stage 15 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 20), Temperature: 18, WindSpeedKmh: 5},
		{Name: "TdF Stage 16 - ITT", CourseID: cIDs["Long ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 7, 22), Temperature: 25, WindSpeedKmh: 10},
		{Name: "TdF Stage 17 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (long)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 23), Temperature: 28, WindSpeedKmh: 8},
		{Name: "TdF Stage 18 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 24), Temperature: 18, WindSpeedKmh: 5},
		{Name: "TdF Stage 19 - Alpine Summit", CourseID: cIDs["Alpine Summit Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 25), Temperature: 16, WindSpeedKmh: 8},
		{Name: "TdF Stage 20 - Mountain", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 26), Temperature: 18, WindSpeedKmh: 5},
		{Name: "TdF Stage 21 - Paris Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 7, 27), Temperature: 25, WindSpeedKmh: 8},

		{Name: "Giro Stage 1 - Prologue TT", CourseID: cIDs["Prologue TT"], RaceType: model.RaceTypeTT, Date: d(2025, 5, 10), Temperature: 18, WindSpeedKmh: 5},
		{Name: "Giro Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 11), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Giro Stage 3 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 12), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Giro Stage 4 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 13), Temperature: 22, WindSpeedKmh: 8},
		{Name: "Giro Stage 5 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 14), Temperature: 20, WindSpeedKmh: 10},
		{Name: "Giro Stage 6 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (long)"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 15), Temperature: 24, WindSpeedKmh: 8},
		{Name: "Giro Stage 7 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 16), Temperature: 18, WindSpeedKmh: 5},
		{Name: "Giro Stage 8 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 17), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Giro Stage 9 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 18), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Giro Stage 10 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 20), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Giro Stage 11 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 21), Temperature: 24, WindSpeedKmh: 8},
		{Name: "Giro Stage 12 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 22), Temperature: 18, WindSpeedKmh: 8},
		{Name: "Giro Stage 13 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 23), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Giro Stage 14 - ITT", CourseID: cIDs["Long ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 5, 24), Temperature: 22, WindSpeedKmh: 8},
		{Name: "Giro Stage 15 - Dolomites Summit", CourseID: cIDs["Dolomites Summit Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 25), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Giro Stage 16 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 27), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Giro Stage 17 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 28), Temperature: 26, WindSpeedKmh: 8},
		{Name: "Giro Stage 18 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 29), Temperature: 24, WindSpeedKmh: 10},
		{Name: "Giro Stage 19 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 30), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Giro Stage 20 - Mountain TT", CourseID: cIDs["Mountain TT"], RaceType: model.RaceTypeTT, Date: d(2025, 5, 31), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Giro Stage 21 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 1), Temperature: 26, WindSpeedKmh: 5},

		{Name: "Vuelta Stage 1 - Short ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 8, 23), Temperature: 30, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 24), Temperature: 32, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 3 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 25), Temperature: 30, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 4 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 26), Temperature: 33, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 5 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 27), Temperature: 28, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 6 - Vuelta Mountain", CourseID: cIDs["Vuelta Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 28), Temperature: 26, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 7 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (long)"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 29), Temperature: 32, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 8 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 30), Temperature: 28, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 9 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 8, 31), Temperature: 22, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 10 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 2), Temperature: 30, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 11 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 3), Temperature: 26, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 12 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 4), Temperature: 28, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 13 - Hilly", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 5), Temperature: 26, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 14 - Vuelta Mountain", CourseID: cIDs["Vuelta Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 6), Temperature: 24, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 15 - Steep Summit", CourseID: cIDs["Vuelta Steep Summit"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 7), Temperature: 20, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 16 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 9), Temperature: 22, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 17 - ITT", CourseID: cIDs["Long ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 9, 10), Temperature: 26, WindSpeedKmh: 8},
		{Name: "Vuelta Stage 18 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 11), Temperature: 24, WindSpeedKmh: 10},
		{Name: "Vuelta Stage 19 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 12), Temperature: 20, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 20 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 13), Temperature: 18, WindSpeedKmh: 5},
		{Name: "Vuelta Stage 21 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 9, 14), Temperature: 26, WindSpeedKmh: 8},

		{Name: "Paris-Nice Stage 1 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 9), Temperature: 8, WindSpeedKmh: 15},
		{Name: "Paris-Nice Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 10), Temperature: 10, WindSpeedKmh: 12},
		{Name: "Paris-Nice Stage 3 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 11), Temperature: 10, WindSpeedKmh: 10},
		{Name: "Paris-Nice Stage 4 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 12), Temperature: 12, WindSpeedKmh: 8},
		{Name: "Paris-Nice Stage 5 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 13), Temperature: 12, WindSpeedKmh: 8},
		{Name: "Paris-Nice Stage 6 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 14), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Paris-Nice Stage 7 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 15), Temperature: 10, WindSpeedKmh: 5},
		{Name: "Paris-Nice Stage 8 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 3, 16), Temperature: 14, WindSpeedKmh: 8},

		{Name: "Tirreno Stage 1 - Short ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 3, 10), Temperature: 12, WindSpeedKmh: 8},
		{Name: "Tirreno Stage 2 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 11), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Tirreno Stage 3 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 12), Temperature: 14, WindSpeedKmh: 8},
		{Name: "Tirreno Stage 4 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 13), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Tirreno Stage 5 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 14), Temperature: 12, WindSpeedKmh: 5},
		{Name: "Tirreno Stage 6 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 15), Temperature: 10, WindSpeedKmh: 8},
		{Name: "Tirreno Stage 7 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 3, 16), Temperature: 14, WindSpeedKmh: 10},

		{Name: "Suisse Stage 1 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 8), Temperature: 18, WindSpeedKmh: 8},
		{Name: "Suisse Stage 2 - Rolling", CourseID: cIDs["Romandie Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 9), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Suisse Stage 3 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 10), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Suisse Stage 4 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 11), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Suisse Stage 5 - Swiss Mountain", CourseID: cIDs["Swiss Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 12), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Suisse Stage 6 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 13), Temperature: 18, WindSpeedKmh: 8},
		{Name: "Suisse Stage 7 - Swiss Mountain", CourseID: cIDs["Swiss Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 14), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Suisse Stage 8 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 6, 15), Temperature: 20, WindSpeedKmh: 8},

		{Name: "Dauphine Stage 1 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 8), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Dauphine Stage 2 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 9), Temperature: 22, WindSpeedKmh: 10},
		{Name: "Dauphine Stage 3 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 10), Temperature: 22, WindSpeedKmh: 8},
		{Name: "Dauphine Stage 4 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 11), Temperature: 18, WindSpeedKmh: 5},
		{Name: "Dauphine Stage 5 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 6, 12), Temperature: 20, WindSpeedKmh: 8},
		{Name: "Dauphine Stage 6 - High Mountain", CourseID: cIDs["High Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 13), Temperature: 16, WindSpeedKmh: 5},
		{Name: "Dauphine Stage 7 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 14), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Dauphine Stage 8 - Alpine Summit", CourseID: cIDs["Alpine Summit Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 6, 15), Temperature: 14, WindSpeedKmh: 8},

		{Name: "Basque Stage 1 - Rolling", CourseID: cIDs["Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 7), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Basque Stage 2 - Hilly", CourseID: cIDs["Hilly Stage with Uphill Finish"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 8), Temperature: 14, WindSpeedKmh: 8},
		{Name: "Basque Stage 3 - Basque Mountain", CourseID: cIDs["Basque Country Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 9), Temperature: 12, WindSpeedKmh: 8},
		{Name: "Basque Stage 4 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 4, 10), Temperature: 14, WindSpeedKmh: 10},
		{Name: "Basque Stage 5 - Basque Mountain", CourseID: cIDs["Basque Country Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 11), Temperature: 12, WindSpeedKmh: 5},
		{Name: "Basque Stage 6 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 12), Temperature: 10, WindSpeedKmh: 8},

		{Name: "Romandie Prologue", CourseID: cIDs["Short Prologue TT"], RaceType: model.RaceTypeTT, Date: d(2025, 4, 29), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Romandie Stage 1 - Flat Sprint", CourseID: cIDs["Flat Sprint Stage (short)"], RaceType: model.RaceTypeRoad, Date: d(2025, 4, 30), Temperature: 16, WindSpeedKmh: 8},
		{Name: "Romandie Stage 2 - Rolling", CourseID: cIDs["Romandie Rolling Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 1), Temperature: 16, WindSpeedKmh: 10},
		{Name: "Romandie Stage 3 - Medium Mountain", CourseID: cIDs["Medium Mountain Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 2), Temperature: 14, WindSpeedKmh: 5},
		{Name: "Romandie Stage 4 - Summit Finish", CourseID: cIDs["Summit Finish Stage"], RaceType: model.RaceTypeRoad, Date: d(2025, 5, 3), Temperature: 12, WindSpeedKmh: 5},
		{Name: "Romandie Stage 5 - ITT", CourseID: cIDs["Short ITT"], RaceType: model.RaceTypeTT, Date: d(2025, 5, 4), Temperature: 16, WindSpeedKmh: 8},
	}

	races = append(races, realStageRaceRaces(cIDs)...)

	ids := make(map[string]int64)
	if existing, err := db.ListRaces(); err == nil {
		for _, r := range existing {
			ids[r.Name] = r.ID
		}
	}
	created, warnings := 0, 0
	for i := range races {
		if _, ok := ids[races[i].Name]; ok {
			continue
		}
		if races[i].CourseID == 0 {
			warnings++
			continue
		}
		if races[i].WindDirectionDeg == 0 {
			races[i].WindDirectionDeg = syntheticWindDirection(races[i].Name)
		}
		if err := db.CreateRace(&races[i]); err != nil {
			warnings++
			continue
		}
		ids[races[i].Name] = races[i].ID
		created++
	}
	return ids, created, warnings
}

func seedStageRaces(db *database.DB, rIDs map[string]int64) (int, int) {
	type srDef struct {
		name   string
		stages []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}
	}

	stg := func(name string, num int, rest bool) struct {
		raceName      string
		stageNum      int
		restDayBefore bool
	} {
		return struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{name, num, rest}
	}

	stageRaces := []srDef{
		{name: "Tour de France", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("TdF Stage 1 - Prologue TT", 1, false),
			stg("TdF Stage 2 - Flat Sprint", 2, false),
			stg("TdF Stage 3 - Flat Sprint", 3, false),
			stg("TdF Stage 4 - Hilly", 4, false),
			stg("TdF Stage 5 - Flat Sprint", 5, false),
			stg("TdF Stage 6 - Cobbles", 6, false),
			stg("TdF Stage 7 - Hilly", 7, false),
			stg("TdF Stage 8 - Medium Mountain", 8, false),
			stg("TdF Stage 9 - Flat Sprint", 9, false),
			stg("TdF Stage 10 - High Mountain", 10, true),
			stg("TdF Stage 11 - Pyrenees Summit", 11, false),
			stg("TdF Stage 12 - Flat Sprint", 12, false),
			stg("TdF Stage 13 - Rolling", 13, false),
			stg("TdF Stage 14 - Medium Mountain", 14, false),
			stg("TdF Stage 15 - Summit Finish", 15, false),
			stg("TdF Stage 16 - ITT", 16, true),
			stg("TdF Stage 17 - Flat Sprint", 17, false),
			stg("TdF Stage 18 - High Mountain", 18, false),
			stg("TdF Stage 19 - Alpine Summit", 19, false),
			stg("TdF Stage 20 - Mountain", 20, false),
			stg("TdF Stage 21 - Paris Sprint", 21, false),
		}},
		{name: "Giro d'Italia", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Giro Stage 1 - Prologue TT", 1, false),
			stg("Giro Stage 2 - Flat Sprint", 2, false),
			stg("Giro Stage 3 - Rolling", 3, false),
			stg("Giro Stage 4 - Flat Sprint", 4, false),
			stg("Giro Stage 5 - Hilly", 5, false),
			stg("Giro Stage 6 - Flat Sprint", 6, false),
			stg("Giro Stage 7 - Medium Mountain", 7, false),
			stg("Giro Stage 8 - Flat Sprint", 8, false),
			stg("Giro Stage 9 - Rolling", 9, false),
			stg("Giro Stage 10 - Summit Finish", 10, true),
			stg("Giro Stage 11 - Flat Sprint", 11, false),
			stg("Giro Stage 12 - Medium Mountain", 12, false),
			stg("Giro Stage 13 - Hilly", 13, false),
			stg("Giro Stage 14 - ITT", 14, false),
			stg("Giro Stage 15 - Dolomites Summit", 15, false),
			stg("Giro Stage 16 - High Mountain", 16, true),
			stg("Giro Stage 17 - Flat Sprint", 17, false),
			stg("Giro Stage 18 - Rolling", 18, false),
			stg("Giro Stage 19 - Summit Finish", 19, false),
			stg("Giro Stage 20 - Mountain TT", 20, false),
			stg("Giro Stage 21 - Flat Sprint", 21, false),
		}},
		{name: "Vuelta a Espana", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Vuelta Stage 1 - Short ITT", 1, false),
			stg("Vuelta Stage 2 - Flat Sprint", 2, false),
			stg("Vuelta Stage 3 - Hilly", 3, false),
			stg("Vuelta Stage 4 - Flat Sprint", 4, false),
			stg("Vuelta Stage 5 - Medium Mountain", 5, false),
			stg("Vuelta Stage 6 - Vuelta Mountain", 6, false),
			stg("Vuelta Stage 7 - Flat Sprint", 7, false),
			stg("Vuelta Stage 8 - Rolling", 8, false),
			stg("Vuelta Stage 9 - Summit Finish", 9, false),
			stg("Vuelta Stage 10 - Flat Sprint", 10, true),
			stg("Vuelta Stage 11 - Medium Mountain", 11, false),
			stg("Vuelta Stage 12 - Flat Sprint", 12, false),
			stg("Vuelta Stage 13 - Hilly", 13, false),
			stg("Vuelta Stage 14 - Vuelta Mountain", 14, false),
			stg("Vuelta Stage 15 - Steep Summit", 15, false),
			stg("Vuelta Stage 16 - High Mountain", 16, true),
			stg("Vuelta Stage 17 - ITT", 17, false),
			stg("Vuelta Stage 18 - Rolling", 18, false),
			stg("Vuelta Stage 19 - Summit Finish", 19, false),
			stg("Vuelta Stage 20 - High Mountain", 20, false),
			stg("Vuelta Stage 21 - Flat Sprint", 21, false),
		}},
		{name: "Paris-Nice", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Paris-Nice Stage 1 - Flat Sprint", 1, false),
			stg("Paris-Nice Stage 2 - Flat Sprint", 2, false),
			stg("Paris-Nice Stage 3 - Rolling", 3, false),
			stg("Paris-Nice Stage 4 - Hilly", 4, false),
			stg("Paris-Nice Stage 5 - Medium Mountain", 5, false),
			stg("Paris-Nice Stage 6 - Flat Sprint", 6, false),
			stg("Paris-Nice Stage 7 - Summit Finish", 7, false),
			stg("Paris-Nice Stage 8 - ITT", 8, false),
		}},
		{name: "Tirreno-Adriatico", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Tirreno Stage 1 - Short ITT", 1, false),
			stg("Tirreno Stage 2 - Flat Sprint", 2, false),
			stg("Tirreno Stage 3 - Rolling", 3, false),
			stg("Tirreno Stage 4 - Hilly", 4, false),
			stg("Tirreno Stage 5 - Medium Mountain", 5, false),
			stg("Tirreno Stage 6 - Summit Finish", 6, false),
			stg("Tirreno Stage 7 - Flat Sprint", 7, false),
		}},
		{name: "Tour de Suisse", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Suisse Stage 1 - Flat Sprint", 1, false),
			stg("Suisse Stage 2 - Rolling", 2, false),
			stg("Suisse Stage 3 - Flat Sprint", 3, false),
			stg("Suisse Stage 4 - Hilly", 4, false),
			stg("Suisse Stage 5 - Swiss Mountain", 5, false),
			stg("Suisse Stage 6 - Medium Mountain", 6, false),
			stg("Suisse Stage 7 - Swiss Mountain", 7, false),
			stg("Suisse Stage 8 - ITT", 8, false),
		}},
		{name: "Criterium du Dauphine", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Dauphine Stage 1 - Flat Sprint", 1, false),
			stg("Dauphine Stage 2 - Rolling", 2, false),
			stg("Dauphine Stage 3 - Hilly", 3, false),
			stg("Dauphine Stage 4 - Medium Mountain", 4, false),
			stg("Dauphine Stage 5 - ITT", 5, false),
			stg("Dauphine Stage 6 - High Mountain", 6, false),
			stg("Dauphine Stage 7 - Summit Finish", 7, false),
			stg("Dauphine Stage 8 - Alpine Summit", 8, false),
		}},
		{name: "Tour of the Basque Country", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Basque Stage 1 - Rolling", 1, false),
			stg("Basque Stage 2 - Hilly", 2, false),
			stg("Basque Stage 3 - Basque Mountain", 3, false),
			stg("Basque Stage 4 - ITT", 4, false),
			stg("Basque Stage 5 - Basque Mountain", 5, false),
			stg("Basque Stage 6 - Summit Finish", 6, false),
		}},
		{name: "Tour de Romandie", stages: []struct {
			raceName      string
			stageNum      int
			restDayBefore bool
		}{
			stg("Romandie Prologue", 1, false),
			stg("Romandie Stage 1 - Flat Sprint", 2, false),
			stg("Romandie Stage 2 - Rolling", 3, false),
			stg("Romandie Stage 3 - Medium Mountain", 4, false),
			stg("Romandie Stage 4 - Summit Finish", 5, false),
			stg("Romandie Stage 5 - ITT", 6, false),
		}},
	}

	for _, r := range realStageRaces() {
		def := srDef{name: r.name}
		for i, s := range r.stages {
			def.stages = append(def.stages, stg(r.stageName(i+1), i+1, s.rest))
		}
		stageRaces = append(stageRaces, def)
	}

	have := map[string]bool{}
	if existing, err := db.ListStageRaces(); err == nil {
		for _, sr := range existing {
			have[sr.Name] = true
		}
	}
	count := 0
	warnings := 0
	for _, def := range stageRaces {
		if have[def.name] {
			continue
		}
		sr := model.StageRace{Name: def.name}
		if err := db.CreateStageRace(&sr); err != nil {
			warnings++
			continue
		}
		for _, st := range def.stages {
			raceID, ok := rIDs[st.raceName]
			if !ok {
				warnings++
				continue
			}
			_ = db.AddStageToRace(&model.StageRaceStage{StageRaceID: sr.ID, RaceID: raceID, StageNumber: st.stageNum, RestDayBefore: st.restDayBefore})
		}
		count++
	}
	return count, warnings
}

// StagePrestigeFile lists how coveted each stage is (stage race name, stage
// number, prestige 0-1); unlisted stages are ordinary.
const StagePrestigeFile = "stage_prestige.csv"

// ApplyStagePrestige sets every stage's prestige from StagePrestigeFile, in
// place (existing stage races keep their results). Returns warnings.
func ApplyStagePrestige(db *database.DB) int {
	f, err := os.Open(findDataFile(StagePrestigeFile))
	if err != nil {
		return 0 // no data: every stage is ordinary
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comment = '#'
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return 1
	}
	want := map[string]map[int]float64{}
	for i, row := range rows {
		if i == 0 || len(row) < 3 {
			continue // header
		}
		n, err1 := strconv.Atoi(strings.TrimSpace(row[1]))
		p, err2 := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
		if err1 != nil || err2 != nil || p < 0 || p > 1 {
			continue
		}
		name := strings.TrimSpace(row[0])
		if want[name] == nil {
			want[name] = map[int]float64{}
		}
		want[name][n] = p
	}
	srs, err := db.ListStageRaces()
	if err != nil {
		return 1
	}
	warnings := 0
	for _, sr := range srs {
		full, err := db.GetStageRace(sr.ID)
		if err != nil {
			warnings++
			continue
		}
		for _, st := range full.Stages {
			p := want[sr.Name][st.StageNumber] // 0 when unlisted
			if p != st.Prestige {
				if db.SetStagePrestige(sr.ID, st.StageNumber, p) != nil {
					warnings++
				}
			}
		}
	}
	return warnings
}

// frontalArea is a cyclist's frontal area in m² (Heil 2001, road position):
// 0.0293·h^0.725·m^0.425 + 0.0604, h in metres, m in kg.
func frontalArea(heightCm, massKg float64) float64 {
	if heightCm <= 0 || massKg <= 0 {
		return 1
	}
	return 0.0293*math.Pow(heightCm/100, 0.725)*math.Pow(massKg, 0.425) + 0.0604
}
