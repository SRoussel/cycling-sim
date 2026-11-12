import { RouteProfile, clamp01, formatRaceClock, profilePath } from '../../sim/shared'
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
  const pct = total > 0 ? clamp01(kmCovered / total) * 100 : 0
  const path = profile && profile.points.length > 0 ? profilePath(profile.points) : ''
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
      <div className="dash-profile" title={`${kmCovered.toFixed(1)} of ${total.toFixed(1)} km`}>
        {path ? (
          <svg viewBox="0 0 100 28" preserveAspectRatio="none" aria-label="Route profile and progress">
            <defs>
              <clipPath id="dash-done">
                <rect x="0" y="0" width={pct} height="28" />
              </clipPath>
            </defs>
            <path d={`${path} L 100 28 L 0 28 Z`} className="dash-profile-fill" />
            <path d={`${path} L 100 28 L 0 28 Z`} className="dash-profile-done" clipPath="url(#dash-done)" />
            <path d={path} className="dash-profile-line" />
          </svg>
        ) : (
          <div className="dash-profile-bar">
            <div className="dash-profile-bar-fill" style={{ width: `${pct}%` }} />
          </div>
        )}
        <div className="dash-profile-marker" style={{ left: `${pct}%` }} />
        <div className="dash-profile-scale">
          <span>0</span>
          <span>{total.toFixed(0)} km</span>
        </div>
      </div>
    </header>
  )
}
