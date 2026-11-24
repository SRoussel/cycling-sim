package simulation

import (
	"math"
	"math/rand"
	"sort"

	"cycling_sim/internal/model"
)

// Planned chases (stage races). A bunch doesn't chase because every rider
// follows a rule: teams decide how much a break may gain, and their
// domestiques ride just hard enough to keep it there.
//
//   - A team has an interest when a break threatens what it rides for: for a
//     GC leader in the group, a GC contender in the break (see gcContender;
//     contenders aren't let go at all, so this is a move that got away);
//     for a sprinter, a break that would take the stage, when the sprinter can
//     stay with the group to the line (sprinterCanFollow; the break must be
//     caught catchMarginS before the line).
//   - A favourite for the stage has their team hunt it when that is
//     affordable: a steady tempo (stageHuntMax) catches the break in time.
//   - The tightest allowance sets the plan: the lowest pace-line intensity at
//     which the chase keeps the break within it at the finish, from the
//     projected times to the line. A break nobody needs to catch is let go.
//     If even an all-out chase can't, the chasers go all in.
//   - Only the interested teams' domestiques chase; protected riders sit in
//     (a leader with no teammates left chases themselves).

// stageHuntMax: a favourite's team chases a break for the stage only if a
// steady tempo (this fraction of threshold) catches it; a costlier chase
// isn't worth the domestiques' legs.
const stageHuntMax = 0.85

// catchMarginS: a sprint team aims to have the break back this long before
// the line (about a kilometre at sprint speed, time to set up a lead-out).
const catchMarginS = 60.0

// chaseAllIn: a plan needing more than this fraction of threshold is an
// all-out chase (threshold plus W' to the line).
const chaseAllIn = 1.0

// chasePlan is a road group's plan for the groups ahead.
// traceChase, when set (trace harnesses only), sees every planned chase.
// traceEffort, when set (trace harnesses only), sees every rider's decided
// effort: action, power, whether his team drives and whether it chases.
var traceEffort func(s *RaceState, rs *RiderState, action Action, power float64, driving, teamChase bool)
var traceChase func(s *RaceState, group []*RiderState, plan chasePlan)

type chasePlan struct {
	intensity float64         // fraction of threshold the chasers ride; 0 = no chase
	teams     map[string]bool // teams whose domestiques chase
}

func (p chasePlan) chasing(rs *RiderState, teammates map[string]int) bool {
	if p.intensity <= 0 || !p.teams[rs.Rider.Team] {
		return false
	}
	switch rs.TeamRole {
	case RoleDomestique:
		return true
	case RoleLeader:
		return teammates[rs.Rider.Team] == 0 // nobody left to do it
	}
	return false
}

