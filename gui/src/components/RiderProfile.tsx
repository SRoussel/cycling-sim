import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './RiderProfile.css'

/**
 * The rider panel (Nov 2026): his power-duration curve as the race model sees
 * it, against the field's 10th-90th percentile band and median, and every
 * rating as a percentile of the field. Opened from anywhere with
 * openRiderProfile(id, seasonId); RiderProfileHost (in Layout) renders it.
 */

const OPEN_EVENT = 'open-rider-profile'

/** Opens the rider panel; with a season, the field is that season's riders. */
export function openRiderProfile(riderId: number, seasonId = 0) {
  window.dispatchEvent(new CustomEvent(OPEN_EVENT, { detail: { riderId, seasonId } }))
}

/** A rider's name that opens his panel. */
export function RiderLink({ id, name, seasonId = 0, bold }: { id: number; name: string; seasonId?: number; bold?: boolean }) {
  return (
    <button
      type="button"
      className={`rider-link${bold ? ' bold' : ''}`}
      onClick={(e) => {
        e.stopPropagation()
        openRiderProfile(id, seasonId)
      }}
      title={`${name}: power curve and ratings`}
    >
      {name}
    </button>
  )
}

export function RiderProfileHost() {
  const [open, setOpen] = useState<{ riderId: number; seasonId: number } | null>(null)
  useEffect(() => {
    const on = (e: Event) => setOpen((e as CustomEvent).detail)
    window.addEventListener(OPEN_EVENT, on)
    return () => window.removeEventListener(OPEN_EVENT, on)
  }, [])
  if (!open) return null
  return <RiderProfilePanel key={`${open.riderId}-${open.seasonId}`} {...open} onClose={() => setOpen(null)} />
}

function fmtDur(s: number): string {
  if (s < 60) return `${s}s`
  if (s < 3600) return `${s / 60}m`
  return `${s / 3600}h`
}

function fmtValue(v: number, unit: string): string {
  switch (unit) {
    case 'W/kg':
      return v.toFixed(2)
    case 'm²':
      return v.toFixed(3)
    case 'kJ':
      return v.toFixed(1)
    case '':
      return v.toFixed(0)
    default:
      return Math.round(v).toString()
  }
}

/** A percentile as a whole number, 0-99 (a rider can't be above everyone). */
function pctNum(n: number): number {
  return Math.min(99, Math.max(0, Math.round(n)))
}

function ordinal(n: number): string {
  const r = pctNum(n)
  const s = r % 100 >= 11 && r % 100 <= 13 ? 'th' : ['th', 'st', 'nd', 'rd'][r % 10] ?? 'th'
  return `${r}${s}`
}

type Unit = 'wkg' | 'w'

