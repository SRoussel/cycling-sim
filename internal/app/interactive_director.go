package app

import (
	"fmt"

	"cycling_sim/internal/director"
	"cycling_sim/internal/simulation"
)

// Decision moments for an interactive session: the director watches the
// race for the player's team, pauses at moments that call for a decision and
// applies the chosen option as rider commands and team orders, some of them
// timed ("for 5 km") or scheduled ("attack near the top").

type timedCommand struct {
	cmd     director.Command
	untilKm float64 // km remaining at which a persistent command ends
}

type timedOrder struct {
	kind    string
	untilKm float64
}

// directorState is the session's director bookkeeping (guarded by s.mu).
type directorState struct {
	dir       *director.Director
	enabled   bool
	keyOnly   bool
	pending   *director.Moment
	stage     int
	km        float64 // km remaining in the race (stage), last seen
	scheduled []director.Command
	timed     []timedCommand
	orders    []timedOrder
	// the race plan's hard sections and attack (see interactive_plan.go)
	scheduledOrders []scheduledOrder
	planAttackKm    float64
	planAttackName  string
	notes           []string // race-plan lines for the log
	planAsked       int      // the stage whose plan was asked for
	stagePoints     []director.PlanPoint
}

// SetDecisionLevel sets how often the race stops for a decision: "off",
// "key" (the moments that decide a race) or "all".
func (s *interactiveSession) SetDecisionLevel(level string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.director.enabled = level != "off"
	s.director.keyOnly = level == "key"
	if s.director.dir != nil {
		s.director.dir.KeyOnly = s.director.keyOnly
	}
}

// Observe is called with every progress frame (before it is emitted): it
// fires scheduled commands, ends timed ones, and on the true end-of-step
// frames asks the director for a moment. A new moment pauses the session and
// is returned to be sent with the frame.
func (s *interactiveSession) Observe(view *simulation.RaceView, profile *simulation.RouteProfile, kmRemaining float64, stage int) *director.Moment {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &s.director
	if d.dir == nil {
		d.dir = director.New(s.controlledTeam)
		d.dir.KeyOnly = d.keyOnly
	}
	if stage != d.stage || profile != nil {
		if stage != d.stage {
			d.scheduled, d.timed, d.orders, d.pending = nil, nil, nil, nil
			// The last stage's plan is done: a new one is asked for.
			d.scheduledOrders, d.planAttackKm = nil, 0
			s.plan = RacePlan{}
		}
		d.stage = stage
		if profile != nil {
			d.dir.SetRoute(profile)
		}
	}
	d.km = kmRemaining
	s.resolvePlanLocked(view)
	s.tickPlanLocked()
	s.tickLocked()
	d.dir.Watch(view)
	if view == nil || !d.enabled || d.pending != nil {
		return nil
	}
	m := d.dir.Next(view)
	if m != nil {
		d.pending = m
		s.paused = true
	}
	return m
}

// tickLocked fires scheduled commands and ends timed commands and orders.
func (s *interactiveSession) tickLocked() {
	d := &s.director
	keep := d.scheduled[:0]
	for _, c := range d.scheduled {
		if d.km <= c.AtKmRemaining {
			if c.Note != "" {
				s.noteLocked(c.Note)
			}
			c.AtKmRemaining = 0
			s.applyCommandLocked(c)
		} else {
			keep = append(keep, c)
		}
	}
	d.scheduled = keep
	timed := d.timed[:0]
	for _, t := range d.timed {
		if d.km <= t.untilKm {
			if s.overrides[t.cmd.RiderID] == actionOf(t.cmd.Action) {
				delete(s.overrides, t.cmd.RiderID) // back to the rider's judgement
				s.markRiderLocked(t.cmd.RiderID, "", "")
			}
		} else {
			timed = append(timed, t)
		}
	}
	d.timed = timed
	orders := d.orders[:0]
	for _, o := range d.orders {
		if d.km <= o.untilKm {
			s.setTeamOrderLocked(o.kind, simulation.OrderAuto)
			s.markOrderLocked(o.kind, simulation.OrderAuto, "", "")
		} else {
			orders = append(orders, o)
		}
	}
	d.orders = orders
}

func actionOf(name string) simulation.Action {
	switch name {
	case "attack":
		return simulation.ActionAttack
	case "chase":
		return simulation.ActionChase
	case "conserve":
		return simulation.ActionConserve
	}
	return simulation.ActionHold
}

func orderOf(value string) simulation.Order {
	switch value {
	case "yes":
		return simulation.OrderYes
	case "no":
		return simulation.OrderNo
	}
	return simulation.OrderAuto
}

func (s *interactiveSession) applyCommandLocked(c director.Command) {
	d := &s.director
	if c.AtKmRemaining > 0 && d.km > c.AtKmRemaining {
		d.scheduled = append(d.scheduled, c)
		return
	}
	act := actionOf(c.Action)
	switch {
	case act == simulation.ActionHold:
		delete(s.overrides, c.RiderID)
		s.markRiderLocked(c.RiderID, "", "")
	case c.Mode == "oneshot":
		s.oneShot[c.RiderID] = act
	default:
		s.overrides[c.RiderID] = act
		d.timed = dropTimed(d.timed, c.RiderID)
		if c.ForKm > 0 {
			d.timed = append(d.timed, timedCommand{cmd: c, untilKm: d.km - c.ForKm})
		}
		s.markRiderLocked(c.RiderID, c.Source, c.Why)
	}
}

// Goal returns today's goal and whether a rider of ours made the break.
func (s *interactiveSession) Goal() (*director.Goal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.director.dir == nil {
		return nil, false
	}
	return s.director.dir.Goal(), s.director.dir.BreakMade()
}

// GCGoal returns a stage race's overall goal.
func (s *interactiveSession) GCGoal() *director.Goal {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.director.dir == nil {
		return nil
	}
	return s.director.dir.GCGoal()
}

// PendingMoment returns the moment waiting for the player, if any.
func (s *interactiveSession) PendingMoment() *director.Moment {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.director.pending
}

// Resolve applies the player's choice for the pending moment and resumes the
// race. It returns the chosen option's label ("" if nothing was pending).
func (s *interactiveSession) Resolve(momentID, optionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &s.director
	if d.pending == nil || d.pending.ID != momentID {
		return "", fmt.Errorf("no pending moment %q", momentID)
	}
	opt, ok := d.pending.Option(optionID)
	if !ok {
		return "", fmt.Errorf("moment %s has no option %q", momentID, optionID)
	}
	why := momentWhy(d.pending.Title)
	for _, c := range opt.Commands {
		c.Source, c.Why = "decision", why
		s.applyCommandLocked(c)
	}
	for _, o := range opt.Orders {
		s.setTeamOrderLocked(o.Kind, orderOf(o.Value))
		s.markOrderLocked(o.Kind, orderOf(o.Value), "decision", why)
		d.orders = removeOrder(d.orders, o.Kind)
		if o.ForKm > 0 {
			d.orders = append(d.orders, timedOrder{kind: o.Kind, untilKm: d.km - o.ForKm})
		}
	}
	d.pending = nil
	s.paused = false
	return opt.Label, nil
}

func removeOrder(orders []timedOrder, kind string) []timedOrder {
	out := orders[:0]
	for _, o := range orders {
		if o.kind != kind {
			out = append(out, o)
		}
	}
	return out
}

// momentWhy is a moment's title without its icon ("Pogačar attacks!").
func momentWhy(title string) string {
	for i, r := range title {
		if r == ' ' {
			return title[i+1:]
		}
	}
	return title
}
