import { RaceGroup, groupClassName } from '../../sim/shared'
import { TeamControl } from '../../sim/useTeamControl'

/** Compact strip of live groups plus one-tap controls for each team rider. */
export function InteractiveHud({ tc, groups }: { tc: TeamControl; groups: RaceGroup[] }) {
  return (
    <section className="card interactive-hud">
      <div className="hud-groups-strip">
        {groups.map((g, i) => (
          <div key={i} className={`hud-group-chip ${g.name.toLowerCase().replace(/\s+/g, '-')}`}>
            <span className="hud-chip-name">{g.name}</span>
            <span className="hud-chip-gap">{g.gap}</span>
            <span className="hud-chip-count">{g.riders.length} riders</span>
          </div>
        ))}
      </div>
      <div className="hud-rider-strip">
        {tc.sortedTeamRiders.map((r) => {
          const telemetry = tc.telemetryForRider(r.id)
          const groupName = tc.riderGroups[r.id] || 'Unknown'
          const locked = tc.isRiderLocked(r.id)
          const queued = tc.riderOneShotQueued[r.id]
          return (
            <div key={r.id} className="hud-rider-card">
              <div className="hud-rider-info">
                <span className="hud-rider-name">{r.name}</span>
                <span className={groupClassName(groupName)}>{groupName}</span>
                {telemetry && (
                  <span className="hud-rider-tel">
                    {Math.round(telemetry.speedKph)} kph · W' {Math.round(telemetry.wPrimePct)}%
                  </span>
                )}
              </div>
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
