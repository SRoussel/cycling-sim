import { useState } from 'react'
import { EFFORT_MAX_PCT, EFFORT_MIN_PCT, RaceGroup, formatGap } from '../../sim/shared'
import { LiveOrder, LiveRider, TEAM_ORDER_KINDS, TEAM_ORDERS, TeamControl, TeamOrderValue } from '../../sim/useTeamControl'
import { EnergyBars, LegsBadge } from './EnergyBars'
import { groupColor } from './RoadView'
import { teamColor } from '../../sim/teamColors'

const ACTION_WORDS: Record<string, string> = {
  hold: 'Racing on his own judgement',
  conserve: 'Saving energy',
  chase: 'Chasing',
  attack: 'Attacking',
}

const MODES: Array<{ value: string; label: string; title: string }> = [
  { value: 'hold', label: 'Own judgement', title: 'Let the rider race on his own judgement' },
  { value: 'conserve', label: 'Save', title: 'Sit in and save energy' },
  { value: 'chase', label: 'Chase', title: 'Ride at the front to close gaps' },
  { value: 'attack', label: 'Keep attacking', title: 'Attack whenever he can' },
]

/** "decision: Pogačar attacks!, 2.1 km left" — who set it, why, for how long. */
function provenance(x: LiveOrder | LiveRider | undefined): string {
  if (!x || !x.source) return ''
  const who = x.source === 'you' ? 'you' : x.source === 'plan' ? 'plan' : 'decision'
  const why = x.note ? `: ${x.note}` : ''
  const left = x.kmLeft && x.kmLeft > 0 ? `, ${x.kmLeft.toFixed(1)} km left` : ''
  return `${who}${why}${left}`
}

/**
 * The player's team as the race applies it. Team orders show their live
 * value and who set it (you, the plan or a decision); each rider shows his
 * state and what he's doing, with manual control folded away.
 */
