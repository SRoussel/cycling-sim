// Package director finds the moments in a live race that call for the
// player's decision, as a team director would see them from the car: a rival
// favourite attacks, a break goes, the leader is dropped, a decisive climb
// is coming, the finale. Each moment offers a few choices, each a set of
// rider commands and team orders the engine already understands.
//
// It only reads simulation.RaceView (the true state at the end of a step);
// it never changes the race itself. Applying a choice is the session's job.
package director

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"cycling_sim/internal/simulation"
)

// Command is one rider instruction. Persistent commands last ForKm (0: until
// changed); AtKmRemaining > 0 schedules it for later (e.g. near a summit).
type Command struct {
	RiderID       int64
	Action        string // attack, chase, conserve, hold
	Mode          string // persistent, oneshot
	ForKm         float64
	AtKmRemaining float64
}

// OrderChange sets a team order (see SetTeamOrder) for ForKm (0: for good).
type OrderChange struct {
	Kind  string
	Value string // auto, yes, no
	ForKm float64
}

// Option is one choice in a moment.
type Option struct {
	ID       string        `json:"id"`
	Label    string        `json:"label"`
	Help     string        `json:"help"`
	Commands []Command     `json:"-"`
	Orders   []OrderChange `json:"-"`
}

// Moment is a decision for the player.
type Moment struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Text        string   `json:"text"`
	KmRemaining float64  `json:"kmRemaining"`
	Options     []Option `json:"options"`
}

// Option returns the option with the given ID.
func (m *Moment) Option(id string) (Option, bool) {
	for _, o := range m.Options {
		if o.ID == id {
			return o, true
		}
	}
	return Option{}, false
}

// Climb is a climb on the route, by distance from the start.
type Climb struct {
	StartKm  float64
	LengthKm float64
	GainM    float64
	AvgPct   float64
}

const (
	minMomentSpacingKm = 2    // between two moments
	attackCooldownKm   = 8    // per attacking rider
	droppedCooldownKm  = 15   // between two "leader dropped" moments
	finaleKm           = 12   // the finale moment
	climbLeadKm        = 3    // warn this far before a decisive climb
	midPhaseKm         = 60   // engine's PhaseFromKm boundaries
	finalPhaseKm       = 20   //
	breakLateKm        = 70   // second look at a break still away
	minBreakGapS       = 60   //
	droppedGapS        = 15   //
	followGapS         = 45   // an attack this far up the road is still answerable
	climbMinGainM      = 80   //
	decisiveClimbShare = 0.35 // climbs in the last 35% of the race (or last 80 km)
)

// Director finds decision moments for one team.
type Director struct {
	Team string

	totalKm      float64
	climbs       []Climb
	lastMomentKm float64
	fired        map[string]bool
	attackAt     map[int64]float64 // km remaining at the rider's last attack moment
	droppedAt    float64
	seq          int
}

// New returns a director for the team.
func New(team string) *Director {
	d := &Director{Team: team}
	d.reset()
	return d
}

func (d *Director) reset() {
	d.lastMomentKm = math.Inf(1)
	d.fired = map[string]bool{}
	d.attackAt = map[int64]float64{}
	d.droppedAt = math.Inf(1)
}

// SetRoute starts a new race (or stage) on this route profile.
func (d *Director) SetRoute(profile *simulation.RouteProfile) {
	d.reset()
	d.climbs, d.totalKm = nil, 0
	if profile == nil {
		return
	}
	d.totalKm = profile.TotalKm
	d.climbs = FindClimbs(profile.Points)
}

// Climbs returns the route's climbs.
func (d *Director) Climbs() []Climb { return d.climbs }