// planChase works out a road group's chase of the groups ahead.
func (s *RaceState) planChase(group []*RiderState) chasePlan {
	if s == nil || len(s.Ahead) == 0 || len(s.Course) == 0 {
		return chasePlan{}
	}
	// Groups ahead, by their lead.
	ahead := map[float64][]*RiderState{}
	for r, lead := range s.Ahead {
		if !r.DNF {
			ahead[lead] = append(ahead[lead], r)
		}
	}
	plan := chasePlan{teams: map[string]bool{}}
	var need float64
	for lead, brk := range ahead {
		allowed := math.Inf(1)
		teams := map[string]bool{}
		hunters := map[string]bool{}       // favourites' teams that would like the stage
		allow := map[string]float64{}      // each interested team's allowance
		placeAllow := map[string]float64{} // place defenders' allowances (affordable chases only)
		for _, p := range group {
			if p.DNF {
				continue
			}
			a := math.Inf(1)
			sprintHunt := false
			switch p.TeamRole {
			case RoleLeader:
				// A GC contender's team defends its leader's place when it is
				// worth a day's work (in the top gcPlacesProtected; before any
				// GC, every contender): the break may gain until its
				// best-placed rider would pass the leader on GC (a rival in
				// the move gains nothing; a break of riders far down may take
				// the stage, or the jersey for a day).
				if !s.IsStageRace {
					// One-day race: the race is the only objective, so a
					// favourite's team has the break back before the line.
					if s.isDangerous(p) {
						a = -catchMarginS
					}
				} else if place := s.gcPlace(p); s.gcContender(p) && place <= gcPlacesProtected {
					for _, r := range brk {
						if r.Rider.Team != p.Rider.Team {
							if g, ok := s.GCGaps[r.Rider.ID]; ok {
								a = math.Min(a, math.Max(0, g-s.GCGaps[p.Rider.ID]))
							}
						}
					}
					// The race leader's team defends the jersey whatever it
					// costs; teams defending a lower place (or, before any
					// GC, every contender's) chase only when it's affordable.
					if place != 1 && a < math.Inf(1) {
						if old, ok := placeAllow[p.Rider.Team]; !ok || a < old {
							placeAllow[p.Rider.Team] = a
						}
						a = math.Inf(1)
					}
				}
			case RoleSprinter:
				if s.teamOrders(p.Rider.Team).Ambition != OrderNo && s.sprinterCanFollow(p, group) {
					// A sprinter who survives even the favourites racing the
					// finale (sprinterSurvivesFinale: true of a flat run-in)
					// has his team chase whatever it costs. One who holds a
					// threshold pace but not their attacks (a puncheur on a
					// hilly stage race day) has it ride for the stage as a
					// hunter does, only when a steady tempo catches the break:
					// chasing whatever it cost, Narváez's and Holter's teams
					// chased every hilly and mountain break of the Giro.
					if !s.IsStageRace || s.sprinterSurvivesFinale(p, group) {
						a = -catchMarginS
					} else {
						sprintHunt = true
					}
				}
			}
			if teamIn(brk, p.Rider.Team) {
				continue
			}
			// Ordered to ride for the stage: the leader's team has the
			// break back before the line, as a one-day favourite's does.
			if p.TeamRole == RoleLeader && s.IsStageRace && s.teamOrders(p.Rider.Team).Ambition == OrderYes {
				a = math.Min(a, -catchMarginS)
			}
			switch s.teamOrders(p.Rider.Team).Chase {
			case OrderNo:
				continue // ordered to sit in
			case OrderYes:
				a = math.Min(a, -catchMarginS) // ordered to bring it back
			}
			if a < math.Inf(1) {
				if old, ok := allow[p.Rider.Team]; !ok || a < old {
					allow[p.Rider.Team] = a
				}
			} else if sprintHunt || p.TeamRole == RoleLeader && s.hunting(p) {
				hunters[p.Rider.Team] = true
			}
		}
		// Only teams whose allowance an easy bunch wouldn't respect chase;
		// the tightest of those sets the pace.
		if len(allow) > 0 {
			all := map[string]bool{}
			for t := range allow {
				all[t] = true
			}
			if line := chaseLine(group, all); len(line) > 0 {
				easyAdv := lead + s.timeToLineAt(line, easyIntensity) - s.timeToLine(topLine(brk))
				for t, a := range allow {
					if a < easyAdv {
						teams[t] = true
						allowed = math.Min(allowed, a)
					}
				}
			}
		}
		f := 0.0
		if len(teams) > 0 && allowed < math.Inf(1) {
			if chasers := chaseLine(group, teams); len(chasers) > 0 {
				f = s.requiredIntensity(chasers, topLine(brk), lead, allowed)
			}
		}
		// Place defence: teams defending a GC place below the lead join the
		// chase when a steady tempo keeps the break within their allowance;
		// a costlier chase isn't worth a day of their domestiques for a
		// minor place.
		if len(placeAllow) > 0 {
			all := map[string]bool{}
			for t := range teams {
				all[t] = true
			}
			tightest := allowed
			for t, a := range placeAllow {
				all[t] = true
				tightest = math.Min(tightest, a)
			}
			if chasers := chaseLine(group, all); len(chasers) > 0 {
				if h := s.requiredIntensity(chasers, topLine(brk), lead, tightest); h <= stageHuntMax {
					if h > easyIntensity {
						teams = all
						f = math.Max(f, h)
					}
				}
			}
		}
		// Stage hunting: a favourite's team chases for the stage when that
		// is affordable (a steady tempo catches the break); otherwise it
		// lets the break have the day and saves its riders.
		if len(hunters) > 0 {
			all := map[string]bool{}
			for t := range teams {
				all[t] = true
			}
			for t := range hunters {
				all[t] = true
			}
			if chasers := chaseLine(group, all); len(chasers) > 0 {
				if h := s.requiredIntensity(chasers, topLine(brk), lead, -catchMarginS); h <= stageHuntMax {
					teams = all
					f = math.Max(f, h)
				}
			}
		}
		if f == 0 {
			continue
		}
		need = math.Max(need, f)
		for t := range teams {
			plan.teams[t] = true
		}
	}
	plan.intensity = need
	return plan
}

