import './ProfileView.css'
import { useLayoutEffect, useRef, useState } from 'react'
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
  surface?: string
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
        c.surface = sec.surface
      }
    }
  }
  return climbs
}

export function stars(n: number): string {
  return n > 0 ? '★'.repeat(n) : ''
}

/** A group drawn on the profile: where it is, its colour and size. */
export interface GroupMark {
  km: number
  color: string
  size: number
  label: string
  mine: boolean
}

/** The element's size in CSS pixels, so the chart draws at its real size
 * (text and lines are never stretched). */
function useSize<T extends HTMLElement>() {
  const ref = useRef<T>(null)
  const [size, setSize] = useState({ w: 0, h: 0 })
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setSize({ w: e.contentRect.width, h: e.contentRect.height }))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, size] as const
}

/** A round elevation step giving three to five grid lines over the range. */
function elevStep(range: number): number {
  for (const s of [25, 50, 100, 200, 250, 500, 1000]) if (range / s <= 5) return s
  return 1000
}

/**
 * The route between fromKm and toKm, coloured by gradient (averaged over
 * ~half a kilometre, so the colours show climbs, not noise), with the
 * riders' position, km marks (as km to go, or km from the start) and, in
 * the detailed views, an elevation scale and each climb named with its
 * length and average gradient.
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
  detail = false,
  showPosition = true,
  marks = [],
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
  /** Elevation scale and climb labels (the road-ahead and whole-course views). */
  detail?: boolean
  /** Draw the riders' position (off before the start). */
  showPosition?: boolean
  marks?: GroupMark[]
}) {
  const [ref, { w: W, h: H }] = useSize<HTMLDivElement>()
  const total = profile.totalKm
  const span = Math.max(0.1, toKm - fromKm)
  const sectors = (profile.sectors || []).filter((sec) => sec.startKm + sec.lengthKm > fromKm && sec.startKm < toKm)
  const climbs = detail ? findClimbs(profile).filter((c) => c.startKm + c.lengthKm > fromKm && c.startKm + c.lengthKm <= toKm) : []

  // Plot box inside the element.
  const padL = detail ? 38 : 0
  const padR = detail ? 8 : 0
  const padT = detail && climbs.length > 0 ? 40 : 6
  const cobbleRow = sectors.length > 0 ? 9 : 0
  const nameRow = showSectorNames && sectors.length > 0 ? 14 : 0
  const padB = 16 + cobbleRow + nameRow
  const pw = Math.max(1, W - padL - padR)
  const ph = Math.max(1, H - padT - padB)
  const base = padT + ph

  const inner = profile.points.filter((p) => p.km > fromKm && p.km < toKm)
  const pts = [{ km: fromKm, elev: elevAt(profile.points, fromKm) }, ...inner, { km: toKm, elev: elevAt(profile.points, toKm) }]
  const rawLo = Math.min(...pts.map((p) => p.elev))
  const rawHi = Math.max(...pts.map((p) => p.elev))
  const range = Math.max(minElevRange, rawHi - rawLo)
  const step = elevStep(range)
  const lo = detail ? Math.floor((rawLo > -15 ? Math.max(0, rawLo) : rawLo) / step) * step : rawLo
  const hi = detail ? Math.max(lo + range, Math.ceil(rawHi / step) * step) : lo + range
  const x = (k: number) => padL + ((k - fromKm) / span) * pw
  const y = (e: number) => base - ((e - lo) / Math.max(1, hi - lo)) * ph * (detail ? 1 : 0.9)

  // Gradient bands: sample across the width, colour each slice by the
  // gradient over the surrounding ~0.5 km.
  const bands = []
  const n = Math.max(2, Math.min(600, Math.round(pw / 2)))
  const half = Math.max(0.25, span / n / 2)
  for (let i = 0; i < n && W > 0; i++) {
    const a = fromKm + (span * i) / n
    const b = fromKm + (span * (i + 1)) / n
    const mid = (a + b) / 2
    const pct = ((elevAt(profile.points, mid + half) - elevAt(profile.points, mid - half)) / (2 * half * 1000)) * 100
    bands.push(
      <polygon
        key={i}
        className={`pv-${gradeClass(pct)}`}
        points={`${x(a).toFixed(1)},${base} ${x(a).toFixed(1)},${y(elevAt(profile.points, a)).toFixed(1)} ${(x(b) + 0.6).toFixed(1)},${y(elevAt(profile.points, b)).toFixed(1)} ${(x(b) + 0.6).toFixed(1)},${base}`}
      />,
    )
  }
  const line = pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.km).toFixed(1)} ${y(p.elev).toFixed(1)}`).join(' ')

  // Marks at round distances to go (or from the start).
  const ticks: Array<{ k: number; label: string }> = []
  if (labels === 'toGo') {
    const firstToGo = Math.floor((total - fromKm) / tickEvery) * tickEvery
    for (let tg = firstToGo; tg >= 0 && total - tg <= toKm; tg -= tickEvery) {
      if (total - tg >= fromKm) ticks.push({ k: total - tg, label: tg === 0 ? 'Finish' : `${tg}` })
    }
  } else {
    for (let k = Math.ceil(fromKm / tickEvery) * tickEvery; k <= toKm; k += tickEvery) {
      ticks.push({ k, label: `${k}` })
    }
  }

  // Climb labels on two rows, skipping one that would overlap its row.
  const charW = 6.4
  const rowFree = [-Infinity, -Infinity]
  const climbLabels = climbs.flatMap((c) => {
    const top = c.startKm + c.lengthKm
    const name = c.name || 'Climb'
    const spec = `${c.lengthKm.toFixed(1)} km ${c.avgPct.toFixed(1)}%`
    const width = (name.length + spec.length + 1) * charW
    const cx = x(top)
    const left = Math.max(padL, Math.min(W - padR - width, cx - width / 2))
    const row = rowFree[0] <= left ? 0 : rowFree[1] <= left ? 1 : -1
    if (row < 0) return []
    rowFree[row] = left + width + 10
    const state = !showPosition ? '' : top < km ? 'done' : climbs.find((d) => d.startKm + d.lengthKm >= km) === c ? 'next' : ''
    return [{ c, name, spec, cx, tx: left + width / 2, ty: row === 0 ? 14 : 30, py: y(elevAt(profile.points, top)), state }]
  })

  // Sector names left to right, skipping one that would run into the last.
  const sectorNames: Array<{ sec: (typeof sectors)[number]; left: number }> = []
  let freeFrom = -Infinity
  for (const sec of showSectorNames ? sectors : []) {
    const left = x(Math.max(fromKm, sec.startKm))
    if (left < freeFrom) continue
    sectorNames.push({ sec, left })
    freeFrom = left + (sec.name.length + sec.stars + 2) * 5.6
  }

  const nowX = x(Math.max(fromKm, Math.min(km, toKm)))
  const showNow = showPosition && km >= fromKm && km <= toKm

  return (
    <div ref={ref} className={`pv ${className || ''}`}>
      {W > 0 && H > 0 && (
        <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} role="img" aria-label="Route profile coloured by gradient">
          {detail && Array.from({ length: Math.floor((hi - lo) / step) + 1 }, (_, i) => lo + i * step).map((e) => (
            <g key={e}>
              <line x1={padL} x2={W - padR} y1={y(e)} y2={y(e)} className="pv-grid" />
              <text x={padL - 6} y={y(e) + 3.5} className="pv-axis" textAnchor="end">{e}</text>
            </g>
          ))}
          {ticks.map((t) => (
            <line key={t.k} x1={x(t.k)} x2={x(t.k)} y1={padT} y2={base} className="pv-tick" />
          ))}
          {bands}
          <path d={line} className="pv-line" />
          <rect x={padL} y={0} width={Math.max(0, nowX - padL)} height={base} className="pv-done" />
          <line x1={padL} x2={W - padR} y1={base} y2={base} className="pv-base" />
          {sectors.map((sec) => (
            <rect
              key={`s${sec.startKm}`}
              x={x(Math.max(fromKm, sec.startKm))}
              y={base + 3}
              width={Math.max(2, x(Math.min(toKm, sec.startKm + sec.lengthKm)) - x(Math.max(fromKm, sec.startKm)))}
              height={5}
              rx={1.5}
              className={`pv-cobbles${sec.surface === 'gravel' ? ' gravel' : ''} s${Math.min(5, Math.max(1, sec.stars || 3))}${sec.startKm + sec.lengthKm < km ? ' done' : ''}`}
            >
              <title>{`${sec.name}: ${sec.lengthKm.toFixed(1)} km of ${sec.surface === 'gravel' ? 'gravel' : 'cobbles'}, ${sec.stars}★`}</title>
            </rect>
          ))}
          {ticks.map((t) => (
            <text key={t.k} x={x(t.k)} y={base + cobbleRow + 13} className="pv-km" textAnchor={t.label === 'Finish' ? 'end' : 'middle'}>{t.label}</text>
          ))}
          {sectorNames.map(({ sec, left }) => (
            <text key={sec.startKm} x={left} y={base + cobbleRow + 27} className={`pv-sector${sec.surface === 'gravel' ? ' gravel' : ''}`}>
              {sec.name} {stars(sec.stars)}
            </text>
          ))}
          {climbLabels.map((l) => (
            <g key={l.c.startKm} className={`pv-climb ${l.state}`}>
              <line x1={l.cx} x2={l.cx} y1={l.ty + 4} y2={l.py - 3} className="pv-leader" />
              <text x={l.tx} y={l.ty} textAnchor="middle">
                <tspan className="pv-climb-name">{l.name}</tspan> <tspan className="pv-climb-spec">{l.spec}</tspan>
              </text>
            </g>
          ))}
          {showNow && (
            <g>
              <line x1={nowX} x2={nowX} y1={padT - 4} y2={base} className="pv-now" />
              <circle cx={nowX} cy={y(elevAt(profile.points, km))} r={detail ? 5 : 3.5} className="pv-now-dot" />
            </g>
          )}
        </svg>
      )}
      <div className="pv-marks">
        {W > 0 && marks
          .filter((m) => m.km >= fromKm && m.km <= toKm)
          .map((m, i) => {
            const d = Math.max(9, Math.min(22, 7 + Math.sqrt(m.size) * 2.2))
            return (
              <span
                key={i}
                className={`pv-mark${m.mine ? ' mine' : ''}`}
                title={m.label}
                style={{ left: x(m.km), top: y(elevAt(profile.points, m.km)), width: d, height: d, background: m.color }}
              >
                {m.size > 1 && d >= 14 ? m.size : ''}
              </span>
            )
          })}
      </div>
    </div>
  )
}
