import { useState } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './Validation.css'

interface ValidationTest {
  id: string
  name: string
  status: 'pending' | 'running' | 'passed' | 'warning' | 'failed'
  correlation?: number
  top10Accuracy?: number
  details?: string
}

function Validation() {
  const [tests, setTests] = useState<ValidationTest[]>([])
  const [notice, setNotice] = useState<string | null>(null)
  const [isRunning, setIsRunning] = useState(false)

  const runTests = async () => {
    setIsRunning(true)
    setNotice(null)
    try {
      const results = await App.RunValidation()
      if (results.length === 0) {
        setTests([])
        setNotice('Validation tests are not implemented yet. See ROADMAP.md for the plan.')
      } else {
        const mapped = results.map((r, i) => mapValidationResult(r, i))
        setTests(mapped)
      }
    } catch (err) {
      console.error('Failed to run validation:', err)
      setTests([])
      setNotice('Validation failed to run. Check the backend logs for details.')
    }
    setIsRunning(false)
  }

  const mapValidationResult = (r: main.ValidationResult, index: number): ValidationTest => {
    const status: ValidationTest['status'] = r.passed ? 'passed' : 'failed'
    return {
      id: `test-${index + 1}`,
      name: r.testName,
      status,
      correlation: r.correlation,
      top10Accuracy: r.top10Accuracy,
      details: r.details,
    }
  }

  const getStatusIcon = (status: ValidationTest['status']) => {
    switch (status) {
      case 'passed': return '✅'
      case 'warning': return '⚠️'
      case 'failed': return '❌'
      case 'running': return '🔄'
      default: return '○'
    }
  }

  return (
    <div className="validation page-enter">
      <header className="validation-header">
        <h1>Validation Dashboard</h1>
        <p>Run tests to validate simulation accuracy against real-world results</p>
      </header>

      <div className="validation-controls card">
        <button
          className="primary"
          onClick={runTests}
          disabled={isRunning}
        >
          {isRunning ? 'Running Tests...' : 'Run All Tests'}
        </button>
      </div>

      {notice && (
        <div className="empty-state">{notice}</div>
      )}

      <section className="card tests-list">
        <h2>Validation Tests</h2>
        {tests.length === 0 ? (
          <div className="empty-state">No validation results yet. Run tests to populate this table.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Status</th>
                <th>Test Name</th>
                <th>Correlation</th>
                <th>Top-10 Accuracy</th>
                <th>Details</th>
              </tr>
            </thead>
            <tbody>
              {tests.map((test) => (
                <tr key={test.id} className={`status-${test.status}`}>
                  <td className="status-cell">{getStatusIcon(test.status)}</td>
                  <td>{test.name}</td>
                  <td>{test.correlation?.toFixed(2) ?? '—'}</td>
                  <td>{test.top10Accuracy ? `${test.top10Accuracy}%` : '—'}</td>
                  <td className="details-cell">{test.details ?? '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section className="card metrics-summary">
        <h2>Summary Metrics</h2>
        <div className="metrics-grid">
          <div className="metric">
            <span className="metric-value">
              {tests.filter((t) => t.status === 'passed').length}
            </span>
            <span className="metric-label">Passed</span>
          </div>
          <div className="metric warning">
            <span className="metric-value">
              {tests.filter((t) => t.status === 'warning').length}
            </span>
            <span className="metric-label">Warnings</span>
          </div>
          <div className="metric danger">
            <span className="metric-value">
              {tests.filter((t) => t.status === 'failed').length}
            </span>
            <span className="metric-label">Failed</span>
          </div>
        </div>
      </section>
    </div>
  )
}

export default Validation
