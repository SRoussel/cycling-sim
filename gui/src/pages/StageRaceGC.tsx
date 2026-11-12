import { useParams, Link, useLocation } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { formatTime, formatGap } from '../utils/format'
import './Results.css'

function StageRaceGC() {
  const { stageRaceId } = useParams()
  const location = useLocation()
  const gcGoal = (location.state?.gcGoal ?? null) as { kind: string; riderName: string; target: number; text: string } | null
  const myTeam = (location.state?.team ?? '') as string
  const [standings, setStandings] = useState<main.GCStanding[]>([])
  const [stageRace, setStageRace] = useState<main.StageRaceInfo | null>(null)
  const [riderStats, setRiderStats] = useState<Record<string, main.RiderInfo>>({})
  const [points, setPoints] = useState<main.PointsStanding[]>([])
  const [mountains, setMountains] = useState<main.MountainsStanding[]>([])
  const [youth, setYouth] = useState<main.YouthStanding[]>([])
  const [activeTab, setActiveTab] = useState<'gc' | 'points' | 'mountains' | 'youth'>('gc')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function load() {
      try {
        const raceId = parseInt(stageRaceId || '0')
        const [stageRaces, riders] = await Promise.all([
          App.GetStageRaces(),
          App.GetRiders(),
        ])
        const race = stageRaces.find((sr: main.StageRaceInfo) => sr.id === raceId)
        if (race) {
          setStageRace(race)
        }

        const riderMap: Record<string, main.RiderInfo> = {}
        riders.forEach((r: main.RiderInfo) => {
          riderMap[r.name] = r
        })
        setRiderStats(riderMap)

        const finalStage = race?.stageCount || 21
        const [gc, pts, kom, u25] = await Promise.all([
          App.GetGCStandings(raceId, finalStage),
          App.GetPointsStandings(raceId, finalStage),
          App.GetMountainsStandings(raceId, finalStage),
          App.GetYouthStandings(raceId, finalStage),
        ])
        setStandings(gc)
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
        <div className="loading">Loading GC standings...</div>
      </div>
    )
  }

  if (error || standings.length === 0) {
    return (
      <div className="results">
        <div className="error-message">
          <h2>No Results</h2>
          <p>{error || 'No GC standings available. Please run a stage race first.'}</p>
          <Link to="/">
            <button className="primary">← Back to Home</button>
          </Link>
        </div>
      </div>
    )
  }

  const top3 = standings.slice(0, 3)

  return (
    <div className="results page-enter">
      <header className="results-header">
        <h1>{stageRace?.name || 'Stage Race'}</h1>
        <p>General Classification - Final</p>
      </header>

      {gcGoal && (() => {
        const ours = standings.filter((s) => s.team === myTeam)
        const goalRider = standings.find((s) => s.riderName === gcGoal.riderName)
        const best = ours[0]
        const met = gcGoal.kind === 'gc' ? !!best && best.position <= gcGoal.target : false
        return (
          <section className={`card gc-verdict ${met ? 'met' : 'missed'}`}>
            <h2>{gcGoal.kind === 'gc' ? `${met ? '✅ Goal met' : '❌ Goal missed'}: ${gcGoal.text}` : `🎯 ${gcGoal.text}: see the stage results`}</h2>
            {gcGoal.kind === 'gc' && (
              <p>
                {goalRider ? `${goalRider.riderName} finished ${goalRider.position}${goalRider.gap > 0 ? `, ${formatGap(goalRider.gap)} down` : ''}.` : `${gcGoal.riderName} didn't finish.`}
                {best && goalRider && best.riderName !== goalRider.riderName && ` Best of ours: ${best.riderName}, ${best.position}.`}
              </p>
            )}
          </section>
        )
      })()}

      <div className="results-actions top-actions">
        <Link to="/">
          <button className="secondary">← Home</button>
        </Link>
        <Link to={`/stage-race-stages/${stageRaceId}`}>
          <button className="secondary">Stage Results</button>
        </Link>
        <Link to={`/stage-race/${stageRaceId}`}>
          <button className="primary">Run Again</button>
        </Link>
      </div>

      <div className="comp-tabs" role="tablist" aria-label="Stage race competitions">
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'gc'}
          aria-controls="tab-gc"
          className={activeTab === 'gc' ? 'tab active' : 'tab'}
          onClick={() => setActiveTab('gc')}
        >
          GC
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'points'}
          aria-controls="tab-points"
          className={activeTab === 'points' ? 'tab active' : 'tab'}
          onClick={() => setActiveTab('points')}
        >
          Points
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'mountains'}
          aria-controls="tab-mountains"
          className={activeTab === 'mountains' ? 'tab active' : 'tab'}
          onClick={() => setActiveTab('mountains')}
        >
          Mountains
        </button>
        <button
          type="button"
          role="tab"
          aria-selected={activeTab === 'youth'}
          aria-controls="tab-youth"
          className={activeTab === 'youth' ? 'tab active' : 'tab'}
          onClick={() => setActiveTab('youth')}
        >
          Youth
        </button>
      </div>

      {activeTab === 'gc' && standings.length >= 3 && (
        <section className="card podium">
          <div className="podium-places">
            <div className="podium-place second">
              <span className="place-num">2</span>
              <span className="place-name">{top3[1].riderName}</span>
              <span className="place-gap">{formatGap(top3[1].totalTime - top3[0].totalTime)}</span>
            </div>
            <div className="podium-place first">
              <span className="place-num">1st</span>
              <span className="place-name">{top3[0].riderName}</span>
              <span className="place-time">{formatTime(top3[0].totalTime)}</span>
            </div>
            <div className="podium-place third">
              <span className="place-num">3</span>
              <span className="place-name">{top3[2].riderName}</span>
              <span className="place-gap">{formatGap(top3[2].totalTime - top3[0].totalTime)}</span>
            </div>
          </div>
        </section>
      )}

      {activeTab === 'gc' && (
        <section className="card full-results" id="tab-gc" role="tabpanel">
          <h2>General Classification</h2>
          <table>
            <thead>
              <tr>
                <th>Pos</th>
                <th>Rider</th>
                <th>Team</th>
                <th>Handling</th>
                <th>Climb</th>
                <th>TT</th>
                <th>Endurance</th>
                <th>Total Time</th>
                <th>Gap</th>
              </tr>
            </thead>
            <tbody>
              {standings.map((s, idx) => {
                const stats = riderStats[s.riderName]
                return (
                  <tr key={idx} className={idx < 3 ? 'podium-row' : ''}>
                    <td>{idx + 1}</td>
                    <td>{s.riderName}</td>
                    <td>{s.team}</td>
                    <td>{stats ? stats.handling : '—'}</td>
                    <td>{stats ? stats.climbing : '—'}</td>
                    <td>{stats ? stats.tt : '—'}</td>
                    <td>{stats ? stats.endurance : '—'}</td>
                    <td>{formatTime(s.totalTime)}</td>
                    <td>{formatGap(s.totalTime - standings[0].totalTime)}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </section>
      )}

      {activeTab === 'points' && (
        <section className="card full-results" id="tab-points" role="tabpanel">
          <h2>Points Classification</h2>
          {points.length === 0 ? (
            <div className="empty-state">No points standings available yet.</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Pos</th>
                  <th>Rider</th>
                  <th>Team</th>
                  <th>Points</th>
                  <th>Gap</th>
                </tr>
              </thead>
              <tbody>
                {points.slice(0, 25).map((s, idx) => (
                  <tr key={idx} className={idx === 0 ? 'podium-row' : ''}>
                    <td>{s.position}</td>
                    <td>{s.riderName}</td>
                    <td>{s.team}</td>
                    <td>{s.points}</td>
                    <td>{s.gap === 0 ? '—' : `-${s.gap}`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}

      {activeTab === 'mountains' && (
        <section className="card full-results" id="tab-mountains" role="tabpanel">
          <h2>Mountains Classification</h2>
          {mountains.length === 0 ? (
            <div className="empty-state">No mountains standings available yet.</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Pos</th>
                  <th>Rider</th>
                  <th>Team</th>
                  <th>Points</th>
                  <th>Gap</th>
                </tr>
              </thead>
              <tbody>
                {mountains.slice(0, 25).map((s, idx) => (
                  <tr key={idx} className={idx === 0 ? 'podium-row' : ''}>
                    <td>{s.position}</td>
                    <td>{s.riderName}</td>
                    <td>{s.team}</td>
                    <td>{s.points}</td>
                    <td>{s.gap === 0 ? '—' : `-${s.gap}`}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}

      {activeTab === 'youth' && (
        <section className="card full-results" id="tab-youth" role="tabpanel">
          <h2>Youth Classification (U25)</h2>
          {youth.length === 0 ? (
            <div className="empty-state">No youth standings available yet.</div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Pos</th>
                  <th>Rider</th>
                  <th>Team</th>
                  <th>Total Time</th>
                  <th>Gap</th>
                </tr>
              </thead>
              <tbody>
                {youth.slice(0, 25).map((s, idx) => (
                  <tr key={idx} className={idx === 0 ? 'podium-row' : ''}>
                    <td>{s.position}</td>
                    <td>{s.riderName}</td>
                    <td>{s.team}</td>
                    <td>{formatTime(s.totalTime)}</td>
                    <td>{formatGap(s.gap)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>
      )}
    </div>
  )
}

export default StageRaceGC
