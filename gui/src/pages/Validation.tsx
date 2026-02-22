import { useState, useEffect } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './Validation.css'

interface ValidationTest {
  id: string
  name: string
  status: 'pending' | 'running' | 'passed' | 'warning' | 'failed' | 'skipped'
  correlation?: number
  top10Accuracy?: number
  timeGapMAE?: number
  matchedRiders?: number
  isRealRace: boolean
  details?: string
}

// Derive release-gate status from a list of results.
function deriveGateStatus(tests: ValidationTest[]): 'ready' | 'not-ready' | 'limited' | 'idle' {
  if (tests.length === 0) return 'idle'
  const synthetic = tests.filter((t) => !t.isRealRace)
  const allSyntheticPass = synthetic.length > 0 && synthetic.every((t) => t.status === 'passed')
  if (!allSyntheticPass) return 'not-ready'
  const realTests = tests.filter((t) => t.isRealRace && t.status !== 'skipped')
  if (realTests.length === 0) return 'limited' // no FC data fetched
  const anyRealFail = realTests.some((t) => t.status === 'failed')
  return anyRealFail ? 'limited' : 'ready'
}

function formatGapMAE(secs: number | undefined): string {
  if (secs === undefined || secs === 0) return '—'
  return `${Math.round(secs)}s`
}

