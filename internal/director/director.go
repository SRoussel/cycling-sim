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
	Name     string // from a cobbled or gravel sector on it (the Flemish bergs), else ""
	Stars    int    // the sector's stars
	Surface  string // the sector's surface (cobbles, gravel), "" for a paved climb
}

// label names the climb for the player.
func (c Climb) label() string {
	if c.Name != "" {
		return c.Name
	}
	return "the climb"
}

const (
	minMomentSpacingKm = 2    // between two moments
	attackCooldownKm   = 15   // per attacking rider
	droppedCooldownKm  = 15   // between two "leader dropped" moments
	finaleKm           = 12   // the finale moment
	climbLeadKm        = 3    // warn this far before a decisive climb
	midPhaseKm         = 60   // engine's PhaseFromKm boundaries
	finalPhaseKm       = 20   //
	breakLateKm        = 70   // second look at a break still away
	minBreakGapS       = 60   //
	droppedGapS        = 15   //
	followGapS         = 45   // an attack this far up the road is still answerable
	climbMinScore      = 300  // gain (m) × average gradient (%): Paterberg ~600, Poggio ~870, Capi Mele ~240
	decisiveClimbs     = 3    // the hardest climbs of the finale (plus the last) are moments
	attackSpacingKm    = 5    // between attack moments (1 km in the last 10 km)
	lowReservePct      = 20   // below this a rider isn't offered an attack
	sectorMinScore     = 20   // length (km) × stars²
	decisiveSectors    = 4    // the hardest flat cobbled sectors of the finale (plus the last 4-5★)
	decisiveClimbShare = 0.35 // climbs in the last 35% of the race (or last 80 km)
)

// Director finds decision moments for one team.
type Director struct {
	Team string

	totalKm         float64
	climbs          []Climb
	decisive        map[float64]bool // decisive climbs by start km
	sectors         []simulation.RouteSector
	decisiveSectors map[float64]bool // decisive cobbled sectors by start km
	lastMomentKm    float64
	fired           map[string]bool
	attackAt        map[int64]float64 // km remaining at the rider's last attack moment
	lastAttackKm    float64           // km remaining at the last attack moment
	droppedAt       float64
	droppedAsked    map[int64]bool // cards asked about since they were last in front
	seq             int
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
	d.lastAttackKm = math.Inf(1)
	d.droppedAt = math.Inf(1)
	d.droppedAsked = map[int64]bool{}
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
	d.sectors = profile.Sectors
	nameClimbs(d.climbs, d.sectors)
	d.decisiveSectors = decisiveSectorSet(d.sectors, d.climbs, d.totalKm)
	// A climb mostly inside a decisive sector is part of that sector's moment.
	var open []Climb
	for _, c := range d.climbs {
		inside := false
		for _, sec := range d.sectors {
			if d.decisiveSectors[sec.StartKm] && overlap(c.StartKm, c.LengthKm, sec.StartKm, sec.LengthKm) >= 0.5*c.LengthKm {
				inside = true
			}
		}
		if !inside {
			open = append(open, c)
		}
	}
	d.decisive = decisiveSet(open, d.totalKm)
}

// overlap is the length two stretches of road share.
func overlap(a0, al, b0, bl float64) float64 {
	return math.Max(0, math.Min(a0+al, b0+bl)-math.Max(a0, b0))
}

// nameClimbs gives climbs the name of a cobbled sector on them.
func nameClimbs(climbs []Climb, sectors []simulation.RouteSector) {
	for i := range climbs {
		c := &climbs[i]
		best := 0.0
		for _, sec := range sectors {
			if o := overlap(c.StartKm, c.LengthKm, sec.StartKm, sec.LengthKm); o > best && o >= 0.3*sec.LengthKm {
				best, c.Name, c.Stars, c.Surface = o, sec.Name, sec.Stars, sec.Surface
			}
		}
	}
}

// sectorScore: length × stars² for cobbles; gravel is rated 1-3★ where
// cobbles reach 5★, so length × (stars+1)² (Monte Sante Marie, 10 km 3★: 160).
func sectorScore(s simulation.RouteSector) float64 {
	st := float64(s.Stars)
	if s.Surface == "gravel" {
		st++
	}
	return s.LengthKm * st * st
}

