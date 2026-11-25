package app

import (
	"cycling_sim/internal/simulation"
)

// The live state of the player's team as the race applies it: every team
// order and rider command, who set it (you, the plan or a decision) and how
// long it has left. The team panel shows this, so it never goes stale when
// the plan or a decision changes something.

// LiveOrder is one team order now.
type LiveOrder struct {
	Value  string  `json:"value"`            // auto, yes, no
	Source string  `json:"source,omitempty"` // you, plan, decision ("" with auto: the team decides)
	Note   string  `json:"note,omitempty"`   // why (the plan point, the decision)
	KmLeft float64 `json:"kmLeft,omitempty"` // until it hands back to the team (0: for good)
}

// LiveRider is a rider's standing command now (riders without one race on
// their own judgement).
type LiveRider struct {
	Action string  `json:"action"` // attack, chase, conserve
	Source string  `json:"source"`
	Note   string  `json:"note,omitempty"`
	KmLeft float64 `json:"kmLeft,omitempty"`
}

// LiveTeam is the player's team's orders and rider commands.
type LiveTeam struct {
	Orders map[string]LiveOrder `json:"orders"`
	Riders map[int64]LiveRider  `json:"riders"`
}

type setBy struct{ source, note string }

func orderValue(o simulation.Order) string {
	switch o {
	case simulation.OrderYes:
		return "yes"
	case simulation.OrderNo:
		return "no"
	}
	return "auto"
}

func actionName(a simulation.Action) string {
	switch a {
	case simulation.ActionAttack:
		return "attack"
	case simulation.ActionChase:
		return "chase"
	case simulation.ActionConserve:
		return "conserve"
	}
	return "hold"
}

// markOrderLocked records who set a team order (cleared when it's auto).
func (s *interactiveSession) markOrderLocked(kind string, order simulation.Order, source, note string) {
	if s.orderSrc == nil {
		s.orderSrc = map[string]setBy{}
	}
	if order == simulation.OrderAuto {
		delete(s.orderSrc, kind)
		return
	}
	s.orderSrc[kind] = setBy{source, note}
}

// markRiderLocked records who gave a rider his standing command.
func (s *interactiveSession) markRiderLocked(id int64, source, note string) {
	if s.riderSrc == nil {
		s.riderSrc = map[int64]setBy{}
	}
	if _, ok := s.overrides[id]; !ok {
		delete(s.riderSrc, id)
		return
	}
	s.riderSrc[id] = setBy{source, note}
}

// Live reports the team's orders and rider commands now.
func (s *interactiveSession) Live() *LiveTeam {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &s.director
	o := s.teamOrders
	live := &LiveTeam{Orders: map[string]LiveOrder{}, Riders: map[int64]LiveRider{}}
	for kind, order := range map[string]simulation.Order{
		"chase": o.Chase, "break": o.Break, "pace": o.Pace, "protect": o.Protect, "leadout": o.Leadout, "ambition": o.Ambition,
	} {
		lo := LiveOrder{Value: orderValue(order)}
		if src, ok := s.orderSrc[kind]; ok && order != simulation.OrderAuto {
			lo.Source, lo.Note = src.source, src.note
		}
		for _, t := range d.orders {
			if t.kind == kind && order != simulation.OrderAuto {
				lo.KmLeft = d.km - t.untilKm
			}
		}
		live.Orders[kind] = lo
	}
	for id, a := range s.overrides {
		lr := LiveRider{Action: actionName(a), Source: "you"}
		if src, ok := s.riderSrc[id]; ok {
			lr.Source, lr.Note = src.source, src.note
		}
		for _, t := range d.timed {
			if t.cmd.RiderID == id {
				lr.KmLeft = d.km - t.untilKm
			}
		}
		live.Riders[id] = lr
	}
	return live
}