// requiredIntensity is the lowest chase intensity keeping the break's
// advantage at the line within allowed (seconds), between the easy tempo
// and an all-out effort.
func (s *RaceState) requiredIntensity(chasers, brk []*RiderState, lead, allowed float64) float64 {
	tBreak := s.timeToLine(brk)
	adv := func(f float64) float64 { return lead + s.timeToLineAt(chasers, f) - tBreak }
	lo, hi := easyIntensity, chaseAllIn
	if lo > hi {
		lo = hi
	}
	if adv(lo) <= allowed {
		return lo // a tempo keeps it in check
	}
	if adv(hi) > allowed {
		return hi + 0.01 // can't be done at threshold: all in
	}
	for i := 0; i < 12; i++ {
		mid := (lo + hi) / 2
		if adv(mid) <= allowed {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi
}

// chaseLine is the pace line the interested teams can field: their
// domestiques (or a leader with none left), strongest first.
func chaseLine(group []*RiderState, teams map[string]bool) []*RiderState {
	mates := map[string]int{}
	for _, rs := range group {
		if rs.TeamRole == RoleDomestique && !rs.DNF {
			mates[rs.Rider.Team]++
		}
	}
	p := chasePlan{intensity: 1, teams: teams}
	var out []*RiderState
	for _, rs := range group {
		if !rs.DNF && p.chasing(rs, mates) {
			out = append(out, rs)
		}
	}
	return topLine(out)
}

// topLine is the strongest pace line (up to maxRotation riders) of a group.
func topLine(group []*RiderState) []*RiderState {
	out := append([]*RiderState(nil), group...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].CurrentFTP() > out[j].CurrentFTP() })
	if len(out) > maxRotation {
		out = out[:maxRotation]
	}
	return out
}

func teamIn(group []*RiderState, team string) bool {
	for _, rs := range group {
		if rs.Rider.Team == team {
			return true
		}
	}
	return false
}

// sprinterCanFollow reports whether a sprinter can stay with this group to
// the line: on every remaining segment, sitting in the bunch, they can hold
// the speed the group's fastest pace line there rides at threshold (how hard a
// bunch races over a climb) with their threshold plus W', one tank spent and
// refilled along the way (not a full one per segment). If not, the finish isn't theirs and their team won't chase for it.
func (s *RaceState) sprinterCanFollow(sp *RiderState, group []*RiderState) bool {
	if v, ok := s.canFollow[sp]; ok {
		return v
	}
	ok := s.sprinterFollows(sp, group)
	if s.canFollow == nil {
		s.canFollow = map[*RiderState]bool{}
	}
	s.canFollow[sp] = ok
	return ok
}

// sprinterSurvivesFinale is sprinterCanFollow with the finale raced: over the
// last finalKm the group's fastest riders ride their threshold plus their
// W' spread over each segment (maxPowerFor: the favourites attacking), not a
// threshold pace line.
func (s *RaceState) sprinterSurvivesFinale(sp *RiderState, group []*RiderState) bool {
	if v, ok := s.survivesFinale[sp]; ok {
		return v
	}
	ok := s.sprinterFollowsAt(sp, group, true)
	if s.survivesFinale == nil {
		s.survivesFinale = map[*RiderState]bool{}
	}
	s.survivesFinale[sp] = ok
	return ok
}

