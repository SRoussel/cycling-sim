import { useParams, Link, useNavigate } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { director, main } from '../../wailsjs/go/models'
import { PlanForm } from '../components/race/PlanForm'
import { ProfileView } from '../components/race/ProfileView'
import { RouteProfile } from '../sim/shared'
import { teamColor } from '../sim/teamColors'
import './RaceSetup.css'

const wkg = (r: main.RiderInfo) => (r.weight > 0 ? r.ftp / r.weight : 0)

/** Each rider's place in this field on a rating, 0 (weakest) to 100. */
function fieldRanks(riders: main.RiderInfo[], key: (r: main.RiderInfo) => number): Map<number, number> {
  const sorted = [...riders].sort((a, b) => key(a) - key(b))
  const n = Math.max(1, sorted.length - 1)
  return new Map(sorted.map((r, i) => [r.id, (i / n) * 100]))
}

function RaceSetup() {
  const { raceId } = useParams()
  const navigate = useNavigate()
  const [race, setRace] = useState<main.RaceInfo | null>(null)
  const [startList, setStartList] = useState<main.RiderInfo[]>([])
  const [profile, setProfile] = useState<RouteProfile | null>(null)
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
          setProfile(((await App.PreviewRaceProfile(found.id)) as unknown as RouteProfile) ?? null)
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
    const plan = { leaderId: planLeader, break: planBreak, hardOn: planHard, attackOn: planAttack }
    navigate(`/simulation/${race.id}`, { state: { race, enableIncidents, interactiveMode, controlledTeam, plan } })
  }

  // The model is watt-based: threshold power (FTP) and W/kg decide a race;
  // the ratings shift power a few percent by terrain. Bars rank each
  // favourite within this field.
  const favourites = [...startList].sort((a, b) => wkg(b) - wkg(a)).slice(0, 10)
  const climbRank = fieldRanks(startList, (r) => r.climbing)
  const punchRank = fieldRanks(startList, (r) => r.punch)
  const sprintRank = fieldRanks(startList, (r) => r.sprint)
  const teams = Array.from(new Set(startList.map((r) => r.team))).sort()
  const climbs = planPoints.filter((p) => p.kind === 'climb').length
  const sectors = (profile?.sectors || []).length

  return (
    <div className="race-setup page-enter">
      <header className="setup-header">
        <div>
          <Link to="/" className="setup-back">← All races</Link>
          <h1>{race?.name || raceId}</h1>
          {race && (
            <p className="setup-facts">
              {race.date} · {race.distance} km
              {climbs > 0 && ` · ${climbs} decisive ${climbs === 1 ? 'climb' : 'climbs'}`}
              {sectors > 0 && ` · ${sectors} ${profile?.sectors?.every((s) => s.surface === 'gravel') ? 'gravel' : 'cobbled'} sectors`}
              {` · ${race.temperature} °C, wind ${race.windSpeed} km/h`}
            </p>
          )}
        </div>
        <button className="primary setup-start" onClick={handleStartSimulation} disabled={!race}>
          {interactiveMode ? 'Start the race' : 'Watch the race'}
        </button>
      </header>

      {profile && profile.points.length > 1 && (
        <section className="card setup-course" aria-label="Course profile">
          <div className="setup-course-head">
            <h2>The course</h2>
            <span className="route-legend" aria-label="Gradient colours">
              <span><i className="pv-g1" />under 3%</span>
              <span><i className="pv-g2" />3–6%</span>
              <span><i className="pv-g3" />6–9%</span>
              <span><i className="pv-g4" />9% and up</span>
            </span>
          </div>
          <ProfileView
            profile={profile}
            km={0}
            fromKm={0}
            toKm={profile.totalKm}
            tickEvery={profile.totalKm > 150 ? 25 : 10}
            minElevRange={300}
            className="setup"
            detail
            showPosition={false}
          />
        </section>
      )}

      <div className="setup-grid">
        <section className="card setup-plan">
          <div className="setup-plan-head">
            <div>
              <h2>Your plan</h2>
              <p className="setup-help">
                {interactiveMode
                  ? 'The team follows it unless you step in. Anything left open, the team decides.'
                  : "You'll watch the race. Every team races its own plan."}
              </p>
            </div>
            <button type="button" role="switch" aria-checked={interactiveMode} className={`switch${interactiveMode ? ' on' : ''}`} onClick={() => setInteractiveMode(!interactiveMode)}>
              <span className="switch-track"><span className="switch-knob" /></span>
              Direct a team
            </button>
          </div>

          {interactiveMode && teams.length > 0 && (
            <>
              <label className="setup-team">
                <span>Team</span>
                <select
                  value={controlledTeam}
                  onChange={(e) => {
                    setControlledTeam(e.target.value)
                    setPlanLeader(0)
                  }}
                >
                  {teams.map((t) => <option key={t} value={t}>{t}</option>)}
                </select>
              </label>
              {controlledTeam && (
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
              )}
            </>
          )}

          <label className="setup-check">
            <input type="checkbox" checked={enableIncidents} onChange={(e) => setEnableIncidents(e.target.checked)} />
            Crashes, punctures and mechanicals
          </label>
        </section>

        <section className="card setup-favs">
          <div className="setup-favs-head">
            <h2>Strongest riders</h2>
            <span className="setup-help">{startList.length} starters</span>
          </div>
          <table>
            <thead>
              <tr>
                <th>Rider</th>
                <th className="num" title="Threshold power per kg: the main lever in the model">W/kg</th>
                <th title="Climbing, ranked in this field">Climbing</th>
                <th title="Punch (short, steep efforts), ranked in this field">Punch</th>
                <th title="Sprint, ranked in this field">Sprint</th>
              </tr>
            </thead>
            <tbody>
              {favourites.map((r) => (
                <tr key={r.id} className={interactiveMode && r.team === controlledTeam ? 'mine' : ''}>
                  <td>
                    <span className="fav-who">
                      <i style={{ background: teamColor(r.team) }} />
                      <span>{r.name}<small>{r.team}</small></span>
                    </span>
                  </td>
                  <td className="num">{wkg(r).toFixed(2)}</td>
                  <td><span className="rank"><i style={{ width: `${climbRank.get(r.id) ?? 0}%` }} /></span></td>
                  <td><span className="rank"><i style={{ width: `${punchRank.get(r.id) ?? 0}%` }} /></span></td>
                  <td><span className="rank"><i style={{ width: `${sprintRank.get(r.id) ?? 0}%` }} /></span></td>
                </tr>
              ))}
            </tbody>
          </table>
          <p className="setup-note">Bars: where the rider ranks in this field, weakest to strongest.</p>
        </section>
      </div>
    </div>
  )
}

export default RaceSetup
