import { useMemo, useState } from 'react'
import { RaceGroup, RouteProfile, clamp01, formatGap, formatRaceClock, sectorNoun } from '../../sim/shared'
import { GroupMark, ProfileView, findClimbs, stars } from './ProfileView'
import { DecisionLevel } from '../../sim/useTeamControl'
import { PlaybackControls } from './RacePanels'

/**
 * The race's always-visible header: where the race is (km to go, clock) and
 * the route profile doubling as the progress bar (shaded up to the riders).
 */
export function RaceTopBar({
  title,
  subtitle,
  kmRemaining,
  kmCovered,
  kmTotal,
  elapsed,
  profile,
  paused,
  speed,
  playbackDisabled,
  onTogglePause,
  onSpeedChange,
  extra,
  onSkip,
  skipping,
  goal,
  decisionLevel,
  onDecisionLevel,
  marks,
  groups,
  road,
}: {
  title: string
  subtitle?: string
  kmRemaining: number
  kmCovered: number
  kmTotal: number
  elapsed: number
  profile: RouteProfile | null
  paused: boolean
  speed: number
  playbackDisabled: boolean
  onTogglePause: () => void
  onSpeedChange: (speed: number) => void
  extra?: React.ReactNode
  onSkip?: () => void
  skipping?: boolean
  goal?: string
  decisionLevel?: DecisionLevel
  onDecisionLevel?: (level: DecisionLevel) => void
  marks?: GroupMark[]
  /** The groups on the road, for the gap between the first two. */
  groups?: RaceGroup[]
  /** The road drawn large (groups by gap), shown in place of the profile
   * when picked. */
  road?: React.ReactNode
}) {
  const total = profile?.totalKm || kmTotal
  const avg = elapsed > 120 ? kmCovered / (elapsed / 3600) : null
  const gap = groups && groups.length > 1 ? groups[1].gapS : undefined
  return (
    <header className="dash-top">
      <div className="dash-top-row">
        <div className="dash-title">
          <h1>{title}</h1>
          {subtitle && <span className="dash-subtitle">{subtitle}</span>}
          {goal && <span className="dash-goal" title="Today's goal for your team">Goal: {goal}</span>}
        <div className="dash-controls">
          {decisionLevel && onDecisionLevel && (
            <label className="dash-stopfor" title="When the race stops to ask you: at every decision, only the moments that decide the race, or never">
              <span>Stop for</span>
              <select value={decisionLevel} onChange={(e) => onDecisionLevel(e.target.value as DecisionLevel)}>
                <option value="all">every decision</option>
                <option value="key">key moments</option>
                <option value="off">nothing</option>
              </select>
            </label>
          )}
          {onSkip && (
            <button className={`dash-skip${skipping ? ' on' : ''}`} onClick={onSkip} disabled={playbackDisabled}
              title="Fast-forward until the next decision moment">
              {skipping ? 'Stop skipping' : 'Next decision'}
            </button>
          )}
          <PlaybackControls
            paused={paused}
            speed={speed}
            disabled={playbackDisabled}
            onTogglePause={onTogglePause}
            onSpeedChange={onSpeedChange}
          />
          </div>
        </div>
        <div className="dash-figures">
          <Figure label="To go" value={kmRemaining.toFixed(1)} unit="km" accent />
          <Figure label="Race time" value={formatRaceClock(elapsed)} />
          {avg !== null && <Figure label="Average" value={avg.toFixed(1)} unit="km/h" />}
          {gap !== undefined && gap > 0 && <Figure label="Gap to the next group" value={formatGap(gap)} />}
          {extra}
        </div>
        <NextUp profile={profile} km={kmCovered} />
      </div>
      <RouteStrip profile={profile} km={kmCovered} kmTotal={total} marks={marks} road={road} />
    </header>
  )
}

