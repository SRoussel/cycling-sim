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

// rngFor is a reproducible random stream for one decision of a season.
func rngFor(s *model.Season, salt int64) *rand.Rand {
	return rand.New(rand.NewSource(s.ID*1_000_003 + int64(s.Year)*7919 + salt))
}

// activeRiders are the riders on a team (not retired, not free agents).
func activeRiders(srs []model.SeasonRider) []model.Rider {
	var out []model.Rider
	for _, sr := range srs {
		if !sr.Retired && sr.Team != "" {
			out = append(out, sr.Rider)
		}
	}
	return out
}

// WageBill is a team's salaries this year (thousand euro).
func WageBill(srs []model.SeasonRider, team string) int {
	n := 0
	for _, sr := range srs {
		if sr.Team == team && !sr.Retired {
			n += sr.Salary
		}
	}
	return n
}

// NextWageBill is what a team has committed for next year: riders under
// contract beyond this year and riders signed for next year.
func NextWageBill(srs []model.SeasonRider, team string, year int) (bill, riders int) {
	for _, sr := range srs {
		switch {
		case sr.Retired:
		case sr.NextTeam == team:
			bill += sr.NextSalary
			riders++
		case sr.NextTeam == "" && sr.Team == team && sr.ContractEnd > year:
			bill += sr.Salary
			riders++
		}
	}
	return bill, riders
}

// TeamRanks ranks teams by strength (1 = strongest): see teamStrength.
func TeamRanks(riders []model.Rider, values map[int64]float64) map[string]int {
	return teamStrength(riders, values)
}

// teamStrength ranks teams by their eight best riders' values (1 = strongest).
func teamStrength(riders []model.Rider, values map[int64]float64) map[string]int {
	byTeam := map[string][]float64{}
	for _, r := range riders {
		byTeam[r.Team] = append(byTeam[r.Team], values[r.ID])
	}
	type ts struct {
		team string
		sum  float64
	}
	var all []ts
	for team, vs := range byTeam {
		sort.Sort(sort.Reverse(sort.Float64Slice(vs)))
		sum := 0.0
		for i := 0; i < len(vs) && i < 8; i++ {
			sum += vs[i]
		}
		all = append(all, ts{team, sum})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].sum != all[j].sum {
			return all[i].sum > all[j].sum
		}
		return all[i].team < all[j].team
	})
	rank := map[string]int{}
	for i, t := range all {
		rank[t.team] = i + 1
	}
	return rank
}