export function TeamDock({ tc, groups }: { tc: TeamControl; kmRemaining: number; groups: RaceGroup[] }) {
  const [manual, setManual] = useState<Record<number, boolean>>({})
  if (!tc.sessionId) {
    return <section className="dash-panel team-dock"><div className="empty-state">Starting interactive session…</div></section>
  }
  const kinds = TEAM_ORDER_KINDS.filter((kind) => tc.stageRace || !TEAM_ORDERS[kind].stageRaceOnly)
  return (
    <section className="dash-panel team-dock">
      <div className="dock-head">
        <h2>{tc.controlledTeam}</h2>
      </div>

      {tc.gc.length > 0 && (
        <div className="dock-gc" title="Live GC: the classification if the stage ended now (place before the stage in brackets)">
          <span className="dock-gc-label">GC live</span>
          {tc.gc.slice(0, 5).map((g) => (
            <span key={g.riderId} className={`dock-gc-entry${g.team === tc.controlledTeam ? ' mine' : ''}`}>
              <i className="team-dot" style={{ background: teamColor(g.team) }} title={g.team} />
              {g.livePos}. {g.name}
              {g.livePos > 1 && <> +{formatGap(g.liveGap)}</>}
              {g.pos > 0 && g.pos !== g.livePos && <span className="dock-gc-was"> (was {g.pos})</span>}
            </span>
          ))}
        </div>
      )}

      <div className="dock-orders">
        {kinds.map((kind) => {
          const spec = TEAM_ORDERS[kind]
          const current = tc.teamOrders[kind]
          const why = provenance(tc.live?.orders[kind])
          return (
            <div key={kind} className={`dock-order${why ? ' set' : ''}`} title={spec.options[current]?.help}>
              <label className="dock-order-field">
              <span>{spec.label}</span>
              <select value={current} onChange={(e) => tc.handleTeamOrder(kind, e.target.value as TeamOrderValue)}>
                {(['auto', 'yes', 'no'] as TeamOrderValue[])
                  .filter((v) => spec.options[v])
                  .map((v) => (
                    <option key={v} value={v}>{spec.options[v]!.label}</option>
                  ))}
              </select>
              </label>
              {why && <small className="dock-order-why">{why}</small>}
              {kind === 'leadout' && current === 'yes' && (
                <details className="dock-train">
                  <summary>
                    Train order <span className={`sprint-phase ${tc.sprintPhaseLabel}`}>{tc.sprintPhaseLabel}</span>
                  </summary>
                  <ol>
                    {tc.normalizedSprintOrder.map((id, idx) => (
                      <li key={id}>
                        <span>{tc.teamRiderByID[id]?.name || `Rider ${id}`}</span>
                        <span>
                          <button type="button" onClick={() => tc.moveSprintTrainRider(id, -1)} disabled={idx === 0}>↑</button>
                          <button type="button" onClick={() => tc.moveSprintTrainRider(id, 1)} disabled={idx === tc.normalizedSprintOrder.length - 1}>↓</button>
                        </span>
                      </li>
                    ))}
                  </ol>
                  <small>From the front; the last rider sprints.</small>
                </details>
              )}
            </div>
          )
        })}
      </div>

      {/* Fixed order (the roster), so a rider's card never moves under the cursor. */}
      <ul className="dock-riders">
        {tc.teamRiders.map((r) => {
          const tel = tc.telemetryForRider(r.id)
          const mode = tc.riderPersistentActions[r.id] || 'hold'
          const liveCmd = tc.live?.riders[r.id]
          const locked = tc.isRiderLocked(r.id)
          const effort = tc.effortTargetForRider(r.id, tel)
          const open = manual[r.id] ?? false
          const why = provenance(liveCmd)
          return (
            <li key={r.id} className={`dock-rider${liveCmd ? ' commanded' : ''}`} style={{ '--gc': groupColor(groups, tc.riderGroups[r.id]) } as React.CSSProperties}>
              <div className="dock-rider-top">
                <span className="dock-rider-name" title={tel ? `${r.name}: ${Math.round(tel.speedKph)} km/h, ${Math.round(tel.powerWatts)} W` : r.name}>{r.name}</span>
                <LegsBadge telemetry={tel} />
                <span className="dock-rider-group" style={{ '--gc': groupColor(groups, tc.riderGroups[r.id]) } as React.CSSProperties}>{tc.riderGroups[r.id] || '—'}</span>
                <span className="dock-rider-spacer" />
                <button
                  className={`dock-attack ${tc.buttonClass(r.id, 'attack', 'oneshot')}`}
                  onClick={() => tc.sendCommand(r.id, 'attack', 'oneshot')}
                  disabled={locked}
                  title="Attack now (one burst)"
                >⚡ Attack</button>
              </div>
              <EnergyBars telemetry={tel} compact />
              <div className="dock-rider-status">
                <span className={`dock-status ${mode}`}>{ACTION_WORDS[mode] ?? mode}</span>
                {why && <span className="dock-status-why">({why})</span>}
                <button type="button" className="dock-manual-toggle" onClick={() => setManual((m) => ({ ...m, [r.id]: !open }))} aria-expanded={open}>
                  {open ? 'Manual ▾' : 'Manual ▸'}
                </button>
              </div>
              {open && (
                <div className="dock-manual">
                  <div className="dock-modes" role="radiogroup" aria-label={`${r.name} mode`}>
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
                  <label className="dock-effort-row" title="Scales how hard he rides whatever he's doing (100% = normal)">
                    <span>Effort</span>
                    <input
                      type="range"
                      className="dock-effort"
                      min={EFFORT_MIN_PCT}
                      max={EFFORT_MAX_PCT}
                      step={1}
                      value={effort}
                      onChange={(e) => tc.handleEffortChange(r.id, Number(e.target.value))}
                    />
                    <span className="dock-effort-value">{effort}%</span>
                  </label>
                  <div className="dock-rider-finale">
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
                    <button className="secondary" onClick={() => tc.handleLaunchSprint(r.id)}>Launch sprint</button>
                  </div>
                </div>
              )}
            </li>
          )
        })}
      </ul>
    </section>
  )
}
