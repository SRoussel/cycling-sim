package simulation

import (
	"math"
	"sort"
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
	if s == nil || !s.IsStageRace || len(s.Ahead) == 0 || len(s.Course) == 0 {
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
		hunters := map[string]bool{} // favourites' teams that would like the stage
		for _, p := range group {
			if p.DNF {
				continue
			}
			a := math.Inf(1)
			switch p.TeamRole {
			case RoleLeader:
				// GC teams defend against contenders only: one in a move
				// gains nothing. Others may take the stage (or the jersey
				// for a day); they lose it in the mountains.
				for _, r := range brk {
					if r.Rider.Team != p.Rider.Team && s.gcContender(r) {
						a = 0
					}
				}
			case RoleSprinter:
				if s.sprinterCanFollow(p, group) {
					a = -catchMarginS
				}
			}
			if teamIn(brk, p.Rider.Team) {
				continue
			}
			if a < math.Inf(1) {
				teams[p.Rider.Team] = true
				allowed = math.Min(allowed, a)
			} else if p.TeamRole == RoleLeader && s.isDangerous(p) {
				hunters[p.Rider.Team] = true
			}
		}
		f := 0.0
		if len(teams) > 0 && allowed < math.Inf(1) {
			if chasers := chaseLine(group, teams); len(chasers) > 0 {
				f = s.requiredIntensity(chasers, topLine(brk), lead, allowed)
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
// the speed the group's strongest pace line rides at threshold (how hard a
// bunch races over a climb) with their threshold plus the W' they can spend
// there. If not, the finish isn't theirs and their team won't chase for it.
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

func (s *RaceState) sprinterFollows(sp *RiderState, group []*RiderState) bool {
	var others []*RiderState
	for _, rs := range group {
		if rs != sp && !rs.DNF {
			others = append(others, rs)
		}
	}
	if len(others) == 0 {
		return true
	}
	line := topLine(others)
	n := float64(len(line))
	r := sp.Rider
	for i := s.SegIndex; i < len(s.Course); i++ {
		seg := &s.Course[i]
		km := seg.DistanceKm
		if i == s.SegIndex && s.SegmentKmLeft > 0 && s.SegmentKmLeft < km {
			km = s.SegmentKmLeft
		}
		var p, m, cda, handling float64
		for _, rs := range line {
			p += EffectivePower(rs.Rider, rs.CurrentFTP(), seg, false)
			m += rs.Rider.TotalMassKg()
			cda += rs.Rider.CdA
			handling += float64(rs.Rider.Handling)
		}
		v := segmentSpeed(p/n, m/n, cda/n*rotationFactor(len(line), seg.SurfaceType), seg, int(handling/n))
		need := segmentPower(v, r.TotalMassKg(), r.CdA*groupDraft(maxRotation+1, false, seg, v), seg, r.Handling)
		avail := EffectivePower(r, sp.CurrentFTP(), seg, false) + sp.WPrime.AvailablePowerAboveFTP(SegmentTime(v, km))
		if need > avail {
			return false
		}
	}
	return true
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
// break into a GC result: a team leader (best climber or freed co-leader)
// within reach of the race lead, i.e. less far behind than a break could
// gain today. Contenders are marked (FormGroups doesn't let their attacks
// go); a leader further down, like Gall 18 min behind before his 2023 stage
// win, may go in the break.
func (s *RaceState) gcContender(rs *RiderState) bool {
	if s == nil || !s.IsStageRace || rs.TeamRole != RoleLeader {
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
