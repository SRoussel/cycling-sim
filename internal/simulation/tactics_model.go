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

type TeamState struct {
	SprintTrainOrder []int64          `json:"sprintTrainOrder"`
	FollowMap        map[int64]*int64 `json:"followMap"`
	SprintTrainPhase string           `json:"sprintTrainPhase"`
	LaunchingRiderID *int64           `json:"launchingRiderId,omitempty"`
}

type RaceState struct {
	KmRemaining           float64
	Phase                 RacePhase
	Segment               *model.Segment
	IsStageRace           bool
	LastActions           map[int64]Action
	HasBreakaway          bool
	BreakawayGap          float64
	BreakawayHasDangerous bool
	SprintChaseActive     bool
	BreakawayActive       bool
	LastSprintChase       bool
	TeamContexts          map[string]*TeamContext
	Rival                 RivalAssessment
	Events                []TacticsEvent
	Overrides             ActionOverrideProvider
}

type TacticsParams struct {
	AttackPowerBoost        float64
	ChasePowerBoost         float64
	ConservePowerPenalty    float64
	AttackAggression        float64
	ConserveWPrimeThresh    float64
	BreakawayCohesionWeight float64
	BreakawaySuccessBias    float64
	GCChaseAggression       float64
	SprintTrainEfficiency   float64
}

func DefaultTacticsParams() TacticsParams {
	return TacticsParams{
		AttackPowerBoost:        1.05,
		ChasePowerBoost:         1.03,
		ConservePowerPenalty:    0.97,
		AttackAggression:        1.1,
		ConserveWPrimeThresh:    0.18,
		BreakawayCohesionWeight: 0.35,
		BreakawaySuccessBias:    1.08,
		GCChaseAggression:       1.15,
		SprintTrainEfficiency:   1.1,
	}
}

type TeamObjective int

const (
	ObjectiveStageWin TeamObjective = iota
	ObjectiveGCProtection
	ObjectiveBreakawayHunt
	ObjectiveSprintWin
)

type TeamStrategy struct {
	PrimaryObjective TeamObjective
	AggressionBias   float64
	RiskTolerance    float64
	EnergyBudget     float64
}

type TeamContext struct {
	Team        string
	LeaderID    int64
	SprinterID  int64
	Domestiques []int64
	Strategy    TeamStrategy
}

type RivalAssessment struct {
	DangerousRiders    map[int64]bool
	SprintTeamsChasing bool
}

type TacticsEvent struct {
	Type   string
	Km     float64
	Reason string
}

func NewRaceState(states []*RiderState, rng *rand.Rand, isStageRace bool) *RaceState {
	contexts, rival := BuildTeamContexts(states, rng, isStageRace)
	return &RaceState{
		TeamContexts: contexts,
		Rival:        rival,
		IsStageRace:  isStageRace,
		LastActions:  make(map[int64]Action),
	}
}

func (rs *RaceState) LogEvent(eventType string, reason string) {
	if rs == nil {
		return
	}
	rs.Events = append(rs.Events, TacticsEvent{Type: eventType, Km: rs.KmRemaining, Reason: reason})
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
	var strategy *TeamStrategy
	if state != nil && state.TeamContexts != nil {
		if ctx, ok := state.TeamContexts[rs.Rider.Team]; ok {
			strategy = &ctx.Strategy
			localParams.AttackAggression *= ctx.Strategy.AggressionBias
			localParams.ConserveWPrimeThresh *= 1.0 + (1.0-ctx.Strategy.EnergyBudget)*0.25
		}
	}

	wFrac := rs.WPrime.FractionRemaining()
	if wFrac < localParams.ConserveWPrimeThresh {
		return ActionConserve
	}

	// Stage-race GC leaders should rarely attack on flat/rolling terrain.
	if state != nil && state.IsStageRace && rs.TeamRole == RoleLeader && state.Segment != nil {
		if state.Segment.SurfaceType != model.SurfaceCobbles && state.Segment.GradientPct < 3.5 {
			// If there's no dangerous move, conserve most of the day.
			if !state.HasBreakaway || (!state.BreakawayHasDangerous && state.Phase != PhaseFinal) {
				if state.Phase != PhaseFinal || rng.Float64() < 0.85 {
					return ActionConserve
				}
			}
		}
	}

	if state != nil {
		if state.Phase == PhaseFinal && rs.TeamRole == RoleSprinter {
			return ActionChase
		}
		if state.HasBreakaway && (state.BreakawayGap > 20 || state.BreakawayHasDangerous) && state.Phase != PhaseEarly {
			return ActionChase
		}
		if strategy != nil {
			switch strategy.PrimaryObjective {
			case ObjectiveSprintWin:
				if state.Phase == PhaseFinal {
					return ActionChase
				}
			case ObjectiveGCProtection:
				if rs.TeamRole == RoleLeader {
					if state.Segment != nil && state.Segment.SurfaceType != model.SurfaceCobbles && state.Segment.GradientPct < 3.5 && state.Phase != PhaseFinal {
						return ActionConserve
					}
					if state.HasBreakaway && (state.BreakawayGap > 25 || state.BreakawayHasDangerous) && state.Phase != PhaseEarly {
						return ActionChase
					}
				}
			case ObjectiveBreakawayHunt:
				if state.Phase == PhaseEarly && rng.Float64() < 0.05 {
					return ActionAttack
				}
			}
		}
	}

	if state != nil && state.Segment != nil && state.Segment.GradientPct > 4.0 && rs.TeamRole == RoleLeader {
		if state.IsStageRace {
			// In stage races, top GC favorites generally don't attack every climb for the stage.
			isFavorite := state.Rival.DangerousRiders[rs.Rider.ID]
			if isFavorite {
				// Only attack late, on steep terrain, and only sometimes.
				if state.KmRemaining < 8 && state.Segment.GradientPct > 6.0 && wFrac > 0.45 && rng.Float64() < 0.03 {
					return ActionAttack
				}
			} else {
				// Non-favorite leaders are more likely to hunt stages in the mountains.
				if state.KmRemaining < 18 && wFrac > 0.35 && rng.Float64() < 0.10 {
					return ActionAttack
				}
			}
		} else {
			if rng.Float64() < 0.08 && wFrac > 0.35 {
				return ActionAttack
			}
		}
	}

	if state != nil && state.Segment != nil {
		if BreakawayAttemptWithParams(rs, state.Segment, state.KmRemaining, rng, hasLeader, localParams) {
			return ActionAttack
		}
	}

	return ActionHold
}

