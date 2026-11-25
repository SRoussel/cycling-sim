import { useState } from 'react'
import * as App from '../api'
import './Settings.css'

/**
 * The game's data: add what's missing from the seed data (keeps results and
 * seasons), or wipe everything. The model has no tunable knobs here: its
 * constants live in the code and are judged by the validation runs.
 */
function Settings() {
  const [seeding, setSeeding] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [dbMessage, setDbMessage] = useState<string | null>(null)

  const handleSeed = async () => {
    setSeeding(true)
    setDbMessage(null)
    try {
      setDbMessage(await App.SeedDatabase())
    } catch (err) {
      setDbMessage('Error: ' + (err as Error).message)
    }
    setSeeding(false)
  }

  const handleClear = async () => {
    if (!confirm('Clear the database? This deletes every rider, course, race, result and season.')) return
    setClearing(true)
    setDbMessage(null)
    try {
      setDbMessage(await App.ClearDatabase())
    } catch (err) {
      setDbMessage('Error: ' + (err as Error).message)
    }
    setClearing(false)
  }

  return (
    <div className="settings page-enter">
      <header className="settings-header">
        <h1>Settings</h1>
        <p>The game's data.</p>
      </header>

      <section className="card settings-row">
        <div>
          <h2>Add missing data</h2>
          <p>Adds any riders, courses, races and stage races the game knows about but your database lacks. Results and seasons are kept.</p>
        </div>
        <button className="primary" onClick={handleSeed} disabled={seeding}>{seeding ? 'Adding…' : 'Add missing data'}</button>
      </section>

      <section className="card settings-row danger-zone">
        <div>
          <h2>Clear everything</h2>
          <p>Deletes every rider, course, race, result and season. You'll need to add the data again before racing.</p>
        </div>
        <button className="secondary danger" onClick={handleClear} disabled={clearing}>{clearing ? 'Clearing…' : 'Clear the database'}</button>
      </section>

      {dbMessage && <p className={`db-message ${dbMessage.startsWith('Error') ? 'error' : 'success'}`}>{dbMessage}</p>}
    </div>
  )
}

export default Settings
