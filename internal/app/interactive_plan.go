package app

import (
	"cycling_sim/internal/director"
	"cycling_sim/internal/simulation"
	"fmt"
	"strings"
)

// RacePlan is the player's plan, set before the start: who the team rides
// for, whether it goes for the break, where it makes the race hard and where
// its leader attacks. Points are PlanPoint keys.
type RacePlan struct {
	LeaderID int64    `json:"leaderId"` // 0: the team decides
	Break    string   `json:"break"`    // auto, yes, no
	HardOn   []string `json:"hardOn"`
	AttackOn string   `json:"attackOn"`
}

// scheduledOrder sets a team order when the race reaches AtKm (remaining)
// and hands it back to the team ForKm later.
type scheduledOrder struct {
	atKm  float64
	kind  string
	order simulation.Order
	forKm float64
	note  string // logged when it starts
	why   string // shown on the team panel
}

// noteLocked queues a line for the race log ("plan: ... | km=..").
func (s *interactiveSession) noteLocked(text string) {
	s.director.notes = append(s.director.notes, fmt.Sprintf("plan: %s | km=%.1f", text, s.director.km))
}

// DrainNotes returns the race-plan lines since the last call.
func (s *interactiveSession) DrainNotes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.director.notes
	s.director.notes = nil
	return n
}

// SetPlan applies a race plan on the route's plan points: the break order
// now, the hard sections and the attack when the race gets there. The
// planned attack's rider is the chosen leader, or the team's leader once the
// race has assigned roles.
func (s *interactiveSession) SetPlan(plan RacePlan, points []director.PlanPoint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plan = plan
	brk := simulation.OrderAuto // the team decides (and a stage plan resets the last stage's)
	switch plan.Break {
	case "yes":
		brk = simulation.OrderYes
	case "no":
		brk = simulation.OrderNo
	}
	s.setTeamOrderLocked("break", brk)
	s.markOrderLocked("break", brk, "plan", "race plan")
	byKey := map[string]director.PlanPoint{}
	for _, p := range points {
		byKey[p.Key] = p
	}
	for _, k := range plan.HardOn {
		if p, ok := byKey[k]; ok {
			s.director.scheduledOrders = append(s.director.scheduledOrders,
				scheduledOrder{atKm: p.HardFromKm(), kind: "pace", order: simulation.OrderYes, forKm: p.HardForKm(),
					note: fmt.Sprintf("Team to the front for %s, as planned", planName(p)), why: planName(p)})
		}
	}
	if p, ok := byKey[plan.AttackOn]; ok {
		s.director.planAttackKm = p.AttackAtKm()
		s.director.planAttackName = planName(p)
	}
}

// LeaderChoice implements simulation.LeaderChoiceProvider.
func (s *interactiveSession) LeaderChoice(team string) (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if team != s.controlledTeam || s.plan.LeaderID == 0 {
		return 0, false
	}
	return s.plan.LeaderID, true
}

// resolvePlanLocked turns the planned attack into a scheduled command once
// the race view shows who leads the team.
func (s *interactiveSession) resolvePlanLocked(view *simulation.RaceView) {
	d := &s.director
	if d.planAttackKm <= 0 || view == nil {
		return
	}
	rider := s.plan.LeaderID
	if rider == 0 {
		for id, r := range view.Riders {
			if r.Team == s.controlledTeam && r.Role == simulation.RoleLeader && (rider == 0 || id < rider) {
				rider = id
			}
		}
	}
	if rider == 0 {
		return
	}
	d.scheduled = append(d.scheduled, director.Command{RiderID: rider, Action: "attack", Mode: "oneshot", AtKmRemaining: d.planAttackKm,
		Note: fmt.Sprintf("%s attacks on %s, as planned", view.Riders[rider].Name, d.planAttackName), Source: "plan", Why: d.planAttackName})
	d.planAttackKm = 0
}

// tickPlanLocked starts scheduled team orders whose place has come.
func (s *interactiveSession) tickPlanLocked() {
	d := &s.director
	keep := d.scheduledOrders[:0]
	for _, o := range d.scheduledOrders {
		if d.km <= o.atKm {
			if o.note != "" {
				s.noteLocked(o.note)
			}
			s.setTeamOrderLocked(o.kind, o.order)
			s.markOrderLocked(o.kind, o.order, "plan", o.why)
			d.orders = removeOrder(d.orders, o.kind)
			d.orders = append(d.orders, timedOrder{kind: o.kind, untilKm: d.km - o.forKm})
		} else {
			keep = append(keep, o)
		}
	}
	d.scheduledOrders = keep
}

// planName is a plan point's name without its details in brackets.
func planName(p director.PlanPoint) string {
	if i := strings.Index(p.Label, " ("); i > 0 {
		return p.Label[:i]
	}
	return p.Label
}

// StagePlanRequest asks the player to plan a stage before it is raced: the
// stage's decisive climbs and sectors to plan around.
type StagePlanRequest struct {
	Stage  int                  `json:"stage"`
	Name   string               `json:"name"`
	Points []director.PlanPoint `json:"points"`
}

// RequestStagePlan pauses before a stage for the player's plan (once per
// stage, and not when decisions are off). It returns the request to send.
func (s *interactiveSession) RequestStagePlan(stage int, name string, points []director.PlanPoint) *StagePlanRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.director.enabled || s.director.planAsked == stage {
		return nil
	}
	s.director.planAsked = stage
	if points == nil {
		points = []director.PlanPoint{} // JSON [], not null
	}
	s.director.stagePoints = points
	s.paused = true
	return &StagePlanRequest{Stage: stage, Name: name, Points: points}
}

// SetStagePlan applies the player's plan for the coming stage and starts it.
func (s *interactiveSession) SetStagePlan(plan RacePlan) {
	s.mu.Lock()
	points := s.director.stagePoints
	s.mu.Unlock()
	s.SetPlan(plan, points)
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
}