func TacticsPowerMultiplier(action Action, params TacticsParams) float64 {
	switch action {
	case ActionAttack:
		return params.AttackPowerBoost
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
		params.AttackPowerBoost *= 1.02
		state.LogEvent("cobbles", "technical sector")
	}
	if segment.GradientPct > 4.0 && state.KmRemaining < 15 {
		params.AttackAggression *= 1.1
		params.AttackPowerBoost *= 1.02
		state.LogEvent("summit", "climb near finish")
	}
	if state.KmRemaining < 10 {
		params.ChasePowerBoost *= 1.05
		state.LogEvent("final", "final 10 km")
	}
	return params
}

func BuildTeamContexts(states []*RiderState, rng *rand.Rand, isStageRace bool) (map[string]*TeamContext, RivalAssessment) {
	contexts := make(map[string]*TeamContext)
	teams := make(map[string][]*RiderState)
	for _, rs := range states {
		teams[rs.Rider.Team] = append(teams[rs.Rider.Team], rs)
	}

	for team, members := range teams {
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
			PrimaryObjective: pickObjective(ctx, members, rng, isStageRace),
			AggressionBias:   clamp(0.85+avgAbility(members, func(r *RiderState) int { return r.Rider.TacticalAwareness })*0.003, 0.75, 1.25),
			RiskTolerance:    clamp(0.8+avgAbility(members, func(r *RiderState) int { return r.Rider.Handling })*0.002, 0.7, 1.2),
			EnergyBudget:     clamp(0.7+avgAbility(members, func(r *RiderState) int { return r.Rider.Endurance })*0.003, 0.6, 1.1),
		}
		contexts[team] = ctx
	}

	rival := BuildRivalAssessment(states)
	return contexts, rival
}

func BuildRivalAssessment(states []*RiderState) RivalAssessment {
	gcCandidates := make([]*RiderState, 0, len(states))
	for _, rs := range states {
		gcCandidates = append(gcCandidates, rs)
	}
	sort.Slice(gcCandidates, func(i, j int) bool {
		gi := gcScore(gcCandidates[i].Rider)
		gj := gcScore(gcCandidates[j].Rider)
		return gi > gj
	})

	dangerous := make(map[int64]bool)
	limit := 5
	if len(gcCandidates) < limit {
		limit = len(gcCandidates)
	}
	for i := 0; i < limit; i++ {
		dangerous[gcCandidates[i].Rider.ID] = true
	}

	sprintTeams := false
	for _, rs := range states {
		if rs.TeamRole == RoleSprinter || rs.Rider.SprintAbility >= 88 {
			sprintTeams = true
			break
		}
	}

	return RivalAssessment{
		DangerousRiders:    dangerous,
		SprintTeamsChasing: sprintTeams,
	}
}

func pickObjective(ctx *TeamContext, members []*RiderState, rng *rand.Rand, isStageRace bool) TeamObjective {
	if !isStageRace {
		if ctx.SprinterID != 0 {
			if rng.Float64() < 0.7 {
				return ObjectiveSprintWin
			}
		}
		if ctx.LeaderID != 0 {
			if rng.Float64() < 0.6 {
				return ObjectiveGCProtection
			}
		}
		if rng.Float64() < 0.5 {
			return ObjectiveBreakawayHunt
		}
		return ObjectiveStageWin
	}

	// Stage races: bias toward sustainable objectives.
	if ctx.SprinterID != 0 && ctx.LeaderID != 0 {
		// Teams with both leaders often protect GC but still target sprints.
		if rng.Float64() < 0.55 {
			return ObjectiveSprintWin
		}
		return ObjectiveGCProtection
	}
	if ctx.SprinterID != 0 {
		if rng.Float64() < 0.8 {
			return ObjectiveSprintWin
		}
		return ObjectiveStageWin
	}
	if ctx.LeaderID != 0 {
		if rng.Float64() < 0.85 {
			return ObjectiveGCProtection
		}
		return ObjectiveStageWin
	}
	if rng.Float64() < 0.65 {
		return ObjectiveBreakawayHunt
	}
	return ObjectiveStageWin
}

func gcScore(r *model.Rider) float64 {
	return float64(r.ClimbingAbility+r.TTAbility+r.Endurance) / 3.0
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

func BreakawayCohesion(riders []*RiderState) float64 {
	if len(riders) == 0 {
		return 0.5
	}
	sum := 0.0
	for _, rs := range riders {
		sum += float64(rs.Rider.TacticalAwareness)
	}
	avg := sum / float64(len(riders))
	cohesion := 0.5 + (avg-70.0)/120.0
	return math.Max(0.2, math.Min(0.9, cohesion))
}
