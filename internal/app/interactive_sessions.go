package app

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"cycling_sim/internal/simulation"
)

type interactiveSession struct {
	id             string
	controlledTeam string

	mu               sync.Mutex
	overrides        map[int64]simulation.Action
	oneShot          map[int64]simulation.Action
	efforts          map[int64]float64
	styles           map[int64]riderStyle // the player's dig and work pace per rider
	trainSprinter    int64                // who the lead-out rides for (0: the fastest finisher in the group)
	followMap        map[int64]int64
	launchingRiderID int64
	teamOrders       simulation.TeamOrders

	director directorState
	plan     RacePlan
	orderSrc map[string]setBy // who set each team order (see interactive_live.go)
	riderSrc map[int64]setBy

	paused  bool
	speed   float64
	stopped bool
	stopCh  chan struct{}
}

func newInteractiveSession(controlledTeam string) *interactiveSession {
	return &interactiveSession{
		id:             newSessionID(),
		controlledTeam: controlledTeam,
		overrides:      make(map[int64]simulation.Action),
		oneShot:        make(map[int64]simulation.Action),
		efforts:        make(map[int64]float64),
		styles:         make(map[int64]riderStyle),
		followMap:      make(map[int64]int64),
		speed:          0.5,
		stopCh:         make(chan struct{}),
		director:       directorState{enabled: true},
	}
}

func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	// Fallback: not cryptographically strong, but fine for in-process IDs.
	return hex.EncodeToString([]byte(time.Now().Format("20060102150405.000000000")))
}

// ActionOverride implements simulation.ActionOverrideProvider.
func (s *interactiveSession) ActionOverride(riderID int64) (simulation.Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.overrides[riderID]
	return a, ok
}

// ConsumeOneShot implements simulation.ActionOverrideProvider.
func (s *interactiveSession) ConsumeOneShot(riderID int64) (simulation.Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.oneShot[riderID]
	if ok {
		delete(s.oneShot, riderID)
	}
	return a, ok
}

// EffortMultiplier implements simulation.ActionOverrideProvider.
func (s *interactiveSession) EffortMultiplier(riderID int64) (float64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.efforts[riderID]
	return m, ok
}

// SprintTrainDirective implements simulation.ActionOverrideProvider. On the
// player's lead-out order the train rides for the sprinter in each road
// group: the rider the player chose if he's in it, otherwise the team's
// fastest finisher there (teamGroup is ordered fastest first). His
// teammates in that group ride the front; he sits in. Riders without a
// teammate in their group race on their own judgement.
func (s *interactiveSession) SprintTrainDirective(riderID int64, riderTeam string, kmRemaining float64, teamGroup []int64) (simulation.Action, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if riderTeam != s.controlledTeam {
		return simulation.ActionHold, false
	}
	if kmRemaining > 8.0 {
		return simulation.ActionHold, false
	}
	if s.launchingRiderID == riderID {
		return simulation.ActionAttack, true
	}
	if _, ok := s.followMap[riderID]; ok {
		return simulation.ActionChase, true
	}
	// The sprint train runs only on the player's lead-out order.
	if s.teamOrders.Leadout != simulation.OrderYes || len(teamGroup) < 2 {
		return simulation.ActionHold, false
	}
	sprinter := teamGroup[0]
	for _, id := range teamGroup {
		if s.trainSprinter != 0 && id == s.trainSprinter {
			sprinter = id
		}
	}
	if riderID == sprinter {
		return simulation.ActionConserve, true
	}
	return simulation.ActionChase, true
}

// SprintTrainState implements simulation.ActionOverrideProvider.
func (s *interactiveSession) SprintTrainState(kmRemaining float64) *simulation.TeamState {
	s.mu.Lock()
	defer s.mu.Unlock()

	var order []int64 // the chosen sprinter, if any
	if s.trainSprinter != 0 {
		order = []int64{s.trainSprinter}
	}

	follow := make(map[int64]*int64, len(s.followMap))
	for riderID, followID := range s.followMap {
		id := followID
		follow[riderID] = &id
	}

	phase := "idle"
	if s.launchingRiderID != 0 {
		phase = "launch"
	} else if kmRemaining <= 8.0 && (s.teamOrders.Leadout == simulation.OrderYes || len(follow) > 0) {
		phase = "following"
	}

	var launching *int64
	if s.launchingRiderID != 0 {
		id := s.launchingRiderID
		launching = &id
	}

	if len(order) == 0 && len(follow) == 0 && launching == nil && s.teamOrders.Leadout != simulation.OrderYes {
		return nil
	}
	return &simulation.TeamState{
		SprintTrainOrder: order,
		FollowMap:        follow,
		SprintTrainPhase: phase,
		LaunchingRiderID: launching,
	}
}

func (s *interactiveSession) SetSpeed(speed float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if speed <= 0 {
		s.speed = 1.0
		return
	}
	if speed > 16 {
		s.speed = 16
		return
	}
	s.speed = speed
}

func (s *interactiveSession) SetPaused(paused bool) {
	s.mu.Lock()
	s.paused = paused
	if !paused {
		s.director.pending = nil // resuming dismisses an open decision
	}
	s.mu.Unlock()
}

