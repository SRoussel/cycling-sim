import { useParams, Link, useLocation, useNavigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './Results.css'

function formatTime(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

function formatGap(seconds: number): string {
  if (seconds === 0) return '—'
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  if (m > 0) {
    return `+${m}:${s.toString().padStart(2, '0')}`
  }
  return `+${s}s`
}

function Results() {
  const { raceId } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const [results, setResults] = useState<main.SimulationResult[]>([])
  const [raceName, setRaceName] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [stageNav, setStageNav] = useState<{
    stageRaceId: number
    stageRaceName: string
    stages: main.StageRaceStageInfo[]
    index: number
  } | null>(null)
  
  // Get results passed from simulation
  const passedResults = location.state?.results as main.RaceResults | undefined

  useEffect(() => {
    async function loadResults() {
      if (passedResults) {
        setResults(passedResults.results)
        setRaceName(passedResults.raceName)
        setLoading(false)
        return
      }

      // If no passed results, try to get from API (for bookmarked URLs)
      try {
        const res = await App.GetRaceResults(Number(raceId))
        setResults(res.results)
        setRaceName(res.raceName)
      } catch (err) {
        setError('No results available. Please run a simulation first.')
      } finally {
        setLoading(false)
      }
    }
    loadResults()
  }, [passedResults, raceId])

  useEffect(() => {
    let cancelled = false

    async function loadStageNav() {
      if (!raceId) return

      const fromState = (location.state as { stageNav?: typeof stageNav })?.stageNav
      if (fromState && fromState.stages && fromState.stages.length > 0) {
        const currentIndex = fromState.stages.findIndex(
          (stage) => stage.raceId === Number(raceId)
        )
        if (!cancelled) {
          setStageNav({
            ...fromState,
            index: currentIndex >= 0 ? currentIndex : fromState.index,
          })
        }
        return
      }

      try {
        const stageRaces = await App.GetStageRaces()
        if (!stageRaces || stageRaces.length === 0) return

        const stageLists = await Promise.all(
          stageRaces.map(async (race: main.StageRaceInfo) => ({
            race,
            stages: await App.GetStageRaceStages(race.id),
          }))
        )

        for (const entry of stageLists) {
          const index = entry.stages.findIndex(
            (stage: main.StageRaceStageInfo) => stage.raceId === Number(raceId)
          )
          if (index >= 0 && !cancelled) {
            setStageNav({
              stageRaceId: entry.race.id,
              stageRaceName: entry.race.name,
              stages: entry.stages,
              index,
            })
            break
          }
        }
      } catch (err) {
        if (!cancelled) {
          setStageNav(null)
        }
      }
    }

    loadStageNav()
    return () => {
      cancelled = true
    }
  }, [location.state, raceId])

  useEffect(() => {
    if (!stageNav) return

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') return
      const target = event.target as HTMLElement | null
      if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA' || target.tagName === 'SELECT')) {
        return
      }

      const direction = event.key === 'ArrowRight' ? 1 : -1
      const nextIndex = stageNav.index + direction
      if (nextIndex < 0 || nextIndex >= stageNav.stages.length) return

      const nextStage = stageNav.stages[nextIndex]
      navigate(`/results/${nextStage.raceId}`, {
        state: {
          stageNav: {
            ...stageNav,
            index: nextIndex,
          },
        },
      })
    }

    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [navigate, stageNav])

  const handleStageNav = (nextIndex: number) => {
    if (!stageNav) return
    const nextStage = stageNav.stages[nextIndex]
    navigate(`/results/${nextStage.raceId}`, {
      state: {
        stageNav: {
          ...stageNav,
          index: nextIndex,
        },
      },
    })
  }

  if (loading) {
    return (
      <div className="results">
        <div className="loading">Loading results...</div>
      </div>
    )
  }

  if (error || results.length === 0) {
    return (
      <div className="results">
        <div className="error-message">
          <h2>No Results</h2>
          <p>{error || 'No race results available.'}</p>
          <Link to="/">
            <button className="primary">← Back to Home</button>
          </Link>
        </div>
      </div>
    )
  }

  const top3 = results.slice(0, 3)
  const avgHandling = top3.reduce((sum, r) => sum + r.handling, 0) / 3
  const specialistsInTop5 = results.slice(0, 5).filter(r => r.handling >= 80).length

  return (
    <div className="results page-enter">
      <header className="results-header">
        <h1>{raceName}</h1>
        <p>Simulation completed</p>
      </header>

      {stageNav && (
        <div className="results-actions top-actions">
          <button
            className="secondary"
            onClick={() => handleStageNav(stageNav.index - 1)}
            disabled={stageNav.index <= 0}
          >
            ← Previous Stage
          </button>
          <Link to={`/stage-race-stages/${stageNav.stageRaceId}`}>
            <button className="secondary">Stage List</button>
          </Link>
          <button
            className="secondary"
            onClick={() => handleStageNav(stageNav.index + 1)}
            disabled={stageNav.index >= stageNav.stages.length - 1}
          >
            Next Stage →
          </button>
        </div>
      )}

      {results.length >= 3 && (
        <section className="card podium">
          <div className="podium-places">
            <div className="podium-place second">
              <span className="place-num">2</span>
              <span className="place-name">{results[1].riderName}</span>
              <span className="place-gap">{formatGap(results[1].gap)}</span>
            </div>
            <div className="podium-place first">
              <span className="place-num">1st</span>
              <span className="place-name">{results[0].riderName}</span>
              <span className="place-time">{formatTime(results[0].time)}</span>
            </div>
            <div className="podium-place third">
              <span className="place-num">3</span>
              <span className="place-name">{results[2].riderName}</span>
              <span className="place-gap">{formatGap(results[2].gap)}</span>
            </div>
          </div>
        </section>
      )}

      <section className="card full-results">
        <h2>Full Results</h2>
        <table>
          <thead>
            <tr>
              <th>Pos</th>
              <th>Rider</th>
              <th>Team</th>
              <th>Time</th>
              <th>Gap</th>
              <th>Handling</th>
            </tr>
          </thead>
          <tbody>
            {results.map((r) => (
              <tr key={`${r.riderId}-${r.position}`} className={r.position > 0 && r.position <= 3 ? 'podium-row' : ''}>
                <td>{r.position === 0 ? 'DNF' : r.position}</td>
                <td>{r.riderName}</td>
                <td>{r.team}</td>
                <td>{r.position === 0 ? '—' : formatTime(r.time)}</td>
                <td>{r.position === 0 ? '—' : formatGap(r.gap)}</td>
                <td>
                  <span className={`handling-badge h-${Math.floor(r.handling / 10) * 10}`}>
                    {r.handling}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <section className="card analysis">
        <h2>Specialist Analysis</h2>
        <div className="analysis-grid">
          <div className="analysis-item">
            <span className="analysis-label">Top-3 Avg Handling</span>
            <span className="analysis-value">{avgHandling.toFixed(1)}</span>
          </div>
          <div className="analysis-item">
            <span className="analysis-label">Winner Handling</span>
            <span className="analysis-value">{results[0].handling}</span>
          </div>
          <div className="analysis-item">
            <span className="analysis-label">Specialists in Top 5</span>
            <span className="analysis-value">{specialistsInTop5} / 5</span>
          </div>
        </div>
      </section>

      <div className="results-actions">
        <Link to="/">
          <button className="secondary">← Home</button>
        </Link>
        <Link to={`/race-setup/${raceId}`}>
          <button className="primary">Run Again</button>
        </Link>
      </div>
    </div>
  )
}

export default Results
