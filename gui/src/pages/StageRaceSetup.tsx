import { useParams, useNavigate, Link } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './RaceSetup.css'

function StageRaceSetup() {
  const { stageRaceId } = useParams()
  const navigate = useNavigate()
  const [stageRace, setStageRace] = useState<main.StageRaceInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [startList, setStartList] = useState<main.RiderInfo[]>([])
  const [interactiveMode, setInteractiveMode] = useState(false)
  const [controlledTeam, setControlledTeam] = useState('')

  useEffect(() => {
    async function loadStageRace() {
      try {
        const stageRaces = await App.GetStageRaces()
        const race = stageRaces.find((sr: main.StageRaceInfo) => sr.id === parseInt(stageRaceId || '0'))
        if (race) {
          setStageRace(race)

          const preview = await App.PreviewStageRaceStartList(race.id)
          setStartList(preview || [])
          if (!controlledTeam && preview && preview.length > 0) {
            setControlledTeam(preview[0].team)
          }
        } else {
          setError('Stage race not found')
        }
      } catch (err) {
        setError('Failed to load stage race')
      } finally {
        setLoading(false)
      }
    }
    loadStageRace()
  }, [stageRaceId])

  async function handleStartSimulation() {
    if (!stageRace) return
    navigate(`/stage-race-sim/${stageRace.id}`, { state: { interactiveMode, controlledTeam } })
  }

  const teams = Array.from(new Set(startList.map((r) => r.team))).sort()

  if (loading) {
    return (
      <div className="race-setup">
        <div className="loading">Loading stage race...</div>
      </div>
    )
  }

  if (error || !stageRace) {
    return (
      <div className="race-setup">
        <div className="error-message">
          <h2>Error</h2>
          <p>{error || 'Stage race not found'}</p>
          <Link to="/">
            <button className="primary">← Back to Home</button>
          </Link>
        </div>
      </div>
    )
  }

  return (
    <div className="race-setup">
      <header className="setup-header">
        <h1>{stageRace.name}</h1>
      </header>

      <div className="setup-grid">
        <section className="card race-info">
          <h2>Race Information</h2>
          <div className="info-grid">
            <div className="info-item">
              <span className="info-label">Stages</span>
              <span className="info-value">{stageRace.stageCount}</span>
            </div>
            <div className="info-item">
              <span className="info-label">Total Distance</span>
              <span className="info-value">{stageRace.totalDistanceKm.toFixed(0)} km</span>
            </div>
            <div className="info-item">
              <span className="info-label">Type</span>
              <span className="info-value">
                {stageRace.stageCount >= 21 ? 'Grand Tour' : 'Stage Race'}
              </span>
            </div>
            <div className="info-item">
              <span className="info-label">Category</span>
              <span className="info-value">
                {stageRace.stageCount >= 21 ? 'World Tour' : 'Pro Series'}
              </span>
            </div>
          </div>
        </section>

        <section className="card">
          <h2>About</h2>
          <p>
            This {stageRace.stageCount}-stage race will simulate each stage in sequence,
            accumulating time for the General Classification (GC).
          </p>
          <p className="note">
            Stage races include time bonuses, mountain points, and GC calculations
            after each stage.
          </p>
        </section>

        <section className="card simulation-options">
          <h2>Simulation Options</h2>
          <div className="option-group">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={interactiveMode}
                onChange={(e) => setInteractiveMode(e.target.checked)}
              />
              Interactive mode (control one team)
            </label>
            <span className="option-help">Adds real-time controls for your team during each stage</span>
          </div>

          {interactiveMode && teams.length > 0 && (
            <div className="option-group">
              <label className="checkbox-label">
                Your team
                <select
                  value={controlledTeam}
                  onChange={(e) => setControlledTeam(e.target.value)}
                  style={{ marginLeft: 10 }}
                >
                  {teams.map((t) => (
                    <option key={t} value={t}>
                      {t}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          )}
        </section>
      </div>

      <div className="setup-actions">
        <Link to="/">
          <button className="secondary">← Back</button>
        </Link>
        <button className="primary" onClick={handleStartSimulation}>
          Start Stage Race →
        </button>
      </div>
    </div>
  )
}

export default StageRaceSetup