// setObjectives: the board's goals for the year, from the team's strength
// and its riders' programmes: a team-ranking place, a number of wins, and up
// to three goals in the biggest races the team's leaders target.
func setObjectives(db *database.DB, s *model.Season) error {
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return err
	}
	riders := activeRiders(srs)
	values := Values(db, riders)
	rank := teamStrength(riders, values)[s.Team]
	teams := len(teamStrength(riders, values))
	objs := []model.Objective{
		{Kind: model.ObjectiveTeamRank, Target: min(teams, rank+2), Weight: 1.5,
			Text: fmt.Sprintf("Finish in the top %d of the team ranking", min(teams, rank+2))},
	}
	wins := 1
	switch {
	case rank <= 2:
		wins = 15
	case rank <= 4:
		wins = 8
	case rank <= 7:
		wins = 5
	case rank <= 11:
		wins = 3
	case rank <= 16:
		wins = 2
	}
	objs = append(objs, model.Objective{Kind: model.ObjectiveWins, Target: wins, Weight: 1,
		Text: fmt.Sprintf("Win %d races (stages count)", wins)})

	events, err := db.ListSeasonEvents(s.ID, s.Year)
	if err != nil {
		return err
	}
	entries, err := db.ListSeasonEntries(s.ID)
	if err != nil {
		return err
	}
	byID := map[int64]model.Rider{}
	for _, r := range riders {
		byID[r.ID] = r
	}
	type cand struct {
		o     model.Objective
		score float64
	}
	var cands []cand
	for _, e := range events {
		var field []model.Rider
		for _, id := range entries[e.ID] {
			if r, ok := byID[id]; ok {
				field = append(field, r)
			}
		}
		if len(field) == 0 {
			continue
		}
		profiles, err := eventProfiles(db, e)
		if err != nil {
			return err
		}
		place := rankIn(field, gcFits(profiles, field))
		var r model.Rider
		for _, x := range field {
			if x.Team == s.Team && (r.ID == 0 || place[x.ID] < place[r.ID]) {
				r = x
			}
		}
		if r.ID == 0 {
			continue
		}
		p := place[r.ID]
		if p <= 8 {
			// Racing is chancy: the target leaves room (expected 2nd: top 6).
			target := max(3, min(15, 2*p+2))
			what := "the GC of"
			if e.Kind == model.EventOneDay {
				what = ""
			}
			text := fmt.Sprintf("Top %d in %s %s (%s)", target, what, e.Name, r.Name)
			if what == "" {
				text = fmt.Sprintf("Top %d at %s (%s)", target, e.Name, r.Name)
			}
			cands = append(cands, cand{model.Objective{Kind: model.ObjectivePlace, EventID: e.ID, RiderID: r.ID, Target: target, Weight: Prestige(e) / 60, Text: text}, Prestige(e) / float64(p)})
		}
		if e.Category == model.CategoryGrandTour && profiles.stageRace {
			stage := rankIn(field, fits(profiles, field))
			for _, sr := range field {
				if sr.Team == s.Team && stage[sr.ID] <= 4 && place[sr.ID] > 3 {
					cands = append(cands, cand{model.Objective{Kind: model.ObjectiveStageWin, EventID: e.ID, RiderID: sr.ID, Target: 1, Weight: Prestige(e) / 80,
						Text: fmt.Sprintf("Win a stage of the %s (%s)", e.Name, sr.Name)}, Prestige(e) / float64(stage[sr.ID]) * 0.8})
					break
				}
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].score > cands[j].score })
	for i := 0; i < len(cands) && i < 3; i++ {
		objs = append(objs, cands[i].o)
	}
	for i := range objs {
		objs[i].SeasonID, objs[i].Year = s.ID, s.Year
	}
	return db.SaveObjectives(s.ID, s.Year, objs)
}

// rankIn turns fit scores into places in a field (1 = best).
func rankIn(field []model.Rider, fit map[int64]float64) map[int64]int {
	ids := make([]int64, 0, len(field))
	for _, r := range field {
		ids = append(ids, r.ID)
	}
	sort.SliceStable(ids, func(i, j int) bool { return fit[ids[i]] > fit[ids[j]] })
	out := make(map[int64]int, len(ids))
	for i, id := range ids {
		out[id] = i + 1
	}
	return out
}

// Progress is an objective with how it stands.
type Progress struct {
	model.Objective
	Current int  // the team's rank, wins, best place (0: not yet raced)
	Met     bool // achieved (for a team rank: as things stand)
	Decided bool // can't change any more this year
}

// ObjectivesProgress evaluates the year's objectives on the results so far.
func ObjectivesProgress(db *database.DB, s *model.Season) ([]Progress, error) {
	objs, err := db.ListObjectives(s.ID, s.Year)
	if err != nil {
		return nil, err
	}
	results, err := db.ListSeasonResults(s.ID, s.Year, 0)
	if err != nil {
		return nil, err
	}
	events, err := db.ListSeasonEvents(s.ID, s.Year)
	if err != nil {
		return nil, err
	}
	done := map[int64]bool{}
	yearDone := true
	for _, e := range events {
		done[e.ID] = e.Done
		yearDone = yearDone && e.Done
	}
	_, teams, err := Standings(db, s, s.Year)
	if err != nil {
		return nil, err
	}
	teamRank, wins := 0, 0
	for i, t := range teams {
		if t.Team == s.Team {
			teamRank, wins = i+1, t.Wins
		}
	}
	if teamRank == 0 {
		teamRank = len(teams) + 1
	}
	var out []Progress
	for _, o := range objs {
		p := Progress{Objective: o}
		switch o.Kind {
		case model.ObjectiveTeamRank:
			p.Current, p.Met, p.Decided = teamRank, teamRank <= o.Target, yearDone
		case model.ObjectiveWins:
			p.Current, p.Met = wins, wins >= o.Target
			p.Decided = p.Met || yearDone
		case model.ObjectivePlace:
			for _, r := range results {
				if r.EventID == o.EventID && r.Team == s.Team && (r.Kind == model.ResultRace || r.Kind == model.ResultGC) {
					if p.Current == 0 || r.Position < p.Current {
						p.Current = r.Position
					}
				}
			}
			p.Met, p.Decided = p.Current > 0 && p.Current <= o.Target, done[o.EventID]
		case model.ObjectiveStageWin:
			for _, r := range results {
				if r.EventID == o.EventID && r.Team == s.Team && r.Kind == model.ResultStage && r.Position == 1 {
					p.Current++
				}
			}
			p.Met, p.Decided = p.Current >= o.Target, done[o.EventID]
		}
		out = append(out, p)
	}
	return out, nil
}

