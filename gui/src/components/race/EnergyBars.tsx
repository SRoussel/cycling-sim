import { RiderTelemetryPoint } from '../../sim/shared'

/** Colour for a 0-100 reserve level: green when fresh, amber, then red. */
function levelClass(pct: number): string {
  if (pct >= 60) return 'ok'
  if (pct >= 30) return 'low'
  return 'empty'
}

// The effort bar spans 0-150% of threshold; 100% is marked.
const EFFORT_SCALE = 150

/** Colour for effort: sitting in, working, or above threshold (burning reserve). */
function effortClass(pct: number): string {
  if (pct > 100) return 'over'
  if (pct >= 80) return 'working'
  return 'easy'
}

/**
 * A rider's state at a glance. Reserve is W' (the anaerobic tank attacks and
 * surges draw on; it refills when riding easier). Effort is his power now as
 * a share of what he can sustain: above 100% he is burning reserve.
 */
export function EnergyBars({ telemetry, compact = false }: { telemetry?: RiderTelemetryPoint | null; compact?: boolean }) {
  const reserve = Math.max(0, Math.min(100, telemetry?.wPrimePct ?? 100))
  const effort = Math.max(0, telemetry?.effortPct ?? 0)
  return (
    <div className={`energy-bars${compact ? ' compact' : ''}`}>
      <div className="energy-row" title="Reserve (W'): the anaerobic tank for attacks and surges; refills when riding easier">
        <span className="energy-label">Reserve</span>
        <span className="energy-track">
          <span className={`energy-fill ${levelClass(reserve)}`} style={{ width: `${reserve}%` }} />
        </span>
        <span className="energy-value">{Math.round(reserve)}%</span>
      </div>
      <div className="energy-row" title="Effort: power now as a share of what he can sustain. Over 100% (red) he is burning reserve.">
        <span className="energy-label">Effort</span>
        <span className="energy-track effort">
          <span className={`energy-fill effort-${effortClass(effort)}`} style={{ width: `${Math.min(100, (effort / EFFORT_SCALE) * 100)}%` }} />
          <span className="effort-threshold" style={{ left: `${(100 / EFFORT_SCALE) * 100}%` }} />
        </span>
        <span className="energy-value">{Math.round(effort)}%</span>
      </div>
    </div>
  )
}

/**
 * Legs: the threshold he can still hold as a share of his fresh one. It only
 * falls over the day (to ~88-92% in a hard classic), so it is a number, not a
 * bar: amber under 92%, red under 88%.
 */
export function LegsBadge({ telemetry }: { telemetry?: RiderTelemetryPoint | null }) {
  const legs = telemetry?.legsPct
  if (legs === undefined) return null
  const cls = legs < 88 ? 'empty' : legs < 92 ? 'low' : 'ok'
  return (
    <span className={`legs-badge ${cls}`} title="Legs: the power he can still sustain, as a share of his fresh threshold. Only falls over the day.">
      Legs {Math.round(legs)}%
    </span>
  )
}
