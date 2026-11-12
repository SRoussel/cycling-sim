import { RaceGroup } from '../../sim/shared'
import { TeamControl } from '../../sim/useTeamControl'

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
                    {others.join(', ')}
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
