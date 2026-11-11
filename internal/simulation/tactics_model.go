package simulation

import (
	"math"
	"math/rand"
	"sort"

	"cycling_sim/internal/model"
)

type RacePhase int

const (
	PhaseEarly RacePhase = iota
	PhaseMid
	PhaseFinal
)

type Action int

const (
	ActionHold Action = iota
	ActionAttack
	ActionChase
	ActionConserve
)

// ActionOverrideProvider can inject per-rider action overrides (e.g. from an interactive UI).
// The simulation checks this at segment boundaries.
type ActionOverrideProvider interface {
	// ActionOverride returns a persistent override for a rider.
	ActionOverride(riderID int64) (Action, bool)
	// ConsumeOneShot returns a one-shot override for a rider (and clears it).
	ConsumeOneShot(riderID int64) (Action, bool)
	// EffortMultiplier returns a per-rider effort multiplier (1.0 = neutral).
	EffortMultiplier(riderID int64) (float64, bool)
	// SprintTrainDirective returns a sprint-train tactical action for a rider.
	SprintTrainDirective(riderID int64, riderTeam string, kmRemaining float64) (Action, bool)
	// SprintTrainState returns current sprint-train state for progress payloads.
	SprintTrainState(kmRemaining float64) *TeamState
}

// Order is a player's standing team order on one question: leave it to the
// team (OrderAuto), do it (OrderYes) or don't (OrderNo).
type Order int

const (
	OrderAuto Order = iota
	OrderYes
	OrderNo
)

// TeamOrders are a player's standing orders for their team.
//   - Chase: Yes, the domestiques bring the groups ahead back before the line;
//     No, they sit in and leave it to others (chase.go decides otherwise).
//   - Break: Yes, the team sends riders after the break until one is in it;
//     No, its riders other than leaders don't go in breaks.
//   - Pace: Yes, the domestiques set the pace for the team leader in their
//     group (as a team driving on its own judgement does, drivingTeams);
//     No, they never drive.
//
// A command the player gives a rider directly always wins.
type TeamOrders struct {
	Chase, Break, Pace Order
}

// TeamOrderProvider is optionally implemented by an ActionOverrideProvider
// (the interactive session): team-level orders for one team.
type TeamOrderProvider interface {
	TeamOrders(team string) TeamOrders
}

// teamOrders returns a team's standing orders (all OrderAuto without any).
func (s *RaceState) teamOrders(team string) TeamOrders {
	if s == nil || s.Overrides == nil {
		return TeamOrders{}
	}
	if tp, ok := s.Overrides.(TeamOrderProvider); ok {
		return tp.TeamOrders(team)
	}
	return TeamOrders{}
}

type TeamState struct {
	SprintTrainOrder []int64          `json:"sprintTrainOrder"`
	FollowMap        map[int64]*int64 `json:"followMap"`
	SprintTrainPhase string           `json:"sprintTrainPhase"`
	LaunchingRiderID *int64           `json:"launchingRiderId,omitempty"`
}

type RaceState struct {
	KmRemaining     float64
	Phase           RacePhase
	Segment         *model.Segment
	IsStageRace     bool
	LastActions     map[int64]Action
	HasBreakaway    bool
	BreakawayGap    float64
	BreakawayRiders map[int64]bool // riders in road groups ahead of the main group
	SegmentKmLeft   float64        // distance to the end of the current course segment
	// Ahead lists riders in road groups ahead of the group being decided, with
	// their lead over it in seconds (set per road group each step).
	Ahead map[*RiderState]float64
	// Road is the road group being decided; Chasers the next road group
	// behind it and ChaseGap its deficit in seconds (nil/0 for the last).
	Road     []*RiderState
	Chasers  []*RiderState
	ChaseGap float64
	// Course is the prepared course and SegIndex the current segment, so
	// riders can look ahead to the line (see escape.go).
	Course                []model.Segment
	SegIndex              int
	lineTime              map[lineKey]float64       // per-step cache of timeToLine
	reach                 float64                   // per-step cache of breakReach
	canFollow             map[*RiderState]bool      // per-step cache of sprinterCanFollow
	fastLines             map[fastKey][]*RiderState // per-step cache of fastestLine
	Climb                 float64                   // ClimbFactor of the current step
	BreakawayHasDangerous bool
	SprintChaseActive     bool
	BreakawayActive       bool
	LastSprintChase       bool
	TeamContexts          map[string]*TeamContext
	Rival                 RivalAssessment
	Events                []TacticsEvent
	Overrides             ActionOverrideProvider
	// GCGaps maps rider ID → cumulative GC deficit in seconds vs. leader (0 = race leader).
	// Populated at the start of each stage race stage. Nil for one-day races.
	GCGaps map[int64]float64
	// WantsBreak: riders who want to be in today's break (setBreakIntent).
	WantsBreak map[int64]bool
	// Hunting: teams riding for today's stage win (stage races; setHunting).
	Hunting map[string]bool
	// Clusters are the road groups of the last step, front first.
	Clusters [][]*RiderState
	// Favourites are the riders the bunch marks (see markFavourites).
	Favourites map[int64]bool
}