func (s *RaceState) sprinterFollows(sp *RiderState, group []*RiderState) bool {
	return s.sprinterFollowsAt(sp, group, false)
}

// finalKm: the finale the favourites race (PhaseFinal's start).
const finalKm = 20.0

func (s *RaceState) sprinterFollowsAt(sp *RiderState, group []*RiderState, raced bool) bool {
	if len(group) <= 1 {
		return true
	}
	// Distance left at the start of each remaining segment.
	left := make([]float64, len(s.Course)+1)
	for i := len(s.Course) - 1; i >= s.SegIndex && i >= 0; i-- {
		left[i] = left[i+1] + s.Course[i].DistanceKm
	}
	r := sp.Rider
	wp := *sp.WPrime
	for i := s.SegIndex; i < len(s.Course); i++ {
		seg := &s.Course[i]
		km := seg.DistanceKm
		if i == s.SegIndex && s.SegmentKmLeft > 0 && s.SegmentKmLeft < km {
			km = s.SegmentKmLeft
		}
		line := s.fastLineWithout(group, i, sp)
		if len(line) == 0 {
			return true
		}
		n := float64(len(line))
		var p, m, cda, handling float64
		finale := raced && left[i] <= finalKm+seg.DistanceKm
		for _, rs := range line {
			thr := EffectivePower(rs.Rider, rs.CurrentFTP(), seg, false)
			if finale {
				v0 := segmentSpeed(thr, rs.Rider.TotalMassKg(), rs.Rider.CdA, seg, rs.Rider.Handling)
				thr = rs.maxPowerFor(thr, thr, SegmentTime(v0, km))
			}
			p += thr
			m += rs.Rider.TotalMassKg()
			cda += rs.Rider.CdA
			handling += float64(rs.Rider.Handling)
		}
		v := segmentSpeed(p/n, m/n, cda/n*rotationFactor(len(line), seg.SurfaceType), seg, int(handling/n))
		need := segmentPower(v, r.TotalMassKg(), r.CdA*groupDraft(maxRotation+1, false, seg, v), seg, r.Handling)
		thr := EffectivePower(r, sp.CurrentFTP(), seg, false)
		t := SegmentTime(v, km)
		if need > thr && (need-thr)*t > wp.WPrimeCurrent {
			return false // W' runs out: dropped
		}
		wp.Update(need, int(thr), t) // one W' tank over the whole run-in

	}
	return true
}

// fastKey identifies a group's fastest line on one course segment.
type fastKey struct {
	first *RiderState
	n     int
	seg   int
}

// fastLineWithout is the pace line the group's fastest riders on course
// segment i would form (climbers on a climb, big engines on the flat),
// leaving out one rider. The ranking is shared by every rider asking within a
// step (cached).
func (s *RaceState) fastLineWithout(group []*RiderState, i int, without *RiderState) []*RiderState {
	key := fastKey{group[0], len(group), i}
	top, ok := s.fastLines[key]
	if !ok {
		top = fastestRiders(group, &s.Course[i], maxRotation+1)
		if s.fastLines == nil {
			s.fastLines = map[fastKey][]*RiderState{}
		}
		s.fastLines[key] = top
	}
	out := make([]*RiderState, 0, maxRotation)
	for _, rs := range top {
		if rs != without && len(out) < maxRotation {
			out = append(out, rs)
		}
	}
	return out
}

// fastestRiders returns the k riders of a group fastest on a segment at
// threshold, fastest first (DNF riders left out).
func fastestRiders(group []*RiderState, seg *model.Segment, k int) []*RiderState {
	type rv struct {
		rs *RiderState
		v  float64
	}
	best := make([]rv, 0, k+1)
	for _, rs := range group {
		if rs.DNF {
			continue
		}
		r := rs.Rider
		v := segmentSpeed(EffectivePower(r, rs.CurrentFTP(), seg, false), r.TotalMassKg(), r.CdA, seg, r.Handling)
		if len(best) == k && v <= best[k-1].v {
			continue
		}
		j := len(best)
		best = append(best, rv{rs, v})
		for j > 0 && best[j-1].v < v {
			best[j] = best[j-1]
			j--
		}
		best[j] = rv{rs, v}
		if len(best) > k {
			best = best[:k]
		}
	}
	out := make([]*RiderState, len(best))
	for i, x := range best {
		out[i] = x.rs
	}
	return out
}

