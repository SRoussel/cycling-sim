import { Link } from 'react-router-dom'
import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './Home.css'

function Home() {
  const [races, setRaces] = useState<main.RaceInfo[]>([])
  const [stageRaces, setStageRaces] = useState<main.StageRaceInfo[]>([])
  const [isSeeded, setIsSeeded] = useState(true)
  const [loading, setLoading] = useState(true)

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
      } catch (err) {
        console.error('Failed to load data:', err)
      }
      setLoading(false)
    }
    loadData()
  }, [])

  const getCourseType = (race: main.RaceInfo): string => {
    // Map course names to types (the API provides this via courseName analysis)
    const name = race.name.toLowerCase()
    if (name.includes('strade')) return 'gravel'
    if (name.includes('roubaix') || name.includes('flanders') || name.includes('e3')) {
      return 'cobbles'
    }
    if (name.includes('liege') || name.includes('lombardia') || name.includes('fleche')) {
      return 'climb'
    }
    if (name.includes('san remo') || name.includes('gent')) {
      return 'flat'
    }
    return 'hilly'
  }

  const getStageRaceBadge = (name: string): string => {
    const lowered = name.toLowerCase()
    if (lowered.includes('tour de france') || lowered.includes('tdf')) return 'tdf'
    if (lowered.includes('giro')) return 'giro'
    if (lowered.includes('vuelta')) return 'vuelta'
    return 'other'
  }

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

  return (
    <div className="home page-enter">
      <header className="home-header">
        <h1>Races</h1>
        <p>Pick a race to direct a team, or to watch it run.</p>
      </header>

      {loading ? (
        <div className="empty-state">Loading races…</div>
      ) : (
        <div className="home-cols">
          <section className="card race-list">
            <h2>One-day races</h2>
            <ul>
              {races.map((race) => {
                const type = getCourseType(race)
                return (
                  <li key={race.id}>
                    <Link to={`/race-setup/${race.id}`}>
                      <span className="race-name">{race.name}</span>
                      <span className={`badge ${type}`}>{type}</span>
                      <span className="race-num">{race.distance} km</span>
                      <span className="race-date">{race.date}</span>
                    </Link>
                  </li>
                )
              })}
            </ul>
          </section>

          <section className="card race-list stages">
            <h2>Stage races</h2>
            <ul>
              {stageRaces.map((sr) => (
                <li key={sr.id}>
                  <Link to={`/stage-race/${sr.id}`}>
                    <span className="race-name">{sr.name}</span>
                    <span className={`badge stage ${getStageRaceBadge(sr.name)}`}>{sr.stageCount >= 21 ? 'grand tour' : 'week-long'}</span>
                    <span className="race-num">{sr.stageCount} stages</span>
                  </Link>
                </li>
              ))}
            </ul>
          </section>
        </div>
      )}
    </div>
  )
}

export default Home