type TacticsParams struct {
	ChasePowerBoost      float64
	ConservePowerPenalty float64
	AttackAggression     float64
	ConserveWPrimeThresh float64
}

func DefaultTacticsParams() TacticsParams {
	return TacticsParams{
		ChasePowerBoost:      1.03,
		ConservePowerPenalty: 0.97,
		AttackAggression:     1.1,
		ConserveWPrimeThresh: 0.18,
	}
}

type TeamStrategy struct {
	AggressionBias float64
	RiskTolerance  float64
	EnergyBudget   float64
}

type TeamContext struct {
	Team        string
	LeaderID    int64
	SprinterID  int64
	Domestiques []int64
	Strategy    TeamStrategy
}

type RivalAssessment struct {
	SprintTeamsChasing bool
}

type TacticsEvent struct {
	Type   string
	Km     float64
	Reason string
}

func NewRaceState(states []*RiderState, rng *rand.Rand, isStageRace bool) *RaceState {
	contexts, rival := BuildTeamContexts(states)
	return &RaceState{
		TeamContexts: contexts,
		Rival:        rival,
		IsStageRace:  isStageRace,
		LastActions:  make(map[int64]Action),
	}
}

func (rs *RaceState) LogEvent(eventType string, reason string) {
	rs.Events = append(rs.Events, TacticsEvent{Type: eventType, Km: rs.KmRemaining, Reason: reason})
}

// logCondition logs a race-condition event once while it persists across
// simulation steps (e.g. "technical sector").
func (rs *RaceState) logCondition(eventType string, reason string) {
	if n := len(rs.Events); n > 0 && rs.Events[n-1].Type == eventType && rs.Events[n-1].Reason == reason {
		return
	}
	rs.LogEvent(eventType, reason)
}

// decisionRefKm is the distance over which tactical probabilities apply:
// a probability p means "chance per 5 km", independent of how the course is
// cut into segments or steps.
const decisionRefKm = 5.0

// ScaleToDistance converts a per-decisionRefKm probability to one for km.
func ScaleToDistance(p, km float64) float64 {
	if p <= 0 || km <= 0 {
		return 0
	}
	if p >= 1 {
		return 1
	}
	return 1 - math.Pow(1-p, km/decisionRefKm)
}

func perStep(p float64, state *RaceState) float64 {
	if state == nil || state.Segment == nil {
		return p
	}
	return ScaleToDistance(p, state.Segment.DistanceKm)
}

func PhaseFromKm(remainingKm float64) RacePhase {
	switch {
	case remainingKm < 20:
		return PhaseFinal
	case remainingKm < 60:
		return PhaseMid
	default:
		return PhaseEarly
	}
}

