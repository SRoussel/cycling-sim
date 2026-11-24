import { Link } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { RouteProfile } from '../sim/shared'
import { findClimbs } from '../components/race/ProfileView'
import './Home.css'

/** Dated names ("Giro d'Italia 2024", "Il Lombardia 2023") are reference
 * editions kept for validation; undated ones are the races to play. */
const isEdition = (name: string) => /\b(19|20)\d\d\b/.test(name)

/** One line on the course's character, from its own profile. */
function character(p: RouteProfile | undefined): string {
  if (!p || p.points.length < 2) return ''
  const climbs = findClimbs(p)
  let gain = 0
  for (let i = 1; i < p.points.length; i++) gain += Math.max(0, p.points[i].elev - p.points[i - 1].elev)
  const sectors = p.sectors || []
  const gravel = sectors.filter((s) => s.surface === 'gravel').length
  const cobbles = sectors.length - gravel
  const parts: string[] = []
  if (cobbles > 0) parts.push(`${cobbles} cobbled ${cobbles === 1 ? 'sector' : 'sectors'}`)
  if (gravel > 0) parts.push(`${gravel} gravel ${gravel === 1 ? 'sector' : 'sectors'}`)
  if (climbs.length > 0) parts.push(`${climbs.length} ${climbs.length === 1 ? 'climb' : 'climbs'}`)
  parts.push(gain >= 400 ? `${Math.round(gain / 100) * 100} m of climbing` : 'mostly flat')
  const steepest = climbs.reduce((m, c) => Math.max(m, c.avgPct), 0)
  const last = climbs[climbs.length - 1]
  if (last && p.totalKm - (last.startKm + last.lengthKm) < 1) parts.push('uphill finish')
  else if (steepest >= 9) parts.push(`steep (up to ${steepest.toFixed(0)}%)`)
  return parts.join(' · ')
}

/** A small profile line for the list. */
function MiniProfile({ p }: { p: RouteProfile | undefined }) {
  if (!p || p.points.length < 2) return <span className="mini-prof" />
  const W = 160
  const H = 34
  const lo = Math.min(...p.points.map((x) => x.elev))
  const hi = Math.max(lo + 300, ...p.points.map((x) => x.elev))
  const x = (k: number) => (k / p.totalKm) * W
  const y = (e: number) => H - 2 - ((e - lo) / (hi - lo)) * (H - 6)
  const d = `M0,${H} ` + p.points.map((q) => `L${x(q.km).toFixed(1)},${y(q.elev).toFixed(1)}`).join(' ') + ` L${W},${H} Z`
  return (
    <svg className="mini-prof" viewBox={`0 0 ${W} ${H}`} width={W} height={H} aria-hidden="true">
      <path d={d} />
    </svg>
  )
}

