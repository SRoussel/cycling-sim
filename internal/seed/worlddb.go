package seed

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cycling_sim/internal/model"
	"cycling_sim/internal/pcmdb"
)

// Riders from a locally installed WorldDB mod (Pro Cycling Manager). The
// mod's data belong to its authors: `cycling-sim import-riders` writes them
// to WorldDBFile under data/local/ (gitignored) and BuildRiders uses the file
// when present. It gives every rider their real age and adds the WorldTeam
// and ProTeam riders riders.csv lacks, with ratings fitted on the riders both
// have (PCM ratings blend several things, so they're used only through that
// fit, never copied).

// WorldDBFile is the imported rider file, relative to the data directory.
const WorldDBFile = "local/riders_worlddb.csv"

// characCols are the PCM rider attributes used as fit features.
var characCols = []string{"plain", "mountain", "medium_mountain", "hill", "downhilling", "cobble",
	"timetrial", "prologue", "sprint", "acceleration", "endurance", "resistance", "recuperation", "baroudeur"}

// Team divisions kept: UCI WorldTeams and ProTeams.
var keepDivisions = map[int]bool{10: true, 11: true}

type wdbRider struct {
	Name      string
	Team      string
	Birth     int // yyyymmdd
	Height    int // cm
	Weight    int // kg
	Charac    [14]float64
	StartYear int // the database's season
}

// ImportWorldDB reads the mod database at path (a mod directory or its
// OfficialRelease.cdb) and writes the WorldTeam and ProTeam riders to out.
func ImportWorldDB(path, out string) (int, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, "OfficialRelease.cdb")
	}
	db, err := pcmdb.Open(path)
	if err != nil {
		return 0, err
	}
	cyc, teams := db.Tables["DYN_cyclist"], db.Tables["DYN_team"]
	if cyc == nil || teams == nil {
		return 0, fmt.Errorf("%s: no rider or team table", path)
	}
	startYear := 0
	if cfg := db.Tables["GAM_config"]; cfg != nil {
		startYear = cfg.Int("game_i_starting_year", 0)
	}
	type team struct {
		name string
		div  int
	}
	byID := map[int]team{}
	for i := 0; i < teams.Rows; i++ {
		byID[teams.Int("IDteam", i)] = team{teams.String("gene_sz_name", i), teams.Int("fkIDdivision", i)}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(out)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	head := []string{"Name", "Team", "Birth", "Height", "Weight", "StartYear"}
	for _, c := range characCols {
		head = append(head, c)
	}
	_ = w.Write(head)
	n := 0
	for i := 0; i < cyc.Rows; i++ {
		t, ok := byID[cyc.Int("fkIDteam", i)]
		if !ok || !keepDivisions[t.div] {
			continue
		}
		name := strings.TrimSpace(cyc.String("gene_sz_firstname", i) + " " + cyc.String("gene_sz_lastname", i))
		row := []string{name, t.name, strconv.Itoa(cyc.Int("gene_i_birthdate", i)),
			strconv.Itoa(cyc.Int("gene_i_size", i)), strconv.Itoa(cyc.Int("gene_i_weight", i)), strconv.Itoa(startYear)}
		for _, c := range characCols {
			row = append(row, strconv.Itoa(cyc.Int("charac_i_"+c, i)))
		}
		_ = w.Write(row)
		n++
	}
	w.Flush()
	return n, w.Error()
}

// loadWorldDB reads the imported rider file (nil when there is none).
func loadWorldDB() ([]wdbRider, error) {
	f, err := os.Open(DataPath(WorldDBFile))
	if err != nil {
		return nil, nil
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", WorldDBFile, err)
	}
	var out []wdbRider
	for _, r := range rows[1:] {
		if len(r) < 6+len(characCols) {
			continue
		}
		var x wdbRider
		x.Name, x.Team = r[0], r[1]
		x.Birth, _ = strconv.Atoi(r[2])
		x.Height, _ = strconv.Atoi(r[3])
		x.Weight, _ = strconv.Atoi(r[4])
		x.StartYear, _ = strconv.Atoi(r[5])
		for j := range characCols {
			x.Charac[j], _ = strconv.ParseFloat(r[6+j], 64)
		}
		out = append(out, x)
	}
	return out, nil
}

