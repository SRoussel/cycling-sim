package season

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"time"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
)

// The year's end and the next year's start:
//
//	EndYear: the board judges the year (budget follows), AI teams renew the
//	contracts they want, signings move, unsigned riders become free agents,
//	riders age (and develop or decline), some retire, young riders turn pro.
//	The off-season transfer window opens: the player signs free agents.
//	StartYear: AI teams fill their rosters from the free agents, the new
//	calendar and programmes are drawn up and the board sets new objectives.

// RosterTarget: AI teams fill their rosters to about this many riders.
const RosterTarget = 28

// ageChange is a rider's yearly change in threshold power at an age (the age
// he turns): young riders improve, riders past 30 decline.
func ageChange(age int) float64 {
	switch {
	case age <= 21:
		return 0.025
	case age == 22:
		return 0.02
	case age == 23:
		return 0.015
	case age == 24:
		return 0.01
	case age == 25:
		return 0.005
	case age <= 29:
		return 0
	case age == 30:
		return -0.005
	case age == 31:
		return -0.01
	case age == 32:
		return -0.015
	case age == 33:
		return -0.02
	case age == 34:
		return -0.025
	}
	return -0.035
}

// retireChance: the chance a rider stops at the end of a year at an age;
// a free agent the market rates low (value under 1) is likelier to stop,
// a good one finds a team.
func retireChance(age int, freeAgent bool, value float64) float64 {
	p := 0.0
	switch {
	case age >= 38:
		p = 0.85
	case age == 37:
		p = 0.65
	case age == 36:
		p = 0.45
	case age == 35:
		p = 0.3
	case age == 34:
		p = 0.18
	case age == 33:
		p = 0.1
	case age >= 31:
		p = 0.04
	}
	if freeAgent && age >= 28 {
		p += 0.35 * math.Max(0, math.Min(1, 1-value))
	}
	return math.Min(0.95, p)
}

// YearOver: every event of the year has been raced.
func YearOver(db *database.DB, s *model.Season) (bool, error) {
	next, err := NextEvent(db, s)
	return next == nil && err == nil, err
}

