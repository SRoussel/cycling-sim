import './RaceDashboard.css'
import { RaceGroup } from '../../sim/shared'
import { TeamControl } from '../../sim/useTeamControl'
import { useMemo } from 'react'
import { EventLog } from './RacePanels'
import { RoadView } from './RoadView'
import { TeamDock } from './TeamDock'

/**
 * The live race on one screen: the top bar above, the road and the race log
 * on the left, the player's team on the right. Each column scrolls on its
 * own, so the race and the controls are always in view together.
 */
export function RaceDashboard({
  top,
  groups,
  tc,
  interactive,
  kmRemaining,
  emptyText,
  footer,
}: {
  top: React.ReactNode
  groups: RaceGroup[]
  tc: TeamControl
  interactive: boolean
  kmRemaining: number
  emptyText: string
  footer?: React.ReactNode
}) {
  // Newest first, so the latest move is always in view.
  const events = useMemo(() => [...tc.events].reverse(), [tc.events])
  return (
    <div className={`race-dash${interactive ? ' interactive' : ''}`}>
      {top}
      <div className="dash-race">
        <RoadView groups={groups} tc={interactive ? tc : undefined} emptyText={emptyText} />
        <EventLog events={events} emptyText="Waiting for the race…" />
        {footer}
      </div>
      {interactive && <TeamDock tc={tc} kmRemaining={kmRemaining} groups={groups} />}
    </div>
  )
}
