import { useState } from 'react'
import { useLocation } from 'react-router-dom'
import { EFFORT_MAX_PCT, EFFORT_MIN_PCT, RaceGroup, RiderTelemetryPoint, formatGap } from '../../sim/shared'
import { LiveOrder, LiveRider, TEAM_ORDER_KINDS, TEAM_ORDERS, TeamControl, TeamOrderValue } from '../../sim/useTeamControl'
import { Jersey } from './Jersey'
import { RiderLink } from '../RiderProfile'

const ACTION_WORDS: Record<string, string> = {
  hold: 'own judgement',
  conserve: 'saving energy',
  chase: 'chasing',
  attack: 'attacking',
}

const MODES: Array<{ value: string; label: string; title: string }> = [
  { value: 'hold', label: 'Own judgement', title: 'Let the rider race on his own judgement' },
  { value: 'conserve', label: 'Save', title: 'Sit in and save energy' },
  { value: 'chase', label: 'Chase', title: 'Ride at the front to close gaps' },
  { value: 'attack', label: 'Keep attacking', title: 'Attack whenever he can' },
]

/** "decision · Pogačar attacks! · 2.1 km" — who set it, why, for how long. */
function provenance(x: LiveOrder | LiveRider | undefined): string {
  if (!x || !x.source) return ''
  const parts = [x.source === 'you' ? 'you' : x.source === 'plan' ? 'plan' : 'decision']
  if (x.note) parts.push(x.note)
  if (x.kmLeft && x.kmLeft > 0) parts.push(`${x.kmLeft.toFixed(1)} km`)
  return parts.join(' · ')
}

/** Where a rider is: his group's name and its gap ("Peloton +24s"). */
function whereIs(groups: RaceGroup[], name: string | undefined): string {
  if (!name) return '—'
  const g = groups.find((x) => x.name === name)
  return g && (g.gapS ?? 0) > 0 ? `${name} ${g.gap}` : name
}

const level = (pct: number, low: number, empty: number) => (pct < empty ? 'empty' : pct < low ? 'low' : 'ok')

/** Reserve (thick, the one to act on) and effort (thin, 100% marked). */
function Bars({ tel }: { tel: RiderTelemetryPoint | null }) {
  const reserve = Math.max(0, Math.min(100, tel?.wPrimePct ?? 100))
  const fuel = Math.max(0, Math.min(100, tel?.fuelPct ?? 100))
  const effort = Math.max(0, tel?.effortPct ?? 0)
  const effortCls = effort > 100 ? 'over' : effort >= 80 ? 'work' : 'easy'
  return (
    <div className="crew-bars">
      <div className="bar reserve" title={`Reserve ${Math.round(reserve)}%: the punch for attacks and surges; refills when riding easier`}>
        <i className={level(reserve, 50, 20)} style={{ width: `${reserve}%` }} />
      </div>
      <div className="bar fuel" title={`Fuel ${Math.round(fuel)}%: carbohydrate left. Hard riding burns it fast, eating tops it up; under 30% his threshold starts to fall`}>
        <i className={level(fuel, 50, 30)} style={{ width: `${fuel}%` }} />
      </div>
      <div className="bar effort" title={`Effort ${Math.round(effort)}%: power now vs what he can sustain; over 100% burns reserve`}>
        <i className={effortCls} style={{ width: `${Math.min(100, (effort / 150) * 100)}%` }} />
        <b style={{ left: `${(100 / 150) * 100}%` }} />
      </div>
    </div>
  )
}

/**
 * The player's team as the race applies it: a one-line summary of the team
 * orders in force (all of them a click away), then one row per rider: his
 * group, reserve and effort, what he's doing and why, and attack; manual
 * control folds away.
 */