function Home() {
  const [races, setRaces] = useState<main.RaceInfo[]>([])
  const [stageRaces, setStageRaces] = useState<main.StageRaceInfo[]>([])
  const [profiles, setProfiles] = useState<Record<number, RouteProfile>>({})
  const [isSeeded, setIsSeeded] = useState(true)
  const [loading, setLoading] = useState(true)
  const [tab, setTab] = useState<'oneday' | 'stage'>('oneday')
  const [showEditions, setShowEditions] = useState(false)

  useEffect(() => {
    const loadData = async () => {
      try {
        const [racesData, stageRacesData, seeded] = await Promise.all([
          App.GetRaces(),
          App.GetStageRaces(),
          App.IsDatabaseSeeded(),
        ])
        setRaces(racesData || [])
        setStageRaces(stageRacesData || [])
        setIsSeeded(seeded)
        // Profiles for the list (cheap: built from the course segments).
        const entries = await Promise.all(
          (racesData || []).map(async (r) => [r.id, (await App.PreviewRaceProfile(r.id)) as unknown as RouteProfile] as const),
        )
        setProfiles(Object.fromEntries(entries.filter(([, p]) => p)))
      } catch (err) {
        console.error('Failed to load data:', err)
      }
      setLoading(false)
    }
    loadData()
  }, [])

  if (!isSeeded) {
    return (
      <div className="home">
        <header className="home-header">
          <h1>Cycling Race Simulator</h1>
          <p>Database needs to be seeded</p>
        </header>
        <section className="card">
          <h2>Setup Required</h2>
          <p>Run the following command to seed the database:</p>
          <pre className="code-block">./cycling-sim seed</pre>
          <p>Or from the project directory:</p>
          <pre className="code-block">go run ./cmd/cycling-sim seed</pre>
        </section>
      </div>
    )
  }

  const byDate = (a: main.RaceInfo, b: main.RaceInfo) => a.date.slice(5).localeCompare(b.date.slice(5))
  const oneDay = races.filter((r) => !isEdition(r.name)).sort(byDate)
  const oneDayEditions = races.filter((r) => isEdition(r.name)).sort((a, b) => a.date.localeCompare(b.date))
  const stages = stageRaces.filter((s) => !isEdition(s.name))
  const stageEditions = stageRaces.filter((s) => isEdition(s.name))
  const day = (iso: string) => {
    const d = new Date(iso)
    return isNaN(d.getTime()) ? iso : d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
  }

  const raceRow = (race: main.RaceInfo, withYear = false) => (
    <li key={race.id}>
      <Link to={`/race-setup/${race.id}`}>
        <span className="race-date">{withYear ? race.date.slice(0, 4) : day(race.date)}</span>
        <span className="race-name">{race.name}</span>
        <span className="race-char">{character(profiles[race.id])}</span>
        <MiniProfile p={profiles[race.id]} />
        <span className="race-num">{race.distance} km</span>
      </Link>
    </li>
  )
  const stageRow = (sr: main.StageRaceInfo) => (
    <li key={sr.id}>
      <Link to={`/stage-race/${sr.id}`}>
        <span className="race-date">{sr.stageCount >= 21 ? 'Grand Tour' : 'Week'}</span>
        <span className="race-name">{sr.name}</span>
        <span className="race-char">{sr.stageCount} stages</span>
        <span className="mini-prof" />
        <span className="race-num">{Math.round(sr.totalDistanceKm).toLocaleString('en-GB')} km</span>
      </Link>
    </li>
  )

  return (
    <div className="home page-enter">
      <header className="home-header">
        <div>
          <h1>Pick a race</h1>
          <p>Race one outside the season: direct a team, or just watch.</p>
        </div>
        <div className="seg-tabs" role="tablist" aria-label="Race type">
          <button type="button" role="tab" aria-selected={tab === 'oneday'} className={tab === 'oneday' ? 'on' : ''} onClick={() => setTab('oneday')}>One-day</button>
          <button type="button" role="tab" aria-selected={tab === 'stage'} className={tab === 'stage' ? 'on' : ''} onClick={() => setTab('stage')}>Stage races</button>
        </div>
      </header>

      {loading ? (
        <div className="empty-state">Loading races…</div>
      ) : (
        <>
          <section className="card race-list">
            <div className="race-list-head" aria-hidden="true">
              <span>{tab === 'oneday' ? 'Date' : 'Kind'}</span><span>Race</span><span>Character</span><span>Profile</span><span className="r">Distance</span>
            </div>
            <ul>{tab === 'oneday' ? oneDay.map((r) => raceRow(r)) : stages.map(stageRow)}</ul>
          </section>

          {(tab === 'oneday' ? oneDayEditions.length : stageEditions.length) > 0 && (
            <section className="editions">
              <button type="button" className="editions-toggle" aria-expanded={showEditions} onClick={() => setShowEditions(!showEditions)}>
                {showEditions ? 'Hide' : 'Show'} past editions ({tab === 'oneday' ? oneDayEditions.length : stageEditions.length})
                <small>Real routes and fields of past years, used to check the model</small>
              </button>
              {showEditions && (
                <div className="card race-list">
                  <ul>{tab === 'oneday' ? oneDayEditions.map((r) => raceRow(r, true)) : stageEditions.map(stageRow)}</ul>
                </div>
              )}
            </section>
          )}
        </>
      )}
    </div>
  )
}

export default Home
