import { useParams, Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './Results.css'

function StageRaceStages() {
  const { stageRaceId } = useParams()
  const stageRaceIdNumber = parseInt(stageRaceId || '0')
  const [stageRace, setStageRace] = useState<main.StageRaceInfo | null>(null)
  const [stages, setStages] = useState<main.StageRaceStageInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function load() {
      try {
        const id = parseInt(stageRaceId || '0')
        const [stageRaces, stageList] = await Promise.all([
          App.GetStageRaces(),
          App.GetStageRaceStages(id),
        ])
        const race = stageRaces.find((sr: main.StageRaceInfo) => sr.id === id) || null
        setStageRace(race)
        setStages(stageList || [])
      } catch (err) {
        setError('Failed to load stages')
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [stageRaceId])

  if (loading) {
    return (
      <div className="results">
        <div className="loading">Loading stages...</div>
      </div>
    )
  }

  if (error || stages.length === 0) {
    return (
      <div className="results">
        <div className="error-message">
          <h2>No Stages</h2>
          <p>{error || 'No stages available for this race.'}</p>
          <Link to={`/stage-race/${stageRaceId}`}>
            <button className="primary">← Back to Stage Race</button>
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="results">
      <header className="results-header">
        <h1>{stageRace?.name || 'Stage Race'}</h1>
        <p>Stage Results</p>
      </header>

      <section className="card full-results">
        <h2>Stages</h2>
        <table>
          <thead>
            <tr>
              <th>Stage</th>
              <th>Race</th>
              <th>Course</th>
              <th>Distance</th>
              <th>Type</th>
              <th>Date</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {stages.map((stage, index) => (
              <tr key={stage.raceId}>
                <td>{stage.stageNumber}</td>
                <td>{stage.raceName}</td>
                <td>{stage.courseName || '—'}</td>
                <td>{stage.distanceKm ? `${stage.distanceKm} km` : '—'}</td>
                <td>{stage.raceType}</td>
                <td>{stage.date || '—'}</td>
                <td>
                  <Link
                    to={`/results/${stage.raceId}`}
                    state={{
                      stageNav: {
                        stageRaceId: stageRace?.id || stageRaceIdNumber,
                        stageRaceName: stageRace?.name || 'Stage Race',
                        stages,
                        index,
                      },
                    }}
                  >
                    <button className="secondary">View Results</button>
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </section>

      <div className="results-actions">
        <Link to={`/stage-race-gc/${stageRaceId}`}>
          <button className="secondary">← GC Standings</button>
        </Link>
        <Link to={`/stage-race/${stageRaceId}`}>
          <button className="primary">Run Again</button>
        </Link>
      </div>
    </div>
  )
}

export default StageRaceStages