function Validation() {
  const [tests, setTests] = useState<ValidationTest[]>([])
  const [fcData, setFcData] = useState<main.FCFetchSummary[]>([])
  const [notice, setNotice] = useState<string | null>(null)
  const [isRunning, setIsRunning] = useState(false)
  const [activeTab, setActiveTab] = useState<'synthetic' | 'real'>('synthetic')

  useEffect(() => {
    App.GetFCFetchedSummary()
      .then((data) => setFcData(data || []))
      .catch(() => setFcData([]))
  }, [])

  const runTests = async () => {
    setIsRunning(true)
    setNotice(null)
    try {
      const results = await App.RunValidation()
      if (results.length === 0) {
        setTests([])
        setNotice('No validation results returned.')
      } else {
        setTests(results.map((r, i) => mapResult(r, i)))
      }
    } catch (err) {
      console.error('Failed to run validation:', err)
      setTests([])
      setNotice('Validation failed to run. Check the backend logs.')
    }
    setIsRunning(false)
  }

  const mapResult = (r: main.ValidationResult, index: number): ValidationTest => {
    let status: ValidationTest['status']
    if (r.isRealRace && r.matchedRiders < 3 && !r.passed) {
      status = 'skipped'
    } else {
      status = r.passed ? 'passed' : 'failed'
    }
    return {
      id: `test-${index + 1}`,
      name: r.testName,
      status,
      correlation: r.correlation,
      top10Accuracy: r.top10Accuracy,
      timeGapMAE: r.timeGapMAE,
      matchedRiders: r.matchedRiders,
      isRealRace: r.isRealRace,
      details: r.details,
    }
  }

  const getStatusIcon = (status: ValidationTest['status']) => {
    switch (status) {
      case 'passed': return '✅'
      case 'warning': return '⚠️'
      case 'failed': return '❌'
      case 'running': return '🔄'
      case 'skipped': return '—'
      default: return '○'
    }
  }

  const gateStatus = deriveGateStatus(tests)
  const syntheticTests = tests.filter((t) => !t.isRealRace)
  const realTests = tests.filter((t) => t.isRealRace)
  const visibleTests = activeTab === 'synthetic' ? syntheticTests : realTests

  return (
    <div className="validation page-enter">
      <header className="validation-header">
        <h1>Validation Dashboard</h1>
        <p>Validate simulation accuracy against synthetic benchmarks and real FirstCycling race results</p>
      </header>

      {/* ── Release Gate ───────────────────────────────────────── */}
      {tests.length > 0 && (
        <div className={`release-gate card gate-${gateStatus}`}>
          <div className="gate-indicator">
            {gateStatus === 'ready' && <>
              <span className="gate-icon">✓</span>
              <span className="gate-title">Release Ready</span>
              <span className="gate-badge">v1.0.0-rc1</span>
            </>}
            {gateStatus === 'not-ready' && <>
              <span className="gate-icon">✗</span>
              <span className="gate-title">Not Release Ready</span>
              <span className="gate-sub">Synthetic test failures must be resolved first</span>
            </>}
            {gateStatus === 'limited' && <>
              <span className="gate-icon">⚠</span>
              <span className="gate-title">Synthetic Pass — Limited Calibration</span>
              <span className="gate-sub">
                Fetch FC reference data or resolve FC test issues for full confidence
              </span>
            </>}
          </div>
          <div className="gate-stats">
            <span>{syntheticTests.filter((t) => t.status === 'passed').length}/{syntheticTests.length} synthetic pass</span>
            <span>{realTests.filter((t) => t.status === 'passed').length}/{realTests.filter((t) => t.status !== 'skipped').length} FC pass</span>
          </div>
        </div>
      )}

      {/* ── Controls ────────────────────────────────────────────── */}
      <div className="validation-controls card">
        <button className="primary" onClick={runTests} disabled={isRunning}>
          {isRunning ? 'Running Tests...' : 'Run All Tests'}
        </button>
        <span className="controls-hint">
          Synthetic: ~5s &nbsp;·&nbsp; FC cases: ~30s each (requires fetched FC data)
        </span>
      </div>

      {notice && <div className="empty-state">{notice}</div>}

      {/* ── FC Data Status ───────────────────────────────────────── */}
      <section className="card fc-status-section">
        <h2>FirstCycling Reference Data</h2>
        {fcData.length === 0 ? (
          <div className="empty-state">
            No FC data fetched yet. Use the CLI to import reference results:
            <pre className="fc-cmd">cycling-sim fc-fetch --race 71 --year 2024  # Paris-Roubaix</pre>
            <pre className="fc-cmd">cycling-sim fc-fetch --race 74 --year 2024  # Milan-San Remo</pre>
            <pre className="fc-cmd">cycling-sim fc-match                         # auto-link riders</pre>
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>FC Race ID</th>
                <th>Year</th>
                <th>Stage</th>
                <th>Results</th>
                <th>Fetched</th>
              </tr>
            </thead>
            <tbody>
              {fcData.map((d, i) => (
                <tr key={i}>
                  <td><code>{d.fcRaceID}</code></td>
                  <td>{d.year}</td>
                  <td>{d.stage === 0 ? 'Overall' : `Stage ${d.stage}`}</td>
                  <td>{d.resultCount}</td>
                  <td className="details-cell">{d.fetchedAt?.slice(0, 10)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      {/* ── Test Results ─────────────────────────────────────────── */}
      <section className="card tests-list">
        <div className="tests-header">
          <h2>Test Results</h2>
          {tests.length > 0 && (
            <div className="comp-tabs" role="tablist">
              <button
                type="button" role="tab"
                className={activeTab === 'synthetic' ? 'tab active' : 'tab'}
                onClick={() => setActiveTab('synthetic')}
              >
                Synthetic ({syntheticTests.length})
              </button>
              <button
                type="button" role="tab"
                className={activeTab === 'real' ? 'tab active' : 'tab'}
                onClick={() => setActiveTab('real')}
              >
                FC Reference ({realTests.length})
              </button>
            </div>
          )}
        </div>

        {tests.length === 0 ? (
          <div className="empty-state">No validation results yet. Run tests to populate this table.</div>
        ) : visibleTests.length === 0 ? (
          <div className="empty-state">No {activeTab === 'synthetic' ? 'synthetic' : 'FC reference'} tests available.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Status</th>
                <th>Test Name</th>
                <th>Corr</th>
                <th>Top-10 %</th>
                {activeTab === 'real' && <th>Gap MAE</th>}
                {activeTab === 'real' && <th>Matched</th>}
                <th>Details</th>
              </tr>
            </thead>
            <tbody>
              {visibleTests.map((test) => (
                <tr key={test.id} className={`status-${test.status}`}>
                  <td className="status-cell">{getStatusIcon(test.status)}</td>
                  <td>{test.name}</td>
                  <td className="num-cell">{test.correlation !== undefined && test.correlation !== 0 ? test.correlation.toFixed(2) : '—'}</td>
                  <td className="num-cell">{test.top10Accuracy ? `${test.top10Accuracy.toFixed(0)}%` : '—'}</td>
                  {activeTab === 'real' && <td className="num-cell">{formatGapMAE(test.timeGapMAE)}</td>}
                  {activeTab === 'real' && <td className="num-cell">{test.matchedRiders || '—'}</td>}
                  <td className="details-cell">{test.details ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      {/* ── Summary Metrics ─────────────────────────────────────── */}
      <section className="card metrics-summary">
        <h2>Summary Metrics</h2>
        <div className="metrics-grid">
          <div className="metric">
            <span className="metric-value">{tests.filter((t) => t.status === 'passed').length}</span>
            <span className="metric-label">Passed</span>
          </div>
          <div className="metric warning">
            <span className="metric-value">{tests.filter((t) => t.status === 'warning').length}</span>
            <span className="metric-label">Warnings</span>
          </div>
          <div className="metric danger">
            <span className="metric-value">{tests.filter((t) => t.status === 'failed').length}</span>
            <span className="metric-label">Failed</span>
          </div>
        </div>
      </section>
    </div>
  )
}

export default Validation
