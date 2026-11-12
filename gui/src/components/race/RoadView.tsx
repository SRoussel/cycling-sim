import { RaceGroup } from '../../sim/shared'
import { TeamControl } from '../../sim/useTeamControl'

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
            const mine = tc ? tc.sortedTeamRiders.filter((r) => tc.riderGroups[r.id] === g.name) : []
            const size = g.size ?? g.riders.length
            return (
              <li key={i} className={`road-group${mine.length > 0 ? ' has-mine' : ''}`}>
                <div className="road-group-head">
                  <span className="road-group-name">{g.name}</span>
                  <span className="road-group-size">{size} {size === 1 ? 'rider' : 'riders'}</span>
                  <span className="road-group-gap">{g.gap}</span>
                </div>
                {mine.length > 0 && (
                  <div className="road-group-mine">{mine.map((r) => r.name).join(', ')}</div>
                )}
                <div className="road-group-riders">{g.riders.join(', ')}</div>
              </li>
            )
          })}
        </ol>
      )}
    </section>
  )
}