// foldName matches names across sources: lower case, accents removed,
// hyphens as spaces, single spaces.
func foldName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if a, ok := unaccent[r]; ok {
			b.WriteString(a)
			continue
		}
		if r == '-' {
			r = ' '
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// unaccent maps the accented letters of riders' names to plain ones.
var unaccent = func() map[rune]string {
	m := map[rune]string{}
	for plain, accented := range map[string]string{
		"a": "àáâãäåāăą", "c": "çćĉċč", "d": "ďđ", "e": "èéêëēĕėęě", "g": "ĝğġģ",
		"i": "ìíîïĩīĭįı", "l": "ĺļľŀł", "n": "ñńņňŉ", "o": "òóôõöøōŏő", "r": "ŕŗř",
		"s": "śŝşšș", "t": "ţťŧț", "u": "ùúûüũūŭůűų", "y": "ýÿŷ", "z": "źżž",
		"ae": "æ", "oe": "œ", "ss": "ß", "th": "þ",
	} {
		for _, r := range accented {
			m[r] = plain
		}
	}
	return m
}()

// sameRider reports whether two folded names are the same rider: equal, one
// a longer form of the other (a second surname: "derek gee" / "derek gee
// west"; a middle name: "daniel martinez" / "daniel felipe martinez"), or a
// spelling variant of the same surname ("tesfatsion" / "tesfazion": same
// first name and surname initial, one or two letters apart).
func sameRider(a, b string) bool {
	if a == b {
		return true
	}
	ta, tb := strings.Fields(a), strings.Fields(b)
	if len(ta) < 2 || len(tb) < 2 || ta[0] != tb[0] {
		return false
	}
	if strings.HasPrefix(a+" ", b+" ") || strings.HasPrefix(b+" ", a+" ") {
		return true
	}
	short, long := ta, tb
	if len(short) > len(long) {
		short, long = long, short
	}
	if short[len(short)-1] == long[len(long)-1] && subsequence(short, long) {
		return true // a middle name added
	}
	la, lb := ta[len(ta)-1], tb[len(tb)-1]
	return len(la) >= 6 && len(lb) >= 6 && la[0] == lb[0] && levenshtein(a, b) <= 2
}

// subsequence reports whether every word of short appears in long, in order.
func subsequence(short, long []string) bool {
	i := 0
	for _, w := range long {
		if i < len(short) && short[i] == w {
			i++
		}
	}
	return i == len(short)
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

// ageIn is a rider's racing age in a season (season year minus birth year).
func ageIn(birth, year int) int {
	if birth < 19000000 || year == 0 {
		return 0
	}
	return year - birth/10000
}

// withWorldDB sets real ages on riders and appends the WorldDB riders they
// lack, rated by fits on train (the same riders without the fitted layer).
func withWorldDB(riders, train []model.Rider, wdb []wdbRider) []model.Rider {
	folded := make([]string, len(riders))
	for i, r := range riders {
		folded[i] = foldName(r.Name)
	}
	match := func(x wdbRider) int {
		f := foldName(x.Name)
		for i := range riders {
			if folded[i] == f {
				return i
			}
		}
		found := -1
		for i := range riders {
			if sameRider(folded[i], f) {
				if found >= 0 {
					return ambiguous // two of ours fit: neither is updated, nobody is added
				}
				found = i
			}
		}
		return found
	}
	trainByName := map[string]model.Rider{}
	for _, r := range train {
		trainByName[r.Name] = r
	}
	var pairs []pair
	teamVotes := map[string]map[string]int{} // WorldDB team -> our team -> riders
	var missing []wdbRider
	for _, x := range wdb {
		i := match(x)
		if i == ambiguous {
			continue
		}
		if i < 0 {
			missing = append(missing, x)
			continue
		}
		if a := ageIn(x.Birth, x.StartYear); a >= 16 && a <= 50 {
			riders[i].Age = a
		}
		// Real height (riders.csv heights are mostly template values: Tarling
		// 178 cm for 194), with the drag area rescaled by frontal area
		// (Heil), as when a layer corrects weight.
		if h := float64(x.Height); h >= 150 && h <= 210 && h != riders[i].HeightCm {
			if a0, a1 := frontalArea(riders[i].HeightCm, riders[i].WeightKg), frontalArea(h, riders[i].WeightKg); a0 > 0 && a1 > 0 && riders[i].CdA > 0 {
				riders[i].CdA = math.Round(riders[i].CdA*a1/a0*1000) / 1000
			}
			riders[i].HeightCm = h
		}
		if t, ok := trainByName[riders[i].Name]; ok {
			pairs = append(pairs, pair{x, t})
		}
		if teamVotes[x.Team] == nil {
			teamVotes[x.Team] = map[string]int{}
		}
		teamVotes[x.Team][riders[i].Team]++
	}
	if len(pairs) < 50 {
		return riders
	}
	matched := map[int]wdbRider{}
	for _, x := range wdb {
		if i := match(x); i >= 0 {
			matched[i] = x
		}
	}
	defer func() { rankRatings(riders, matched) }()
	if len(missing) == 0 {
		return riders
	}
	fit := fitRatings(pairs)
	// CdA for added riders comes from their body (frontal area) at the
	// field's median CdA per m², not from a fit to template values.
	var perArea []float64
	for _, r := range train {
		if a := frontalArea(r.HeightCm, r.WeightKg); r.CdA > 0 && a > 0 {
			perArea = append(perArea, r.CdA/a)
		}
	}
	sort.Float64s(perArea)
	cdaPerArea := 0.0
	if len(perArea) > 0 {
		cdaPerArea = perArea[len(perArea)/2]
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Name < missing[j].Name })
	for _, x := range missing {
		r := fit.rider(x)
		if cdaPerArea > 0 && r.HeightCm > 0 && r.WeightKg > 0 {
			r.CdA = math.Round(cdaPerArea*frontalArea(r.HeightCm, r.WeightKg)*1000) / 1000
		}
		r.Team = x.Team
		best := 0
		for t, n := range teamVotes[x.Team] {
			if n > best {
				best, r.Team = n, t
			}
		}
		if r.Validate() == nil {
			riders = append(riders, r)
			matched[len(riders)-1] = x
		}
	}
	return riders
}

// rankedRatings are the ratings that come from templates for many of our
// riders (dozens share the same endurance/recovery/handling bundle), each
// with the WorldDB attribute that orders the field on the same quality.
var rankedRatings = []struct {
	charac  int // index into wdbRider.Charac (see ImportWorldDB's attribute list)
	charac2 int // a second attribute summed with the first (-1: none)
	get     func(*model.Rider) *int
}{
	{10, -1, func(r *model.Rider) *int { return &r.Endurance }},         // endurance: fading over a long race
	{12, -1, func(r *model.Rider) *int { return &r.Recovery }},          // recuperation: between days
	{5, -1, func(r *model.Rider) *int { return &r.Handling }},           // cobble: rough surfaces
	{4, -1, func(r *model.Rider) *int { return &r.DescendingAbility }},  // downhilling
	{13, -1, func(r *model.Rider) *int { return &r.TacticalAwareness }}, // baroudeur: going in breaks
	{3, 9, func(r *model.Rider) *int { return &r.Punch }},               // hill + acceleration: short climbs
	{11, -1, func(r *model.Rider) *int { return &r.Repeatability }},     // resistance: repeated efforts in a race
}

// rankRatings re-deals each ranked rating among the riders WorldDB covers:
// the rating keeps exactly its current values (our scale, mean and spread),
// handed out in the order WorldDB ranks the riders on that quality. PCM's
// numbers themselves are never used, only the order of the field (the
// user's caveat: they blend several things).
func rankRatings(riders []model.Rider, matched map[int]wdbRider) {
	idx := make([]int, 0, len(matched))
	for i := range matched {
		if i < len(riders) {
			idx = append(idx, i)
		}
	}
	if len(idx) < 50 {
		return
	}
	for _, rr := range rankedRatings {
		vals := make([]int, len(idx))
		for k, i := range idx {
			vals[k] = *rr.get(&riders[i])
		}
		sort.Ints(vals)
		order := append([]int(nil), idx...)
		sort.SliceStable(order, func(a, b int) bool {
			ca, cb := matched[order[a]].Charac[rr.charac], matched[order[b]].Charac[rr.charac]
			if rr.charac2 >= 0 {
				ca += matched[order[a]].Charac[rr.charac2]
				cb += matched[order[b]].Charac[rr.charac2]
			}
			if ca != cb {
				return ca < cb
			}
			return riders[order[a]].Name < riders[order[b]].Name
		})
		for k, i := range order {
			*rr.get(&riders[i]) = vals[k]
		}
	}
}

// ambiguous: a WorldDB name that fits more than one of our riders.
const ambiguous = -2

type pair struct {
	x wdbRider
	r model.Rider
}

// ratingFit holds one linear fit per rider field, from the PCM attributes,
// weight and height.
type ratingFit struct {
	coef map[string][]float64
}

func features(x wdbRider) []float64 {
	f := []float64{1}
	f = append(f, x.Charac[:]...)
	return append(f, float64(x.Weight), float64(x.Height))
}

// fields are the rider values fitted, as (get, set) on a rider. Power is
// fitted per kilogram, the rest as is.
var fields = []struct {
	name string
	get  func(model.Rider) float64
	set  func(*model.Rider, float64)
}{
	{"wkg", func(r model.Rider) float64 { return float64(r.FTPWatts) / r.WeightKg },
		func(r *model.Rider, v float64) { r.FTPWatts = int(math.Round(v * r.WeightKg)) }},
	{"sprintwkg", func(r model.Rider) float64 { return float64(r.MaxSprintWatts) / r.WeightKg },
		func(r *model.Rider, v float64) { r.MaxSprintWatts = int(math.Round(v * r.WeightKg)) }},
	{"vo2ratio", func(r model.Rider) float64 { return float64(r.VO2MaxWatts) / float64(r.FTPWatts) },
		func(r *model.Rider, v float64) { r.VO2MaxWatts = int(math.Round(v * float64(r.FTPWatts))) }},
	{"cda", func(r model.Rider) float64 { return r.CdA }, func(r *model.Rider, v float64) { r.CdA = math.Round(v*1000) / 1000 }},
	{"climbing", func(r model.Rider) float64 { return float64(r.ClimbingAbility) }, func(r *model.Rider, v float64) { r.ClimbingAbility = ability(v) }},
	{"sprint", func(r model.Rider) float64 { return float64(r.SprintAbility) }, func(r *model.Rider, v float64) { r.SprintAbility = ability(v) }},
	{"tt", func(r model.Rider) float64 { return float64(r.TTAbility) }, func(r *model.Rider, v float64) { r.TTAbility = ability(v) }},
	{"descending", func(r model.Rider) float64 { return float64(r.DescendingAbility) }, func(r *model.Rider, v float64) { r.DescendingAbility = ability(v) }},
	{"endurance", func(r model.Rider) float64 { return float64(r.Endurance) }, func(r *model.Rider, v float64) { r.Endurance = ability(v) }},
	{"recovery", func(r model.Rider) float64 { return float64(r.Recovery) }, func(r *model.Rider, v float64) { r.Recovery = ability(v) }},
	{"consistency", func(r model.Rider) float64 { return float64(r.Consistency) }, func(r *model.Rider, v float64) { r.Consistency = ability(v) }},
	{"tactical", func(r model.Rider) float64 { return float64(r.TacticalAwareness) }, func(r *model.Rider, v float64) { r.TacticalAwareness = ability(v) }},
	{"handling", func(r model.Rider) float64 { return float64(r.Handling) }, func(r *model.Rider, v float64) { r.Handling = ability(v) }},
	{"punch", func(r model.Rider) float64 { return float64(r.PunchRating()) }, func(r *model.Rider, v float64) { r.Punch = ability(v) }},
	{"repeatability", func(r model.Rider) float64 { return float64(r.RepeatabilityRating()) }, func(r *model.Rider, v float64) { r.Repeatability = ability(v) }},
}

func ability(v float64) int { return int(math.Round(math.Max(1, math.Min(100, v)))) }

// fitRatings fits each field by ridge regression on the matched riders.
func fitRatings(pairs []pair) ratingFit {
	fit := ratingFit{coef: map[string][]float64{}}
	X := make([][]float64, len(pairs))
	for i, p := range pairs {
		X[i] = features(p.x)
	}
	for _, f := range fields {
		y := make([]float64, len(pairs))
		for i, p := range pairs {
			y[i] = f.get(p.r)
		}
		fit.coef[f.name] = ridge(X, y, 1.0)
	}
	return fit
}

// rider builds a rider from WorldDB facts and the fitted ratings.
func (f ratingFit) rider(x wdbRider) model.Rider {
	r := model.Rider{
		Name:     x.Name,
		Age:      ageIn(x.Birth, x.StartYear),
		WeightKg: float64(x.Weight),
		HeightCm: float64(x.Height),
	}
	if r.WeightKg < 45 || r.WeightKg > 100 {
		r.WeightKg = 70
	}
	if r.HeightCm < 150 || r.HeightCm > 210 {
		r.HeightCm = 180
	}
	if r.Age < 16 || r.Age > 50 {
		r.Age = 0
	}
	feat := features(x)
	for _, fd := range fields {
		c := f.coef[fd.name]
		v := 0.0
		for j := range c {
			v += c[j] * feat[j]
		}
		fd.set(&r, v)
	}
	r.FTPWatts = int(math.Max(200, math.Min(500, float64(r.FTPWatts))))
	r.MaxSprintWatts = int(math.Max(800, math.Min(2500, float64(r.MaxSprintWatts))))
	r.VO2MaxWatts = int(math.Max(300, math.Min(600, float64(r.VO2MaxWatts))))
	r.CdA = math.Max(0.2, math.Min(0.5, r.CdA))
	return r
}

// ridge solves (XᵀX + λI)β = Xᵀy (the intercept, column 0, unpenalised).
func ridge(X [][]float64, y []float64, lambda float64) []float64 {
	k := len(X[0])
	A := make([][]float64, k)
	for i := range A {
		A[i] = make([]float64, k+1)
	}
	for r, row := range X {
		for i := 0; i < k; i++ {
			for j := 0; j < k; j++ {
				A[i][j] += row[i] * row[j]
			}
			A[i][k] += row[i] * y[r]
		}
	}
	for i := 1; i < k; i++ {
		A[i][i] += lambda
	}
	// Gaussian elimination with partial pivoting.
	for c := 0; c < k; c++ {
		p := c
		for r := c + 1; r < k; r++ {
			if math.Abs(A[r][c]) > math.Abs(A[p][c]) {
				p = r
			}
		}
		A[c], A[p] = A[p], A[c]
		if A[c][c] == 0 {
			continue
		}
		for r := 0; r < k; r++ {
			if r == c {
				continue
			}
			m := A[r][c] / A[c][c]
			for j := c; j <= k; j++ {
				A[r][j] -= m * A[c][j]
			}
		}
	}
	beta := make([]float64, k)
	for i := 0; i < k; i++ {
		if A[i][i] != 0 {
			beta[i] = A[i][k] / A[i][i]
		}
	}
	return beta
}