// EndYear closes the year (see the package comment above) and opens the
// off-season transfer window. Returns the year's news.
func EndYear(db *database.DB, s *model.Season) ([]model.NewsItem, error) {
	if s.Phase != model.PhaseRacing {
		return nil, fmt.Errorf("the %d season is already over", s.Year)
	}
	if over, err := YearOver(db, s); err != nil || !over {
		return nil, fmt.Errorf("the %d calendar isn't finished", s.Year)
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, err
	}
	rng := rngFor(s, 1)
	date := time.Date(s.Year, 12, 31, 0, 0, 0, 0, time.UTC)
	var news []model.NewsItem
	say := func(mine bool, f string, a ...any) {
		news = append(news, model.NewsItem{SeasonID: s.ID, Year: s.Year, Date: date, Mine: mine, Text: fmt.Sprintf(f, a...)})
	}

	// 1. The board's verdict and next year's budget.
	progress, err := ObjectivesProgress(db, s)
	if err != nil {
		return nil, err
	}
	conf := BoardConfidence(progress)
	met := 0
	for _, p := range progress {
		if p.Met {
			met++
		}
	}
	old := s.Budget
	s.Budget = NextBudget(s.Budget, conf)
	say(true, "The board's verdict on %d: %d of %d objectives met, confidence %.0f%%. Budget for %d: %s (was %s).",
		s.Year, met, len(progress), conf, s.Year+1, Money(s.Budget), Money(old))

	// 2. AI teams renew the riders they want (normally done at the start
	// of the year; a season made before that rule does it now).
	if s.RenewedYear < s.Year {
		renewAITeams(srs, Values(db, activeRiders(srs)), s, rng)
	}

	// 3. Contracts: signings move, unsigned riders are free.
	for i := range srs {
		sr := &srs[i]
		if sr.Retired {
			continue
		}
		switch {
		case sr.NextTeam != "":
			if sr.NextTeam != sr.Team && sr.Team != "" {
				say(sr.NextTeam == s.Team || sr.Team == s.Team, "%s moves from %s to %s.", sr.Name, sr.Team, sr.NextTeam)
			}
			sr.Team, sr.Salary, sr.ContractEnd = sr.NextTeam, sr.NextSalary, sr.NextContractEnd
			sr.NextTeam, sr.NextSalary, sr.NextContractEnd = "", 0, 0
		case sr.Team != "" && sr.ContractEnd <= s.Year:
			if sr.Team == s.Team {
				say(true, "%s's contract has run out: he is a free agent.", sr.Name)
			}
			sr.Team = ""
		}
	}

	values := Values(db, activeRiders(srs))
	// 4. Retirements, then ageing.
	retired := 0
	for i := range srs {
		sr := &srs[i]
		if sr.Retired {
			continue
		}
		underContract := sr.Team != ""
		chance := retireChance(sr.Age, !underContract, values[sr.ID])
		if underContract && sr.Age < 37 {
			chance *= 0.3 // most riders see their contract out
		}
		if rng.Float64() < chance {
			sr.Retired = true
			retired++
			if sr.Team == s.Team || values[sr.ID] > 1.5 {
				say(sr.Team == s.Team, "%s (%d) retires.", sr.Name, sr.Age)
			}
			sr.Team = ""
			continue
		}
		develop(sr, rng)
		sr.RaceDays, sr.Load = 0, 0
		sr.Morale += (MoraleNeutral - sr.Morale) * 0.5
	}

	// 5. Young riders turn pro (as many as retired), as free agents.
	srs = append(srs, neoPros(srs, retired, rng, s.Year+1)...)
	if err := db.SaveSeasonRiders(s.ID, srs); err != nil {
		return nil, err
	}
	s.Phase = model.PhaseOffseason
	s.Year++
	s.Date = time.Date(s.Year, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := db.UpdateSeason(s); err != nil {
		return nil, err
	}
	say(true, "The transfer window is open: sign free agents before the %d season starts.", s.Year)
	if err := db.AddNews(news); err != nil {
		return nil, err
	}
	return news, nil
}

// develop ages a rider a year: his threshold, VO2max and sprint power change
// with his age (growth scaled by his potential), with a little chance.
func develop(sr *model.SeasonRider, rng *rand.Rand) {
	sr.Age++
	ch := ageChange(sr.Age)
	if ch > 0 {
		ch *= sr.Potential
	}
	ch += rng.NormFloat64() * 0.007
	sprint := ch
	if sprint < 0 {
		sprint *= 1.3 // sprint power fades sooner
	}
	sr.FTPWatts = clampInt(int(math.Round(float64(sr.FTPWatts)*(1+ch))), 200, 500)
	sr.VO2MaxWatts = clampInt(int(math.Round(float64(sr.VO2MaxWatts)*(1+ch))), 300, 600)
	sr.MaxSprintWatts = clampInt(int(math.Round(float64(sr.MaxSprintWatts)*(1+sprint))), 800, 2500)
	if sr.Age <= 28 {
		sr.Endurance = clampInt(sr.Endurance+1, 1, 100)
		sr.TacticalAwareness = clampInt(sr.TacticalAwareness+1, 1, 100)
	}
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(hi, v))
}