func DecideAction(rs *RiderState, state *RaceState, hasLeader bool, rng *rand.Rand, params TacticsParams) Action {
	if rs.DNF {
		return ActionConserve
	}
	localParams := params
	if state != nil && state.TeamContexts != nil {
		if ctx, ok := state.TeamContexts[rs.Rider.Team]; ok {
			localParams.AttackAggression *= ctx.Strategy.AggressionBias
			localParams.ConserveWPrimeThresh *= 1.0 + (1.0-ctx.Strategy.EnergyBudget)*0.25
		}
	}

	wFrac := rs.WPrime.FractionRemaining()
	if wFrac < localParams.ConserveWPrimeThresh {
		return ActionConserve
	}
	// Leaders go when the race (or time on GC) can be won from here (escape.go).
	if state != nil && state.Segment != nil && state.attackPays(rs, state.Segment) {
		return ActionAttack
	}
	// One-day races: a favourite who gains from a hard race presses on hard
	// sections (escape.go), surging to drain rivals' W'.
	if state != nil && state.Segment != nil && state.pressPays(rs, state.Segment) {
		return ActionAttack
	}

	// ------------------------------------------------------------------
	// Stage-race GC leader on flat/rolling terrain: conserve energy.
	// This block only fires when gradient < 3.5 (never on real mountains).
	// ------------------------------------------------------------------
	if state != nil && state.IsStageRace && rs.TeamRole == RoleLeader && state.Segment != nil {
		if state.Segment.SurfaceType != model.SurfaceCobbles && climbOf(state.Segment) < terrainRolling {
			if !state.HasBreakaway || (!state.BreakawayHasDangerous && state.Phase != PhaseFinal) {
				if state.Phase != PhaseFinal || rng.Float64() < 0.85 {
					return ActionConserve
				}
			}
		}
	}

	// ------------------------------------------------------------------
	// Phase and objective-based decisions (sprinters, team strategy).
	// ------------------------------------------------------------------
	if state != nil {
		if state.Phase == PhaseFinal && rs.TeamRole == RoleSprinter {
			return ActionChase
		}
		// Breaks are chased to a plan (chase.go, applied in FormGroups): teams
		// with an interest chase at the pace that brings the break back in
		// time, not by rule. One-day races: once the race is on, the race is
		// the result, so every group behind a move races for placings.
		if !state.IsStageRace && state.HasBreakaway && state.Phase != PhaseEarly && !state.BreakawayRiders[rs.Rider.ID] {
			return ActionChase
		}
		// One-day races: a rider dropped behind the main field chases to get
		// back into the race (in a stage race the gruppetto rides to the cut).
		if !state.IsStageRace && state.behindMainField() {
			return ActionChase
		}
		// What a team rides for follows from its roster, not a random
		// objective: a team with a sprinter drives the final for the sprint;
		// a leader saves themselves on flat and rolling roads until the final.
		if ctx := state.TeamContexts[rs.Rider.Team]; ctx != nil && ctx.SprinterID != 0 && state.Phase == PhaseFinal && !state.IsStageRace {
			return ActionChase
		}
		if rs.TeamRole == RoleLeader {
			if state.Segment != nil && state.Segment.SurfaceType != model.SurfaceCobbles && climbOf(state.Segment) < terrainRolling && state.Phase != PhaseFinal {
				return ActionConserve
			}
		}
	}

	// ------------------------------------------------------------------
	// Generic breakaway attempt (all riders, via tactical awareness roll).
	// ------------------------------------------------------------------
	// Favourites and GC contenders don't take the opportunistic roll: their
	// attacks are deliberate (attackPays above), and a marked rider's move
	// makes the whole bunch respond.
	if state != nil && state.Segment != nil && !state.isDangerous(rs) && !state.gcContender(rs) {
		if BreakawayAttemptWithParams(rs, state.Segment, state.KmRemaining, rng, hasLeader, localParams) {
			return ActionAttack
		}
	}

	return ActionHold
}

func TacticsPowerMultiplier(action Action, params TacticsParams) float64 {
	switch action {
	case ActionAttack:
		return 1.0 // attacks ride at full threshold; the extra comes from W'
	case ActionChase:
		return params.ChasePowerBoost
	case ActionConserve:
		return params.ConservePowerPenalty
	default:
		return 1.0
	}
}

func ApplyEventModifiers(params TacticsParams, state *RaceState) TacticsParams {
	if state == nil || state.Segment == nil {
		return params
	}
	segment := state.Segment
	if segment.SurfaceType == model.SurfaceCobbles || segment.IsTechnical {
		params.AttackAggression *= 1.15
		state.logCondition("cobbles", "technical sector")
	}
	if climbOf(segment) > terrainClimb && state.KmRemaining < 15 {
		params.AttackAggression *= 1.1
		state.logCondition("summit", "climb near finish")
	}
	if state.KmRemaining < 10 {
		state.logCondition("final", "final 10 km")
	}
	return params
}

func BuildTeamContexts(states []*RiderState) (map[string]*TeamContext, RivalAssessment) {
	contexts := make(map[string]*TeamContext)
	teams := make(map[string][]*RiderState)
	for _, rs := range states {
		teams[rs.Rider.Team] = append(teams[rs.Rider.Team], rs)
	}

	// Sorted for a deterministic order (Go maps iterate in random order).
	teamNames := make([]string, 0, len(teams))
	for team := range teams {
		teamNames = append(teamNames, team)
	}
	sort.Strings(teamNames)

	for _, team := range teamNames {
		members := teams[team]
		ctx := &TeamContext{Team: team}
		for _, rs := range members {
			switch rs.TeamRole {
			case RoleLeader:
				ctx.LeaderID = rs.Rider.ID
			case RoleSprinter:
				ctx.SprinterID = rs.Rider.ID
			default:
				ctx.Domestiques = append(ctx.Domestiques, rs.Rider.ID)
			}
		}

		ctx.Strategy = TeamStrategy{
			AggressionBias: clamp(0.85+avgAbility(members, func(r *RiderState) int { return r.Rider.TacticalAwareness })*0.003, 0.75, 1.25),
			RiskTolerance:  clamp(0.8+avgAbility(members, func(r *RiderState) int { return r.Rider.Handling })*0.002, 0.7, 1.2),
			EnergyBudget:   clamp(0.7+avgAbility(members, func(r *RiderState) int { return r.Rider.Endurance })*0.003, 0.6, 1.1),
		}
		contexts[team] = ctx
	}

	rival := BuildRivalAssessment(states)
	return contexts, rival
}

