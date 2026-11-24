import { useParams, useNavigate, Link } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { StageStrip, useStageProfiles } from '../components/race/StageStrip'
import './RaceSetup.css'

function StageRaceSetup() {
  const { stageRaceId } = useParams()
  const navigate = useNavigate()
  const [stageRace, setStageRace] = useState<main.StageRaceInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [startList, setStartList] = useState<main.RiderInfo[]>([])
  const [stages, setStages] = useState<main.StageRaceStageInfo[]>([])
  const profiles = useStageProfiles(stages)
  const [interactiveMode, setInteractiveMode] = useState(false)
  const [controlledTeam, setControlledTeam] = useState('')

  useEffect(() => {
    async function loadStageRace() {
      try {
        const stageRaces = await App.GetStageRaces()
        const race = stageRaces.find((sr: main.StageRaceInfo) => sr.id === parseInt(stageRaceId || '0'))
        if (race) {
          setStageRace(race)

          setStages((await App.GetStageRaceStages(race.id)) || [])
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

  const wkg = (r: main.RiderInfo) => (r.weight > 0 ? r.ftp / r.weight : 0)
  const contenders = [...startList].sort((a, b) => wkg(b) * (1 + b.climbing / 400) - wkg(a) * (1 + a.climbing / 400)).slice(0, 8)

  return (
    <div className="race-setup page-enter">
      <header className="setup-header">
        <div>
          <Link to="/" className="setup-back">← All races</Link>
          <h1>{stageRace.name}</h1>
          <p className="setup-facts">
            {stageRace.stageCount >= 21 ? 'Grand Tour' : 'Week-long stage race'} · {stageRace.stageCount} stages · {stageRace.totalDistanceKm.toFixed(0)} km
          </p>
        </div>
        <button className="primary setup-start" onClick={handleStartSimulation}>
          {interactiveMode ? 'Start the race' : 'Watch the race'}
        </button>
      </header>

      {stages.length > 0 && <StageStrip stages={stages} profiles={profiles} />}

      <div className="setup-grid">
        <section className="card setup-plan">
          <div className="setup-plan-head">
            <div>
              <h2>Your team</h2>
              <p className="setup-help">
                {interactiveMode
                  ? "You'll plan each stage, make the calls during it and chase a GC goal."
                  : "You'll watch the race. Every team races its own plan."}
              </p>
            </div>
            <button type="button" role="switch" aria-checked={interactiveMode} className={`switch${interactiveMode ? ' on' : ''}`} onClick={() => setInteractiveMode(!interactiveMode)}>
              <span className="switch-track"><span className="switch-knob" /></span>
              Direct a team
            </button>
          </div>
          {interactiveMode && teams.length > 0 && (
            <label className="setup-team">
              <span>Team</span>
              <select value={controlledTeam} onChange={(e) => setControlledTeam(e.target.value)}>
                {teams.map((t) => <option key={t} value={t}>{t}</option>)}
              </select>
            </label>
          )}
        </section>

        <section className="card setup-favs">
          <div className="setup-favs-head">
            <h2>GC contenders</h2>
            <span className="setup-help">{startList.length} starters</span>
          </div>
          <table>
            <thead>
              <tr><th>Rider</th><th className="num">W/kg</th><th className="num">Threshold</th></tr>
            </thead>
            <tbody>
              {contenders.map((r) => (
                <tr key={r.id} className={interactiveMode && r.team === controlledTeam ? 'mine' : ''}>
                  <td><span className="fav-who"><span>{r.name}<small>{r.team}</small></span></span></td>
                  <td className="num">{wkg(r).toFixed(2)}</td>
                  <td className="num">{r.ftp} W</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      </div>
    </div>
  )
}

export default StageRaceSetup
