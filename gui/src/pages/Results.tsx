import { useParams, Link, useLocation, useNavigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../api'
import { app } from '../api'
import { formatTime, formatGap } from '../utils/format'
import { teamColor } from '../sim/teamColors'
import './Results.css'

function Results() {
  const { raceId } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const [results, setResults] = useState<app.SimulationResult[]>([])
  const [raceName, setRaceName] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [stageNav, setStageNav] = useState<{
    stageRaceId: number
    stageRaceName: string
    stages: app.StageRaceStageInfo[]
    index: number
  } | null>(null)
  
  // Get results passed from simulation
  const passedResults = location.state?.results as app.RaceResults | undefined

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
          stageRaces.map(async (race: app.StageRaceInfo) => ({
            race,
            stages: await App.GetStageRaceStages(race.id),
          }))
        )

        for (const entry of stageLists) {
          const index = entry.stages.findIndex(
            (stage: app.StageRaceStageInfo) => stage.raceId === Number(raceId)
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

  // Finishing groups: riders on the same time finished together.
  const finishers = results.filter((r) => r.position > 0)
  const dnf = results.filter((r) => r.position === 0)
  const groups: Array<{ gap: number; riders: app.SimulationResult[] }> = []
  for (const r of finishers) {
    const last = groups[groups.length - 1]
    if (last && Math.abs(r.gap - last.gap) < 1) last.riders.push(r)
    else groups.push({ gap: r.gap, riders: [r] })
  }
  const winner = finishers[0]
  const first = groups[0]
  const second = groups[1]
  let headline = winner ? `${winner.riderName} wins` : raceName
  if (winner && first) {
    if (first.riders.length === 1 && second) headline = `${winner.riderName} wins solo by ${formatGap(second.gap).replace('+', '')}`
    else if (first.riders.length === 2) headline = `${winner.riderName} wins a two-up sprint`
    else if (first.riders.length > 2) headline = `${winner.riderName} wins a sprint of ${first.riders.length}`
  }
  const groupLabel = (g: { gap: number; riders: app.SimulationResult[] }, i: number) =>
    i === 0 ? (g.riders.length === 1 ? 'Winner' : `Front group of ${g.riders.length}`) : g.riders.length === 1 ? 'Alone' : `Group of ${g.riders.length}`

  return (
    <div className="results page-enter">
      <header className="results-header">
        <span className="results-kicker">{raceName}{winner ? ` · ${formatTime(winner.time)}` : ''}</span>
        <h1>{headline}</h1>
        {finishers.length >= 3 && (
          <p className="results-podium">
            {finishers.slice(1, 3).map((r) => `${r.position === 2 ? '2nd' : '3rd'} ${r.riderName}${r.gap >= 1 ? ` (${formatGap(r.gap)})` : ''}`).join(' · ')}
            {` · ${finishers.length} finishers`}{dnf.length > 0 ? `, ${dnf.length} did not finish` : ''}
          </p>
        )}
      </header>

      {stageNav && (
        <div className="results-actions top-actions">
          <button
            className="secondary"
            onClick={() => handleStageNav(stageNav.index - 1)}
            disabled={stageNav.index <= 0}
          >
            ← Previous stage
          </button>
          <Link to={`/stage-race-stages/${stageNav.stageRaceId}`}>
            <button className="secondary">Stage list</button>
          </Link>
          <button
            className="secondary"
            onClick={() => handleStageNav(stageNav.index + 1)}
            disabled={stageNav.index >= stageNav.stages.length - 1}
          >
            Next stage →
          </button>
        </div>
      )}

      <section className="card result-groups" aria-label="Result">
        {groups.map((g, i) => (
          <div key={i} className="rg">
            <div className="rg-head">
              <span>{groupLabel(g, i)}</span>
              <span className="rg-gap">{i === 0 ? formatTime(g.riders[0].time) : formatGap(g.gap)}</span>
            </div>
            <ol className="rg-riders">
              {g.riders.map((r) => (
                <li key={`${r.riderId}-${r.position}`} className={r.position <= 3 ? 'podium' : ''}>
                  <span className="rg-pos">{r.position}</span>
                  <span className="rg-name"><i style={{ background: teamColor(r.team) }} />{r.riderName}</span>
                  <span className="rg-team">{r.team}</span>
                </li>
              ))}
            </ol>
          </div>
        ))}
        {dnf.length > 0 && (
          <div className="rg">
            <div className="rg-head"><span>Did not finish</span><span className="rg-gap">{dnf.length}</span></div>
            <ol className="rg-riders">
              {dnf.map((r) => (
                <li key={`${r.riderId}-dnf`}>
                  <span className="rg-pos">—</span>
                  <span className="rg-name"><i style={{ background: teamColor(r.team) }} />{r.riderName}</span>
                  <span className="rg-team">{r.team}</span>
                </li>
              ))}
            </ol>
          </div>
        )}
      </section>

      <div className="results-actions">
        <Link to="/">
          <button className="secondary">← All races</button>
        </Link>
        <Link to={`/race-setup/${raceId}`}>
          <button className="primary">Race it again</button>
        </Link>
      </div>
    </div>
  )
}

export default Results
