import './PlanForm.css'
import { director, main } from '../../../wailsjs/go/models'

export interface PlanValue {
  leaderId: number
  break: string
  hardOn: string[]
  attackOn: string
}

export const EMPTY_PLAN: PlanValue = { leaderId: 0, break: 'auto', hardOn: [], attackOn: '' }

const wkg = (r: main.RiderInfo) => (r.weight > 0 ? r.ftp / r.weight : 0)
const icon = (kind: string) => (kind === 'climb' ? '⛰' : kind === 'gravel' ? '💨' : '🪨')

/**
 * The race plan's choices: who the team rides for, the break, where the
 * leader attacks and where the team makes it hard. Used before a one-day
 * race and before each stage of a stage race.
 */
export function PlanForm({
  riders,
  points,
  value,
  onChange,
}: {
  riders: main.RiderInfo[]
  points: director.PlanPoint[]
  value: PlanValue
  onChange: (v: PlanValue) => void
}) {
  const set = (patch: Partial<PlanValue>) => onChange({ ...value, ...patch })
  return (
    <div className="plan-form">
      <div className="plan-grid">
        <label>
          <span>Ride for</span>
          <select value={value.leaderId} onChange={(e) => set({ leaderId: Number(e.target.value) })}>
            <option value={0}>Team decides</option>
            {riders.map((r) => (
              <option key={r.id} value={r.id}>{r.name} ({wkg(r).toFixed(1)} W/kg, {r.ftp} W)</option>
            ))}
          </select>
        </label>
        <label>
          <span>The break</span>
          <select value={value.break} onChange={(e) => set({ break: e.target.value })}>
            <option value="auto">Team decides</option>
            <option value="yes">Put a rider in the break</option>
            <option value="no">Nobody in the break</option>
          </select>
        </label>
        <label>
          <span>Leader attacks on</span>
          <select value={value.attackOn} onChange={(e) => set({ attackOn: e.target.value })} disabled={points.length === 0}>
            <option value="">Wherever he sees fit</option>
            {points.map((p) => (
              <option key={p.key} value={p.key}>{p.label}</option>
            ))}
          </select>
        </label>
      </div>
      {points.length > 0 ? (
        <fieldset className="plan-hard">
          <legend>Make it hard on (the team drives into and over it)</legend>
          {points.map((p) => (
            <label key={p.key} className="checkbox-label">
              <input
                type="checkbox"
                checked={value.hardOn.includes(p.key)}
                onChange={(e) => set({ hardOn: e.target.checked ? [...value.hardOn, p.key] : value.hardOn.filter((k) => k !== p.key) })}
              />
              {icon(p.kind)} {p.label}
            </label>
          ))}
        </fieldset>
      ) : (
        <p className="option-help">No decisive climbs or sectors: it's one for the sprinters.</p>
      )}
    </div>
  )
}