// BoardConfidence (0-100): 50 with nothing decided; each objective met adds
// to it, each one missed takes away, weighted by how much the board cares.
func BoardConfidence(ps []Progress) float64 {
	c := 50.0
	for _, p := range ps {
		switch {
		case p.Met:
			c += 15 * p.Weight
		case p.Decided:
			c -= 12 * p.Weight
		}
	}
	return math.Max(0, math.Min(100, c))
}

// AskingSalary is what a rider wants to sign (thousand euro/year): his value's
// salary, more if he's unhappy and more again to leave his team.
func AskingSalary(sr model.SeasonRider, value float64, leaving bool) int {
	return AskingSalaryTo(sr, value, leaving, 0)
}

// AskingSalaryTo is the asking salary to move to a team stepsDown places
// weaker than his own (team strength ranks): 3% more per place (a rider
// leaving the best team for the 15th asks ~40% more).
func AskingSalaryTo(sr model.SeasonRider, value float64, leaving bool, stepsDown int) int {
	ask := float64(SalaryFor(value, sr.Age+1)) * (1 + (MoraleNeutral-sr.Morale)/200)
	if leaving {
		ask *= 1.15 * (1 + stepDownPremium*float64(max(0, stepsDown)))
	}
	return int(math.Round(ask/10) * 10)
}

// stepDownPremium: the extra salary a rider asks per place of team strength
// he'd drop by moving.
const stepDownPremium = 0.03

// TransferOpens is the first day other teams' riders may agree to join for
// next year (the UCI transfer period opens on 1 August).
func TransferOpens(year int) time.Time { return time.Date(year, 8, 1, 0, 0, 0, 0, time.UTC) }

// MaxRoster is the most riders a team may have under contract.
const MaxRoster = 30