// timeToLineAt is timeToLine at a fraction of threshold (not cached).
func (s *RaceState) timeToLineAt(line []*RiderState, f float64) float64 {
	n := float64(len(line))
	t := 0.0
	for i := s.SegIndex; i < len(s.Course); i++ {
		seg := &s.Course[i]
		km := seg.DistanceKm
		if i == s.SegIndex && s.SegmentKmLeft > 0 && s.SegmentKmLeft < km {
			km = s.SegmentKmLeft
		}
		var p, m, cda, handling float64
		for _, rs := range line {
			p += EffectivePower(rs.Rider, rs.CurrentFTP(), seg, false) * f
			m += rs.Rider.TotalMassKg()
			cda += rs.Rider.CdA
			handling += float64(rs.Rider.Handling)
		}
		v := segmentSpeed(p/n, m/n, cda/n*rotationFactor(len(line), seg.SurfaceType), seg, int(handling/n))
		t += SegmentTime(v, km)
	}
	return t
}

// easyIntensity: the fraction of threshold a stage-race bunch rides at when a
// break is up the road and no team has a reason to chase it (the race
// leader's team rides tempo; ~41-42 km/h on the flat).
const easyIntensity = 0.70

// powerDurationExp: sustainable power falls as t^-0.1 beyond an hour
// (threshold ~ one-hour power): ~93% for two hours, ~87% for four, before
// the in-race durability fade.
const powerDurationExp = 0.1

// sustainableFraction is the share of threshold sustainable for hours.
func sustainableFraction(hours float64) float64 {
	if hours <= 1 {
		return 1
	}
	return math.Pow(hours, -powerDurationExp)
}

// gcContender reports whether a stage-race rider could turn time gained in a
// break into a GC result: a team leader (best climber or freed co-leader), or
// any rider high on GC (teams watch the top of the standings whatever a
// rider's role), within reach of the race lead, i.e. less far behind than a
// break could gain today. Contenders are marked (FormGroups doesn't let their
// attacks go); a leader further down, like Gall 18 min behind before his 2023
// stage win, may go in the break.
func (s *RaceState) gcContender(rs *RiderState) bool {
	if s == nil || !s.IsStageRace || (rs.TeamRole != RoleLeader && !s.highOnGC(rs)) {
		return false
	}
	lead := math.Inf(1)
	for _, g := range s.GCGaps {
		lead = math.Min(lead, g)
	}
	if lead == math.Inf(1) {
		lead = 0
	}
	return s.GCGaps[rs.Rider.ID]-lead < s.breakReach()
}

// breakReach is the time a break could gain over the rest of the stage if
// let go: an easy bunch against a break riding to stay away (both modelled as
// a pace line of the riders here). Cached per step.
func (s *RaceState) breakReach() float64 {
	if s.reach > 0 || len(s.Course) == 0 || len(s.Road) == 0 {
		return s.reach
	}
	line := topLine(s.Road)
	easy := s.timeToLineAt(line, easyIntensity)
	t := s.timeToLine(line)
	s.reach = math.Max(easy-s.timeToLineAt(line, sustainableFraction(t/3600)), 1)
	return s.reach
}

// breakAppetite: the share of free riders of average aggression (tactical
// awareness 70) who want to be in today's break; scales with (TA/70)^2.
const breakAppetite = 0.25

