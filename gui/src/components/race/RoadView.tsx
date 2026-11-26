import { useState } from 'react'
import { RaceGroup } from '../../sim/shared'
import { TeamControl } from '../../sim/useTeamControl'
import { GroupMark } from './ProfileView'
import { GroupSheet } from './GroupSheet'
import { teamColor } from '../../sim/teamColors'

/** A group's dot: the front of the race white, the rest grey (groups with
 * the player's riders get an orange ring where drawn). */
export function groupTone(index: number): string {
  return index === 0 ? '#f2c230' : '#8a929c'
}


/**
 * The road: every group from the front, with its gap and size, and which of
 * the player's riders are in it.
 */
export function RoadView({ groups, tc, emptyText }: { groups: RaceGroup[]; tc?: TeamControl; emptyText: string }) {
  const [open, setOpen] = useState<string | null>(null)
  return (
    <section className="panel road">
      <header className="panel-head">
        <h2>On the road</h2>
        {groups.length > 1 && <span className="panel-meta">{groups.length} groups</span>}
      </header>
      {groups.length === 0 ? (
        <div className="empty-state">{emptyText}</div>
      ) : (
        <>
          {groups.length > 1 && <GapGraphic groups={groups} tc={tc} onPick={tc ? setOpen : undefined} />}
          <ol className="groups">
            {groups.map((g, i) => {
              const mine = tc ? tc.teamRiders.filter((r) => tc.riderGroups[r.id] === g.name) : []
              const mineNames = new Set(mine.map((r) => r.name))
              const size = g.size ?? g.riders.length
              // Names other than ours: the backend lists everyone in small
              // groups, the first few (and "+N more") in big ones.
              const named = g.riders.filter((n) => !mineNames.has(n) && !/^\+\d+ more$/.test(n))
              const shown = named.slice(0, 6)
              const rest = size - mine.length - shown.length
              return (
                <li
                  key={i}
                  className={`${mine.length > 0 ? 'mine' : ''}${tc ? ' clickable' : ''}`}
                  onClick={tc ? () => setOpen(g.name) : undefined}
                  title={tc ? 'Show every rider in this group' : undefined}
                >
                  <span className="g-label">
                    <span className={`g-name${i === 0 ? ' front' : ''}`}>{g.name}</span>
                    <span className="g-size">{size} {size === 1 ? 'rider' : 'riders'}</span>
                  </span>
                  <div className="g-riders">
                    {mine.map((r) => (
                      <span key={r.id} className="g-rider g-ours"><i style={{ background: teamColor(tc!.controlledTeam) }} />{r.name}</span>
                    ))}
                    {shown.map((n, j) => (
                      <span key={j} className="g-rider" title={tc?.teamByName[n]}>
                        {tc?.teamByName[n] && <i style={{ background: teamColor(tc.teamByName[n]) }} />}{n}
                      </span>
                    ))}
                    {rest > 0 && <span className="g-more">+{rest} more</span>}
                  </div>
                  <span className="g-gap">{(g.gapS ?? 0) > 0 ? g.gap : ''}</span>
                </li>
              )
            })}
          </ol>
        </>
      )}
      {open && tc && <GroupSheet name={open} groups={groups} tc={tc} onClose={() => setOpen(null)} />}
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
      color: groupTone(i),
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
export function GapGraphic({ groups, tc, onPick, large = false }: { groups: RaceGroup[]; tc?: TeamControl; onPick?: (name: string) => void; large?: boolean }) {
  const shown = groups.slice(0, large ? 12 : 8)
  const maxGap = Math.max(30, ...shown.map((g) => g.gapS ?? 0))
  const mineIn = new Set(tc ? tc.teamRiders.map((r) => tc.riderGroups[r.id]) : [])
  const ticks = [0, 0.25, 0.5, 0.75, 1].map((f) => f * maxGap)
  // Name the first groups, skipping one too close to the last one named.
  const labelled = new Set<number>()
  let lastPos = -Infinity
  shown.forEach((g, i) => {
    const pos = ((g.gapS ?? 0) / maxGap) * 94
    if ((large || i < 5) && pos - lastPos >= (large ? 7 : 9)) {
      labelled.add(i)
      lastPos = pos
    }
  })
  return (
    <div className={`gap-graphic${large ? ' large' : ''}`} aria-label="Gaps between groups">
      <div className="gap-axis" />
      {ticks.map((t, i) => (
        <span key={i} className="gap-tick" style={{ right: `${(t / maxGap) * 94 + 3}%` }}>
          {t === 0 ? 'Front' : `-${tickLabel(t)}`}
        </span>
      ))}
      {shown.map((g, i) => {
        const size = g.size ?? g.riders.length
        const d = large ? Math.max(14, Math.min(40, 10 + Math.sqrt(size) * 3.6)) : Math.max(10, Math.min(26, 8 + Math.sqrt(size) * 2.4))
        return (
          <span
            key={i}
            onClick={onPick ? () => onPick(g.name) : undefined}
            className={`gap-dot${mineIn.has(g.name) ? ' mine' : ''}${onPick ? ' clickable' : ''}`}
            title={`${g.name}: ${size} ${size === 1 ? 'rider' : 'riders'}${(g.gapS ?? 0) > 0 ? `, ${g.gap}` : ''}`}
            style={{ right: `${(((g.gapS ?? 0) / maxGap) * 94 + 3)}%`, width: d, height: d, background: groupTone(i) }}
          >
            {d >= 16 ? size : ''}
            {labelled.has(i) && <span className="gap-dot-label">{g.name}{large && <small>{size} · {(g.gapS ?? 0) > 0 ? g.gap : 'front'}</small>}</span>}
          </span>
        )
      })}
    </div>
  )
}

/** A scale tick: whole seconds, rounded before splitting (59.6 s is 1:00, not 0:60). */
function tickLabel(t: number): string {
  const r = Math.round(t)
  return r >= 60 ? `${Math.floor(r / 60)}:${String(r % 60).padStart(2, '0')}` : `${r}s`
}
