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

const BREAK_OPTIONS: Array<[string, string, string]> = [
  ['auto', 'Team decides', 'Riders the team can spare may go with moves.'],
  ['yes', 'Get a rider in', 'Your strongest domestiques attack until one is up the road.'],
  ['no', 'Stay out', "Your domestiques don't go in breaks."],
]

/** "Paterberg (0.5 km at 10.1%, 56 km to go)" → name and details. */
function splitLabel(label: string): { name: string; spec: string } {
  const m = label.match(/^(.*?)\s*\((.*)\)\s*$/)
  return m ? { name: m[1], spec: m[2] } : { name: label, spec: '' }
}

/**
 * The race plan's choices: who the team rides for, the break, and for each
 * decisive climb or sector whether the team drives it hard and whether the
 * leader attacks there. Used before a one-day race, before each stage of a
 * stage race and in the season hub.
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
  const byPower = [...riders].sort((a, b) => wkg(b) - wkg(a))
  return (
    <div className="plan-form">
      <div className="plan-row">
        <span className="plan-label">Ride for</span>
        <div className="plan-chips" role="radiogroup" aria-label="Ride for">
          <button type="button" role="radio" aria-checked={value.leaderId === 0} className={value.leaderId === 0 ? 'on' : ''} onClick={() => set({ leaderId: 0 })}>
            Team decides<small>on the day</small>
          </button>
          {byPower.map((r) => (
            <button key={r.id} type="button" role="radio" aria-checked={value.leaderId === r.id} className={value.leaderId === r.id ? 'on' : ''} onClick={() => set({ leaderId: r.id })}>
              {r.name}<small>{wkg(r).toFixed(1)} W/kg · {r.ftp} W</small>
            </button>
          ))}
        </div>
      </div>

      <div className="plan-row">
        <span className="plan-label">The break</span>
        <div className="plan-seg" role="radiogroup" aria-label="The break">
          {BREAK_OPTIONS.map(([v, label, help]) => (
            <button key={v} type="button" role="radio" aria-checked={value.break === v} className={value.break === v ? 'on' : ''} title={help} onClick={() => set({ break: v })}>{label}</button>
          ))}
        </div>
      </div>

      <div className="plan-row">
        <span className="plan-label">Climbs and sectors</span>
        {points.length === 0 ? (
          <p className="plan-none">No decisive climbs or sectors: it's one for the sprinters.</p>
        ) : (
          <div className="plan-points">
            <div className="plan-points-head" aria-hidden="true"><span /><span>Drive it hard</span><span>Leader attacks</span></div>
            {points.map((p) => {
              const { name, spec } = splitLabel(p.label)
              const hard = value.hardOn.includes(p.key)
              const attack = value.attackOn === p.key
              return (
                <div key={p.key} className="plan-point">
                  <span><b>{name}</b> <small>{spec}</small></span>
                  <button type="button" aria-pressed={hard} aria-label={`Drive hard on ${name}`} className={hard ? 'on' : ''}
                    onClick={() => set({ hardOn: hard ? value.hardOn.filter((k) => k !== p.key) : [...value.hardOn, p.key] })}>
                    {hard ? 'Hard' : '—'}
                  </button>
                  <button type="button" aria-pressed={attack} aria-label={`Leader attacks on ${name}`} className={attack ? 'on' : ''}
                    onClick={() => set({ attackOn: attack ? '' : p.key })}>
                    {attack ? 'Attack' : '—'}
                  </button>
                </div>
              )
            })}
            <p className="plan-note">The leader attacks at one place at most; with none picked he goes where he sees fit.</p>
          </div>
        )}
      </div>
    </div>
  )
}
