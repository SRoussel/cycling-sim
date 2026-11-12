import { RaceGroup } from '../../sim/shared'
import { TeamControl } from '../../sim/useTeamControl'
import { GroupMark } from './ProfileView'
import { teamColor } from '../../sim/teamColors'

/** One colour per group position on the road, shared with the team cards. */
export const GROUP_COLORS = ['#f59e0b', '#ef4444', '#a855f7', '#3b82f6', '#14b8a6', '#84cc16', '#ec4899', '#94a3b8']

export function groupColor(groups: RaceGroup[], name: string | undefined): string {
  const idx = groups.findIndex((g) => g.name === name)
  return idx >= 0 ? GROUP_COLORS[idx % GROUP_COLORS.length] : '#64748b'
}

/**
 * The road: every group from the front, with its gap and size, and which of
 * the player's riders are in it.
 */
export function RoadView({ groups, tc, emptyText }: { groups: RaceGroup[]; tc?: TeamControl; emptyText: string }) {
  return (
    <section className="dash-panel road-view">
      <h2>On the road</h2>
      {groups.length > 1 && <GapGraphic groups={groups} tc={tc} />}
      {groups.length === 0 ? (
        <div className="empty-state">{emptyText}</div>
      ) : (
        <ol className="road-groups">
          {groups.map((g, i) => {
            const mine = tc ? tc.teamRiders.filter((r) => tc.riderGroups[r.id] === g.name) : []
            const mineNames = new Set(mine.map((r) => r.name))
            const others = g.riders.filter((n) => !mineNames.has(n))
            const size = g.size ?? g.riders.length
            return (
              <li key={i} className={`road-group${mine.length > 0 ? ' has-mine' : ''}`} style={{ '--gc': GROUP_COLORS[i % GROUP_COLORS.length] } as React.CSSProperties}>
                <div className="road-group-head">
                  <span className="road-group-dot" />
                  <span className="road-group-name">{g.name}</span>
                  <span className="road-group-size">{size} {size === 1 ? 'rider' : 'riders'}</span>
                  <span className="road-group-gap">{g.gap}</span>
                </div>
                {mine.length > 0 && (
                  <div className="road-group-mine">★ {mine.map((r) => r.name).join(', ')}</div>
                )}
                {others.length > 0 && (
                  <div className={`road-group-riders${size <= 30 ? ' full' : ''}`}>
                    {others.map((n, j) => (
                      <span key={j} className="road-rider">
                        {tc && tc.teamByName[n] && <i className="team-dot" style={{ background: teamColor(tc.teamByName[n]) }} title={tc.teamByName[n]} />}
                        {n}{j < others.length - 1 ? ', ' : ''}
                      </span>
                    ))}
                  </div>
                )}
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}

/**
 * Where each group is on the road, for the profile: the front of the race
 * at the race's km, each group behind it by its gap at its own speed.
 */
export function groupMarks(groups: RaceGroup[], tc: TeamControl): GroupMark[] {
  const front = tc.routeKm
  if (groups.length === 0 || front <= 0) return []
  const speedOf: Record<string, number> = {}
  for (const [idStr, name] of Object.entries(tc.riderGroups)) {
    if (speedOf[name] !== undefined) continue
    const t = tc.telemetryForRider(Number(idStr))
    if (t && t.speedKph > 0) speedOf[name] = t.speedKph
  }
  const mineIn = new Set(tc.teamRiders.map((r) => tc.riderGroups[r.id]))
  return groups.map((g, i) => {
    const gap = g.gapS ?? 0
    const kph = speedOf[g.name] ?? 42
    const size = g.size ?? g.riders.length
    return {
      km: Math.max(0, front - (gap * kph) / 3600),
      color: GROUP_COLORS[i % GROUP_COLORS.length],
      size,
      label: `${g.name}: ${size} ${size === 1 ? 'rider' : 'riders'}${gap > 0 ? `, ${g.gap}` : ''}`,
      mine: mineIn.has(g.name),
    }
  })
}

/**
 * The TV gap graphic: the leaders on the right, every group placed by its
 * time gap (scale fits the groups shown), sized by riders; ours ringed.
 */
function GapGraphic({ groups, tc }: { groups: RaceGroup[]; tc?: TeamControl }) {
  const shown = groups.slice(0, 8)
  const maxGap = Math.max(30, ...shown.map((g) => g.gapS ?? 0))
  const mineIn = new Set(tc ? tc.teamRiders.map((r) => tc.riderGroups[r.id]) : [])
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => f * maxGap)
  return (
    <div className="gap-graphic" aria-label="Gaps between groups">
      <div className="gap-axis" />
      {ticks.map((t, i) => (
        <span key={i} className="gap-tick" style={{ right: `${(t / maxGap) * 94 + 3}%` }}>
          {t === 0 ? '🏁' : `-${t >= 60 ? `${Math.floor(t / 60)}:${String(Math.round(t % 60)).padStart(2, '0')}` : `${Math.round(t)}s`}`}
        </span>
      ))}
      {shown.map((g, i) => {
        const size = g.size ?? g.riders.length
        const d = Math.max(10, Math.min(26, 8 + Math.sqrt(size) * 2.4))
        return (
          <span
            key={i}
            className={`gap-dot${mineIn.has(g.name) ? ' mine' : ''}`}
            title={`${g.name}: ${size} ${size === 1 ? 'rider' : 'riders'}${(g.gapS ?? 0) > 0 ? `, ${g.gap}` : ''}`}
            style={{ right: `${(((g.gapS ?? 0) / maxGap) * 94 + 3)}%`, width: d, height: d, background: GROUP_COLORS[i % GROUP_COLORS.length] }}
          >
            {d >= 16 ? size : ''}
          </span>
        )
      })}
    </div>
  )
}
