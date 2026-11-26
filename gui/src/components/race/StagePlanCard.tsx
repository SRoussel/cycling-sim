import { useState } from 'react'
import { ReportLine, StageReport, TeamControl } from '../../sim/useTeamControl'
import { formatGap } from '../../sim/shared'
import { Jersey } from './Jersey'
import { EMPTY_PLAN, PlanForm, PlanValue } from './PlanForm'

/** Before a stage: the player's plan for it (the race waits). */
export function StagePlanCard({ tc }: { tc: TeamControl }) {
  const req = tc.stagePlan
  const [plan, setPlan] = useState<PlanValue>(EMPTY_PLAN)
  if (!req) return null
  return (
    <section className="decision-card kind-plan stage-plan" role="dialog" aria-label={`Stage ${req.stage} plan`}>
      <div className="decision-head">
        <h2>Stage {req.stage}: {req.name}</h2>
        <span className="decision-km">your plan</span>
      </div>
      {req.report && <StageReportView report={req.report} team={tc.controlledTeam} />}
      <PlanForm riders={tc.teamRiders} points={req.points ?? []} value={plan} onChange={setPlan} />
      <div className="stage-plan-actions">
        <button className="secondary" onClick={() => { setPlan(EMPTY_PLAN); void tc.submitStagePlan(EMPTY_PLAN) }}>Team decides</button>
        <button className="primary" onClick={() => { void tc.submitStagePlan(plan); setPlan(EMPTY_PLAN) }}>Start the stage</button>
      </div>
    </section>
  )
}

/** Yesterday's stage and the GC now, so the plan is made knowing them. */
function StageReportView({ report, team }: { report: StageReport; team: string }) {
  const row = (l: ReportLine, gc: boolean) => (
    <li key={l.riderId} className={l.team === team ? 'mine' : ''}>
      <span className="pos">{l.pos}</span>
      <Jersey team={l.team} />
      <span className="who">{l.name}</span>
      <span className="gap">{l.pos === 1 ? (gc ? 'leader' : 'winner') : l.gap < 0.5 ? 's.t.' : formatGap(l.gap)}</span>
      {gc && <span className={`move ${l.move > 0 ? 'up' : l.move < 0 ? 'down' : ''}`}>{l.move > 0 ? `▲${l.move}` : l.move < 0 ? `▼${-l.move}` : ''}</span>}
    </li>
  )
  return (
    <div className="stage-report">
      <div>
        <h3>Stage {report.stage}: {report.name}</h3>
        <ol>{report.top.map((l) => row(l, false))}</ol>
        {report.ours.length > 0 && <ol className="ours">{report.ours.map((l) => row(l, false))}</ol>}
      </div>
      <div>
        <h3>General classification</h3>
        <ol>{report.gc.map((l) => row(l, true))}</ol>
        {report.ourGc.length > 0 && <ol className="ours">{report.ourGc.map((l) => row(l, true))}</ol>}
      </div>
    </div>
  )
}