// FindClimbs finds stretches climbing 2%+ (a dip of up to 1 km doesn't end
// one) that gain at least 40 m at 3%+ on average.
func FindClimbs(pts []simulation.ProfilePoint) []Climb {
	var out []Climb
	start, lastUp := -1, -1
	closeClimb := func() {
		if start < 0 || lastUp <= start {
			return
		}
		l := pts[lastUp].Km - pts[start].Km
		gain := pts[lastUp].Elev - pts[start].Elev
		if l > 0 && gain >= 40 && gain/(l*10) >= 3 {
			out = append(out, Climb{StartKm: pts[start].Km, LengthKm: l, GainM: gain, AvgPct: gain / (l * 10)})
		}
	}
	for i := 1; i < len(pts); i++ {
		dk := pts[i].Km - pts[i-1].Km
		pct := 0.0
		if dk > 0 {
			pct = (pts[i].Elev - pts[i-1].Elev) / (dk * 1000) * 100
		}
		if pct >= 2 {
			if start < 0 {
				start = i - 1
			}
			lastUp = i
		} else if start >= 0 && pts[i].Km-pts[lastUp].Km > 1 {
			closeClimb()
			start = -1
		}
	}
	closeClimb()
	return out
}

// view is a RaceView with the lookups the rules need.
type view struct {
	*simulation.RaceView
	team    string
	groupOf map[int64]int
	main    int // the largest group
}

func newView(v *simulation.RaceView, team string) *view {
	w := &view{RaceView: v, team: team, groupOf: map[int64]int{}}
	for gi, g := range v.Road {
		for _, id := range g.IDs {
			w.groupOf[id] = gi
		}
		if len(g.IDs) > len(v.Road[w.main].IDs) {
			w.main = gi
		}
	}
	return w
}

func (w *view) name(id int64) string { return w.Riders[id].Name }

func (w *view) mine(id int64) bool { return w.Riders[id].Team == w.team }

func (w *view) gap(gi int) float64 { return w.Road[gi].GapSeconds }

