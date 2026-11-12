import { RouteProfile } from '../../sim/shared'

/** Gradient classes, from the profile's own slope between points. */
function gradeClass(pct: number): string {
  if (pct >= 9) return 'g4'
  if (pct >= 6) return 'g3'
  if (pct >= 3) return 'g2'
  if (pct >= 1) return 'g1'
  return 'g0'
}

function elevAt(points: RouteProfile['points'], km: number): number {
  if (points.length === 0) return 0
  if (km <= points[0].km) return points[0].elev
  for (let i = 1; i < points.length; i++) {
    const a = points[i - 1]
    const b = points[i]
    if (km <= b.km) {
      const t = b.km > a.km ? (km - a.km) / (b.km - a.km) : 0
      return a.elev + (b.elev - a.elev) * t
    }
  }
  return points[points.length - 1].elev
}

export interface Climb {
  startKm: number
  lengthKm: number
  gainM: number
  avgPct: number
  name?: string
  stars?: number
}

/**
 * Climbs on the route: stretches averaging 3%+ that gain at least 40 m
 * (short dips inside a climb don't end it).
 */
export function findClimbs(profile: RouteProfile | null): Climb[] {
  const pts = profile?.points || []
  const climbs: Climb[] = []
  let start = -1
  let lastUp = -1
  const close = () => {
    if (start < 0 || lastUp <= start) return
    const len = pts[lastUp].km - pts[start].km
    const gain = pts[lastUp].elev - pts[start].elev
    if (len > 0 && gain >= 40 && gain / (len * 10) >= 3) {
      climbs.push({ startKm: pts[start].km, lengthKm: len, gainM: gain, avgPct: gain / (len * 10) })
    }
  }
  for (let i = 1; i < pts.length; i++) {
    const dk = pts[i].km - pts[i - 1].km
    const pct = dk > 0 ? ((pts[i].elev - pts[i - 1].elev) / (dk * 1000)) * 100 : 0
    if (pct >= 2) {
      if (start < 0) start = i - 1
      lastUp = i
    } else if (start >= 0) {
      // A climb ends after 1 km without climbing.
      if (pts[i].km - pts[lastUp].km > 1) {
        close()
        start = -1
      }
    }
  }
  close()
  // A cobbled sector on a climb names it (the Flemish bergs).
  for (const c of climbs) {
    let best = 0
    for (const sec of profile?.sectors || []) {
      const o = Math.max(0, Math.min(c.startKm + c.lengthKm, sec.startKm + sec.lengthKm) - Math.max(c.startKm, sec.startKm))
      if (o > best && o >= 0.3 * sec.lengthKm) {
        best = o
        c.name = sec.name
        c.stars = sec.stars
      }
    }
  }
  return climbs
}

export function stars(n: number): string {
  return n > 0 ? '★'.repeat(n) : ''
}

const W = 1000
const H = 100

/**
 * The route between fromKm and toKm, coloured by gradient, with the riders'
 * position and km marks (as km to go, or km from the start).
 */
