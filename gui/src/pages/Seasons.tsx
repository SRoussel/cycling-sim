import { useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { Jersey } from '../components/race/Jersey'
import './Season.css'

/** Season mode's front door: saved seasons and a new one. */
function Seasons() {
  const navigate = useNavigate()
  const [seasons, setSeasons] = useState<main.SeasonInfo[]>([])
  const [teams, setTeams] = useState<string[]>([])
  const [team, setTeam] = useState('')
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const load = async () => {
    const [ss, ts] = await Promise.all([App.ListSeasons(), App.GetTeamNames()])
    setSeasons(ss ?? [])
    setTeams(ts ?? [])
    if (!team && ts?.length) setTeam(ts.includes('Lidl-Trek') ? 'Lidl-Trek' : ts[0])
  }
  useEffect(() => {
    load().catch((e) => setError(String(e)))
  }, [])

  const create = async () => {
    setBusy(true)
    setError('')
    try {
      const s = await App.NewSeason(team, name)
      navigate(`/season/${s.id}`)
    } catch (e) {
      setError(String(e))
      setBusy(false)
    }
  }

  const remove = async (s: main.SeasonInfo) => {
    if (!window.confirm(`Delete "${s.name}"? This can't be undone.`)) return
    await App.DeleteSeason(s.id)
    load()
  }

  return (
    <div className="seasons page-enter">
      <header className="home-header">
        <h1>Season</h1>
        <p>Run a team through the year: pick squads, race or simulate each race, meet the board's objectives, sign riders and build for the next season.</p>
      </header>
      <div className="season-cols">
        <section className="card">
          <h2>New season</h2>
          <div className="new-season">
            <label>
              <span>Team</span>
              <select value={team} onChange={(e) => setTeam(e.target.value)}>
                {teams.map((t) => (
                  <option key={t} value={t}>{t}</option>
                ))}
              </select>
            </label>
            <label>
              <span>Name (optional)</span>
              <input type="text" value={name} placeholder={`${team} 2026`} onChange={(e) => setName(e.target.value)} />
            </label>
            <button className="primary" onClick={create} disabled={!team || busy}>
              {busy ? 'Setting up the season…' : 'Start season'}
            </button>
          </div>
          {error && <p className="season-error">{error}</p>}
        </section>
        <section className="card">
          <h2>Saved seasons</h2>
          {seasons.length === 0 ? (
            <div className="empty-state">No seasons yet.</div>
          ) : (
            <ul className="season-list">
              {seasons.map((s) => (
                <li key={s.id}>
                  <Link to={`/season/${s.id}`}>
                    <span className="race-name"><Jersey team={s.team} size={14} /> {s.name}</span>
                    <span className="muted">{s.team}</span>
                    <span className="race-num">{s.phase === 'offseason' ? `${s.year} transfer window` : `${s.year} · ${s.date}`}</span>
                  </Link>
                  <button className="link danger" onClick={() => remove(s)} title="Delete this season">Delete</button>
                </li>
              ))}
            </ul>
          )}
        </section>
      </div>
    </div>
  )
}

export default Seasons