// neoPros makes n young riders (19-21) from young pros' bodies and ratings,
// a little weaker, with new names and a potential drawn wide.
func neoPros(srs []model.SeasonRider, n int, rng *rand.Rand, year int) []model.SeasonRider {
	var young []model.SeasonRider
	var maxID int64
	for _, sr := range srs {
		maxID = max(maxID, sr.ID)
		if !sr.Retired && sr.Age <= 24 {
			young = append(young, sr)
		}
	}
	if len(young) == 0 {
		return nil
	}
	taken := map[string]bool{}
	for _, sr := range srs {
		taken[sr.Name] = true
	}
	var out []model.SeasonRider
	for i := 0; i < n; i++ {
		t := young[rng.Intn(len(young))]
		r := t.Rider
		r.ID = maxID + int64(i) + 1
		r.Team = ""
		r.Age = 19 + rng.Intn(3)
		r.Nationality, r.Name = newName(rng, taken)
		scale := 0.90 + rng.Float64()*0.06
		r.FTPWatts = clampInt(int(float64(r.FTPWatts)*scale), 200, 500)
		r.VO2MaxWatts = clampInt(int(float64(r.VO2MaxWatts)*scale), 300, 600)
		r.MaxSprintWatts = clampInt(int(float64(r.MaxSprintWatts)*(0.95+rng.Float64()*0.06)), 800, 2500)
		r.WeightKg = math.Round(r.WeightKg*(0.97+rng.Float64()*0.06)*10) / 10
		for _, a := range []*int{&r.ClimbingAbility, &r.SprintAbility, &r.TTAbility, &r.DescendingAbility, &r.Handling, &r.Recovery, &r.Consistency} {
			*a = clampInt(*a+rng.Intn(11)-5, 1, 100)
		}
		r.Endurance = clampInt(r.Endurance-4, 1, 100)
		r.TacticalAwareness = clampInt(r.TacticalAwareness-6, 1, 100)
		out = append(out, model.SeasonRider{Rider: r, Morale: MoraleNeutral, Potential: potential(rng), ContractEnd: year})
	}
	return out
}

// potential: how much a rider still develops (1 typical, 0.3-1.8).
func potential(rng *rand.Rand) float64 {
	return math.Max(0.3, math.Min(1.8, 1+rng.NormFloat64()*0.3))
}

// StartYear closes the transfer window: AI teams sign free agents up to their
// roster target (the best riders go to the strongest teams, more or less),
// then the year's calendar, programmes and objectives are drawn up.
func StartYear(db *database.DB, s *model.Season) ([]model.NewsItem, error) {
	if s.Phase != model.PhaseOffseason {
		return nil, fmt.Errorf("the %d season is under way", s.Year)
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, err
	}
	rng := rngFor(s, 2)
	values := Values(db, activeRiders(srs))
	for i := range srs {
		if srs[i].FreeAgent() {
			values[srs[i].ID] = valueOf(db, srs[i].Rider, activeRiders(srs))
		}
	}
	// Team pull: last year's points.
	_, teams, err := Standings(db, s, s.Year-1)
	if err != nil {
		return nil, err
	}
	pull := map[string]float64{}
	for _, t := range teams {
		pull[t.Team] = math.Sqrt(float64(t.Points) + 100)
	}
	roster := map[string]int{}
	for _, sr := range srs {
		if !sr.Retired && sr.Team != "" {
			roster[sr.Team]++
		}
	}
	var fas []int
	for i, sr := range srs {
		if sr.FreeAgent() {
			fas = append(fas, i)
		}
	}
	sort.SliceStable(fas, func(a, b int) bool { return values[srs[fas[a]].ID] > values[srs[fas[b]].ID] })
	var news []model.NewsItem
	date := s.Date
	// The player's team never starts a year short: before the other teams
	// sign anyone, it takes the cheapest free agents up to 22 riders.
	if roster[s.Team] < 22 {
		var left []int
		for _, i := range fas {
			if srs[i].FreeAgent() {
				left = append(left, i)
			}
		}
		sort.SliceStable(left, func(a, b int) bool { return srs[left[a]].Salary < srs[left[b]].Salary })
		for _, i := range left {
			if roster[s.Team] >= 22 {
				break
			}
			sr := &srs[i]
			sr.Team, sr.Salary, sr.ContractEnd = s.Team, SalaryFor(values[sr.ID], sr.Age), s.Year
			roster[s.Team]++
			news = append(news, model.NewsItem{SeasonID: s.ID, Year: s.Year, Date: date, Mine: true, Text: fmt.Sprintf("%s joins %s on a one-year contract to fill the roster.", sr.Name, s.Team)})
		}
	}
	for _, i := range fas {
		if !srs[i].FreeAgent() {
			continue // taken by the player's team
		}
		best, bestScore := "", 0.0
		for team, n := range roster {
			if team == s.Team || n >= RosterTarget {
				continue
			}
			if score := (pull[team] + 1) * (0.5 + rng.Float64()); score > bestScore {
				best, bestScore = team, score
			}
		}
		if best == "" {
			break
		}
		sr := &srs[i]
		sr.Team, sr.Salary, sr.ContractEnd = best, SalaryFor(values[sr.ID], sr.Age), s.Year+rng.Intn(2)
		roster[best]++
		if values[sr.ID] > 1 {
			news = append(news, model.NewsItem{SeasonID: s.ID, Year: s.Year, Date: date, Text: fmt.Sprintf("%s signs for %s.", sr.Name, best)})
		}
	}
	// AI teams extend the contracts that end this year before the transfer
	// period opens.
	renewAITeams(srs, values, s, rng)
	if err := db.SaveSeasonRiders(s.ID, srs); err != nil {
		return nil, err
	}
	s.Phase = model.PhaseRacing
	if err := db.UpdateSeason(s); err != nil {
		return nil, err
	}
	if err := buildCalendar(db, s); err != nil {
		return nil, err
	}
	if err := planYear(db, s); err != nil {
		return nil, err
	}
	if err := setObjectives(db, s); err != nil {
		return nil, err
	}
	news = append(news, model.NewsItem{SeasonID: s.ID, Year: s.Year, Date: date, Mine: true, Text: fmt.Sprintf("The %d season begins.", s.Year)})
	if err := db.AddNews(news); err != nil {
		return nil, err
	}
	return news, nil
}

