import { useState } from 'react'
import { useLocation } from 'react-router-dom'
import { EFFORT_MAX_PCT, EFFORT_MIN_PCT, RaceGroup, formatGap } from '../../sim/shared'
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

/** ", on the front" / ", 14th in line" (nothing for a rider alone). */
function linePlace(pos: number | undefined, g: RaceGroup | undefined): string {
  const size = g?.size ?? g?.riders.length ?? 0
  if (!pos || size <= 1) return ''
  if (pos === 1) return ', on the front'
  const s = pos % 100 >= 11 && pos % 100 <= 13 ? 'th' : ({ 1: 'st', 2: 'nd', 3: 'rd' } as Record<number, string>)[pos % 10] ?? 'th'
  return `, ${pos}${s} in line`
}

const level = (pct: number, low: number, empty: number) => (pct < empty ? 'empty' : pct < low ? 'low' : 'ok')

/** A labelled meter: a bar and its number (reserve thick, fuel thin). */
function Meter({ pct, cls, thin, title }: { pct: number; cls: string; thin?: boolean; title: string }) {
  return (
    <div className={`meter${thin ? ' thin' : ''}`} title={title}>
      <div className="meter-track"><i className={cls} style={{ width: `${pct}%` }} /></div>
      <span className={`meter-num ${cls}`}>{Math.round(pct)}</span>
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
        <h2><Jersey team={tc.controlledTeam} size={16} />{tc.controlledTeam}</h2>
        {active.length === 0 ? <span className="panel-meta">The team decides everything</span> : <span className="panel-meta">{active.length} {active.length === 1 ? 'order' : 'orders'} set</span>}
      </header>

      {/* Team orders, always in view: one segmented switch each. */}
      <div className="orders-grid">
        {kinds.map((kind) => {
          const spec = TEAM_ORDERS[kind]
          const current = tc.teamOrders[kind]
          const why = provenance(tc.live?.orders[kind])
          const values = (['auto', 'yes', 'no'] as TeamOrderValue[]).filter((v) => spec.options[v])
          return (
            <div key={kind} className={`order${current !== 'auto' ? ' set' : ''}`}>
              <span className="order-label">{spec.label}</span>
              <div className="seg" role="radiogroup" aria-label={spec.label}>
                {values.map((v) => (
                  <button
                    key={v}
                    type="button"
                    role="radio"
                    aria-checked={current === v}
                    className={current === v ? 'on' : ''}
                    title={spec.options[v]!.help}
                    onClick={() => tc.handleTeamOrder(kind, v)}
                  >{v === 'auto' ? 'Team' : spec.options[v]!.label}</button>
                ))}
              </div>
              {why && <small className="order-why">{why}</small>}
              {kind === 'leadout' && current === 'yes' && (
                <label className="train">
                  <span>Lead out for</span>
                  <select
                    value={tc.trainSprinter ?? ''}
                    onChange={(e) => tc.setTrainSprinter(e.target.value === '' ? null : Number(e.target.value))}
                    title="The train rides only in the group its sprinter is in; teammates elsewhere race for themselves"
                  >
                    <option value="">Fastest finisher in each group</option>
                    {tc.teamRiders.map((r) => (
                      <option key={r.id} value={r.id}>{r.name}</option>
                    ))}
                  </select>
                </label>
              )}
            </div>
          )
        })}
      </div>

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

      <div className="crew-head" aria-hidden="true">
        <span>Rider</span><span className="r">Power</span><span>Reserve · fuel</span><span className="r">Legs</span><span>Order</span>
      </div>
      {/* Fixed order (the roster), so a row never moves under the cursor. */}
      <ul className="crew">
        {tc.teamRiders.map((r) => {
          const tel = tc.telemetryForRider(r.id)
          const mode = tc.riderPersistentActions[r.id] || 'hold'
          const liveCmd = tc.live?.riders[r.id]
          const locked = tc.isRiderLocked(r.id)
          const why = provenance(liveCmd)
          const legs = tel?.legsPct
          const reserve = Math.max(0, Math.min(100, tel?.wPrimePct ?? 100))
          const fuel = Math.max(0, Math.min(100, tel?.fuelPct ?? 100))
          const kg = tc.riderById[r.id]?.weight
          const watts = tel ? Math.round(tel.powerWatts) : null
          const effort = Math.max(0, tel?.effortPct ?? 0)
          const open = manual === r.id
          return (
            <li key={r.id} className={`crew-row${liveCmd ? ' commanded' : ''}${open ? ' open' : ''}`}>
              <div className="crew-who">
                <span className="crew-name"><RiderLink id={r.id} name={r.name} seasonId={seasonId} /></span>
                <span className="crew-where">{whereIs(groups, tc.riderGroups[r.id])}{linePlace(tel?.groupPos, groups.find((g) => g.name === tc.riderGroups[r.id]))}</span>
                <span className={`crew-status ${mode}`}>{ACTION_WORDS[mode] ?? mode}{why && <> · {why}</>}</span>
                {tel && <EffortLine effort={effort} perMin={tel.reservePerMin ?? 0} why={tel.why} />}
              </div>
              <div className={`crew-power${effort > 100 ? ' over' : ''}`} title={tel ? `Effort ${Math.round(effort)}% of what he can sustain; ${Math.round(tel.speedKph)} km/h` : ''}>
                {watts !== null ? <>{watts} W<small>{kg ? `${(watts / kg).toFixed(1)} W/kg` : ''}</small></> : '—'}
              </div>
              <div className="crew-meters">
                <Meter pct={reserve} cls={level(reserve, 50, 20)} title={`Reserve ${Math.round(reserve)}%: the punch for attacks and surges; refills when riding easier`} />
                <Meter pct={fuel} cls={level(fuel, 50, 30)} thin title={`Fuel ${Math.round(fuel)}%: carbohydrate left. Hard riding burns it fast, eating tops it up; under 30% his threshold starts to fall`} />
              </div>
              <span
                className={`crew-legs ${legs === undefined ? '' : legs < 86 ? 'empty' : legs < 89 ? 'low' : ''}`}
                title={legs === undefined ? '' : `Fade: his threshold is ${Math.round(100 - legs)}% below this morning's${tc.riderById[r.id]?.ftp ? ` (${Math.round((tc.riderById[r.id].ftp * legs) / 100)} W of ${tc.riderById[r.id].ftp} W)` : ''}. It grows with the work he does and never comes back in a race.`}
              >
                {legs === undefined ? '' : `−${Math.max(0, Math.round(100 - legs))}%`}
              </span>
              <div className="crew-order">
                <select
                  value={mode}
                  onChange={(e) => tc.handleTacticChange(r.id, e.target.value)}
                  disabled={locked}
                  aria-label={`${r.name}: order`}
                >
                  {MODES.map((m) => <option key={m.value} value={m.value}>{m.label}</option>)}
                </select>
                <button
                  className={`crew-attack ${tc.buttonClass(r.id, 'attack', 'oneshot')}`}
                  onClick={() => tc.sendCommand(r.id, 'attack', 'oneshot')}
                  disabled={locked}
                  title={`${r.name}: attack now`}
                >Attack</button>
                <button type="button" className="crew-more" onClick={() => setManual(open ? null : r.id)} aria-expanded={open} aria-label={`${r.name}: effort and sprint`} title="Effort and sprint">
                  <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><circle cx="5" cy="12" r="2" /><circle cx="12" cy="12" r="2" /><circle cx="19" cy="12" r="2" /></svg>
                </button>
              </div>
              {open && (
                <div className="crew-manual">
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
      <p className="crew-key">Power: now, and per kg. Reserve: the anaerobic reserve left (thin bar: fuel). Legs: how far his threshold has faded today.</p>
    </section>
  )
}

/** How hard a rider is riding (effort zone) and what's doing to his reserve:
 * draining, and why, or refilling. */
function EffortLine({ effort, perMin, why }: { effort: number; perMin: number; why?: string }) {
  const zone =
    effort >= 102 ? { label: 'Above threshold', cls: 'above' }
      : effort >= 90 ? { label: 'Threshold', cls: 'threshold' }
        : effort >= 75 ? { label: 'Tempo', cls: 'tempo' }
          : { label: 'Easy', cls: 'easy' }
  let trend = ''
  if (why) trend = `reserve ${Math.round(perMin)}%/min: ${why}`
  else if (perMin > 0.5) trend = 'reserve refilling'
  return (
    <span className="crew-effort" title="Effort: power against what he can hold for an hour. Above threshold he spends his reserve; below it the reserve refills. The reason is over the last minute and a half of racing.">
      <b className={`zone ${zone.cls}`}>{zone.label}</b>
      {trend && <span className={why ? 'drain' : 'refill'}>{trend}</span>}
    </span>
  )
}
