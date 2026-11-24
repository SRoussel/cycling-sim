import { useState } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import './Validation.css'

type Status = 'passed' | 'failed' | 'skipped'

interface Check {
  name: string
  status: Status
  correlation?: number
  top10Accuracy?: number
  timeGapMAE?: number
  matchedRiders?: number
  isRealRace: boolean
  details?: string
}

const STATUS_LABEL: Record<Status, string> = { passed: 'Pass', failed: 'Fail', skipped: 'No data' }

/**
 * Quick model checks from inside the app: synthetic benchmarks (physics and
 * race-shape sanity) and the older real-race cases. The check that decides
 * model changes is the CLI's 2026 run (calibration and race history).
 */
function Validation() {
  const [checks, setChecks] = useState<Check[]>([])
  const [notice, setNotice] = useState<string | null>(null)
  const [running, setRunning] = useState(false)
  const [tab, setTab] = useState<'synthetic' | 'real'>('synthetic')

  const run = async () => {
    setRunning(true)
    setNotice(null)
    try {
      const results = await App.RunValidation()
      setChecks((results || []).map(toCheck))
      if (!results || results.length === 0) setNotice('No results came back.')
    } catch (err) {
      console.error('Failed to run validation:', err)
      setChecks([])
      setNotice('The checks failed to run. See the backend log.')
    }
    setRunning(false)
  }

  const synthetic = checks.filter((c) => !c.isRealRace)
  const real = checks.filter((c) => c.isRealRace)
  const shown = tab === 'synthetic' ? synthetic : real
  const count = (list: Check[], s: Status) => list.filter((c) => c.status === s).length

  return (
    <div className="validation page-enter">
      <header className="validation-header">
        <div>
          <h1>Quick checks</h1>
          <p>
            Synthetic benchmarks and older real-race cases, run from here in a minute or two. The check that decides model
            changes is the 2026 run from the command line: <code>cycling-sim validate --sets 2 --season 2026</code> (calibration
            and each race's history).
          </p>
        </div>
        <button className="primary" onClick={run} disabled={running}>{running ? 'Running…' : 'Run the checks'}</button>
      </header>

      {notice && <div className="empty-state">{notice}</div>}

      {checks.length > 0 && (
        <div className="val-figures">
          <div><span>Synthetic</span><b>{count(synthetic, 'passed')}<small>/{synthetic.length} pass</small></b></div>
          <div><span>Real races</span><b>{count(real, 'passed')}<small>/{real.length - count(real, 'skipped')} pass</small></b></div>
          <div><span>Failing</span><b className={count(checks, 'failed') > 0 ? 'warn' : ''}>{count(checks, 'failed')}</b></div>
        </div>
      )}

      <section className="card val-list">
        <div className="val-head">
          <div className="seg-tabs" role="tablist" aria-label="Kind of check">
            <button type="button" role="tab" aria-selected={tab === 'synthetic'} className={tab === 'synthetic' ? 'on' : ''} onClick={() => setTab('synthetic')}>Synthetic ({synthetic.length})</button>
            <button type="button" role="tab" aria-selected={tab === 'real'} className={tab === 'real' ? 'on' : ''} onClick={() => setTab('real')}>Real races ({real.length})</button>
          </div>
        </div>
        {checks.length === 0 ? (
          <div className="empty-state">{running ? 'Running the checks…' : 'Nothing run yet.'}</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Result</th>
                <th>Check</th>
                <th className="num" title="Rank correlation with the reference">Corr.</th>
                <th className="num" title="Share of the reference top 10 found in the simulated top 10">Top 10</th>
                {tab === 'real' && <th className="num" title="Mean gap error (a diagnostic)">Gap err.</th>}
                {tab === 'real' && <th className="num">Riders</th>}
                <th>Details</th>
              </tr>
            </thead>
            <tbody>
              {shown.map((c, i) => (
                <tr key={i}>
                  <td><span className={`val-status ${c.status}`}>{STATUS_LABEL[c.status]}</span></td>
                  <td className="val-name">{c.name}</td>
                  <td className="num">{c.correlation ? c.correlation.toFixed(2) : '—'}</td>
                  <td className="num">{c.top10Accuracy ? `${c.top10Accuracy.toFixed(0)}%` : '—'}</td>
                  {tab === 'real' && <td className="num">{c.timeGapMAE ? `${Math.round(c.timeGapMAE)} s` : '—'}</td>}
                  {tab === 'real' && <td className="num">{c.matchedRiders || '—'}</td>}
                  <td className="val-details">{c.details || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
    </div>
  )
}

function toCheck(r: main.ValidationResult): Check {
  return {
    name: r.testName,
    status: r.isRealRace && r.matchedRiders < 3 && !r.passed ? 'skipped' : r.passed ? 'passed' : 'failed',
    correlation: r.correlation,
    top10Accuracy: r.top10Accuracy,
    timeGapMAE: r.timeGapMAE,
    matchedRiders: r.matchedRiders,
    isRealRace: r.isRealRace,
    details: r.details,
  }
}

export default Validation
