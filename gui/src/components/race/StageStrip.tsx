import { useEffect, useState } from 'react'
import * as App from '../../../wailsjs/go/main/App'
import { main } from '../../../wailsjs/go/models'
import { RouteProfile } from '../../sim/shared'
import { findClimbs } from './ProfileView'
import './StageStrip.css'

/** What kind of stage it is, from its own profile. */
function stageKind(stage: main.StageRaceStageInfo, p: RouteProfile | undefined): string {
  if (stage.raceType === 'tt' || /\bITT\b|time trial/i.test(stage.raceName)) return 'Time trial'
  if (!p || p.points.length < 2) return ''
  let gain = 0
  for (let i = 1; i < p.points.length; i++) gain += Math.max(0, p.points[i].elev - p.points[i - 1].elev)
  const climbs = findClimbs(p)
  const last = climbs[climbs.length - 1]
  const uphill = !!last && p.totalKm - (last.startKm + last.lengthKm) < 1
  const kind = gain >= 2800 ? 'Mountains' : gain >= 1400 ? 'Hilly' : 'Flat'
  return uphill ? `${kind}, uphill finish` : kind
}

/** Each stage's profile, loaded once per list of stages. */
export function useStageProfiles(stages: main.StageRaceStageInfo[]): Record<number, RouteProfile> {
  const [profiles, setProfiles] = useState<Record<number, RouteProfile>>({})
  useEffect(() => {
    let live = true
    Promise.all(stages.map(async (s) => [s.raceId, (await App.PreviewRaceProfile(s.raceId)) as unknown as RouteProfile] as const)).then((e) => {
      if (live) setProfiles(Object.fromEntries(e.filter(([, p]) => p)))
    })
    return () => {
      live = false
    }
  }, [stages])
  return profiles
}

function Mini({ p }: { p: RouteProfile | undefined }) {
  if (!p || p.points.length < 2) return <svg className="ss-mini" viewBox="0 0 120 36" aria-hidden="true" />
  const lo = Math.min(...p.points.map((x) => x.elev))
  const hi = Math.max(lo + 1200, ...p.points.map((x) => x.elev))
  const x = (k: number) => (k / p.totalKm) * 120
  const y = (e: number) => 34 - ((e - lo) / (hi - lo)) * 30
  const d = 'M0,36 ' + p.points.map((q) => `L${x(q.km).toFixed(1)},${y(q.elev).toFixed(1)}`).join(' ') + ' L120,36 Z'
  return (
    <svg className="ss-mini" viewBox="0 0 120 36" preserveAspectRatio="none" aria-hidden="true">
      <path d={d} />
    </svg>
  )
}

/**
 * The stages as a row of cards: number, distance, a small profile drawn to
 * one scale (so mountains look like mountains), and what kind of stage it
 * is; optionally a link and a line under each (a result).
 */
export function StageStrip({
  stages,
  profiles,
  link,
  note,
}: {
  stages: main.StageRaceStageInfo[]
  profiles: Record<number, RouteProfile>
  link?: (stage: main.StageRaceStageInfo, index: number) => React.ReactNode
  note?: (stage: main.StageRaceStageInfo) => string
}) {
  return (
    <ol className="stage-strip">
      {stages.map((s, i) => {
        const p = profiles[s.raceId]
        const name = s.raceName.replace(/^.*?Stage \d+\s*[:-]\s*/i, '')
        return (
          <li key={s.raceId} className={s.restDayBefore ? 'rest' : ''}>
            <div className="ss-top">
              <span>Stage {s.stageNumber}</span>
              <span className="ss-km">{s.distanceKm ? `${Math.round(s.distanceKm)} km` : ''}</span>
            </div>
            <Mini p={p} />
            <span className="ss-name" title={s.raceName}>{name}</span>
            <span className="ss-kind">{note ? note(s) : stageKind(s, p)}</span>
            {link && <span className="ss-link">{link(s, i)}</span>}
          </li>
        )
      })}
    </ol>
  )
}
