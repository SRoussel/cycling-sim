import { useEffect, useMemo, useState } from 'react'
import { RaceGroup } from '../../sim/shared'
import { TeamControl } from '../../sim/useTeamControl'
import { Jersey } from './Jersey'

type SortKey = 'name' | 'wkg' | 'now' | 'reserve' | 'fuel' | 'effort'

const level = (pct: number, low: number, empty: number) => (pct < empty ? 'empty' : pct < low ? 'low' : 'ok')

/**
 * Every rider in one road group, live: team, W/kg and threshold watts,
 * reserve, effort and legs. Our riders first; columns sort.
 */
export function GroupSheet({ name, groups, tc, onClose }: { name: string; groups: RaceGroup[]; tc: TeamControl; onClose: () => void }) {
  const [sort, setSort] = useState<SortKey>('wkg')
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && onClose()
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  const group = groups.find((g) => g.name === name)
  const rows = useMemo(() => {
    const out = Object.entries(tc.riderGroups)
      .filter(([, g]) => g === name)
      .map(([id]) => {
        const rid = Number(id)
        const info = tc.riderById[rid]
        const tel = tc.telemetryByRider[rid]
        const wkg = info && info.weight > 0 ? info.ftp / info.weight : 0
        return {
          id: rid,
          name: info?.name ?? `Rider ${rid}`,
          team: info?.team ?? '',
          mine: info?.team === tc.controlledTeam,
          ftp: info?.ftp ?? 0,
          wkg,
          reserve: tel?.wPrimePct ?? 100,
          fuel: tel?.fuelPct ?? 100,
          effort: tel?.effortPct ?? 0,
          legs: tel?.legsPct ?? 100,
          now: ((info?.ftp ?? 0) * (tel?.legsPct ?? 100)) / 100,
          nowWkg: info && info.weight > 0 ? (info.ftp * (tel?.legsPct ?? 100)) / 100 / info.weight : 0,
        }
      })
    const by: Record<SortKey, (a: (typeof out)[number], b: (typeof out)[number]) => number> = {
      name: (a, b) => a.name.localeCompare(b.name),
      wkg: (a, b) => b.wkg - a.wkg,
      reserve: (a, b) => b.reserve - a.reserve,
      fuel: (a, b) => a.fuel - b.fuel,
      effort: (a, b) => b.effort - a.effort,
      now: (a, b) => b.nowWkg - a.nowWkg,
    }
    return out.sort((a, b) => (a.mine !== b.mine ? (a.mine ? -1 : 1) : by[sort](a, b)))
  }, [tc.riderGroups, tc.riderById, tc.telemetryByRider, tc.controlledTeam, name, sort])

  const th = (key: SortKey, label: string, title?: string) => (
    <th className={sort === key ? 'sorted' : ''} onClick={() => setSort(key)} title={title}>{label}</th>
  )
  return (
    <div className="sheet-backdrop" onClick={onClose}>
      <section className="sheet" role="dialog" aria-label={`${name}: riders`} onClick={(e) => e.stopPropagation()}>
        <header className="sheet-head">
          <h2>{name}</h2>
          <span className="panel-meta">
            {rows.length} {rows.length === 1 ? 'rider' : 'riders'}
            {group && (group.gapS ?? 0) > 0 ? ` · ${group.gap}` : group ? ' · front of the race' : ''}
          </span>
          <button type="button" className="link" onClick={onClose}>Close</button>
        </header>
        {rows.length === 0 ? (
          <p className="muted">This group is no longer on the road.</p>
        ) : (
          <table className="sheet-table">
            <thead>
              <tr>
                {th('name', 'Rider')}
                {th('wkg', 'W/kg', 'Threshold power per kg this morning (and watts)')}
                {th('now', 'Threshold now', 'What he can sustain now, after today\'s fade: watts, W/kg, and how much he has lost')}
                {th('reserve', 'Reserve', 'The punch for attacks and surges; refills when riding easier')}
                {th('fuel', 'Fuel', 'Carbohydrate left; under 30% his threshold starts to fall (sorts emptiest first)')}
                {th('effort', 'Effort', 'Power now vs what he can sustain; over 100% burns reserve')}
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} className={r.mine ? 'mine' : ''}>
                  <td className="who" title={r.team}>
                    <Jersey team={r.team} />
                    <span>{r.name}</span>
                    <small>{r.team}</small>
                  </td>
                  <td className="num">{r.wkg ? r.wkg.toFixed(2) : '—'}<small> {r.ftp ? `${r.ftp} W` : ''}</small></td>
                  <td className={`num ${r.legs < 86 ? 'empty' : r.legs < 89 ? 'low' : ''}`}>
                    {r.now ? `${Math.round(r.now)} W` : '—'}
                    <small> {r.nowWkg ? r.nowWkg.toFixed(2) : ''} · −{Math.max(0, Math.round(100 - r.legs))}%</small>
                  </td>
                  <td className="res">
                    <div className="bar reserve"><i className={level(r.reserve, 50, 20)} style={{ width: `${Math.max(0, Math.min(100, r.reserve))}%` }} /></div>
                    <span className={`num ${level(r.reserve, 50, 20)}`}>{Math.round(r.reserve)}%</span>
                  </td>
                  <td className={`num ${level(r.fuel, 50, 30)}`}>{Math.round(r.fuel)}%</td>
                  <td className={`num ${r.effort > 100 ? 'over' : ''}`}>{Math.round(r.effort)}%</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  )
}
