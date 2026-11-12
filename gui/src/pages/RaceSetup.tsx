import { useParams, Link, useNavigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { director, main } from '../../wailsjs/go/models'
import { PlanForm } from '../components/race/PlanForm'
import './RaceSetup.css'

function RaceSetup() {
  const { raceId } = useParams()
  const navigate = useNavigate()
  const [race, setRace] = useState<main.RaceInfo | null>(null)
  const [startList, setStartList] = useState<main.RiderInfo[]>([])
  const [enableIncidents, setEnableIncidents] = useState(false)

  const [interactiveMode, setInteractiveMode] = useState(false)
  const [controlledTeam, setControlledTeam] = useState('')
  // Race plan (interactive): who we ride for, the break, where to make it
  // hard and where the leader goes. Points are the course's decisive climbs
  // and sectors.
  const [planPoints, setPlanPoints] = useState<director.PlanPoint[]>([])
  const [planLeader, setPlanLeader] = useState(0)
  const [planBreak, setPlanBreak] = useState('auto')
  const [planHard, setPlanHard] = useState<string[]>([])
  const [planAttack, setPlanAttack] = useState('')

  useEffect(() => {
    const loadData = async () => {
      try {
        const racesData = await App.GetRaces()
        const found = racesData?.find(r => r.id === Number(raceId))
        setRace(found || null)

        if (found) {
          const preview = await App.PreviewRaceStartList(found.id)
          setStartList(preview || [])
          setPlanPoints((await App.PreviewRacePlan(found.id)) ?? [])
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
    const plan = { leaderId: planLeader, break: planBreak, hardOn: planHard, attackOn: planAttack }
    navigate(`/simulation/${race.id}`, { state: { race, enableIncidents, interactiveMode, controlledTeam, plan } })
  }

  // Get top riders by handling for preview
  // The model is watt-based: threshold power (FTP) and W/kg are what decide a
  // race; the 1-100 ratings only shift power a few percent by terrain.
  const wkg = (r: main.RiderInfo) => (r.weight > 0 ? r.ftp / r.weight : 0)
  const topRiders = [...startList].sort((a, b) => wkg(b) - wkg(a)).slice(0, 8)
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
                  onChange={(e) => {
                    setControlledTeam(e.target.value)
                    setPlanLeader(0)
                  }}
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

        {interactiveMode && controlledTeam && (
          <section className="card race-plan">
            <h2>Race plan</h2>
            <p className="option-help">Your team follows the plan unless you change course during the race. Leave anything to the team.</p>
            <PlanForm
              riders={startList.filter((r) => r.team === controlledTeam)}
              points={planPoints}
              value={{ leaderId: planLeader, break: planBreak, hardOn: planHard, attackOn: planAttack }}
              onChange={(v) => {
                setPlanLeader(v.leaderId)
                setPlanBreak(v.break)
                setPlanHard(v.hardOn)
                setPlanAttack(v.attackOn)
              }}
            />
          </section>
        )}

        <section className="card start-list">
          <h2>Start List Preview</h2>
          <table>
            <thead>
              <tr>
                <th>Rider</th>
                <th>Team</th>
                <th title="Threshold power per kg: the main lever in the model">W/kg</th>
                <th title="Threshold power (FTP)">FTP</th>
                <th title="Rating (1-100): shifts power on climbs by up to ±4%">Climbing</th>
                <th title="Rating (1-100)">Sprint</th>
              </tr>
            </thead>
            <tbody>
              {topRiders.map((rider) => (
                <tr key={rider.id}>
                  <td>{rider.name}</td>
                  <td>{rider.team}</td>
                  <td>{wkg(rider).toFixed(2)}</td>
                  <td style={{ whiteSpace: 'nowrap' }}>{rider.ftp} W</td>
                  <td>{rider.climbing}</td>
                  <td>{rider.sprint}</td>
                </tr>
              ))}
              <tr>
                <td colSpan={6} className="more-riders">+ {Math.max(0, startList.length - topRiders.length)} more riders...</td>
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
