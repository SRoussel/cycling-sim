import { RaceGroup, RouteProfile, clamp01, profilePath } from '../../sim/shared'

export function RouteProfileCard({ profile, km }: { profile: RouteProfile | null; km: number }) {
  if (!profile || profile.points.length === 0) {
    return (
      <section className="card route-profile-card">
        <h2>Route Profile</h2>
        <div className="empty-state">Profile loading…</div>
      </section>
    )
  }
  const markerKm = Math.max(0, Math.min(profile.totalKm, km))
  const markerPct = profile.totalKm > 0 ? clamp01(markerKm / profile.totalKm) * 100 : 0
  return (
    <section className="card route-profile-card">
      <h2>Route Profile</h2>
      <div className="route-profile-wrap">
        <svg className="route-profile-svg" viewBox="0 0 100 28" preserveAspectRatio="none" aria-label="Route profile">
          <path d={profilePath(profile.points || [])} className="route-profile-line" />
        </svg>
        <div className="route-profile-marker" style={{ left: `${markerPct}%` }} />
        <div className="route-profile-info">
          <span>0 km</span>
          <span>{markerKm.toFixed(1)} km · {(profile.totalKm - markerKm).toFixed(1)} to go</span>
          <span>{profile.totalKm.toFixed(1)} km</span>
        </div>
      </div>
    </section>
  )
}

export function RaceSituationPanel({ groups, emptyText }: { groups: RaceGroup[]; emptyText: string }) {
  return (
    <section className="card groups-panel">
      <h2>Race Situation</h2>
      {groups.length === 0 ? (
        <div className="empty-state">{emptyText}</div>
      ) : (
        groups.map((group, i) => (
          <div key={i} className={`group ${group.name.toLowerCase().replace(/\s+/g, '-')}`}>
            <div className="group-header">
              <span className="group-name">{group.name}</span>
              <span className="group-gap">{group.gap}</span>
            </div>
            <div className="group-riders">{group.riders.join(', ')}</div>
          </div>
        ))
      )}
    </section>
  )
}

export function EventLog({ events, emptyText }: { events: string[]; emptyText?: string }) {
  return (
    <section className="card events-panel">
      <h2>Event Log</h2>
      <ul className="events-list">
        {events.length === 0 && emptyText ? <li>{emptyText}</li> : events.map((event, i) => <li key={i}>{event}</li>)}
      </ul>
    </section>
  )
}

const SPEEDS = [0.25, 0.5, 1, 2, 4, 8]

export function PlaybackControls({
  paused,
  speed,
  disabled,
  onTogglePause,
  onSpeedChange,
}: {
  paused: boolean
  speed: number
  disabled: boolean
  onTogglePause: () => void
  onSpeedChange: (speed: number) => void
}) {
  return (
    <div className="sim-controls">
      <button onClick={onTogglePause} disabled={disabled}>
        {paused ? '▶ Play' : '⏸ Pause'}
      </button>
      <select value={speed} onChange={(e) => onSpeedChange(Number(e.target.value))} disabled={disabled}>
        {SPEEDS.map((s) => (
          <option key={s} value={s}>{s}x</option>
        ))}
      </select>
    </div>
  )
}
