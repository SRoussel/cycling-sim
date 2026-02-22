import { useParams, Link, useNavigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './RaceSetup.css'

function RaceSetup() {
  const { raceId } = useParams()
  const navigate = useNavigate()
  const [race, setRace] = useState<main.RaceInfo | null>(null)
  const [startList, setStartList] = useState<main.RiderInfo[]>([])
  const [enableIncidents, setEnableIncidents] = useState(false)

  const [interactiveMode, setInteractiveMode] = useState(false)
  const [controlledTeam, setControlledTeam] = useState('')

  useEffect(() => {
    const loadData = async () => {
      try {
        const racesData = await App.GetRaces()
        const found = racesData?.find(r => r.id === Number(raceId))
        setRace(found || null)

        if (found) {
          const preview = await App.PreviewRaceStartList(found.id)
          setStartList(preview || [])
          if (!controlledTeam && preview && preview.length > 0) {
            setControlledTeam(preview[0].team)
          }
        } else {
          setStartList([])
        }
      } catch (err) {
        console.error('Failed to load data:', err)
      }
    }
    loadData()
  }, [raceId])

  const handleStartSimulation = async () => {
    if (!race) return
    // Navigate to simulation page which will run the simulation
    navigate(`/simulation/${race.id}`, { state: { race, enableIncidents, interactiveMode, controlledTeam } })
  }

  // Get top riders by handling for preview
  const topRiders = [...startList].sort((a, b) => b.handling - a.handling).slice(0, 5)
  const teams = Array.from(new Set(startList.map((r) => r.team))).sort()

  return (
    <div className="race-setup page-enter">
      <header className="setup-header">
        <h1>Race Setup: {race?.name || raceId}</h1>
        <p>Configure simulation parameters</p>
      </header>

      <div className="setup-grid">
        <section className="card race-info">
          <h2>Race Information</h2>
          {race && (
            <div className="info-grid">
              <div className="info-item">
                <span className="info-label">Distance</span>
                <span className="info-value">{race.distance} km</span>
              </div>
              <div className="info-item">
                <span className="info-label">Date</span>
                <span className="info-value">{race.date}</span>
              </div>
              <div className="info-item">
                <span className="info-label">Temperature</span>
                <span className="info-value">{race.temperature}°C</span>
              </div>
              <div className="info-item">
                <span className="info-label">Wind</span>
                <span className="info-value">{race.windSpeed} km/h</span>
              </div>
            </div>
          )}
        </section>

        <section className="card simulation-options">
          <h2>Simulation Options</h2>
          <div className="option-group">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={enableIncidents}
                onChange={(e) => setEnableIncidents(e.target.checked)}
              />
              Enable incidents (crashes, punctures)
            </label>
            <span className="option-help">Adds randomness with mechanical issues and crashes</span>
          </div>

          <div className="option-group">
            <label className="checkbox-label">
              <input
                type="checkbox"
                checked={interactiveMode}
                onChange={(e) => setInteractiveMode(e.target.checked)}
              />
              Interactive mode (control one team)
            </label>
            <span className="option-help">Adds real-time controls for your team during the race</span>
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

        <section className="card start-list">
          <h2>Start List Preview</h2>
          <table>
            <thead>
              <tr>
                <th>Rider</th>
                <th>Team</th>
                <th>Climbing</th>
                <th>Sprint</th>
                <th>Handling</th>
              </tr>
            </thead>
            <tbody>
              {topRiders.map((rider) => (
                <tr key={rider.id}>
                  <td>{rider.name}</td>
                  <td>{rider.team}</td>
                  <td>{rider.climbing}</td>
                  <td>{rider.sprint}</td>
                  <td>{rider.handling}</td>
                </tr>
              ))}
              <tr>
                <td colSpan={5} className="more-riders">+ {Math.max(0, startList.length - 5)} more riders...</td>
              </tr>
            </tbody>
          </table>
        </section>
      </div>

      <div className="setup-actions">
        <Link to="/">
          <button className="secondary">← Back</button>
        </Link>
        <button 
          className="primary" 
          onClick={handleStartSimulation}
          disabled={!race}
        >
          Start Simulation →
        </button>
      </div>
    </div>
  )
}

export default RaceSetup