function RiderProfilePanel({ riderId, seasonId, onClose }: { riderId: number; seasonId: number; onClose: () => void }) {
  const [profile, setProfile] = useState<main.RiderProfile | null>(null)
  const [compare, setCompare] = useState<main.RiderProfile | null>(null)
  const [names, setNames] = useState<main.ProfileRiderRef[]>([])
  const [query, setQuery] = useState('')
  const [unit, setUnit] = useState<Unit>('wkg')
  const [error, setError] = useState('')

  useEffect(() => {
    App.GetRiderProfile(riderId, seasonId).then(setProfile).catch((e) => setError(String(e)))
    App.GetProfileRiders(seasonId).then(setNames).catch(() => setNames([]))
  }, [riderId, seasonId])

  useEffect(() => {
    const key = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [onClose])

  const pickCompare = useCallback(
    (name: string) => {
      setQuery(name)
      const hit = names.find((n) => n.name === name)
      if (!hit || hit.id === riderId) return
      App.GetRiderProfile(hit.id, seasonId).then(setCompare).catch(() => setCompare(null))
    },
    [names, riderId, seasonId],
  )

  return (
    <div className="rp-overlay" onClick={onClose}>
      <div className="rp-panel" role="dialog" aria-modal="true" aria-label={profile ? `${profile.name}: profile` : 'Rider profile'} onClick={(e) => e.stopPropagation()}>
        <header className="rp-head">
          <div>
            <h1>{profile?.name ?? '…'}</h1>
            {profile && (
              <p className="muted">
                {profile.team || 'no team'} · {profile.age} · {profile.specialty} · percentiles among {profile.field === 'all riders' ? `all ${profile.fieldSize} riders` : `this season's ${profile.fieldSize} riders`}
              </p>
            )}
          </div>
          <button type="button" className="rp-close" onClick={onClose} aria-label="Close">×</button>
        </header>
        {error && <p className="warn">{error}</p>}
        {profile && (
          <>
            <div className="rp-controls">
              <label className="rp-compare">
                Compare with
                <input
                  list="rp-names"
                  value={query}
                  placeholder="rider name"
                  onChange={(e) => pickCompare(e.target.value)}
                />
                <datalist id="rp-names">
                  {names.map((n) => (
                    <option key={n.id} value={n.name}>{n.team}</option>
                  ))}
                </datalist>
              </label>
              {compare && (
                <button type="button" className="rp-clear" onClick={() => { setCompare(null); setQuery('') }}>
                  Clear
                </button>
              )}
              <div className="rp-toggle" role="radiogroup" aria-label="Power unit">
                <button type="button" role="radio" aria-checked={unit === 'wkg'} className={unit === 'wkg' ? 'on' : ''} onClick={() => setUnit('wkg')}>W/kg</button>
                <button type="button" role="radio" aria-checked={unit === 'w'} className={unit === 'w' ? 'on' : ''} onClick={() => setUnit('w')}>Watts</button>
              </div>
            </div>

            <section className="rp-section">
              <h2>Power curve</h2>
              <p className="rp-note muted">
                Fresh, on the flat, as the race model computes it: peak sprint, then threshold plus the anaerobic reserve spread over the effort,
                fading over long efforts with Endurance.
              </p>
              <PowerCurveChart profile={profile} compare={compare} unit={unit} />
              <PercentileStrip profile={profile} compare={compare} unit={unit} />
            </section>

            <section className="rp-section">
              <h2>Ratings</h2>
              <StatBars stats={profile.ratings} compare={compare?.ratings} />
            </section>

            <section className="rp-section">
              <h2>Body and engine</h2>
              <StatBars stats={profile.physical} compare={compare?.physical} showUnits />
            </section>
          </>
        )}
      </div>
    </div>
  )
}

// ---- Power curve -------------------------------------------------------

const W = 760
const H = 300
const M = { top: 26, right: 16, bottom: 30, left: 48 }
const X_TICKS = [5, 15, 60, 300, 1200, 3600, 4 * 3600]

function PowerCurveChart({ profile, compare, unit }: { profile: main.RiderProfile; compare: main.RiderProfile | null; unit: Unit }) {
  const [hover, setHover] = useState<number | null>(null)
  const svgRef = useRef<SVGSVGElement>(null)
  const pts = profile.curve
  const get = (p: main.CurvePoint) => (unit === 'wkg' ? p.wkg : p.watts)
  const band = (p: main.CurvePoint) => (unit === 'wkg' ? [p.wkgP10, p.wkgP50, p.wkgP90] : [p.wP10, p.wP50, p.wP90])

  const { x, y, yTicks } = useMemo(() => {
    const lo = Math.log(pts[0].seconds)
    const hi = Math.log(pts[pts.length - 1].seconds)
    let max = 0
    for (const p of pts) max = Math.max(max, get(p), band(p)[2])
    for (const p of compare?.curve ?? []) max = Math.max(max, get(p))
    const step = unit === 'wkg' ? (max > 20 ? 5 : 2) : max > 1200 ? 250 : 100
    const top = Math.ceil(max / step) * step
    const ticks: number[] = []
    for (let v = 0; v <= top + 1e-9; v += step) ticks.push(v)
    return {
      x: (s: number) => M.left + ((Math.log(s) - lo) / (hi - lo)) * (W - M.left - M.right),
      y: (v: number) => M.top + (1 - v / top) * (H - M.top - M.bottom),
      yTicks: ticks,
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pts, compare, unit])

  const line = (curve: main.CurvePoint[]) => curve.map((p, i) => `${i ? 'L' : 'M'}${x(p.seconds).toFixed(1)},${y(get(p)).toFixed(1)}`).join('')
  const area =
    pts.map((p, i) => `${i ? 'L' : 'M'}${x(p.seconds).toFixed(1)},${y(band(p)[2]).toFixed(1)}`).join('') +
    [...pts].reverse().map((p) => `L${x(p.seconds).toFixed(1)},${y(band(p)[0]).toFixed(1)}`).join('') +
    'Z'
  const median = pts.map((p, i) => `${i ? 'L' : 'M'}${x(p.seconds).toFixed(1)},${y(band(p)[1]).toFixed(1)}`).join('')

  const onMove = (e: React.MouseEvent<SVGSVGElement>) => {
    const svg = svgRef.current
    if (!svg) return
    const r = svg.getBoundingClientRect()
    const px = ((e.clientX - r.left) / r.width) * W
    let best = 0
    pts.forEach((p, i) => {
      if (Math.abs(x(p.seconds) - px) < Math.abs(x(pts[best].seconds) - px)) best = i
    })
    setHover(best)
  }

  const unitLabel = unit === 'wkg' ? 'W/kg' : 'W'
  const fmt = (v: number) => (unit === 'wkg' ? v.toFixed(2) : Math.round(v).toString())
  const hp = hover !== null ? pts[hover] : null
  const hc = hover !== null && compare ? compare.curve[hover] : null

  return (
    <div className="rp-chart">
      <ul className="rp-legend">
        <li><span className="key line rider" /> {profile.name}</li>
        {compare && <li><span className="key line compare" /> {compare.name}</li>}
        <li><span className="key line median" /> Field median</li>
        <li><span className="key band" /> Field 10th–90th percentile</li>
      </ul>
      <div className="rp-plot">
        <svg ref={svgRef} viewBox={`0 0 ${W} ${H}`} role="img" aria-label={`${profile.name}'s power curve in ${unitLabel} against the field`} onMouseMove={onMove} onMouseLeave={() => setHover(null)}>
          {yTicks.map((v) => (
            <g key={v}>
              <line className="grid" x1={M.left} x2={W - M.right} y1={y(v)} y2={y(v)} />
              <text className="tick" x={M.left - 8} y={y(v)} dy="0.32em" textAnchor="end">{unit === 'wkg' ? v : v}</text>
            </g>
          ))}
          {X_TICKS.map((s) => (
            <text key={s} className="tick" x={x(s)} y={H - 10} textAnchor="middle">{fmtDur(s)}</text>
          ))}
          <text className="tick axis-unit" x={M.left - 8} y={11} textAnchor="end">{unitLabel}</text>
          <path className="band" d={area} />
          <path className="median" d={median} />
          {compare && <path className="curve compare" d={line(compare.curve)} />}
          <path className="curve rider" d={line(pts)} />
          {hp && (
            <g>
              <line className="crosshair" x1={x(hp.seconds)} x2={x(hp.seconds)} y1={M.top} y2={H - M.bottom} />
              {hc && <circle className="dot compare" cx={x(hc.seconds)} cy={y(get(hc))} r={4} />}
              <circle className="dot rider" cx={x(hp.seconds)} cy={y(get(hp))} r={4} />
            </g>
          )}
        </svg>
        {hp && (
          <div className="rp-tip" style={{ left: `${(x(hp.seconds) / W) * 100}%` }}>
            <b>{fmtDur(hp.seconds)}</b>
            <div><span className="key line rider" /> {fmt(get(hp))} {unitLabel} · {ordinal(unit === 'wkg' ? hp.pctWkg : hp.pctW)} pct</div>
            {hc && <div><span className="key line compare" /> {fmt(get(hc))} {unitLabel} · {ordinal(unit === 'wkg' ? hc.pctWkg : hc.pctW)} pct</div>}
            <div className="muted">Field median {fmt(band(hp)[1])} · 10th–90th {fmt(band(hp)[0])}–{fmt(band(hp)[2])}</div>
          </div>
        )}
      </div>
    </div>
  )
}

const STRIP = [5, 30, 60, 120, 300, 600, 1200, 3600, 7200, 4 * 3600]

/** The rider's percentile in the field at each duration (numbers, shaded). */
function PercentileStrip({ profile, compare, unit }: { profile: main.RiderProfile; compare: main.RiderProfile | null; unit: Unit }) {
  const rows = [profile, ...(compare ? [compare] : [])]
  return (
    <table className="rp-strip">
      <caption className="muted">Percentile in the field ({unit === 'wkg' ? 'W/kg' : 'watts'})</caption>
      <thead>
        <tr>
          <th />
          {STRIP.map((s) => <th key={s}>{fmtDur(s)}</th>)}
        </tr>
      </thead>
      <tbody>
        {rows.map((p, i) => (
          <tr key={p.id}>
            <th><span className={`key line ${i ? 'compare' : 'rider'}`} /> {p.name.split(' ').slice(-1)[0]}</th>
            {STRIP.map((s) => {
              const pt = p.curve.find((c) => c.seconds === s)
              const pct = pt ? (unit === 'wkg' ? pt.pctWkg : pt.pctW) : 0
              return (
                <td key={s} style={{ background: `rgba(57, 135, 229, ${(0.06 + 0.6 * (pct / 100)).toFixed(2)})` }}>
                  {pctNum(pct)}
                </td>
              )
            })}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

// ---- Ratings -----------------------------------------------------------

function StatBars({ stats, compare, showUnits }: { stats: main.ProfileStat[]; compare?: main.ProfileStat[]; showUnits?: boolean }) {
  return (
    <ul className="rp-bars">
      {stats.map((s) => {
        const c = compare?.find((x) => x.key === s.key && x.unit === s.unit)
        return (
          <li key={`${s.key}-${s.unit}`}>
            <div className="rp-bar-label">
              <span>{s.label}{showUnits && s.unit ? <span className="muted"> ({s.unit})</span> : null}</span>
              <span className="rp-about muted">{s.about}</span>
            </div>
            <div className="rp-track" aria-hidden="true">
              <div className="rp-fill" style={{ width: `${Math.max(1, s.pct)}%` }} />
              {c && <div className="rp-mark" style={{ left: `${c.pct}%` }} />}
            </div>
            <div className="rp-bar-value">
              <b>{fmtValue(s.value, s.unit)}</b> <span className="muted">{ordinal(s.pct)} pct</span>
              {c && <div className="rp-cmp"><span className="key line compare" /> {fmtValue(c.value, c.unit)} <span className="muted">{ordinal(c.pct)} pct</span></div>}
            </div>
          </li>
        )
      })}
    </ul>
  )
}
