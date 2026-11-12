package season

import (
	"math"
	"sort"
	"sync"

	"cycling_sim/internal/database"
	"cycling_sim/internal/model"
	"cycling_sim/internal/selection"
	"cycling_sim/internal/service"
	"cycling_sim/internal/simulation"
)

// How well a rider suits a race, measured as the race engine sees riders:
// time over the course's hard sections (climbs, cobbles, gravel) ridden at
// threshold plus anaerobic reserve, as one-day leaders are picked
// (simulation.HardTime); a time trial's whole course at threshold; and the
// sprint (peak power per drag area) where the race comes down to one.

// sprintFreeMin: a course whose hard sections take the field (median) this
// long or more is decided on them; under it the sprint counts more, all of
// it on a course with none.
const sprintFreeMin = 40.0

// gcHardMin: a stage counts for the GC when its hard sections take the
// field at least this long (a mountain stage), or it is a time trial.
const gcHardMin = 15.0

type stageCourse struct {
	id      int64
	segs    []model.Segment
	tt      bool
	profile selection.CourseProfile // the selector's demands (ratings)
}

// eventCourses is an event's courses: a one-day race's, or each stage's.
type eventCourses struct {
	stageRace bool
	stages    []stageCourse
}

var (
	coursesMu    sync.Mutex
	coursesCache = map[*database.DB]map[[2]int64]*eventCourses{}
	timesMu      sync.Mutex
	timesCache   = map[timeKey][2]float64{}
)

type timeKey struct {
	r      model.Rider
	course int64
}

// eventProfiles loads an event's courses (cached per database).
func eventProfiles(db *database.DB, e model.SeasonEvent) (*eventCourses, error) {
	key := [2]int64{0, e.RefID}
	if e.Kind == model.EventStageRace {
		key[0] = 1
	}
	coursesMu.Lock()
	defer coursesMu.Unlock()
	if coursesCache[db] == nil {
		coursesCache[db] = map[[2]int64]*eventCourses{}
	}
	if ev, ok := coursesCache[db][key]; ok {
		return ev, nil
	}
	ev := &eventCourses{stageRace: e.Kind == model.EventStageRace}
	if !ev.stageRace {
		race, err := db.GetRace(e.RefID)
		if err != nil {
			return nil, err
		}
		course, err := db.GetCourse(race.CourseID)
		if err != nil {
			return nil, err
		}
		ev.stages = []stageCourse{{course.ID, course.Segments, race.RaceType == model.RaceTypeTT, selection.AnalyzeCourse(course, race.RaceType)}}
	} else {
		sr, err := db.GetStageRace(e.RefID)
		if err != nil {
			return nil, err
		}
		_, infos, err := service.BuildStageConfigs(db, sr, false)
		if err != nil {
			return nil, err
		}
		for _, in := range infos {
			ev.stages = append(ev.stages, stageCourse{in.Course.ID, in.Course.Segments, in.RaceType == model.RaceTypeTT, selection.AnalyzeCourse(in.Course, in.RaceType)})
		}
	}
	coursesCache[db][key] = ev
	return ev, nil
}

// times: a rider's hard-section time (0 if the course has none) and whole
// course time at threshold, in seconds (cached per rider and course).
func times(r model.Rider, c stageCourse) (hard, whole float64) {
	k := timeKey{r, c.id}
	timesMu.Lock()
	t, ok := timesCache[k]
	timesMu.Unlock()
	if ok {
		return t[0], t[1]
	}
	if c.tt {
		whole = simulation.CourseTime(r, c.segs)
	} else if h, any := simulation.HardTime(r, c.segs); any {
		hard = h
	}
	timesMu.Lock()
	timesCache[k] = [2]float64{hard, whole}
	timesMu.Unlock()
	return hard, whole
}

// zScores turns values (higher better) into standard deviations above the
// field's mean.
func zScores(vals map[int64]float64) map[int64]float64 {
	var sum, sq float64
	for _, v := range vals {
		sum += v
		sq += v * v
	}
	n := float64(len(vals))
	out := make(map[int64]float64, len(vals))
	if n == 0 {
		return out
	}
	mean := sum / n
	sd := math.Sqrt(math.Max(sq/n-mean*mean, 1e-12))
	for id, v := range vals {
		out[id] = (v - mean) / sd
	}
	return out
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	return s[len(s)/2]
}

// courseFit: how far above the field each rider is on one course: a time
// trial on its whole course; otherwise his hard-section time, blended with
// the sprint as far as the hard sections are short.
func courseFit(c stageCourse, riders []model.Rider, sprint map[int64]float64) (map[int64]float64, float64) {
	hard := make(map[int64]float64, len(riders))
	var ts []float64
	for _, r := range riders {
		h, w := times(r, c)
		if c.tt {
			h = w
		}
		if h > 0 {
			hard[r.ID] = -h
			ts = append(ts, h)
		}
	}
	if len(ts) == 0 {
		return sprint, 0
	}
	hardMin := median(ts) / 60
	zh := zScores(hard)
	if c.tt {
		return zh, hardMin
	}
	share := math.Max(0, 1-hardMin/sprintFreeMin)
	out := make(map[int64]float64, len(riders))
	for _, r := range riders {
		out[r.ID] = (1-share)*zh[r.ID] + share*sprint[r.ID]
	}
	return out, hardMin
}