// sectorWords returns the surface's noun and icon.
func sectorWords(surface string) (noun, icon string) {
	if surface == "gravel" {
		return "gravel", "💨"
	}
	return "cobbles", "🪨"
}

// decisiveSectorSet picks the flat cobbled sectors worth a decision: in the
// last 40% (or 100 km), hard enough (length x stars²: Arenberg ~52,
// Mons-en-Pévèle ~72, a 2 km 3★ sector 18), the hardest few plus the last
// 4-5★ one. Cobbled climbs are the climb moment's.
func decisiveSectorSet(sectors []simulation.RouteSector, climbs []Climb, totalKm float64) map[float64]bool {
	var cands []simulation.RouteSector
	for _, sec := range sectors {
		onClimb := false
		for _, c := range climbs {
			// A cobbled berg is the climb's moment; a long sector with a
			// short rise in it (Monte Sante Marie) stays a sector.
			if overlap(c.StartKm, c.LengthKm, sec.StartKm, sec.LengthKm) >= 0.5*sec.LengthKm {
				onClimb = true
			}
		}
		if !onClimb && totalKm-sec.StartKm <= math.Max(100, 0.4*totalKm) && sectorScore(sec) >= sectorMinScore {
			cands = append(cands, sec)
		}
	}
	out := map[float64]bool{}
	for i := len(cands) - 1; i >= 0; i-- {
		if sectorScore(cands[i]) >= 2*sectorMinScore { // the last hard one (Carrefour de l'Arbre: 50)
			out[cands[i].StartKm] = true
			break
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return sectorScore(cands[i]) > sectorScore(cands[j]) })
	for i := 0; i < len(cands) && i < decisiveSectors; i++ {
		out[cands[i].StartKm] = true
	}
	return out
}

func climbScore(c Climb) float64 { return c.GainM * c.AvgPct }