// setBreakIntent decides, once per stage, who wants to be in the break: free
// riders, i.e. domestiques whose team doesn't need them today (no sprinter
// who can contest the finish, no leader who is a favourite or GC contender)
// and leaders out of contention. They go with moves (FormGroups), so breaks
// form from many riders at once, as real ones do.
func (s *RaceState) setBreakIntent(states []*RiderState, segments []model.Segment, rng *rand.Rand) {
	if s == nil || !s.IsStageRace {
		return
	}
	// The whole field is on the road at the start (breakReach needs it).
	s.Course, s.SegIndex, s.SegmentKmLeft, s.Road = segments, 0, 0, states
	needs := map[string]bool{}
	for _, rs := range states {
		switch {
		case rs.TeamRole == RoleSprinter && s.sprinterCanFollow(rs, states):
			needs[rs.Rider.Team] = true
		case rs.TeamRole == RoleLeader && (s.isDangerous(rs) || s.gcContender(rs)):
			needs[rs.Rider.Team] = true
		}
	}
	s.WantsBreak = map[int64]bool{}
	for _, rs := range states {
		free := !s.isDangerous(rs) && !s.gcContender(rs) &&
			((rs.TeamRole == RoleDomestique && !needs[rs.Rider.Team]) || rs.TeamRole == RoleLeader)
		ta := float64(rs.Rider.TacticalAwareness) / 70
		if free && rng.Float64() < breakAppetite*ta*ta {
			s.WantsBreak[rs.Rider.ID] = true
		}
	}
	s.canFollow, s.fastLines, s.reach, s.Road = nil, nil, 0, nil
}

// gcWatched: teams watch the riders this high on GC (twice the protected
// top-10 places: riders 11-20 can still move into them).
const gcWatched = 2 * gcPlacesProtected

// highOnGC reports whether a rider is among the gcWatched best on GC.
func (s *RaceState) highOnGC(rs *RiderState) bool {
	p := s.gcPlace(rs)
	return p > 0 && p <= gcWatched
}

// gcPlace is a rider's place on GC (0 before any GC or if unclassified).
func (s *RaceState) gcPlace(rs *RiderState) int {
	mine, ok := s.GCGaps[rs.Rider.ID]
	if !ok {
		return 0
	}
	better := 0
	for id, g := range s.GCGaps {
		if g < mine || (g == mine && id < rs.Rider.ID) {
			better++
		}
	}
	return better + 1
}

// leaderGoes reports whether a leader chasing a rival should ride the chase
// themself: their teammates' pace line (at threshold) loses time to the
// rival's group over the rest of the stage, and the leader alone would ride
// to the line faster than the teammates.
func (s *RaceState) leaderGoes(leader *RiderState, mates []*RiderState) bool {
	if s == nil || len(s.Course) == 0 {
		return false
	}
	if len(mates) == 0 {
		return true
	}
	rivals := s.rivalGroup(leader)
	if len(rivals) == 0 {
		return false
	}
	team := s.timeToLineAt(topLine(mates), 1)
	if team <= s.timeToLine(topLine(rivals)) {
		return false // the team is closing
	}
	return s.timeToLineAt([]*RiderState{leader}, 1) < team
}

// rivalGroup is the closest group ahead holding a rival leader (see
// rivalAhead).
func (s *RaceState) rivalGroup(rs *RiderState) []*RiderState {
	best := math.Inf(1)
	for other, lead := range s.Ahead {
		if other.TeamRole == RoleLeader && other.Rider.Team != rs.Rider.Team && lead < best &&
			(!s.IsStageRace || s.GCGaps[other.Rider.ID] < s.GCGaps[rs.Rider.ID]+lead) {
			best = lead
		}
	}
	var out []*RiderState
	for other, lead := range s.Ahead {
		if lead == best && !other.DNF {
			out = append(out, other)
		}
	}
	return out
}

// Stage hunting. Whether a favourite's team rides for the stage is a choice
// made each morning, not a rule: teams let big breaks go on many mountain
// stages, and are keener on the coveted ones (prestige) than once their
// leader has a stage win or a comfortable lead.

// huntOrdinary: the chance a favourite's team hunts an ordinary stage (a
// stage with prestige 1 is always hunted, before wins and comfort).
const huntOrdinary = 0.5

// huntComfortS: a race leader this far clear of the next rider is half as
// keen to hunt (the margin is theirs to keep, not to grow).
const huntComfortS = 180.0

