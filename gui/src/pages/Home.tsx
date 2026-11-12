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
        <h1>Cycling Race Simulator</h1>
        <p>Select a race to simulate or run validation tests</p>
      </header>

      {loading ? (
        <div className="loading">Loading races...</div>
      ) : (
        <>
          <section className="race-section">
            <h2>One-Day Classics</h2>
            <div className="races-grid">
              {races.map((race) => {
                const type = getCourseType(race)
                return (
                  <Link to={`/race-setup/${race.id}`} key={race.id} className="race-card">
                    <div className="race-card-header">
                      <span className={`badge ${type}`}>{type}</span>
                    </div>
                    <h3>{race.name}</h3>
                    <div className="race-meta">
                      <span>{race.distance} km</span>
                      <span>{race.date}</span>
                    </div>
                  </Link>
                )
              })}
            </div>
          </section>

          <section className="race-section">
            <h2>Stage Races</h2>
            <div className="races-grid">
              {stageRaces.map((sr) => (
                <Link
                  to={`/stage-race/${sr.id}`}
                  key={sr.id}
                  className={`race-card stage-race ${getStageRaceBadge(sr.name)}`}
                >
                  <div className="race-card-header">
                    <span className={`badge stage ${getStageRaceBadge(sr.name)}`}>
                      Stage Race
                    </span>
                  </div>
                  <h3>{sr.name}</h3>
                  <div className="race-meta">
                    <span>{sr.stageCount} stages</span>
                  </div>
                </Link>
              ))}
            </div>
          </section>
        </>
      )}

      <section className="quick-actions card">
        <h2>Quick Actions</h2>
        <div className="action-buttons">
          <Link to="/validation">
            <button className="primary">Run Validation Tests</button>
          </Link>
          <Link to="/settings">
            <button className="secondary">Settings</button>
          </Link>
        </div>
      </section>
    </div>
  )
}

export default Home