func sprintZ(riders []model.Rider) map[int64]float64 {
	vals := make(map[int64]float64, len(riders))
	for i := range riders {
		vals[riders[i].ID] = simulation.SprintPower(&riders[i]) / riders[i].CdA
	}
	return zScores(vals)
}

// gcFits: a one-day race's fit, or a stage race's GC: time over its
// mountain stages' hard sections plus its time trials.
func physicsGC(ev *eventCourses, riders []model.Rider) map[int64]float64 {
	sprint := sprintZ(riders)
	if !ev.stageRace {
		f, _ := courseFit(ev.stages[0], riders, sprint)
		return f
	}
	total := make(map[int64]float64, len(riders))
	counted := false
	for _, c := range ev.stages {
		var ts []float64
		for _, r := range riders {
			h, w := times(r, c)
			ts = append(ts, h+w)
		}
		if !c.tt && median(ts)/60 < gcHardMin {
			continue
		}
		counted = true
		for _, r := range riders {
			h, w := times(r, c)
			total[r.ID] -= h + w
		}
	}
	if !counted {
		return sprint
	}
	return zScores(total)
}

// fits rates how well each rider suits an event, in standard deviations above
// the field: a one-day race on its course; a stage race on its GC or on its
// third-best stage (a sprinter or stage hunter with several chances).
func physicsFits(ev *eventCourses, riders []model.Rider) map[int64]float64 {
	out := physicsGC(ev, riders)
	if !ev.stageRace {
		return out
	}
	sprint := sprintZ(riders)
	var stages []map[int64]float64
	for _, c := range ev.stages {
		if c.tt {
			continue
		}
		f, _ := courseFit(c, riders, sprint)
		stages = append(stages, f)
	}
	if len(stages) == 0 {
		return out
	}
	for id, gc := range out {
		zs := make([]float64, 0, len(stages))
		for _, st := range stages {
			zs = append(zs, st[id])
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(zs)))
		out[id] = math.Max(gc, zs[min(2, len(zs)-1)])
	}
	return out
}

// ratingFits: the same from the 1-100 ratings against the selector's course
// demands (they carry what the physics proxy misses: handling on cobbles,
// a sprint that survives a hard day). gcOnly: a stage race's GC profile
// (mountain and time-trial stages) only.
func ratingFits(ev *eventCourses, riders []model.Rider, gcOnly bool) map[int64]float64 {
	z := func(p selection.CourseProfile) map[int64]float64 {
		vals := make(map[int64]float64, len(riders))
		for i := range riders {
			vals[riders[i].ID] = selection.RiderScore(&riders[i], p)
		}
		return zScores(vals)
	}
	if !ev.stageRace {
		return z(ev.stages[0].profile)
	}
	var each, hard []selection.CourseProfile
	for _, c := range ev.stages {
		each = append(each, c.profile)
		if c.tt || c.profile.ClimbingDemand > 0.3 {
			hard = append(hard, c.profile)
		}
	}
	if len(hard) == 0 {
		hard = each
	}
	out := z(selection.AverageProfile(hard))
	if gcOnly {
		return out
	}
	stages := make([]map[int64]float64, 0, len(each))
	for _, p := range each {
		stages = append(stages, z(p))
	}
	for id, gc := range out {
		zs := make([]float64, 0, len(stages))
		for _, st := range stages {
			zs = append(zs, st[id])
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(zs)))
		out[id] = math.Max(gc, zs[min(2, len(zs)-1)])
	}
	return out
}

// physicsWeight: the share of a fit from the physics measures (the rest
// from the ratings).
const physicsWeight = 0.5

func blend(a, b map[int64]float64) map[int64]float64 {
	out := make(map[int64]float64, len(a))
	for id, v := range a {
		out[id] = physicsWeight*v + (1-physicsWeight)*b[id]
	}
	return out
}

// gcFits: how well riders suit a one-day race, or a stage race's GC.
func gcFits(ev *eventCourses, riders []model.Rider) map[int64]float64 {
	return blend(physicsGC(ev, riders), ratingFits(ev, riders, true))
}

// fits rates how well each rider suits an event, in standard deviations above
// the field: a one-day race on its course; a stage race on its GC or on its
// third-best stage (a sprinter or stage hunter with several chances).
func fits(ev *eventCourses, riders []model.Rider) map[int64]float64 {
	return blend(physicsFits(ev, riders), ratingFits(ev, riders, false))
}