/** One of the top bar's big live figures. */
function Figure({ label, value, unit, accent }: { label: string; value: string; unit?: string; accent?: boolean }) {
  return (
    <div className={`dash-fig${accent ? ' accent' : ''}`}>
      <span className="dash-fig-label">{label}</span>
      <span className="dash-fig-value">{value}{unit && <small>{unit}</small>}</span>
    </div>
  )
}

/** What's next on the road: the climb or sector the riders are on, or the
 * next one (sectors within 30 km). */
function NextUp({ profile, km }: { profile: RouteProfile | null; km: number }) {
  const climbs = useMemo(() => findClimbs(profile), [profile])
  if (!profile) return null
  const sectors = profile.sectors || []
  const onSector = sectors.find((sec) => km >= sec.startKm && km < sec.startKm + sec.lengthKm)
  const nextSector = sectors.find((sec) => sec.startKm > km && sec.startKm - km <= 30)
  const current = climbs.find((c) => km >= c.startKm && km < c.startKm + c.lengthKm)
  const next = climbs.find((c) => c.startKm > km)
  const lines: Array<{ head: React.ReactNode; sub: string; on: boolean }> = []
  if (current) {
    lines.push({ head: <>On {current.name || 'a climb'} <em>{(current.startKm + current.lengthKm - km).toFixed(1)} km to the top</em></>, sub: `${current.lengthKm.toFixed(1)} km at ${current.avgPct.toFixed(1)}%`, on: true })
  } else if (next) {
    lines.push({ head: <>{next.name || 'Climb'} <em>in {(next.startKm - km).toFixed(1)} km</em></>, sub: `${next.lengthKm.toFixed(1)} km at ${next.avgPct.toFixed(1)}%${next.surface ? `, ${sectorNoun(next.surface)}` : ''}`, on: false })
  }
  if (onSector && onSector.name !== current?.name) {
    lines.push({ head: <>{onSector.name} {stars(onSector.stars)} <em>{(onSector.startKm + onSector.lengthKm - km).toFixed(1)} km left</em></>, sub: sectorNoun(onSector.surface), on: true })
  } else if (!onSector && nextSector && nextSector.name !== next?.name) {
    lines.push({ head: <>{nextSector.name} {stars(nextSector.stars)} <em>in {(nextSector.startKm - km).toFixed(1)} km</em></>, sub: `${nextSector.lengthKm.toFixed(1)} km of ${sectorNoun(nextSector.surface)}`, on: false })
  }
  if (lines.length === 0) return null
  return (
    <div className="dash-next">
      <span className="dash-fig-label">{lines.some((l) => l.on) ? 'Now' : 'Next'}</span>
      {lines.slice(0, 2).map((l, i) => (
        <span key={i} className={`dash-next-line${l.on ? ' on' : ''}`}>{l.head}<small>{l.sub}</small></span>
      ))}
    </div>
  )
}

/** Close-up of the road ahead (where racing decisions are made), the next
 * climb, and the whole course as a small strip that opens a large view. */
const ZOOM_BEHIND_KM = 3
const ZOOM_SPAN_KM = 28

