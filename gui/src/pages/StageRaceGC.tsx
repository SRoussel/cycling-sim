import { useParams, Link, useLocation } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { formatTime, formatGap } from '../utils/format'
import { teamColor } from '../sim/teamColors'
import './Results.css'

function StageRaceGC() {
  const { stageRaceId } = useParams()
  const location = useLocation()
  const gcGoal = (location.state?.gcGoal ?? null) as { kind: string; riderName: string; target: number; text: string } | null
  const myTeam = (location.state?.team ?? '') as string
  const [standings, setStandings] = useState<main.GCStanding[]>([])
  const [stageRace, setStageRace] = useState<main.StageRaceInfo | null>(null)
  const [points, setPoints] = useState<main.PointsStanding[]>([])
  const [mountains, setMountains] = useState<main.MountainsStanding[]>([])
  const [youth, setYouth] = useState<main.YouthStanding[]>([])
  const [activeTab, setActiveTab] = useState<'gc' | 'points' | 'mountains' | 'youth'>('gc')
  const [history, setHistory] = useState<main.GCStanding[][]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function load() {
      try {
        const raceId = parseInt(stageRaceId || '0')
        const stageRaces = await App.GetStageRaces()
        const race = stageRaces.find((sr: main.StageRaceInfo) => sr.id === raceId)
        if (race) {
          setStageRace(race)
        }

        const finalStage = race?.stageCount || 21
        const [gc, pts, kom, u25] = await Promise.all([
          App.GetGCStandings(raceId, finalStage),
          App.GetPointsStandings(raceId, finalStage),
          App.GetMountainsStandings(raceId, finalStage),
          App.GetYouthStandings(raceId, finalStage),
        ])
        setStandings(gc)
        // The GC after each stage, for how the final top five got there.
        const perStage = await Promise.all(
          Array.from({ length: finalStage }, (_, i) => App.GetGCStandings(raceId, i + 1).catch(() => [] as main.GCStanding[])),
        )
        setHistory(perStage.map((st) => st || []))
        setPoints(pts || [])
        setMountains(kom || [])
        setYouth(u25 || [])
      } catch (err) {
        setError('Failed to load GC standings')
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [stageRaceId])

  if (loading) {
    return (
      <div className="results">
        <div className="loading">Loading the standings…</div>
      </div>
    )
  }

  if (error || standings.length === 0) {
    return (
      <div className="results">
        <div className="error-message">
          <h2>No results</h2>
          <p>{error || 'No GC standings yet: run the stage race first.'}</p>
          <Link to="/">
            <button className="primary">← All races</button>
          </Link>
        </div>
      </div>
    )
  }

  const leader = standings[0]
  const second = standings[1]
  const tabs: Array<[typeof activeTab, string]> = [['gc', 'General'], ['points', 'Points'], ['mountains', 'Mountains'], ['youth', 'Youth']]
  const rows: StandingRow[] =
    activeTab === 'gc'
      ? standings.map((s) => ({ pos: s.position, name: s.riderName, team: s.team, value: s.position === 1 ? formatTime(s.totalTime) : formatGap(s.gap), bar: s.gap }))
      : activeTab === 'youth'
        ? youth.slice(0, 25).map((s) => ({ pos: s.position, name: s.riderName, team: s.team, value: s.position === 1 ? formatTime(s.totalTime) : formatGap(s.gap), bar: s.gap }))
        : (activeTab === 'points' ? points : mountains).slice(0, 25).map((s) => ({ pos: s.position, name: s.riderName, team: s.team, value: `${s.points}`, bar: s.points }))

  let verdict: React.ReactNode = null
  if (gcGoal) {
    const ours = standings.filter((s) => s.team === myTeam)
    const goalRider = standings.find((s) => s.riderName === gcGoal.riderName)
    const best = ours[0]
    const met = gcGoal.kind === 'gc' ? !!best && best.position <= gcGoal.target : false
    verdict = gcGoal.kind === 'gc' ? (
      <p className={`gc-goal ${met ? 'met' : 'missed'}`}>
        <b>Goal {met ? 'met' : 'missed'}: {gcGoal.text}.</b>{' '}
        {goalRider ? `${goalRider.riderName} finished ${goalRider.position}${goalRider.gap > 0 ? `, ${formatGap(goalRider.gap).replace('+', '')} down` : ''}.` : `${gcGoal.riderName} didn't finish.`}
        {best && goalRider && best.riderName !== goalRider.riderName && ` Best of ours: ${best.riderName}, ${best.position}.`}
      </p>
    ) : (
      <p className="gc-goal"><b>{gcGoal.text}:</b> see the stage results.</p>
    )
  }

  return (
    <div className="results page-enter">
      <header className="results-header">
        <span className="results-kicker">{stageRace?.name || 'Stage race'} · final classification</span>
        <h1>{leader.riderName} wins{second ? ` by ${formatGap(second.gap).replace('+', '')}` : ''}</h1>
        {standings.length >= 3 && (
          <p className="results-podium">2nd {standings[1].riderName} · 3rd {standings[2].riderName}{standings[2].gap >= 1 ? ` (${formatGap(standings[2].gap)})` : ''}</p>
        )}
        {verdict}
      </header>

      <div className="results-actions top-actions">
        <Link to={`/stage-race-stages/${stageRaceId}`}>
          <button className="secondary">Stage results</button>
        </Link>
        <Link to={`/stage-race/${stageRaceId}`}>
          <button className="primary">Race it again</button>
        </Link>
      </div>

      <div className="gc-cols">
        <section className="card standings-card">
          <div className="seg-tabs" role="tablist" aria-label="Classification">
            {tabs.map(([k, label]) => (
              <button key={k} type="button" role="tab" aria-selected={activeTab === k} className={activeTab === k ? 'on' : ''} onClick={() => setActiveTab(k)}>{label}</button>
            ))}
          </div>
          {rows.length === 0 ? (
            <div className="empty-state">No standings yet.</div>
          ) : (
            <StandingsTable rows={rows} myTeam={myTeam} points={activeTab === 'points' || activeTab === 'mountains'} />
          )}
        </section>

        {history.length > 1 && <GCStory history={history} final={standings} />}
      </div>
    </div>
  )
}

interface StandingRow {
  pos: number
  name: string
  team: string
  value: string
  /** Gap in seconds (times) or points: drawn as a bar against the longest. */
  bar: number
}

function StandingsTable({ rows, myTeam, points }: { rows: StandingRow[]; myTeam: string; points: boolean }) {
  const max = Math.max(1, ...rows.map((r) => r.bar))
  return (
    <ol className="standings-list">
      {rows.map((r) => (
        <li key={`${r.pos}-${r.name}`} className={`${r.pos === 1 ? 'lead' : ''}${myTeam && r.team === myTeam ? ' mine' : ''}`}>
          <span className="sl-pos">{r.pos}</span>
          <span className="sl-who"><i style={{ background: teamColor(r.team) }} /><span>{r.name}<small>{r.team}</small></span></span>
          <span className="sl-bar"><i style={{ width: `${Math.max(points ? 2 : 0, (r.bar / max) * 100)}%` }} /></span>
          <span className="sl-val">{r.value}</span>
        </li>
      ))}
    </ol>
  )
}

/** The time behind the leader after each stage for the final top five. */
function GCStory({ history, final }: { history: main.GCStanding[][]; final: main.GCStanding[] }) {
  const top = final.slice(0, 5)
  const W = 520
  const H = 260
  const padL = 46
  const padR = 150
  const padT = 16
  const padB = 34
  const n = history.length
  const series = top.map((t) => history.map((st) => {
    const row = st.find((r) => r.riderName === t.riderName)
    return row ? row.gap : null
  }))
  const maxGap = Math.max(30, ...series.flat().filter((g): g is number => g !== null))
  const x = (i: number) => padL + (n === 1 ? 0 : (i / (n - 1)) * (W - padL - padR))
  const y = (g: number) => padT + (Math.min(g, maxGap) / maxGap) * (H - padT - padB)
  const step = maxGap > 600 ? 300 : maxGap > 180 ? 60 : 30
  const ticks = Array.from({ length: Math.floor(maxGap / step) + 1 }, (_, i) => i * step)
  const fmt = (g: number) => `${Math.floor(g / 60)}:${String(Math.round(g % 60)).padStart(2, '0')}`
  // End labels, pushed apart so they don't overlap.
  const ends = top.map((t, k) => ({ t, k, y: y(series[k][n - 1] ?? 0) })).sort((a, b) => a.y - b.y)
  for (let i = 1; i < ends.length; i++) ends[i].y = Math.max(ends[i].y, ends[i - 1].y + 15)
  const every = n > 12 ? 3 : 1
  return (
    <section className="card gc-story" aria-label="How the GC moved">
      <div className="gc-story-head">
        <h2>How the GC moved</h2>
        <span>Time behind the leader after each stage</span>
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" role="img" aria-label="Time behind the leader after each stage for the final top five">
        {ticks.map((g) => (
          <g key={g}>
            <line x1={padL} x2={W - padR} y1={y(g)} y2={y(g)} className="gs-grid" />
            <text x={padL - 6} y={y(g) + 3.5} textAnchor="end" className="gs-axis">{fmt(g)}</text>
          </g>
        ))}
        {history.map((_, i) => (i % every === 0 || i === n - 1) && (
          <text key={i} x={x(i)} y={H - padB + 16} textAnchor="middle" className="gs-axis">{i + 1}</text>
        ))}
        <text x={(padL + W - padR) / 2} y={H - 4} textAnchor="middle" className="gs-caption">Stage</text>
        {series.map((pts, k) => {
          const d = pts.map((g, i) => (g === null ? '' : `${i === 0 || pts[i - 1] === null ? 'M' : 'L'}${x(i).toFixed(1)},${y(g).toFixed(1)}`)).join(' ')
          return <path key={k} d={d} className={`gs-line${k === 0 ? ' lead' : ''}`} style={{ stroke: teamColor(top[k].team) }} />
        })}
        {ends.map(({ t, k, y: ly }) => (
          <text key={k} x={W - padR + 8} y={ly + 4} className={`gs-label${k === 0 ? ' lead' : ''}`}>
            {t.riderName.split(' ').slice(-1)[0]} {k > 0 ? formatGap(t.gap) : ''}
          </text>
        ))}
      </svg>
    </section>
  )
}

export default StageRaceGC
