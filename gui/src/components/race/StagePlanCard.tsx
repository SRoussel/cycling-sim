import { useState } from 'react'
import { TeamControl } from '../../sim/useTeamControl'
import { EMPTY_PLAN, PlanForm, PlanValue } from './PlanForm'

/** Before a stage: the player's plan for it (the race waits). */
export function StagePlanCard({ tc }: { tc: TeamControl }) {
  const req = tc.stagePlan
  const [plan, setPlan] = useState<PlanValue>(EMPTY_PLAN)
  if (!req) return null
  return (
    <section className="decision-card kind-plan stage-plan" role="dialog" aria-label={`Stage ${req.stage} plan`}>
      <div className="decision-head">
        <h2>📋 Stage {req.stage}: {req.name}</h2>
        <span className="decision-km">your plan</span>
      </div>
      <PlanForm riders={tc.teamRiders} points={req.points ?? []} value={plan} onChange={setPlan} />
      <div className="stage-plan-actions">
        <button className="secondary" onClick={() => { setPlan(EMPTY_PLAN); void tc.submitStagePlan(EMPTY_PLAN) }}>Team decides</button>
        <button className="primary" onClick={() => { void tc.submitStagePlan(plan); setPlan(EMPTY_PLAN) }}>Start the stage →</button>
      </div>
    </section>
  )
}