func (s *interactiveSession) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	close(s.stopCh)
	s.mu.Unlock()
}

func (s *interactiveSession) IsStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopped
}

func (s *interactiveSession) SetRiderCommand(riderID int64, action simulation.Action, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// "hold" means: let AI decide (clear persistent override).
	if action == simulation.ActionHold {
		delete(s.overrides, riderID)
		s.markRiderLocked(riderID, "", "")
		return
	}

	if mode == "oneshot" {
		s.oneShot[riderID] = action
		return
	}

	s.overrides[riderID] = action
	s.markRiderLocked(riderID, "you", "")
	s.director.timed = dropTimed(s.director.timed, riderID) // his own order: no timer
}

func dropTimed(ts []timedCommand, riderID int64) []timedCommand {
	out := ts[:0]
	for _, t := range ts {
		if t.cmd.RiderID != riderID {
			out = append(out, t)
		}
	}
	return out
}

func (s *interactiveSession) SetRiderEffort(riderID int64, effortMultiplier float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if effortMultiplier <= 0 {
		delete(s.efforts, riderID)
		return
	}
	if effortMultiplier < 0.6 {
		effortMultiplier = 0.6
	}
	if effortMultiplier > 1.3 {
		effortMultiplier = 1.3
	}
	s.efforts[riderID] = effortMultiplier
}

// SetSprintTrainOrder sets who the lead-out rides for: the first valid ID
// (none: the team's fastest finisher in each group).
func (s *interactiveSession) SetSprintTrainOrder(riderIDs []int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.trainSprinter = 0
	for _, id := range riderIDs {
		if id > 0 {
			s.trainSprinter = id
			break
		}
	}
}

func (s *interactiveSession) SetRiderFollow(riderID int64, followRiderID int64, enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !enabled || followRiderID <= 0 || followRiderID == riderID {
		delete(s.followMap, riderID)
		return
	}
	s.followMap[riderID] = followRiderID
}

func (s *interactiveSession) LaunchSprint(riderID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if riderID <= 0 {
		s.launchingRiderID = 0
		return
	}
	s.launchingRiderID = riderID
}

// Pace blocks while paused and adds a small delay per update so the UI can interact.
// Only call this for interactive mode.
func (s *interactiveSession) Pace() {
	// Fast checks without holding the lock for sleeps.
	for {
		s.mu.Lock()
		paused := s.paused
		speed := s.speed
		stopCh := s.stopCh
		s.mu.Unlock()

		select {
		case <-stopCh:
			return
		default:
		}

		if !paused {
			// Interactive mode pacing: this is intentionally "human speed" so the user
			// can react and issue commands between segment updates.
			base := 120 * time.Millisecond
			d := time.Duration(float64(base) / speed)
			if d < 20*time.Millisecond {
				d = 20 * time.Millisecond
			}
			time.Sleep(d)
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// TeamOrders implements simulation.TeamOrderProvider: the player's orders
// apply to their own team.
func (s *interactiveSession) TeamOrders(team string) simulation.TeamOrders {
	s.mu.Lock()
	defer s.mu.Unlock()
	if team != s.controlledTeam {
		return simulation.TeamOrders{}
	}
	return s.teamOrders
}

// SetTeamOrder sets one of the controlled team's orders: kind "chase",
// "break" or "pace". It reports whether the kind is known.
func (s *interactiveSession) SetTeamOrder(kind string, order simulation.Order) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.director.orders = removeOrder(s.director.orders, kind) // the player's order stands
	ok := s.setTeamOrderLocked(kind, order)
	s.markOrderLocked(kind, order, "you", "")
	return ok
}

func (s *interactiveSession) setTeamOrderLocked(kind string, order simulation.Order) bool {
	switch kind {
	case "chase":
		s.teamOrders.Chase = order
	case "break":
		s.teamOrders.Break = order
	case "pace":
		s.teamOrders.Pace = order
	case "protect":
		s.teamOrders.Protect = order
	case "leadout":
		s.teamOrders.Leadout = order
	case "ambition":
		s.teamOrders.Ambition = order
	default:
		return false
	}
	return true
}

// riderStyle is a player's setting for how a rider spends himself.
type riderStyle struct {
	dig  simulation.Dig
	pace simulation.WorkPace
}

// RiderStyle implements simulation.RiderStyleProvider.
func (s *interactiveSession) RiderStyle(riderID int64) (simulation.Dig, simulation.WorkPace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.styles[riderID]
	return st.dig, st.pace
}

// SetRiderStyle sets how deep a rider digs to hold his group ("any",
// "reserve", "own") and how hard he works ("auto", "tempo", "threshold",
// "allout"). Unknown values keep the default.
func (s *interactiveSession) SetRiderStyle(riderID int64, dig, pace string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := riderStyle{}
	switch dig {
	case "reserve":
		st.dig = simulation.DigKeepReserve
	case "own":
		st.dig = simulation.DigOwnPace
	}
	switch pace {
	case "tempo":
		st.pace = simulation.PaceTempo
	case "threshold":
		st.pace = simulation.PaceThreshold
	case "allout":
		st.pace = simulation.PaceAllOut
	}
	if st == (riderStyle{}) {
		delete(s.styles, riderID)
		return
	}
	s.styles[riderID] = st
}
