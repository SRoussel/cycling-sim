import { useMemo, useState } from 'react'
import { RouteProfile, clamp01, formatRaceClock } from '../../sim/shared'
import { ProfileView, findClimbs } from './ProfileView'
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
}) {
  const total = profile?.totalKm || kmTotal
  return (
    <header className="dash-top">
      <div className="dash-top-row">
        <div className="dash-title">
          <h1>{title}</h1>
          {subtitle && <span className="dash-subtitle">{subtitle}</span>}
        </div>
        <div className="dash-stat dash-km">
          <span className="dash-stat-value">{kmRemaining.toFixed(1)}</span>
          <span className="dash-stat-label">km to go</span>
        </div>
        <div className="dash-stat">
          <span className="dash-stat-value">{formatRaceClock(elapsed)}</span>
          <span className="dash-stat-label">race time</span>
        </div>
        {extra}
        <PlaybackControls
          paused={paused}
          speed={speed}
          disabled={playbackDisabled}
          onTogglePause={onTogglePause}
          onSpeedChange={onSpeedChange}
        />
      </div>
      <RouteStrip profile={profile} km={kmCovered} kmTotal={total} />
    </header>
  )
}

/** Close-up of the road ahead (where racing decisions are made), the next
 * climb, and the whole course as a small strip that opens a large view. */
const ZOOM_BEHIND_KM = 3
const ZOOM_SPAN_KM = 28

function RouteStrip({ profile, km, kmTotal }: { profile: RouteProfile | null; km: number; kmTotal: number }) {
  const [open, setOpen] = useState(false)
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
  const current = climbs.find((c) => km >= c.startKm && km < c.startKm + c.lengthKm)
  const next = climbs.find((c) => c.startKm > km)
  return (
    <div className="route-strip">
      <div className="route-zoom">
        <div className="route-zoom-head">
          <span className="route-zoom-title">Road ahead · km to go</span>
          {current ? (
            <span className="route-climb on">
              ⛰ On a climb: {(current.startKm + current.lengthKm - km).toFixed(1)} km to the top at {current.avgPct.toFixed(1)}%
            </span>
          ) : next ? (
            <span className="route-climb">
              Next climb in {(next.startKm - km).toFixed(1)} km: {next.lengthKm.toFixed(1)} km at {next.avgPct.toFixed(1)}%
              {' '}({(total - next.startKm).toFixed(0)} km to go)
            </span>
          ) : (
            <span className="route-climb">No climbs left</span>
          )}
        </div>
        <ProfileView profile={profile} km={km} fromKm={from} toKm={from + span} tickEvery={span > 15 ? 5 : 2} className="zoom" />
      </div>
      <button className="route-mini" onClick={() => setOpen(true)} title="Show the whole course">
        <ProfileView profile={profile} km={km} fromKm={0} toKm={total} tickEvery={total > 150 ? 50 : 25} minElevRange={600} labels="km" className="mini" />
        <span className="route-mini-hint">Whole course ⤢</span>
      </button>
      {open && (
        <div className="route-modal" onClick={() => setOpen(false)}>
          <div className="route-modal-body" onClick={(e) => e.stopPropagation()}>
            <div className="route-modal-head">
              <h2>Course profile · {total.toFixed(0)} km · {Math.max(0, total - km).toFixed(1)} km to go</h2>
              <button className="secondary" onClick={() => setOpen(false)}>Close</button>
            </div>
            <ProfileView profile={profile} km={km} fromKm={0} toKm={total} tickEvery={total > 150 ? 20 : 10} minElevRange={600} className="large" />
            {climbs.length > 0 && (
              <table className="route-climbs">
                <thead>
                  <tr><th>Climb starts</th><th>km to go</th><th>Length</th><th>Avg</th><th>Gain</th></tr>
                </thead>
                <tbody>
                  {climbs.map((c) => (
                    <tr key={c.startKm} className={c.startKm + c.lengthKm < km ? 'done' : ''}>
                      <td>km {c.startKm.toFixed(1)}</td>
                      <td>{(total - c.startKm).toFixed(1)}</td>
                      <td>{c.lengthKm.toFixed(1)} km</td>
                      <td>{c.avgPct.toFixed(1)}%</td>
                      <td>{Math.round(c.gainM)} m</td>
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
