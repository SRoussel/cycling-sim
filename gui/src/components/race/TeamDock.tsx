import { EFFORT_MAX_PCT, EFFORT_MIN_PCT } from '../../sim/shared'
import { DecisionLevel, TEAM_ORDER_KINDS, TEAM_ORDERS, TeamControl, TeamOrderValue } from '../../sim/useTeamControl'
import { EnergyBars, LegsBadge } from './EnergyBars'
import { RaceGroup } from '../../sim/shared'
import { groupColor } from './RoadView'

const MODES: Array<{ value: string; label: string; title: string }> = [
  { value: 'hold', label: 'AI', title: 'Let the rider race on his own judgement' },
  { value: 'conserve', label: 'Save', title: 'Sit in and save energy' },
  { value: 'chase', label: 'Chase', title: 'Ride at the front to close gaps' },
  { value: 'attack', label: 'Go', title: 'Keep attacking' },
]

/** The player's team: orders, then every rider with energy and one-click commands. */
export function TeamDock({ tc, kmRemaining, groups }: { tc: TeamControl; kmRemaining: number; groups: RaceGroup[] }) {
  const finale = kmRemaining <= 10
  if (!tc.sessionId) {
    return <section className="dash-panel team-dock"><div className="empty-state">Starting interactive session…</div></section>
  }
  return (
    <section className="dash-panel team-dock">
      <div className="dock-head">
        <h2>{tc.controlledTeam}</h2>
        <label className="dock-pause" title="Pause at key moments (attacks, breaks, a dropped leader, decisive climbs, the finale) and ask what to do">
          Stop for
          <select value={tc.decisionLevel} onChange={(e) => tc.setDecisionLevel(e.target.value as DecisionLevel)}>
            <option value="all">every decision</option>
            <option value="key">key moments</option>
            <option value="off">nothing</option>
          </select>
        </label>
      </div>

      <div className="dock-orders">
        {TEAM_ORDER_KINDS.filter((kind) => tc.stageRace || !TEAM_ORDERS[kind].stageRaceOnly).map((kind) => {
          const spec = TEAM_ORDERS[kind]
          const current = tc.teamOrders[kind]
          return (
            <label key={kind} className="dock-order" title={spec.options[current]?.help}>
              <span>{spec.label}</span>
              <select value={current} onChange={(e) => tc.handleTeamOrder(kind, e.target.value as TeamOrderValue)}>
                {(['auto', 'yes', 'no'] as TeamOrderValue[])
                  .filter((v) => spec.options[v])
                  .map((v) => (
                    <option key={v} value={v}>{spec.options[v]!.label}</option>
                  ))}
              </select>
            </label>
          )
        })}
      </div>

      <p className="dock-legend">
        <b>Reserve</b>: the punch for attacks and surges; refills when riding easier.{' '}
        <b>Effort</b>: power now vs what he can sustain; over 100% burns reserve.{' '}
        <b>Legs</b>: what's left for the day; only falls.
      </p>

      {/* Fixed order (the roster), so a rider's card never moves under the cursor. */}
      <ul className="dock-riders">
        {tc.teamRiders.map((r) => {
          const tel = tc.telemetryForRider(r.id)
          const mode = tc.riderPersistentActions[r.id] || 'hold'
          const locked = tc.isRiderLocked(r.id)
          const effort = tc.effortTargetForRider(r.id, tel)
          return (
            <li key={r.id} className="dock-rider">
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
              <div className="dock-rider-cmds">
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
                <input
                  type="range"
                  className="dock-effort"
                  min={EFFORT_MIN_PCT}
                  max={EFFORT_MAX_PCT}
                  step={1}
                  value={effort}
                  onChange={(e) => tc.handleEffortChange(r.id, Number(e.target.value))}
                  title="Effort"
                />
                <span className="dock-effort-value">{effort}%</span>
              </div>
              {finale && (
                <div className="dock-rider-finale">
                  <select
                    value={tc.followMap[r.id] ?? ''}
                    onChange={(e) => tc.handleFollowChange(r.id, e.target.value === '' ? null : Number(e.target.value))}
                  >
                    <option value="">Follow: none</option>
                    {tc.normalizedSprintOrder.filter((id) => id !== r.id).map((id) => (
                      <option key={id} value={id}>Follow {tc.teamRiderByID[id]?.name || `Rider ${id}`}</option>
                    ))}
                  </select>
                  <button className="secondary" onClick={() => tc.handleLaunchSprint(r.id)}>Launch sprint</button>
                </div>
              )}
            </li>
          )
        })}
      </ul>

      <details className="dock-train">
        <summary>
          Sprint train <span className={`sprint-phase ${tc.sprintPhaseLabel}`}>{tc.sprintPhaseLabel}</span>
        </summary>
        <p className="dock-hint">Order from the front; the last rider sprints. Runs when Lead-out is set to Sprint train.</p>
        <ol>
          {tc.normalizedSprintOrder.map((id, idx) => (
            <li key={id}>
              <span>{tc.teamRiderByID[id]?.name || `Rider ${id}`}</span>
              <span>
                <button onClick={() => tc.moveSprintTrainRider(id, -1)} disabled={idx === 0}>↑</button>
                <button onClick={() => tc.moveSprintTrainRider(id, 1)} disabled={idx === tc.normalizedSprintOrder.length - 1}>↓</button>
              </span>
            </li>
          ))}
        </ol>
      </details>
    </section>
  )
}