export function TeamDock({ tc, groups }: { tc: TeamControl; kmRemaining: number; groups: RaceGroup[] }) {
  const [ordersOpen, setOrdersOpen] = useState(false)
  const [manual, setManual] = useState<number | null>(null)
  // In a season race the panel compares within the season's field.
  const seasonId = (useLocation().state as { season?: { seasonId: number } } | null)?.season?.seasonId ?? 0
  if (!tc.sessionId) {
    return <section className="panel team"><div className="empty-state">Starting…</div></section>
  }
  const kinds = TEAM_ORDER_KINDS.filter((kind) => tc.stageRace || !TEAM_ORDERS[kind].stageRaceOnly)
  const active = kinds.filter((k) => tc.teamOrders[k] !== 'auto')
  return (
    <section className="panel team">
      <header className="panel-head">
        <h2><Jersey team={tc.controlledTeam} size={14} />{tc.controlledTeam}</h2>
        <button type="button" className="link" onClick={() => setOrdersOpen((o) => !o)} aria-expanded={ordersOpen}>
          {ordersOpen ? 'Done' : 'Team orders'}
        </button>
      </header>

      {!ordersOpen && (
        <div className="orders-summary">
          {active.length === 0 ? (
            <span className="muted">The team decides everything</span>
          ) : (
            active.map((k) => {
              const why = provenance(tc.live?.orders[k])
              return (
                <button key={k} type="button" className="chip" onClick={() => setOrdersOpen(true)} title={why ? `Set by ${why}` : undefined}>
                  {TEAM_ORDERS[k].label}: <b>{TEAM_ORDERS[k].options[tc.teamOrders[k]]?.label}</b>
                  {why && <small> · {why}</small>}
                </button>
              )
            })
          )}
        </div>
      )}

      {ordersOpen && (
        <div className="orders-grid">
          {kinds.map((kind) => {
            const spec = TEAM_ORDERS[kind]
            const current = tc.teamOrders[kind]
            const why = provenance(tc.live?.orders[kind])
            return (
              <div key={kind} className={`order${current !== 'auto' ? ' set' : ''}`} title={spec.options[current]?.help}>
                <label>
                  <span>{spec.label}</span>
                  <select value={current} onChange={(e) => tc.handleTeamOrder(kind, e.target.value as TeamOrderValue)}>
                    {(['auto', 'yes', 'no'] as TeamOrderValue[])
                      .filter((v) => spec.options[v])
                      .map((v) => (
                        <option key={v} value={v}>{spec.options[v]!.label}</option>
                      ))}
                  </select>
                </label>
                {why && <small className="order-why">{why}</small>}
                {kind === 'leadout' && current === 'yes' && (
                  <details className="train">
                    <summary>Train order</summary>
                    <ol>
                      {tc.normalizedSprintOrder.map((id, idx) => (
                        <li key={id}>
                          <span>{tc.teamRiderByID[id]?.name || `Rider ${id}`}</span>
                          <button type="button" onClick={() => tc.moveSprintTrainRider(id, -1)} disabled={idx === 0} aria-label="Move up">↑</button>
                          <button type="button" onClick={() => tc.moveSprintTrainRider(id, 1)} disabled={idx === tc.normalizedSprintOrder.length - 1} aria-label="Move down">↓</button>
                        </li>
                      ))}
                    </ol>
                  </details>
                )}
              </div>
            )
          })}
        </div>
      )}

      {tc.gc.length > 0 && (
        <div className="gc-strip" title="Live GC: the classification if the stage ended now (place before the stage in brackets)">
          <span className="gc-label">GC</span>
          {tc.gc.slice(0, 5).map((g) => (
            <span key={g.riderId} className={g.team === tc.controlledTeam ? 'mine' : ''}>
              <Jersey team={g.team} />
              {g.livePos}. {g.name}
              {g.livePos > 1 && <> +{formatGap(g.liveGap)}</>}
              {g.pos > 0 && g.pos !== g.livePos && <small> ({g.pos})</small>}
            </span>
          ))}
        </div>
      )}

      {/* Fixed order (the roster), so a row never moves under the cursor. */}
      <ul className="crew">
        {tc.teamRiders.map((r) => {
          const tel = tc.telemetryForRider(r.id)
          const mode = tc.riderPersistentActions[r.id] || 'hold'
          const liveCmd = tc.live?.riders[r.id]
          const locked = tc.isRiderLocked(r.id)
          const why = provenance(liveCmd)
          const legs = tel?.legsPct
          const reserve = Math.round(tel?.wPrimePct ?? 100)
          const open = manual === r.id
          return (
            <li key={r.id} className={`crew-row${liveCmd ? ' commanded' : ''}${open ? ' open' : ''}`}>
              <div className="crew-who">
                <span className="crew-name" title={tel ? `${r.name}: ${Math.round(tel.speedKph)} km/h, ${Math.round(tel.powerWatts)} W` : r.name}><RiderLink id={r.id} name={r.name} seasonId={seasonId} /></span>
                <span className={`crew-status ${mode}`}>
                  <span className="crew-where">{whereIs(groups, tc.riderGroups[r.id])}</span>
                  {' · '}
                  {ACTION_WORDS[mode] ?? mode}
                  {why && <> · {why}</>}
                </span>
              </div>
              <Bars tel={tel} />
              <span className={`crew-num ${level(reserve, 50, 20)}`} title="Reserve">{reserve}%</span>
              <span
                className={`crew-legs ${legs === undefined ? '' : legs < 86 ? 'empty' : legs < 89 ? 'low' : ''}`}
                title={legs === undefined ? '' : `Fade: his threshold is ${Math.round(100 - legs)}% below this morning's${tc.riderById[r.id]?.ftp ? ` (${Math.round((tc.riderById[r.id].ftp * legs) / 100)} W of ${tc.riderById[r.id].ftp} W)` : ''}. It grows with the work he does and never comes back in a race.`}
              >
                {legs === undefined ? '' : `−${Math.max(0, Math.round(100 - legs))}%`}
              </span>
              <button
                className={`crew-attack ${tc.buttonClass(r.id, 'attack', 'oneshot')}`}
                onClick={() => tc.sendCommand(r.id, 'attack', 'oneshot')}
                disabled={locked}
                title={`${r.name}: attack now`}
                aria-label={`${r.name}: attack now`}
              >⚡</button>
              <button type="button" className="crew-more" onClick={() => setManual(open ? null : r.id)} aria-expanded={open} title="Manual control">⋯</button>
              {open && (
                <div className="crew-manual">
                  <div className="modes" role="radiogroup" aria-label={`${r.name} mode`}>
                    {MODES.map((m) => (
                      <button
                        key={m.value}
                        role="radio"
                        aria-checked={mode === m.value}
                        className={mode === m.value ? 'on' : ''}
                        onClick={() => tc.handleTacticChange(r.id, m.value)}
                        disabled={locked}
                        title={m.title}
                      >{m.label}</button>
                    ))}
                  </div>
                  <label className="effort" title="Scales how hard he rides whatever he's doing (100% = normal)">
                    <span>Effort</span>
                    <input
                      type="range"
                      min={EFFORT_MIN_PCT}
                      max={EFFORT_MAX_PCT}
                      step={1}
                      value={tc.effortTargetForRider(r.id, tel)}
                      onChange={(e) => tc.handleEffortChange(r.id, Number(e.target.value))}
                    />
                    <span>{tc.effortTargetForRider(r.id, tel)}%</span>
                  </label>
                  <div className="sprint">
                    <select
                      value={tc.followMap[r.id] ?? ''}
                      onChange={(e) => tc.handleFollowChange(r.id, e.target.value === '' ? null : Number(e.target.value))}
                      title="In the sprint, follow a teammate's wheel"
                    >
                      <option value="">Sprint: follow nobody</option>
                      {tc.normalizedSprintOrder.filter((id) => id !== r.id).map((id) => (
                        <option key={id} value={id}>Sprint: follow {tc.teamRiderByID[id]?.name || `Rider ${id}`}</option>
                      ))}
                    </select>
                    <button type="button" onClick={() => tc.handleLaunchSprint(r.id)}>Launch sprint</button>
                  </div>
                </div>
              )}
            </li>
          )
        })}
      </ul>
      <p className="crew-key">Bars: reserve (thick), fuel, effort (thin, line at 100%). −%: how far his threshold has faded today.</p>
    </section>
  )
}
