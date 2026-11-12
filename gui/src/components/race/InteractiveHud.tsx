import { RaceGroup, groupClassName } from '../../sim/shared'
import { TeamControl } from '../../sim/useTeamControl'
import { EnergyBars } from './EnergyBars'

/** Compact strip of live groups plus one-tap controls for each team rider. */
export function InteractiveHud({ tc, groups }: { tc: TeamControl; groups: RaceGroup[] }) {
  return (
    <section className="card interactive-hud">
      <div className="hud-groups-strip">
        {groups.map((g, i) => {
          const mine = tc.sortedTeamRiders.filter((r) => tc.riderGroups[r.id] === g.name).length
          const size = g.size ?? g.riders.length
          return (
            <div
              key={i}
              className={`hud-group-chip ${g.name.toLowerCase().replace(/\s+/g, '-')}${mine > 0 ? ' has-mine' : ''}`}
              title={g.riders.join(', ')}
            >
              <span className="hud-chip-name">{g.name}</span>
              <span className="hud-chip-gap">{g.gap}</span>
              <span className="hud-chip-count">
                {size} {size === 1 ? 'rider' : 'riders'}
                {mine > 0 && <span className="hud-chip-mine"> · {mine} yours</span>}
              </span>
            </div>
          )
        })}
      </div>
      <div className="hud-rider-strip">
        {tc.sortedTeamRiders.map((r) => {
          const telemetry = tc.telemetryForRider(r.id)
          const groupName = tc.riderGroups[r.id] || '—'
          const locked = tc.isRiderLocked(r.id)
          const queued = tc.riderOneShotQueued[r.id]
          return (
            <div key={r.id} className="hud-rider-card">
              <div className="hud-rider-info">
                <span className="hud-rider-name">{r.name}</span>
                <span className={groupClassName(groupName)}>{groupName}</span>
                {telemetry && (
                  <span className="hud-rider-tel">{Math.round(telemetry.speedKph)} kph</span>
                )}
              </div>
              <EnergyBars telemetry={telemetry} compact />
              <div className="hud-rider-btns">
                <button
                  className={`hud-attack-btn${queued ? ' queued' : ''}${tc.isAccepted(r.id, 'attack', 'oneshot') ? ' accepted' : ''}`}
                  onClick={() => tc.sendCommand(r.id, 'attack', 'oneshot')}
                  disabled={locked}
                  title="One-shot attack burst"
                >⚡ Attack</button>
                <select
                  className="hud-tactic-select"
                  value={tc.riderPersistentActions[r.id] || 'hold'}
                  onChange={(e) => tc.handleTacticChange(r.id, e.target.value)}
                  disabled={locked}
                >
                  <option value="hold">Hold</option>
                  <option value="conserve">Conserve</option>
                  <option value="chase">Chase</option>
                  <option value="attack">Attack (sustain)</option>
                </select>
                {locked && <span className="hud-badge cooldown">⏳</span>}
                {queued && !locked && <span className="hud-badge queued">⚡ queued</span>}
              </div>
            </div>
          )
        })}
      </div>
    </section>
  )
}
