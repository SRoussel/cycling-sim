import { useParams, Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import * as App from '../api'
import { app } from '../api'
import { StageStrip, useStageProfiles } from '../components/race/StageStrip'
import './Results.css'

function StageRaceStages() {
  const { stageRaceId } = useParams()
  const stageRaceIdNumber = parseInt(stageRaceId || '0')
  const [stageRace, setStageRace] = useState<app.StageRaceInfo | null>(null)
  const [stages, setStages] = useState<app.StageRaceStageInfo[]>([])
  const profiles = useStageProfiles(stages)
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
        const race = stageRaces.find((sr: app.StageRaceInfo) => sr.id === id) || null
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
    <div className="results page-enter">
      <header className="results-header">
        <span className="results-kicker">{stageRace?.name || 'Stage race'}</span>
        <h1>The stages</h1>
      </header>
      <div className="results-actions top-actions">
        <Link to={`/stage-race-gc/${stageRaceId}`}>
          <button className="secondary">Final standings</button>
        </Link>
        <Link to={`/stage-race/${stageRaceId}`}>
          <button className="primary">Race it again</button>
        </Link>
      </div>
      <StageStrip
        stages={stages}
        profiles={profiles}
        link={(stage, index) => (
          <Link
            to={`/results/${stage.raceId}`}
            state={{
              stageNav: {
                stageRaceId: stageRace?.id || stageRaceIdNumber,
                stageRaceName: stageRace?.name || 'Stage race',
                stages,
                index,
              },
            }}
          >
            Stage result →
          </Link>
        )}
      />
    </div>
  )
}

export default StageRaceStages
