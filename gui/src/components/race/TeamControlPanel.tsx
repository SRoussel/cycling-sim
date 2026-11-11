import { EFFORT_MAX_PCT, EFFORT_MIN_PCT, formatCommandLabel, groupClassName } from '../../sim/shared'
import { TEAM_ORDER_KINDS, TEAM_ORDERS, TeamControl, TeamOrderValue } from '../../sim/useTeamControl'

export function TeamControlPanel({ tc }: { tc: TeamControl }) {
  return (
    <section className="card team-controls">
      <h2>Team Control</h2>
      {!tc.sessionId ? (
        <div className="empty-state">Starting interactive session...</div>
      ) : tc.teamRiders.length === 0 ? (
        <div className="empty-state">No riders found for {tc.controlledTeam}.</div>
      ) : (
        <>
        <div className="team-orders-panel">
          <div className="sprint-train-header">
            <span>Team orders</span>
          </div>
          {TEAM_ORDER_KINDS.filter((kind) => tc.stageRace || !TEAM_ORDERS[kind].stageRaceOnly).map((kind) => {
            const spec = TEAM_ORDERS[kind]
            const current = tc.teamOrders[kind]
            return (
              <div key={kind} className="team-order-block">
                <div className="team-orders-row" role="radiogroup" aria-label={spec.label}>
                  <span className="team-orders-label">{spec.label}</span>
                  {(['auto', 'yes', 'no'] as TeamOrderValue[]).filter((value) => spec.options[value]).map((value) => (
                    <button
                      key={value}
                      role="radio"
                      aria-checked={current === value}
                      className={current === value ? 'team-order selected' : 'team-order secondary'}
                      onClick={() => tc.handleTeamOrder(kind, value)}
                      disabled={!tc.sessionId}
                    >
                      {spec.options[value]?.label}
                    </button>
                  ))}
                </div>
                <div className="team-orders-help">{spec.options[current]?.help}</div>
              </div>
            )
          })}
        </div>
        <div className="sprint-train-panel">
          <div className="sprint-train-header">
            <span>Sprint Train</span>
            <span className={`sprint-phase ${tc.sprintPhaseLabel}`}>{tc.sprintPhaseLabel.toUpperCase()}</span>
          </div>
          {tc.teamOrders.leadout !== 'yes' && (
            <div className="team-orders-help">Runs when Lead-out is set to Sprint train.</div>
          )}
          <div className="attack-pause-toggle">
            <label>
              <input
                type="checkbox"
                checked={tc.pauseOnRivalAttacks}
                onChange={(e) => tc.setPauseOnRivalAttacks(e.target.checked)}
                disabled={!tc.sessionId}
              />
              Pause on rival attacks
            </label>
          </div>
          {tc.attackAlert && (
            <div className="attack-alert">
              <div>
                ⚠️ Rival attack{tc.attackAlert.team ? ` (${tc.attackAlert.team})` : ''}
              </div>
              <div className="attack-alert-actions">
                <select
                  value={tc.followAttackRiderId ?? ''}
                  onChange={(e) => tc.setFollowAttackRiderId(e.target.value === '' ? null : Number(e.target.value))}
                  disabled={!tc.sessionId}
                >
                  <option value="">Select rider…</option>
                  {tc.normalizedSprintOrder.map((id) => (
                    <option key={id} value={id}>{tc.teamRiderByID[id]?.name || `Rider ${id}`}</option>
                  ))}
                </select>
                <button className="secondary" onClick={tc.handleFollowAttack} disabled={!tc.sessionId || !tc.followAttackRiderId}>Follow attack</button>
              </div>
            </div>
          )}
          <div className="sprint-train-order">
            {tc.normalizedSprintOrder.map((riderID, idx) => (
              <div key={riderID} className="sprint-train-order-row">
                <span>{idx + 1}. {tc.teamRiderByID[riderID]?.name || `Rider ${riderID}`}</span>
                <div>
                  <button className="secondary" onClick={() => tc.moveSprintTrainRider(riderID, -1)} disabled={idx === 0}>↑</button>
                  <button className="secondary" onClick={() => tc.moveSprintTrainRider(riderID, 1)} disabled={idx === tc.normalizedSprintOrder.length - 1}>↓</button>
                </div>
              </div>
            ))}
          </div>
        </div>
        <div className="team-roster">
          {tc.groupedTeamRiders.map((section) => (
            <div key={section.key} className="team-group-section">
              <div className={`team-group-header ${groupClassName(section.label)}`}>{section.label}</div>
              {section.riders.map((r) => (
                <div key={r.id} className="team-rider">
                  {(() => {
                    const telemetry = tc.telemetryForRider(r.id)
                    return (
                      <>
                  <div className="team-rider-row">
                    <div className="team-rider-main">
                      <div className="team-rider-name">{r.name}</div>
                      <div className="team-rider-meta">
                        <span className={groupClassName(tc.riderGroups[r.id])}>{tc.riderGroups[r.id] || 'Unknown'}</span>
                        <span>{telemetry ? `${Math.round(telemetry.speedKph)}kph` : '—'}</span>
                        <span>{telemetry ? `${Math.round(telemetry.powerWatts)}W` : '—'}</span>
                        <span>{telemetry ? `${Math.round(telemetry.effortPct)}%` : '—'}</span>
                        <span>{formatCommandLabel(tc.riderPersistentActions[r.id] || 'hold')}</span>
                        {tc.riderOneShotQueued[r.id] && <span>Attack queued</span>}
                        {tc.isRiderLocked(r.id) && <span>Cooldown…</span>}
                      </div>
                    </div>

                    <div className="team-rider-controls">
                      <div className="team-rider-effort compact">
                        <label htmlFor={`effort-${r.id}`}>Effort</label>
                        <input
                          id={`effort-${r.id}`}
                          type="range"
                          min={EFFORT_MIN_PCT}
                          max={EFFORT_MAX_PCT}
                          step={1}
                          value={tc.effortTargetForRider(r.id, telemetry)}
                          onChange={(e) => tc.handleEffortChange(r.id, Number(e.target.value))}
                          disabled={!tc.sessionId}
                        />
                        <span>{tc.effortTargetForRider(r.id, telemetry)}%</span>
                      </div>

                      <div className="team-rider-actions compact">
                        <select
                          value={tc.riderPersistentActions[r.id] || 'hold'}
                          onChange={(e) => tc.handleTacticChange(r.id, e.target.value)}
                          disabled={tc.isRiderLocked(r.id)}
                        >
                          <option value="hold">AI (Hold)</option>
                          <option value="conserve">Conserve</option>
                          <option value="chase">Chase</option>
                          <option value="attack">Attack</option>
                        </select>
                        <button className={tc.buttonClass(r.id, 'attack', 'oneshot')} onClick={() => tc.sendCommand(r.id, 'attack', 'oneshot')} disabled={tc.isRiderLocked(r.id)}>Attack now</button>
                        <select
                          value={tc.followMap[r.id] ?? ''}
                          onChange={(e) => tc.handleFollowChange(r.id, e.target.value === '' ? null : Number(e.target.value))}
                          disabled={!tc.sessionId}
                        >
                          <option value="">Follow: None</option>
                          {tc.normalizedSprintOrder
                            .filter((id) => id !== r.id)
                            .map((id) => (
                              <option key={id} value={id}>Follow: {tc.teamRiderByID[id]?.name || `Rider ${id}`}</option>
                            ))}
                        </select>
                        <button className="secondary" onClick={() => tc.handleLaunchSprint(r.id)} disabled={!tc.sessionId}>Launch</button>
                      </div>
                    </div>
                  </div>
                      </>
                    )
                  })()}
                </div>
              ))}
            </div>
          ))}
        </div>
        </>
      )}
    </section>
  )
}