// stageBonuses: time bonuses (seconds) for the first three on a road stage
// of a stage race, as at the Grand Tours and most week-long races.
var stageBonuses = [3]float64{10, 6, 4}

// stageBonus is the GC time bonus for a finishing position (none in a time
// trial).
func stageBonus(position int, rt model.RaceType) float64 {
	if rt == model.RaceTypeTT || position < 1 || position > len(stageBonuses) {
		return 0
	}
	return stageBonuses[position-1]
}

// huntChance is the chance a favourite's team hunts today's stage.
func (s *RaceState) huntChance(leader *RiderState, prestige float64, wins int) float64 {
	p := huntOrdinary + (1-huntOrdinary)*math.Max(0, math.Min(1, prestige))
	p *= math.Pow(0.5, float64(wins)) // each stage won halves the hunger
	if mine, ok := s.GCGaps[leader.Rider.ID]; ok && mine == 0 {
		next := math.Inf(1)
		for id, g := range s.GCGaps {
			if id != leader.Rider.ID {
				next = math.Min(next, g)
			}
		}
		if next < math.Inf(1) {
			p *= huntComfortS / (huntComfortS + next)
		}
	}
	return p
}

// setHunting draws, once before a stage-race stage, which favourites' teams
// ride for the stage win today.
func (s *RaceState) setHunting(states []*RiderState, prestige float64, wins map[int64]int, rng *rand.Rand) {
	if s == nil || !s.IsStageRace {
		return
	}
	s.Hunting = map[string]bool{}
	for _, rs := range states {
		if rs.TeamRole == RoleLeader && s.isDangerous(rs) && !s.Hunting[rs.Rider.Team] {
			s.Hunting[rs.Rider.Team] = rng.Float64() < s.huntChance(rs, prestige, wins[rs.Rider.ID])
		}
	}
}

// tempoDrops reports whether a leader's tempo (the phase intensity, ridden by
// the team at the speed the leader holds on the wheel) drops the finisher on
// the hard stretch ahead: the run of segments from here where the finisher
// needs more than threshold on the wheel, with one W' tank for the whole
// stretch (not a full one per segment). Without a course, this segment alone.
func (s *RaceState) tempoDrops(leader, finisher *RiderState, segment *model.Segment, finisherFTP float64) bool {
	f := finisher.Rider
	intensity := IntensityForPhase(s.Phase)
	needOn := func(seg *model.Segment) (need, thr, v float64) {
		r := leader.Rider
		v = segmentSpeed(EffectivePower(r, leader.CurrentFTP(), seg, false)*intensity, r.TotalMassKg(), r.CdA, seg, r.Handling)
		need = segmentPower(v, f.TotalMassKg(), f.CdA*groupDraft(3, false, seg, v), seg, f.Handling)
		return need, EffectivePower(f, finisher.CurrentFTP(), seg, false), v
	}
	if len(s.Course) == 0 {
		need, _, v := needOn(segment)
		avail := finisher.maxPowerFor(finisherFTP, finisherFTP, SegmentTime(v, segment.DistanceKm))
		return need >= avail*(1-driveMargin)
	}
	w := finisher.WPrime.WPrimeCurrent
	for i := s.SegIndex; i < len(s.Course); i++ {
		seg := &s.Course[i]
		km := seg.DistanceKm
		if i == s.SegIndex && s.SegmentKmLeft > 0 && s.SegmentKmLeft < km {
			km = s.SegmentKmLeft
		}
		need, thr, v := needOn(seg)
		if need <= thr*(1-driveMargin) {
			return false // the stretch ends before the finisher cracks
		}
		w -= (need - thr*(1-driveMargin)) * SegmentTime(v, km)
		if w <= 0 {
			return true
		}
	}
	return false
}

// hunting reports whether a leader's team rides for today's stage: the
// player's ambition order, else the team's morning decision for a
// favourite (setHunting).
func (s *RaceState) hunting(leader *RiderState) bool {
	switch s.teamOrders(leader.Rider.Team).Ambition {
	case OrderYes:
		return true
	case OrderNo:
		return false
	}
	return s.isDangerous(leader) && s.Hunting[leader.Rider.Team]
}