export function ProfileView({
  profile,
  km,
  fromKm,
  toKm,
  tickEvery,
  minElevRange = 150,
  labels = 'toGo',
  className,
  showSectorNames = false,
}: {
  profile: RouteProfile
  km: number
  fromKm: number
  toKm: number
  tickEvery: number
  minElevRange?: number
  labels?: 'toGo' | 'km'
  className?: string
  showSectorNames?: boolean
}) {
  const total = profile.totalKm
  const span = Math.max(0.1, toKm - fromKm)
  const inner = profile.points.filter((p) => p.km > fromKm && p.km < toKm)
  const pts = [{ km: fromKm, elev: elevAt(profile.points, fromKm) }, ...inner, { km: toKm, elev: elevAt(profile.points, toKm) }]
  const lo = Math.min(...pts.map((p) => p.elev))
  const hi = Math.max(...pts.map((p) => p.elev))
  const range = Math.max(minElevRange, hi - lo)
  const x = (k: number) => ((k - fromKm) / span) * W
  const y = (e: number) => H - 4 - ((e - lo) / range) * (H - 18)

  const polys = []
  for (let i = 1; i < pts.length; i++) {
    const a = pts[i - 1]
    const b = pts[i]
    const dk = b.km - a.km
    if (dk <= 0) continue
    const pct = ((b.elev - a.elev) / (dk * 1000)) * 100
    polys.push(
      <polygon
        key={i}
        className={`pv-${gradeClass(pct)}`}
        points={`${x(a.km)},${H} ${x(a.km)},${y(a.elev)} ${x(b.km)},${y(b.elev)} ${x(b.km)},${H}`}
      />,
    )
  }
  const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.km).toFixed(1)} ${y(p.elev).toFixed(1)}`).join(' ')

  // Marks at round distances to go (or from the start).
  const ticks: Array<{ k: number; label: string }> = []
  if (labels === 'toGo') {
    const firstToGo = Math.floor((total - fromKm) / tickEvery) * tickEvery
    for (let tg = firstToGo; tg >= 0 && total - tg <= toKm; tg -= tickEvery) {
      if (total - tg >= fromKm) ticks.push({ k: total - tg, label: tg === 0 ? '🏁' : `${tg}` })
    }
  } else {
    for (let k = Math.ceil(fromKm / tickEvery) * tickEvery; k <= toKm; k += tickEvery) {
      ticks.push({ k, label: `${k}` })
    }
  }
  const markerPct = Math.max(0, Math.min(100, ((km - fromKm) / span) * 100))
  const sectors = (profile.sectors || []).filter((sec) => sec.startKm + sec.lengthKm > fromKm && sec.startKm < toKm)
  // Name sectors left to right, skipping a name that would run into the
  // previous one (~0.55% of the width per character at this size).
  const labelled: Array<{ sec: (typeof sectors)[number]; left: number }> = []
  let freeFrom = -Infinity
  for (const sec of sectors) {
    const left = ((Math.max(fromKm, sec.startKm) - fromKm) / span) * 100
    if (left < freeFrom) continue
    labelled.push({ sec, left })
    freeFrom = left + (sec.name.length + sec.stars + 2) * 0.55
  }

  return (
    <div className={`pv ${className || ''}`}>
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" aria-label="Route profile">
        {polys}
        <path d={line} className="pv-line" />
        {ticks.map((t) => (
          <line key={t.k} x1={x(t.k)} x2={x(t.k)} y1={0} y2={H} className="pv-tick" />
        ))}
        {sectors.map((sec) => (
          <rect
            key={`s${sec.startKm}`}
            x={x(Math.max(fromKm, sec.startKm))}
            y={H - 7}
            width={Math.max(2, x(Math.min(toKm, sec.startKm + sec.lengthKm)) - x(Math.max(fromKm, sec.startKm)))}
            height={7}
            className={`pv-cobbles s${Math.min(5, Math.max(1, sec.stars || 3))}`}
          />
        ))}
        <rect x={0} y={0} width={x(Math.max(fromKm, Math.min(km, toKm)))} height={H} className="pv-done" />
      </svg>
      <div className="pv-marker" style={{ left: `${markerPct}%` }} />
      <div className="pv-labels">
        {ticks.map((t) => (
          <span key={t.k} style={{ left: `${((t.k - fromKm) / span) * 100}%` }}>{t.label}</span>
        ))}
      </div>
      {showSectorNames && (
        <div className="pv-sector-names">
          {labelled.map(({ sec, left }) => (
            <span key={sec.startKm} style={{ left: `${left}%` }} title={`${sec.name}: ${sec.lengthKm.toFixed(1)} km, ${sec.stars}★`}>
              {sec.name} {stars(sec.stars)}
            </span>
          ))}
        </div>
      )}
      <div className="pv-elev">{Math.round(hi - lo)} m</div>
    </div>
  )
}