// BuildRivalAssessment notes whether sprint teams are present to chase. Who
// is dangerous is a role, not a ranking: team leaders (see isDangerous).
func BuildRivalAssessment(states []*RiderState) RivalAssessment {
	sprintTeams := false
	for _, rs := range states {
		if rs.TeamRole == RoleSprinter || rs.Rider.SprintAbility >= 88 {
			sprintTeams = true
			break
		}
	}

	return RivalAssessment{SprintTeamsChasing: sprintTeams}
}

func avgAbility(members []*RiderState, getter func(*RiderState) int) float64 {
	if len(members) == 0 {
		return 70
	}
	sum := 0
	for _, r := range members {
		sum += getter(r)
	}
	return float64(sum) / float64(len(members))
}

func clamp(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// rivalAhead reports whether a rider has a rival up the road and so must
// race instead of sitting in (they chase to the line and take turns):
//   - stage race: a leader, with a leader ahead who would pass them on GC if
//     the gap held;
//   - one-day race, after the early phase: any rider (sprinters excepted)
//     with another team's leader ahead and no leader of their own team up the
//     road, since placings are decided now. Riders don't chase their own
//     leader.
func (s *RaceState) rivalAhead(rs *RiderState) bool {
	if s == nil || len(s.Ahead) == 0 || rs.TeamRole == RoleSprinter {
		return false
	}
	if s.IsStageRace {
		// Only a GC contender races for GC places; a leader out of contention
		// lets a rival far down on GC go.
		if rs.TeamRole != RoleLeader || !s.gcContender(rs) {
			return false
		}
		mine := s.GCGaps[rs.Rider.ID]
		for other, lead := range s.Ahead {
			if other.TeamRole == RoleLeader && other.Rider.Team != rs.Rider.Team && s.GCGaps[other.Rider.ID] < mine+lead {
				return true
			}
		}
		return false
	}
	if s.Phase == PhaseEarly {
		return false
	}
	rival := false
	for other := range s.Ahead {
		if other.TeamRole != RoleLeader {
			continue
		}
		if other.Rider.Team == rs.Rider.Team {
			return false // own leader up the road: don't chase them
		}
		rival = true
	}
	return rival
}

// favouriteCount: how many riders the bunch marks as favourites. A move or
// break containing one is followed or chased at once; others are let go.
// (Marking every team leader instead made the bunch ride every attack and
// shattered races: gap sum 3849 s vs 2413 s.)
const favouriteCount = 5

// markFavourites ranks the riders by time over this course at threshold (the
// physics behind AssignCourseLeaders) and marks the fastest as favourites.
func (s *RaceState) markFavourites(states []*RiderState, segments []model.Segment) {
	type ranked struct {
		id int64
		t  float64
	}
	var all []ranked
	for _, rs := range states {
		if !rs.DNF {
			all = append(all, ranked{rs.Rider.ID, courseTime(rs, segments)})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].t < all[j].t })
	s.Favourites = map[int64]bool{}
	for i := 0; i < len(all) && i < favouriteCount; i++ {
		s.Favourites[all[i].id] = true
	}
}

// isDangerous reports whether a rider is one of the race favourites.
func (s *RaceState) isDangerous(rs *RiderState) bool {
	return s != nil && s.Favourites[rs.Rider.ID]
}

// behindMainField reports whether the road group being decided (s.Road) is
// behind the main field: the largest road group is further up the road.
func (s *RaceState) behindMainField() bool {
	if s == nil || len(s.Road) == 0 || len(s.Clusters) < 2 {
		return false
	}
	main, mine := -1, -1
	for i, c := range s.Clusters {
		if main < 0 || len(c) > len(s.Clusters[main]) {
			main = i
		}
		if len(c) > 0 && c[0] == s.Road[0] {
			mine = i
		}
	}
	return mine > main
}