function RouteStrip({ profile, km, kmTotal, marks, road }: { profile: RouteProfile | null; km: number; kmTotal: number; marks?: GroupMark[]; road?: React.ReactNode }) {
  const [open, setOpen] = useState(false)
  const [view, setView] = useState<'course' | 'road'>('course')
  const climbs = useMemo(() => findClimbs(profile), [profile])
  if (!profile || profile.points.length < 2) {
    const pct = kmTotal > 0 ? clamp01(km / kmTotal) * 100 : 0
    return (
      <div className="dash-profile-bar">
        <div className="dash-profile-bar-fill" style={{ width: `${pct}%` }} />
      </div>
    )
  }
  const total = profile.totalKm
  const span = Math.min(ZOOM_SPAN_KM, total)
  const from = Math.max(0, Math.min(km - ZOOM_BEHIND_KM, total - span))
  const sectors = profile.sectors || []
  return (
    <div className="route-strip">
      <div className="route-zoom">
        <div className="route-zoom-head">
          {road ? (
            <span className="seg-tabs small" role="tablist" aria-label="View">
              <button type="button" role="tab" aria-selected={view === 'course'} className={view === 'course' ? 'on' : ''} onClick={() => setView('course')}>Road ahead</button>
              <button type="button" role="tab" aria-selected={view === 'road'} className={view === 'road' ? 'on' : ''} onClick={() => setView('road')}>On the road</button>
            </span>
          ) : (
            <span className="route-zoom-title">Road ahead · km to go</span>
          )}
          {view === 'road' && road ? <span className="route-legend">Groups by time behind the front, sized by riders</span> : <span className="route-legend" aria-label="Gradient colours">
            <span><i className="pv-g1" />under 3%</span>
            <span><i className="pv-g2" />3–6%</span>
            <span><i className="pv-g3" />6–9%</span>
            <span><i className="pv-g4" />9% and up</span>
            {sectors.length > 0 && <span><i className="cob" />{sectors.every((x) => x.surface === 'gravel') ? 'gravel' : 'cobbles'}</span>}
          </span>}
        </div>
        {view === 'road' && road ? <div className="route-road">{road}</div> : (
        <ProfileView profile={profile} km={km} fromKm={from} toKm={from + span} tickEvery={span > 15 ? 5 : 2} className="zoom" showSectorNames detail />
        )}
      </div>
      <button className="route-mini" onClick={() => setOpen(true)} title="Show the whole course">
        <ProfileView profile={profile} km={km} fromKm={0} toKm={total} tickEvery={total > 150 ? 50 : 25} minElevRange={600} labels="km" className="mini" marks={marks} />
        <span className="route-mini-hint">Whole course ⤢</span>
      </button>
      {open && (
        <div className="route-modal" onClick={() => setOpen(false)}>
          <div className="route-modal-body" onClick={(e) => e.stopPropagation()}>
            <div className="route-modal-head">
              <h2>Course profile · {total.toFixed(0)} km · {Math.max(0, total - km).toFixed(1)} km to go</h2>
              <button className="secondary" onClick={() => setOpen(false)}>Close</button>
            </div>
            <ProfileView profile={profile} km={km} fromKm={0} toKm={total} tickEvery={total > 150 ? 20 : 10} minElevRange={600} className="large" marks={marks} detail showSectorNames />
            {climbs.length > 0 && (
              <table className="route-climbs">
                <thead>
                  <tr><th>Climb</th><th>km to go</th><th>Length</th><th>Avg</th><th>Gain</th></tr>
                </thead>
                <tbody>
                  {climbs.map((c) => (
                    <tr key={c.startKm} className={c.startKm + c.lengthKm < km ? 'done' : ''}>
                      <td>{c.name ? `${c.name} ${stars(c.stars || 0)}` : `km ${c.startKm.toFixed(1)}`}</td>
                      <td>{(total - c.startKm).toFixed(1)}</td>
                      <td>{c.lengthKm.toFixed(1)} km</td>
                      <td>{c.avgPct.toFixed(1)}%</td>
                      <td>{Math.round(c.gainM)} m</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            {sectors.length > 0 && (
              <table className="route-climbs">
                <thead>
                  <tr><th>{sectors.every((s) => s.surface === 'gravel') ? 'Gravel sector' : sectors.some((s) => s.surface === 'gravel') ? 'Sector' : 'Cobbled sector'}</th><th>km to go</th><th>Length</th><th>Difficulty</th></tr>
                </thead>
                <tbody>
                  {sectors.map((sec) => (
                    <tr key={sec.startKm} className={sec.startKm + sec.lengthKm < km ? 'done' : ''}>
                      <td>{sec.name}</td>
                      <td>{(total - sec.startKm).toFixed(1)}</td>
                      <td>{sec.lengthKm.toFixed(1)} km</td>
                      <td className="stars">{stars(sec.stars)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
