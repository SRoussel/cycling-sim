import { RiderTelemetryPoint } from '../../sim/shared'

/** Colour for a 0-100 energy level: green when fresh, amber, then red. */
function levelClass(pct: number): string {
  if (pct >= 60) return 'ok'
  if (pct >= 30) return 'low'
  return 'empty'
}

/**
 * A rider's energy: Reserve is W' (the anaerobic tank attacks and surges
 * draw on; it refills when riding easier) and Legs is the threshold he can
 * still hold as a share of his fresh one (it only falls over the day).
 */
export function EnergyBars({ telemetry, compact = false }: { telemetry?: RiderTelemetryPoint | null; compact?: boolean }) {
  const reserve = Math.max(0, Math.min(100, telemetry?.wPrimePct ?? 100))
  const legs = Math.max(0, Math.min(100, telemetry?.legsPct ?? 100))
  return (
    <div className={`energy-bars${compact ? ' compact' : ''}`}>
      <div className="energy-row" title="Reserve (W'): the anaerobic tank for attacks and surges; refills when riding easier">
        <span className="energy-label">Reserve</span>
        <span className="energy-track">
          <span className={`energy-fill ${levelClass(reserve)}`} style={{ width: `${reserve}%` }} />
        </span>
        <span className="energy-value">{Math.round(reserve)}%</span>
      </div>
      <div className="energy-row" title="Legs: the power he can still sustain, as a share of his fresh threshold">
        <span className="energy-label">Legs</span>
        <span className="energy-track">
          <span className={`energy-fill legs ${levelClass((legs - 80) * 5)}`} style={{ width: `${legs}%` }} />
        </span>
        <span className="energy-value">{Math.round(legs)}%</span>
      </div>
    </div>
  )
}