// renewAITeams: AI teams extend the contracts ending this year of the riders
// they want: their 24 best under 35, the eight best nearly always. Teams do
// it early in the year, before the transfer period (1 August) lets other
// teams approach their riders. Marks the year done on s.
func renewAITeams(srs []model.SeasonRider, values map[int64]float64, s *model.Season, rng *rand.Rand) {
	byTeam := map[string][]int{}
	for i, sr := range srs {
		if !sr.Retired && sr.Team != "" && sr.Team != s.Team {
			byTeam[sr.Team] = append(byTeam[sr.Team], i)
		}
	}
	teams := make([]string, 0, len(byTeam))
	for t := range byTeam {
		teams = append(teams, t)
	}
	sort.Strings(teams) // a fixed order: the draws don't depend on map order
	for _, t := range teams {
		idx := byTeam[t]
		sort.SliceStable(idx, func(a, b int) bool { return values[srs[idx[a]].ID] > values[srs[idx[b]].ID] })
		for rank, i := range idx {
			sr := &srs[i]
			if sr.ContractEnd > s.Year || sr.NextTeam != "" {
				continue
			}
			keep := 0.85
			if rank < 8 {
				keep = 0.95
			}
			if rank < 24 && sr.Age < 35 && rng.Float64() < keep {
				sr.NextTeam, sr.NextSalary = sr.Team, SalaryFor(values[sr.ID], sr.Age+1)
				sr.NextContractEnd = s.Year + 1 + rng.Intn(3)
			}
		}
	}
	s.RenewedYear = s.Year
}

// EnsureRenewals runs the AI teams' renewals for the season's year if they
// haven't been (seasons made before the rule; a new year's start does it).
func EnsureRenewals(db *database.DB, s *model.Season) error {
	if s.RenewedYear >= s.Year || s.Phase != model.PhaseRacing {
		return nil
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return err
	}
	renewAITeams(srs, Values(db, activeRiders(srs)), s, rngFor(s, 3))
	if err := db.SaveSeasonRiders(s.ID, srs); err != nil {
		return err
	}
	return db.UpdateSeason(s)
}

// NextBudget is next year's budget from this year's and the board's
// confidence (%): from 88% of it at no confidence to 112% at full.
func NextBudget(budget int, confidence float64) int {
	return int(math.Round(float64(budget)*(0.88+0.24*confidence/100)/10) * 10)
}