// decisiveSet picks the climbs worth a decision: in the finale (last 35% or
// 80 km), hard enough, and among the hardest few or the last one.
func decisiveSet(climbs []Climb, totalKm float64) map[float64]bool {
	var cands []Climb
	for _, c := range climbs {
		if totalKm-c.StartKm <= math.Max(80, decisiveClimbShare*totalKm) && climbScore(c) >= climbMinScore {
			cands = append(cands, c)
		}
	}
	out := map[float64]bool{}
	if len(cands) == 0 {
		return out
	}
	out[cands[len(cands)-1].StartKm] = true
	sort.SliceStable(cands, func(i, j int) bool { return climbScore(cands[i]) > climbScore(cands[j]) })
	for i := 0; i < len(cands) && i < decisiveClimbs; i++ {
		out[cands[i].StartKm] = true
	}
	return out
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

// cards are the team's riders worth racing for: leaders, sprinters and any
// rider the race marks as a favourite (a Van Aert riding for Jorgenson is
// still a card to play). Furthest up the road first, then leader, favourite,
// sprinter, then most reserve.
func (w *view) cards() []int64 {
	rank := func(r simulation.RiderBrief) int {
		switch {
		case r.Role == simulation.RoleLeader:
			return 0
		case r.Favourite:
			return 1
		case r.Role == simulation.RoleSprinter:
			return 2
		}
		return 3
	}
	var out []int64
	for id, r := range w.Riders {
		if r.Team == w.team && rank(r) < 3 {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if ga, gb := w.groupOf[a], w.groupOf[b]; ga != gb {
			return ga < gb
		}
		if ra, rb := rank(w.Riders[a]), rank(w.Riders[b]); ra != rb {
			return ra < rb
		}
		if wa, wb := w.Riders[a].WPrimePct, w.Riders[b].WPrimePct; wa != wb {
			return wa > wb
		}
		return a < b
	})
	return out
}

func (w *view) isCard(id int64) bool {
	for _, c := range w.cards() {
		if c == id {
			return true
		}
	}
	return false
}

// helpers returns the team's riders in group gi who aren't cards (and not
// except), most reserve first.
func (w *view) helpers(gi int, except int64) []int64 {
	var out []int64
	for _, id := range w.myRiders(gi) {
		if id != except && !w.isCard(id) {
			out = append(out, id)
		}
	}
	return out
}

// whereIs says where rider id is relative to group ref, with his reserve.
func (w *view) whereIs(id int64, ref int) string {
	g := w.groupOf[id]
	r := w.Riders[id]
	switch {
	case g == ref:
		return fmt.Sprintf("%s (%.0f%% reserve)", r.Name, r.WPrimePct)
	case g < ref:
		return fmt.Sprintf("%s (%s ahead, %.0f%%)", r.Name, fmtGap(w.gap(ref)-w.gap(g)), r.WPrimePct)
	default:
		return fmt.Sprintf("%s (%s behind, %.0f%%)", r.Name, fmtGap(w.gap(g)-w.gap(ref)), r.WPrimePct)
	}
}

func joinAnd(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}

// groupLabel names group gi as the player sees it.
func (w *view) groupLabel(gi int) string {
	switch {
	case gi == w.main && len(w.Road[gi].IDs) >= 15:
		return "the peloton"
	case gi == 0:
		return "the front group"
	case len(w.Road[gi].IDs) == 1:
		return "a group of his own"
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
		rules = append(rules, d.leaderDropped, d.keyClimb, d.cobbles, d.finale, d.breakaway)
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
		spacing := attackSpacingKm
		if km <= 10 {
			spacing = 1
		}
		if d.lastAttackKm-km < float64(spacing) {
			continue
		}
		if !att.Favourite && km > finalPhaseKm {
			continue // other teams' leaders matter only in the finale
		}
		ag := w.groupOf[ev.RiderID]
		// Who can answer: our cards in the attacker's group or close
		// behind; failing those, our freshest rider there.
		inReach := func(id int64) bool {
			g := w.groupOf[id]
			return g == ag || (g > ag && w.gap(g)-w.gap(ag) <= followGapS)
		}
		var answer []int64
		for _, c := range w.cards() {
			if inReach(c) && len(answer) < 2 {
				answer = append(answer, c)
			}
		}
		if len(answer) == 0 {
			for _, gi := range []int{ag, ag + 1} {
				if gi < len(w.Road) && w.gap(gi)-w.gap(ag) <= followGapS {
					if mine := w.myRiders(gi); len(mine) > 0 && w.Riders[mine[0]].WPrimePct >= 40 {
						answer = append(answer, mine[0])
						break
					}
				}
			}
		}
		if len(answer) == 0 {
			continue
		}
		d.attackAt[ev.RiderID] = km
		d.lastAttackKm = km
		var where []string
		for _, id := range answer {
			where = append(where, w.whereIs(id, ag))
		}
		m := &Moment{
			Kind:  "attack",
			Title: fmt.Sprintf("⚡ %s attacks!", att.Name),
			Text: fmt.Sprintf("%s (%s) goes with %.1f km to go%s. Ours: %s.",
				att.Name, att.Team, km, d.climbContext(km), joinAnd(where)),
		}
		for _, id := range answer {
			help := "Chase the move for the next 3 km: costs reserve, keeps him in the race."
			if w.Riders[id].WPrimePct < lowReservePct {
				help = fmt.Sprintf("Only %.0f%% reserve: he may blow trying.", w.Riders[id].WPrimePct)
			}
			m.Options = append(m.Options, Option{
				ID: fmt.Sprintf("follow-%d", id), Label: fmt.Sprintf("%s goes with him", w.name(id)),
				Help:     help,
				Commands: []Command{{RiderID: id, Action: "chase", Mode: "persistent", ForKm: 3}},
			})
		}
		first := answer[0]
		if hs := w.helpers(w.groupOf[first], first); len(hs) > 0 && w.Riders[hs[0]].WPrimePct >= 40 && w.isCard(first) {
			m.Options = append(m.Options, Option{
				ID: "mark", Label: fmt.Sprintf("%s chases for us", w.name(hs[0])),
				Help:     "A teammate closes the gap for 5 km; our leaders save their reserve in the draft.",
				Commands: []Command{{RiderID: hs[0], Action: "chase", Mode: "persistent", ForKm: 5}},
			})
		}
		for _, id := range answer {
			if w.Riders[id].WPrimePct >= 60 && km <= midPhaseKm {
				m.Options = append(m.Options, Option{
					ID: fmt.Sprintf("counter-%d", id), Label: fmt.Sprintf("%s counter-attacks", w.name(id)),
					Help:     "Go over the top of the move: a hard burst now.",
					Commands: []Command{{RiderID: id, Action: "attack", Mode: "oneshot"}},
				})
				break
			}
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
		if climbScore(c) < climbMinScore {
			continue
		}
		name := "a climb"
		if c.Name != "" {
			name = c.Name
		}
		switch {
		case pos >= c.StartKm && pos < c.StartKm+c.LengthKm:
			return fmt.Sprintf(", %.1f km from the top of %s (%.1f%%)", c.StartKm+c.LengthKm-pos, name, c.AvgPct)
		case c.StartKm-pos > 0 && c.StartKm-pos <= 1.5:
			return fmt.Sprintf(", at the foot of %s (%.1f km at %.1f%%)", name, c.LengthKm, c.AvgPct)
		}
	}
	for _, sec := range d.sectors {
		switch {
		case pos >= sec.StartKm && pos < sec.StartKm+sec.LengthKm:
			noun, _ := sectorWords(sec.Surface)
			return fmt.Sprintf(", on the %s of %s (%d★)", noun, sec.Name, sec.Stars)
		case sec.StartKm-pos > 0 && sec.StartKm-pos <= 1:
			noun, _ := sectorWords(sec.Surface)
			return fmt.Sprintf(", just before %s (%d★ %s)", sec.Name, sec.Stars, noun)
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
	cards := w.cards()
	if len(cards) == 0 {
		return nil
	}
	// The front that matters: the first group with a rival favourite, or
	// the peloton, whichever is ahead.
	ref := w.main
	for gi := 0; gi < ref; gi++ {
		for _, id := range w.Road[gi].IDs {
			if w.Riders[id].Favourite && !w.mine(id) {
				ref = gi
				break
			}
		}
	}
	// The card that's dropped, and any still up front.
	var p int64
	var front []int64
	for _, c := range cards {
		g := w.groupOf[c]
		if g <= ref {
			front = append(front, c)
			delete(d.droppedAsked, c) // back in front: may be asked again
		} else if gap := w.gap(g) - w.gap(ref); p == 0 && !d.droppedAsked[c] && gap >= droppedGapS && gap <= math.Max(60, 6*km) {
			// (further back than a chase could close: no decision left)
			p = c
		}
	}
	if p == 0 {
		return nil
	}
	pg := w.groupOf[p]
	behind := w.gap(pg) - w.gap(ref)
	d.droppedAt = km
	d.droppedAsked[p] = true
	pr := w.Riders[p]
	with := w.helpers(pg, p)
	var ahead []int64
	for gi := 0; gi < pg; gi++ {
		ahead = append(ahead, w.helpers(gi, p)...)
	}
	text := fmt.Sprintf("%s is %s behind %s with %.1f km to go (reserve %.0f%%, legs %.0f%%). Teammates with him: %d, up the road: %d.",
		pr.Name, fmtGap(behind), w.groupLabel(ref), km, pr.WPrimePct, pr.LegsPct, len(with), len(ahead))
	if len(front) > 0 {
		var names []string
		for _, c := range front {
			names = append(names, w.name(c))
		}
		text += fmt.Sprintf(" Still up front: %s.", joinAnd(names))
	}
	m := &Moment{Kind: "dropped", Title: fmt.Sprintf("🚨 %s is dropped", pr.Name), Text: text}
	switch {
	case pr.Role == simulation.RoleLeader && len(with)+len(ahead) > 0:
		// The engine's Protect order waits for the team leader.
		m.Options = append(m.Options, Option{
			ID: "wait", Label: "The team brings him back",
			Help:   "Teammates with him chase; those up the road sit up and wait (for 15 km).",
			Orders: []OrderChange{{Kind: "protect", Value: "yes", ForKm: 15}},
		})
	case len(with) > 0:
		var cmds []Command
		for _, h := range with {
			cmds = append(cmds, Command{RiderID: h, Action: "chase", Mode: "persistent", ForKm: 10})
		}
		m.Options = append(m.Options, Option{
			ID: "wait", Label: fmt.Sprintf("%d teammate(s) pace him back", len(with)),
			Help: "The teammates with him chase for 10 km; he sits on.", Commands: cmds,
		})
	}
	m.Options = append(m.Options, Option{
		ID: "solo", Label: fmt.Sprintf("%s chases alone", pr.Name),
		Help:     "He rides to close the gap himself for 5 km; the team races on.",
		Commands: []Command{{RiderID: p, Action: "chase", Mode: "persistent", ForKm: 5}},
	})
	if len(front) > 0 {
		m.Options = append(m.Options, Option{
			ID: "let", Label: fmt.Sprintf("Race for %s instead", w.name(front[0])),
			Help:   "Nobody waits: the team rides for the riders still in front.",
			Orders: []OrderChange{{Kind: "protect", Value: "no"}},
		})
	} else {
		m.Options = append(m.Options, Option{
			ID: "let", Label: "Race on without him",
			Help:   "Nobody waits: riders up the road race for themselves.",
			Orders: []OrderChange{{Kind: "protect", Value: "no"}},
		})
	}
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
	// Our riders in front, cards first.
	var front []int64
	for _, c := range w.cards() {
		if w.groupOf[c] == 0 {
			front = append(front, c)
		}
	}
	for _, id := range mineFront {
		if !w.isCard(id) {
			front = append(front, id)
		}
	}
	var where []string
	for _, id := range front {
		where = append(where, w.whereIs(id, 0))
	}
	text := fmt.Sprintf("%.1f km to go, front group of %d. Ours: %s", km, size, joinAnd(where))
	if len(w.Road) > 1 {
		text += fmt.Sprintf(". Next group at %s.", fmtGap(w.gap(1)))
	} else {
		text += "."
	}
	return &Moment{
		Kind:    "finale",
		Title:   "🏁 The finale",
		Text:    text,
		Options: finaleOptions(w, front, km),
	}
}

func finaleOptions(w *view, front []int64, km float64) []Option {
	var opts []Option
	for _, id := range front {
		if len(opts) == 2 || w.Riders[id].WPrimePct < lowReservePct {
			continue
		}
		opts = append(opts, Option{ID: fmt.Sprintf("attack-%d", id), Label: fmt.Sprintf("%s attacks now", w.name(id)),
			Help:     "Go clear and try to hold it to the line.",
			Commands: []Command{{RiderID: id, Action: "attack", Mode: "oneshot"}}})
	}
	var wait, work []Command
	for _, id := range front {
		wait = append(wait, Command{RiderID: id, Action: "conserve", Mode: "persistent", ForKm: math.Max(0, km-1)})
		work = append(work, Command{RiderID: id, Action: "chase", Mode: "persistent"})
	}
	return append(opts,
		Option{ID: "wait", Label: "Wait for the sprint", Help: "Sit on and save everything for the final kilometre.", Commands: wait},
		Option{ID: "work", Label: "Ride to keep the group away", Help: "Take turns so the chasers don't come back.", Commands: work})
}

// keyClimb: a decisive climb starts within a few km.
func (d *Director) keyClimb(w *view) *Moment {
	km := w.KmRemaining
	pos := d.totalKm - km
	if d.totalKm == 0 {
		return nil
	}
	// Our cards still in contention (dropped ones are the other moment).
	var inRace []int64
	for _, c := range w.cards() {
		if w.groupOf[c] <= w.main {
			inRace = append(inRace, c)
		}
	}
	if len(inRace) == 0 {
		return nil
	}
	p := inRace[0]
	pg := w.groupOf[p]
	for _, c := range d.climbs {
		toFoot := c.StartKm - pos
		// Up to a third of the way up still counts: the view may arrive late.
		if toFoot <= -c.LengthKm/3 || toFoot > climbLeadKm || !d.decisive[c.StartKm] {
			continue
		}
		toGoAtFoot := d.totalKm - c.StartKm
		key := fmt.Sprintf("climb-%.1f", c.StartKm)
		if d.fired[key] {
			continue
		}
		d.fired[key] = true
		top := d.totalKm - c.StartKm - c.LengthKm
		nearTop := d.totalKm - c.StartKm - 0.75*c.LengthKm
		title := fmt.Sprintf("⛰ Climb in %.1f km", toFoot)
		if c.Name != "" {
			title = fmt.Sprintf("⛰ %s in %.1f km", c.Name, toFoot)
		}
		if toFoot < 0.3 && toFoot > 0 {
			title = fmt.Sprintf("⛰ Onto %s now", c.label())
		}
		if toFoot <= 0 {
			title = fmt.Sprintf("⛰ On %s: %.1f km to the top", c.label(), km-top)
		}
		cobbled := ""
		if c.Surface == "gravel" {
			cobbled = fmt.Sprintf(", gravel (%d★)", c.Stars)
		} else if c.Stars > 0 {
			cobbled = fmt.Sprintf(", cobbled (%d★)", c.Stars)
		}
		m := &Moment{
			Kind:  "climb",
			Title: title,
			Text: fmt.Sprintf("%.1f km at %.1f%% (%.0f m)%s, the top %.1f km from the finish. Ours in %s: %s; %d teammates to help.",
				c.LengthKm, c.AvgPct, c.GainM, cobbled, top, w.groupLabel(pg), joinAnd(climbWhere(w, inRace, pg)), len(w.helpers(pg, 0))),
		}
		attackers := 0
		for _, id := range inRace {
			if attackers == 2 || w.Riders[id].WPrimePct < lowReservePct {
				continue
			}
			attackers++
			m.Options = append(m.Options, Option{ID: fmt.Sprintf("attack-top-%d", id), Label: fmt.Sprintf("%s attacks near the top", w.name(id)),
				Help:     "Follow the wheels, then go with a quarter of the climb left.",
				Commands: []Command{{RiderID: id, Action: "attack", Mode: "oneshot", AtKmRemaining: nearTop}}})
		}
		if toFoot > 0 && w.Riders[p].WPrimePct >= lowReservePct {
			m.Options = append(m.Options, Option{ID: "attack-foot", Label: fmt.Sprintf("%s attacks at the foot", w.name(p)),
				Help:     "A long attack: the whole climb to make the difference.",
				Commands: []Command{{RiderID: p, Action: "attack", Mode: "oneshot", AtKmRemaining: toGoAtFoot}}})
		}
		if len(w.helpers(pg, 0)) > 0 {
			m.Options = append(m.Options, Option{ID: "tempo", Label: "Team sets a hard tempo", Help: "Teammates drive the climb to thin the group for him.",
				Orders: []OrderChange{{Kind: "pace", Value: "yes", ForKm: km - top}}})
		}
		m.Options = append(m.Options, Option{ID: "sit", Label: "Sit in and follow", Help: "Save the reserve; react to the others."})
		return m
	}
	return nil
}

// cobbles: a decisive cobbled sector starts within a few km.
func (d *Director) cobbles(w *view) *Moment {
	km := w.KmRemaining
	pos := d.totalKm - km
	if d.totalKm == 0 || len(d.decisiveSectors) == 0 {
		return nil
	}
	var inRace []int64
	for _, c := range w.cards() {
		if w.groupOf[c] <= w.main {
			inRace = append(inRace, c)
		}
	}
	if len(inRace) == 0 {
		return nil
	}
	pg := w.groupOf[inRace[0]]
	for _, sec := range d.sectors {
		toStart := sec.StartKm - pos
		if toStart <= -sec.LengthKm/3 || toStart > climbLeadKm || !d.decisiveSectors[sec.StartKm] {
			continue
		}
		key := fmt.Sprintf("sector-%.1f", sec.StartKm)
		if d.fired[key] {
			continue
		}
		d.fired[key] = true
		end := d.totalKm - sec.StartKm - sec.LengthKm
		noun, icon := sectorWords(sec.Surface)
		title := fmt.Sprintf("%s %s in %.1f km", icon, sec.Name, toStart)
		if toStart < 0.3 && toStart > 0 {
			title = fmt.Sprintf("%s Into %s now", icon, sec.Name)
		}
		if toStart <= 0 {
			title = fmt.Sprintf("%s On %s: %.1f km of %s left", icon, sec.Name, km-end, noun)
		}
		m := &Moment{
			Kind:  noun,
			Title: title,
			Text: fmt.Sprintf("%.1f km of %d★ %s, ending %.1f km from the finish. Ours in %s: %s; %d teammates to help.",
				sec.LengthKm, sec.Stars, noun, end, w.groupLabel(pg), joinAnd(climbWhere(w, inRace, pg)), len(w.helpers(pg, 0))),
		}
		into := math.Max(0, toStart) + sec.LengthKm
		if len(w.helpers(pg, 0)) > 0 {
			m.Options = append(m.Options, Option{ID: "front", Label: "Team leads into the sector",
				Help:   fmt.Sprintf("Teammates ride the front onto the %s and through them: our leaders sit in the front and the group thins behind.", noun),
				Orders: []OrderChange{{Kind: "pace", Value: "yes", ForKm: into}}})
		}
		// Attack a third of the way in: past the fight for the entrance.
		at := d.totalKm - sec.StartKm - sec.LengthKm/3
		attackers := 0
		for _, id := range inRace {
			if attackers == 2 || w.Riders[id].WPrimePct < lowReservePct {
				continue
			}
			attackers++
			m.Options = append(m.Options, Option{ID: fmt.Sprintf("attack-%d", id), Label: fmt.Sprintf("%s attacks on the %s", w.name(id), noun),
				Help:     fmt.Sprintf("Go hard a third of the way into %s: the %s hurt those on the wheel too.", sec.Name, noun),
				Commands: []Command{{RiderID: id, Action: "attack", Mode: "oneshot", AtKmRemaining: math.Min(km, at)}}})
		}
		m.Options = append(m.Options, Option{ID: "sit", Label: "Sit in and follow", Help: fmt.Sprintf("Save the reserve; follow the wheels over the %s.", noun)})
		return m
	}
	return nil
}

func climbWhere(w *view, ids []int64, ref int) []string {
	var out []string
	for _, id := range ids {
		out = append(out, w.whereIs(id, ref))
	}
	return out
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
	for _, id := range ahead {
		if w.Riders[id].Favourite && !w.mine(id) {
			return nil // rival favourites up the road: that's the race, not a break
		}
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
		if len(ahead) == 1 {
			return &Moment{
				Kind:  "break",
				Title: fmt.Sprintf("🏃 %s is clear alone", rb.Name),
				Text:  fmt.Sprintf("%s is %s ahead of %s with %.0f km to go, reserve %.0f%%, legs %.0f%%.", rb.Name, fmtGap(gap), w.groupLabel(w.main), km, rb.WPrimePct, rb.LegsPct),
				Options: []Option{
					{ID: "commit", Label: "Commit: ride to the line", Help: "He rides his limit alone: a long way, but nobody to share the work with the chase either.",
						Commands: []Command{{RiderID: r, Action: "chase", Mode: "persistent"}}},
					{ID: "wait", Label: "Sit up and wait for the group", Help: "Save his legs for later; the group will catch him.",
						Commands: []Command{{RiderID: r, Action: "conserve", Mode: "persistent", ForKm: 5}}},
					{ID: "auto", Label: "Let him decide", Help: "He judges it himself."},
				},
			}
		}
		return &Moment{
			Kind:  "break",
			Title: fmt.Sprintf("🏃 %s is in the break", rb.Name),
			Text: fmt.Sprintf("He's in a break of %d, %s clear with %.0f km to go: %s.",
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
	text := fmt.Sprintf("%d riders from %d teams are %s up with %.0f km to go: %s.",
		len(ahead), len(teams), fmtGap(gap), km, names(w, ahead, 8))
	if len(ahead) == 1 {
		text = fmt.Sprintf("%s (%s) is %s up alone with %.0f km to go.", w.name(ahead[0]), w.Riders[ahead[0]].Team, fmtGap(gap), km)
	}
	return &Moment{
		Kind:  "break",
		Title: title,
		Text:  text,
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