// myRiders returns the team's riders in group gi (all groups if gi < 0),
// most reserve first.
func (w *view) myRiders(gi int) []int64 {
	var out []int64
	for g, grp := range w.Road {
		if gi >= 0 && g != gi {
			continue
		}
		for _, id := range grp.IDs {
			if w.mine(id) {
				out = append(out, id)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return w.Riders[out[i]].WPrimePct > w.Riders[out[j]].WPrimePct })
	return out
}

// protected is the team's rider the race is ridden for: the leader furthest
// up the road, else the sprinter. 0 if neither is in the race.
func (w *view) protected() int64 {
	best, bestGroup := int64(0), math.MaxInt
	for _, role := range []simulation.TeamRole{simulation.RoleLeader, simulation.RoleSprinter} {
		for id, r := range w.Riders {
			if r.Team != w.team || r.Role != role {
				continue
			}
			if g := w.groupOf[id]; g < bestGroup || (g == bestGroup && id < best) {
				best, bestGroup = id, g
			}
		}
		if best != 0 {
			return best
		}
	}
	return 0
}

// domestiques returns the team's riders in group gi other than except.
func (w *view) helpers(gi int, except int64) []int64 {
	var out []int64
	for _, id := range w.myRiders(gi) {
		if id != except && w.Riders[id].Role == simulation.RoleDomestique {
			out = append(out, id)
		}
	}
	return out
}

// groupLabel names group gi as the player sees it.
func (w *view) groupLabel(gi int) string {
	switch {
	case gi == w.main && len(w.Road[gi].IDs) >= 15:
		return "the peloton"
	case gi == 0:
		return "the front group"
	default:
		return fmt.Sprintf("a group of %d", len(w.Road[gi].IDs))
	}
}

func fmtGap(s float64) string {
	s = math.Round(math.Max(0, s))
	if s < 60 {
		return fmt.Sprintf("%.0f s", s)
	}
	return fmt.Sprintf("%d:%02d", int(s)/60, int(s)%60)
}

func names(w *view, ids []int64, limit int) string {
	var parts []string
	for i, id := range ids {
		if i == limit {
			parts = append(parts, fmt.Sprintf("%d more", len(ids)-limit))
			break
		}
		parts = append(parts, w.name(id))
	}
	return strings.Join(parts, ", ")
}

// Next returns the moment this step calls for, or nil.
func (d *Director) Next(v *simulation.RaceView) *Moment {
	if v == nil || len(v.Road) == 0 || v.KmRemaining <= 0.5 {
		return nil
	}
	if d.totalKm == 0 {
		d.totalKm = v.KmTotal
	}
	w := newView(v, d.Team)
	if len(w.myRiders(-1)) == 0 {
		return nil
	}
	km := v.KmRemaining
	spaced := d.lastMomentKm-km >= minMomentSpacingKm
	// Attacks can't wait; the rest keep a little road between moments
	// (rules mark themselves fired, so only run those that may fire).
	rules := []func(*view) *Moment{d.rivalAttack}
	if spaced {
		rules = append(rules, d.leaderDropped, d.keyClimb, d.finale, d.breakaway)
	}
	var m *Moment
	for _, rule := range rules {
		if m = rule(w); m != nil {
			break
		}
	}
	if m == nil {
		return nil
	}
	d.seq++
	m.ID = fmt.Sprintf("m%d", d.seq)
	m.KmRemaining = km
	d.lastMomentKm = km
	return m
}

// rivalAttack: a key rider of another team attacks within reach of ours.
func (d *Director) rivalAttack(w *view) *Moment {
	km := w.KmRemaining
	for _, ev := range w.Events {
		if ev.Type != "attack" || !ev.Key || ev.RiderID == 0 || w.mine(ev.RiderID) {
			continue
		}
		att, ok := w.Riders[ev.RiderID]
		if !ok {
			continue
		}
		if last, seen := d.attackAt[ev.RiderID]; seen && last-km < attackCooldownKm {
			continue
		}
		if !att.Favourite && km > finalPhaseKm {
			continue // other teams' leaders matter only in the finale
		}
		ag := w.groupOf[ev.RiderID]
		// Who answers: the protected rider if close, else our freshest
		// rider in the attacker's group or the one just behind it.
		p := w.protected()
		follower := int64(0)
		if p != 0 {
			pg := w.groupOf[p]
			if pg == ag || (pg > ag && w.gap(pg)-w.gap(ag) <= followGapS) {
				follower = p
			}
		}
		if follower == 0 {
			for _, gi := range []int{ag, ag + 1} {
				if gi < len(w.Road) && w.gap(gi)-w.gap(ag) <= followGapS {
					if mine := w.myRiders(gi); len(mine) > 0 && w.Riders[mine[0]].WPrimePct >= 40 {
						follower = mine[0]
						break
					}
				}
			}
		}
		if follower == 0 {
			continue
		}
		d.attackAt[ev.RiderID] = km
		f := w.Riders[follower]
		fg := w.groupOf[follower]
		where := "in his group"
		if fg != ag {
			where = fmt.Sprintf("%s behind", fmtGap(w.gap(fg)-w.gap(ag)))
		}
		m := &Moment{
			Kind:  "attack",
			Title: fmt.Sprintf("⚡ %s attacks!", att.Name),
			Text: fmt.Sprintf("%s (%s) goes with %.1f km to go%s. %s is %s with %.0f%% reserve.",
				att.Name, att.Team, km, d.climbContext(km), f.Name, where, f.WPrimePct),
		}
		m.Options = append(m.Options, Option{
			ID: "follow", Label: fmt.Sprintf("%s goes with him", f.Name),
			Help:     "Chase the move for the next 3 km: costs reserve, keeps him in the race.",
			Commands: []Command{{RiderID: follower, Action: "chase", Mode: "persistent", ForKm: 3}},
		})
		if hs := w.helpers(fg, follower); len(hs) > 0 && w.Riders[hs[0]].WPrimePct >= 40 && follower == p {
			m.Options = append(m.Options, Option{
				ID: "mark", Label: fmt.Sprintf("%s chases, %s on his wheel", w.name(hs[0]), f.Name),
				Help:     "A teammate closes the gap for 5 km; the leader saves his reserve in the draft.",
				Commands: []Command{{RiderID: hs[0], Action: "chase", Mode: "persistent", ForKm: 5}},
			})
		}
		if f.WPrimePct >= 60 && km <= midPhaseKm {
			m.Options = append(m.Options, Option{
				ID: "counter", Label: fmt.Sprintf("%s counter-attacks", f.Name),
				Help:     "Go over the top of the move: a hard burst now.",
				Commands: []Command{{RiderID: follower, Action: "attack", Mode: "oneshot"}},
			})
		}
		m.Options = append(m.Options, Option{
			ID: "let", Label: "Let him go", Help: "Others may chase. Our riders keep their reserve.",
		})
		return m
	}
	return nil
}

// climbContext describes the climb the race is on or about to start.
func (d *Director) climbContext(km float64) string {
	pos := d.totalKm - km
	for _, c := range d.climbs {
		if c.GainM < climbMinGainM {
			continue
		}
		switch {
		case pos >= c.StartKm && pos < c.StartKm+c.LengthKm:
			return fmt.Sprintf(", %.1f km from the top of a climb at %.1f%%", c.StartKm+c.LengthKm-pos, c.AvgPct)
		case c.StartKm-pos > 0 && c.StartKm-pos <= 1.5:
			return fmt.Sprintf(", at the foot of a %.1f km climb at %.1f%%", c.LengthKm, c.AvgPct)
		}
	}
	return ""
}

// leaderDropped: the rider we ride for is behind the race's front.
func (d *Director) leaderDropped(w *view) *Moment {
	km := w.KmRemaining
	if km > midPhaseKm || km < 3 || d.droppedAt-km < droppedCooldownKm {
		return nil
	}
	p := w.protected()
	if p == 0 {
		return nil
	}
	// The front that matters: the first group with a favourite, or the
	// peloton, whichever is ahead.
	ref := w.main
	for gi := 0; gi < ref; gi++ {
		for _, id := range w.Road[gi].IDs {
			if w.Riders[id].Favourite && !w.mine(id) {
				ref = gi
				break
			}
		}
	}
	pg := w.groupOf[p]
	behind := w.gap(pg) - w.gap(ref)
	if pg <= ref || behind < droppedGapS {
		return nil
	}
	d.droppedAt = km
	pr := w.Riders[p]
	with := w.helpers(pg, p)
	var ahead []int64
	for gi := 0; gi < pg; gi++ {
		ahead = append(ahead, w.helpers(gi, p)...)
	}
	m := &Moment{
		Kind:  "dropped",
		Title: fmt.Sprintf("🚨 %s is dropped", pr.Name),
		Text: fmt.Sprintf("%s is %s behind %s with %.1f km to go (reserve %.0f%%, legs %.0f%%). Teammates with him: %d, up the road: %d.",
			pr.Name, fmtGap(behind), w.groupLabel(ref), km, pr.WPrimePct, pr.LegsPct, len(with), len(ahead)),
	}
	if len(with)+len(ahead) > 0 {
		m.Options = append(m.Options, Option{
			ID: "wait", Label: "The team brings him back",
			Help:   "Teammates with him chase; those up the road sit up and wait (for 15 km).",
			Orders: []OrderChange{{Kind: "protect", Value: "yes", ForKm: 15}},
		})
	}
	m.Options = append(m.Options,
		Option{
			ID: "solo", Label: fmt.Sprintf("%s chases alone", pr.Name),
			Help:     "He rides to close the gap himself for 5 km; the team races on.",
			Commands: []Command{{RiderID: p, Action: "chase", Mode: "persistent", ForKm: 5}},
		},
		Option{
			ID: "let", Label: "Race on without him",
			Help:   "Nobody waits: riders up the road race for themselves.",
			Orders: []OrderChange{{Kind: "protect", Value: "no"}},
		},
	)
	return m
}

// finale: the run-in, once.
func (d *Director) finale(w *view) *Moment {
	km := w.KmRemaining
	if km > finaleKm || d.fired["finale"] {
		return nil
	}
	mineFront := w.myRiders(0)
	if len(mineFront) == 0 {
		return nil
	}
	d.fired["finale"] = true
	size := len(w.Road[0].IDs)
	var sprinter int64
	for _, id := range mineFront {
		if w.Riders[id].Role == simulation.RoleSprinter {
			sprinter = id
		}
	}
	if size >= 15 && sprinter != 0 {
		s := w.Riders[sprinter]
		return &Moment{
			Kind:  "finale",
			Title: "🏁 Bunch sprint coming",
			Text: fmt.Sprintf("%.1f km to go, %d riders together. %s has %.0f%% reserve, %d teammates with him.",
				km, size, s.Name, s.WPrimePct, len(mineFront)-1),
			Options: []Option{
				{ID: "train", Label: fmt.Sprintf("Lead-out train for %s", s.Name), Help: "Teammates lead him out from 8 km; the last man launches him.",
					Orders: []OrderChange{{Kind: "leadout", Value: "yes"}}},
				{ID: "free", Label: "Every man for himself", Help: "No lead-out: each rider finds his own way.",
					Orders: []OrderChange{{Kind: "leadout", Value: "no"}}},
				{ID: "auto", Label: "The riders decide", Help: "Leave the finale to the team."},
			},
		}
	}
	r := mineFront[0]
	rb := w.Riders[r]
	others := len(mineFront) - 1
	text := fmt.Sprintf("%s is in the front group of %d with %.1f km to go, reserve %.0f%%", rb.Name, size, km, rb.WPrimePct)
	if others > 0 {
		text += fmt.Sprintf(" (%d teammates with him)", others)
	}
	if len(w.Road) > 1 {
		text += fmt.Sprintf(". Next group at %s.", fmtGap(w.gap(1)))
	} else {
		text += "."
	}
	return &Moment{
		Kind:  "finale",
		Title: "🏁 The finale",
		Text:  text,
		Options: []Option{
			{ID: "attack", Label: fmt.Sprintf("%s attacks now", rb.Name), Help: "Go clear and try to hold it to the line.",
				Commands: []Command{{RiderID: r, Action: "attack", Mode: "oneshot"}}},
			{ID: "wait", Label: "Wait for the sprint", Help: "Sit on and save everything for the final kilometre.",
				Commands: []Command{{RiderID: r, Action: "conserve", Mode: "persistent", ForKm: math.Max(0, km-1)}}},
			{ID: "work", Label: "Ride to keep the group away", Help: "Take turns so the chasers don't come back.",
				Commands: []Command{{RiderID: r, Action: "chase", Mode: "persistent"}}},
		},
	}
}

// keyClimb: a decisive climb starts within a few km.
func (d *Director) keyClimb(w *view) *Moment {
	km := w.KmRemaining
	pos := d.totalKm - km
	p := w.protected()
	if p == 0 || d.totalKm == 0 {
		return nil
	}
	pg := w.groupOf[p]
	if pg > w.main {
		return nil // he's dropped: that's the other moment
	}
	for _, c := range d.climbs {
		toFoot := c.StartKm - pos
		// Up to a third of the way up still counts: the view may arrive late.
		if toFoot <= -c.LengthKm/3 || toFoot > climbLeadKm || c.GainM < climbMinGainM {
			continue
		}
		toGoAtFoot := d.totalKm - c.StartKm
		if toGoAtFoot > math.Max(80, decisiveClimbShare*d.totalKm) {
			continue
		}
		key := fmt.Sprintf("climb-%.1f", c.StartKm)
		if d.fired[key] {
			continue
		}
		d.fired[key] = true
		pr := w.Riders[p]
		top := d.totalKm - c.StartKm - c.LengthKm
		nearTop := d.totalKm - c.StartKm - 0.75*c.LengthKm
		title := fmt.Sprintf("⛰ Climb in %.1f km", toFoot)
		if toFoot <= 0 {
			title = fmt.Sprintf("⛰ On the climb: %.1f km to the top", km-top)
		}
		m := &Moment{
			Kind:  "climb",
			Title: title,
			Text: fmt.Sprintf("%.1f km at %.1f%% (%.0f m), the top %.1f km from the finish. %s is in %s with %.0f%% reserve, %d teammates with him.",
				c.LengthKm, c.AvgPct, c.GainM, top, pr.Name, w.groupLabel(pg), pr.WPrimePct, len(w.helpers(pg, p))),
		}
		m.Options = append(m.Options, Option{ID: "attack-top", Label: fmt.Sprintf("%s attacks near the top", pr.Name),
			Help:     "Follow the wheels, then go with a quarter of the climb left.",
			Commands: []Command{{RiderID: p, Action: "attack", Mode: "oneshot", AtKmRemaining: nearTop}}})
		if toFoot > 0 {
			m.Options = append(m.Options, Option{ID: "attack-foot", Label: fmt.Sprintf("%s attacks at the foot", pr.Name),
				Help:     "A long attack: the whole climb to make the difference.",
				Commands: []Command{{RiderID: p, Action: "attack", Mode: "oneshot", AtKmRemaining: toGoAtFoot}}})
		}
		m.Options = append(m.Options,
			Option{ID: "tempo", Label: "Team sets a hard tempo", Help: "Teammates drive the climb to thin the group for him.",
				Orders: []OrderChange{{Kind: "pace", Value: "yes", ForKm: km - top}}},
			Option{ID: "sit", Label: "Sit in and follow", Help: "Save the reserve; react to the others."})
		return m
	}
	return nil
}

// breakaway: a break is up the road without us, or with one of ours.
func (d *Director) breakaway(w *view) *Moment {
	km := w.KmRemaining
	if km < finalPhaseKm || w.main == 0 {
		return nil
	}
	gap := w.gap(w.main)
	if gap < minBreakGapS {
		return nil
	}
	var ahead []int64 // the break: everyone up the road
	for gi := 0; gi < w.main; gi++ {
		ahead = append(ahead, w.Road[gi].IDs...)
	}
	if len(ahead) > 25 {
		return nil
	}
	var ours []int64
	for _, id := range ahead {
		if w.mine(id) {
			ours = append(ours, id)
		}
	}
	if len(ours) > 0 {
		r := ours[0]
		key := fmt.Sprintf("inbreak-%d", r)
		if d.fired[key] {
			return nil
		}
		d.fired[key] = true
		rb := w.Riders[r]
		return &Moment{
			Kind:  "break",
			Title: fmt.Sprintf("🏃 %s is in the break", rb.Name),
			Text: fmt.Sprintf("A break of %d is %s clear with %.0f km to go: %s.",
				len(ahead), fmtGap(gap), km, names(w, ahead, 8)),
			Options: []Option{
				{ID: "work", Label: "Ride to keep it away", Help: "He takes full turns: the break goes faster, he tires sooner.",
					Commands: []Command{{RiderID: r, Action: "chase", Mode: "persistent"}}},
				{ID: "sit", Label: "Sit on, save for the finish", Help: "He skips turns: fresher at the end, but the break may be caught.",
					Commands: []Command{{RiderID: r, Action: "conserve", Mode: "persistent"}}},
				{ID: "auto", Label: "Let him race", Help: "He decides for himself."},
			},
		}
	}
	key := "break"
	if d.fired["break"] {
		if km > breakLateKm || gap < 1.5*minBreakGapS || d.fired["break-late"] {
			return nil
		}
		key = "break-late"
	}
	d.fired[key] = true
	title := "🏃 A break is away"
	if key == "break-late" {
		title = "⏱ The break is still away"
	}
	teams := map[string]bool{}
	for _, id := range ahead {
		teams[w.Riders[id].Team] = true
	}
	return &Moment{
		Kind:  "break",
		Title: title,
		Text: fmt.Sprintf("%d riders from %d teams are %s up with %.0f km to go: %s.",
			len(ahead), len(teams), fmtGap(gap), km, names(w, ahead, 8)),
		Options: []Option{
			{ID: "chase", Label: "Put the team on the front", Help: "Our domestiques chase to bring it back before the finish.",
				Orders: []OrderChange{{Kind: "chase", Value: "yes"}}},
			{ID: "share", Label: "Leave it to the others", Help: "Chase only if it threatens our race; other teams usually ride.",
				Orders: []OrderChange{{Kind: "chase", Value: "auto"}}},
			{ID: "ignore", Label: "Don't chase", Help: "Save the team's legs; the break may win.",
				Orders: []OrderChange{{Kind: "chase", Value: "no"}}},
		},
	}
}

// LastMomentKm is the km remaining at the last moment (for traces).
func (d *Director) LastMomentKm() float64 { return d.lastMomentKm }