// Offer is the player's contract offer to a rider: his own (a renewal) whose
// contract ends this year, another team's whose contract ends this year
// (signed for next year), or a free agent in the off-season (joins now).
// The rider accepts at his asking salary or more, if the team can pay and
// has room. Returns whether he signed and his answer.
func Offer(db *database.DB, s *model.Season, riderID int64, salary, years int) (bool, string, error) {
	if err := EnsureRenewals(db, s); err != nil {
		return false, "", err
	}
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return false, "", err
	}
	idx := -1
	for i := range srs {
		if srs[i].ID == riderID {
			idx = i
		}
	}
	if idx < 0 {
		return false, "", fmt.Errorf("no rider %d", riderID)
	}
	sr := &srs[idx]
	switch {
	case sr.Retired:
		return false, sr.Name + " has retired.", nil
	case sr.NextTeam != "":
		return false, fmt.Sprintf("%s has already signed with %s for next year.", sr.Name, sr.NextTeam), nil
	case sr.Team != "" && sr.ContractEnd > s.Year:
		return false, fmt.Sprintf("%s is under contract until %d.", sr.Name, sr.ContractEnd), nil
	case years < 1 || years > 4:
		return false, "Contracts run 1 to 4 years.", nil
	case sr.Team != "" && sr.Team != s.Team && s.Phase == model.PhaseRacing && s.Date.Before(TransferOpens(s.Year)):
		return false, fmt.Sprintf("%s rides for %s: other teams' riders can agree to join from 1 August (the transfer period).", sr.Name, sr.Team), nil
	}
	values := Values(db, activeRiders(srs))
	v, ok := values[sr.ID]
	if !ok {
		v = valueOf(db, sr.Rider, activeRiders(srs))
	}
	leaving := sr.Team != s.Team && sr.Team != ""
	steps := 0
	if leaving {
		rank := teamStrength(activeRiders(srs), values)
		steps = rank[s.Team] - rank[sr.Team]
	}
	ask := AskingSalaryTo(*sr, v, leaving, steps)
	if salary < ask {
		return false, fmt.Sprintf("%s wants at least %s a year.", sr.Name, Money(ask)), nil
	}
	free := s.Phase == model.PhaseOffseason && sr.FreeAgent()
	bill, count := NextWageBill(srs, s.Team, s.Year)
	if free {
		bill, count = 0, 0
		for _, x := range srs {
			if x.Team == s.Team && !x.Retired {
				bill += x.Salary
				count++
			}
		}
	}
	if sr.Team == s.Team {
		count-- // a renewal doesn't add a rider
	}
	if count+1 > MaxRoster {
		return false, fmt.Sprintf("The roster is full (%d riders).", MaxRoster), nil
	}
	if bill+salary > s.Budget {
		return false, fmt.Sprintf("Over budget: %s committed of %s.", Money(bill), Money(s.Budget)), nil
	}
	if free {
		sr.Team, sr.Salary, sr.ContractEnd = s.Team, salary, s.Year+years-1
	} else {
		sr.NextTeam, sr.NextSalary, sr.NextContractEnd = s.Team, salary, s.Year+years
	}
	sr.Morale = math.Min(100, sr.Morale+5)
	if err := db.SaveSeasonRiders(s.ID, []model.SeasonRider{*sr}); err != nil {
		return false, "", err
	}
	what := "joins"
	if sr.Team == s.Team && !free {
		what = "renews with"
	}
	_ = db.AddNews([]model.NewsItem{{SeasonID: s.ID, Year: s.Year, Date: s.Date, Mine: true,
		Text: fmt.Sprintf("%s %s %s: %d years at %s.", sr.Name, what, s.Team, years, Money(salary))}})
	return true, fmt.Sprintf("%s signs: %d years at %s.", sr.Name, years, Money(salary)), nil
}

// valueOf rates a rider against a field he isn't in (a free agent).
func valueOf(db *database.DB, r model.Rider, field []model.Rider) float64 {
	return Values(db, append(append([]model.Rider{}, field...), r))[r.ID]
}

// Money formats thousands of euro ("€1.2M", "€450k").
func Money(k int) string {
	if k >= 1000 {
		return fmt.Sprintf("€%.1fM", float64(k)/1000)
	}
	return fmt.Sprintf("€%dk", k)
}

// AutoRenew renews the player's riders whose contracts end this year, best
// first, at their asking salary for two years, while the budget allows and
// up to keep riders under contract next year. Returns the replies.
func AutoRenew(db *database.DB, s *model.Season, keep int) ([]string, error) {
	srs, err := db.ListSeasonRiders(s.ID)
	if err != nil {
		return nil, err
	}
	values := Values(db, activeRiders(srs))
	var mine []model.SeasonRider
	for _, sr := range srs {
		if sr.Team == s.Team && !sr.Retired && sr.ContractEnd <= s.Year && sr.NextTeam == "" && sr.Age < 35 {
			mine = append(mine, sr)
		}
	}
	sort.SliceStable(mine, func(i, j int) bool { return values[mine[i].ID] > values[mine[j].ID] })
	_, count := NextWageBill(srs, s.Team, s.Year)
	var out []string
	for _, sr := range mine {
		if count >= keep {
			break
		}
		ok, reply, err := Offer(db, s, sr.ID, AskingSalary(sr, values[sr.ID], false), 2)
		if err != nil {
			return out, err
		}
		out = append(out, reply)
		if ok {
			count++
		}
	}
	return out, nil
}
