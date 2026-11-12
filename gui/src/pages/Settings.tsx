import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './Settings.css'

function Settings() {
  const [params, setParams] = useState<main.SimParams>({
    handlingEffect: 0.018,
    draftFactor: 0.25,
    fatigueRate: 1.0,
    attackThreshold: 0.7,
  })
  const [saving, setSaving] = useState(false)
  const [seeding, setSeeding] = useState(false)
  const [clearing, setClearing] = useState(false)
  const [dbMessage, setDbMessage] = useState<string | null>(null)

  useEffect(() => {
    App.GetSimulationParams()
      .then((p) => {
        setParams(p)
      })
      .catch((err) => {
        console.error('Settings: Failed to load params:', err)
      })
  }, [])

  const handleSave = async () => {
    setSaving(true)
    try {
      await App.SaveSimulationParams(params)
    } catch (err) {
      console.error('Failed to save params:', err)
    }
    setSaving(false)
  }

  const handleSeed = async () => {
    setSeeding(true)
    setDbMessage(null)
    try {
      const result = await App.SeedDatabase()
      setDbMessage(result)
    } catch (err) {
      setDbMessage('Error: ' + (err as Error).message)
    }
    setSeeding(false)
  }

  const handleClear = async () => {
    if (!confirm('Are you sure you want to clear the database? This will delete all riders, courses, races, and results.')) {
      return
    }
    setClearing(true)
    setDbMessage(null)
    try {
      const result = await App.ClearDatabase()
      setDbMessage(result)
    } catch (err) {
      setDbMessage('Error: ' + (err as Error).message)
    }
    setClearing(false)
  }

  return (
    <div className="settings page-enter">
      <header className="settings-header">
        <h1>Settings</h1>
        <p>Configure simulation parameters and manage data</p>
      </header>

      <section className="card">
        <h2>Database Management</h2>
        <p className="setting-desc">Seed or clear the database with riders, courses, and races</p>
        {dbMessage && (
          <div className={`db-message ${dbMessage.startsWith('Error') ? 'error' : 'success'}`}>
            {dbMessage}
          </div>
        )}
        <div className="setting-actions">
          <button className="primary" onClick={handleSeed} disabled={seeding || clearing}>
            {seeding ? 'Seeding...' : 'Seed Database'}
          </button>
          <button className="secondary danger" onClick={handleClear} disabled={seeding || clearing}>
            {clearing ? 'Clearing...' : 'Clear Database'}
          </button>
        </div>
      </section>

      <section className="card">
        <h2>Simulation Parameters</h2>
        <div className="param-grid">
          <div className="param-item">
            <label>Handling Effect (per point)</label>
            <input
              type="number"
              value={params.handlingEffect}
              onChange={(e) => setParams({ ...params, handlingEffect: parseFloat(e.target.value) })}
              step={0.001}
            />
            <span className="param-help">Controls cobble rolling resistance reduction</span>
          </div>
          <div className="param-item">
            <label>Draft Factor (peloton)</label>
            <input
              type="number"
              value={params.draftFactor}
              onChange={(e) => setParams({ ...params, draftFactor: parseFloat(e.target.value) })}
              step={0.05}
            />
            <span className="param-help">Aero drag reduction in large group</span>
          </div>
          <div className="param-item">
            <label>Fatigue Rate</label>
            <input
              type="number"
              value={params.fatigueRate}
              onChange={(e) => setParams({ ...params, fatigueRate: parseFloat(e.target.value) })}
              step={0.1}
            />
            <span className="param-help">Multiplier for W' depletion</span>
          </div>
          <div className="param-item">
            <label>Attack Threshold</label>
            <input
              type="number"
              value={params.attackThreshold}
              onChange={(e) => setParams({ ...params, attackThreshold: parseFloat(e.target.value) })}
              step={0.05}
            />
            <span className="param-help">W' level that triggers attack decisions</span>
          </div>
        </div>
        <button className="primary" style={{ marginTop: '1rem' }} onClick={handleSave} disabled={saving}>
          {saving ? 'Saving...' : 'Save Parameters'}
        </button>
      </section>

      <section className="card">
        <h2>Backend Status</h2>
        <div className="api-status">
          <span className="status-indicator online"></span>
          <span>Simulation engine active</span>
        </div>
      </section>
    </div>
  )
}

export default Settings
