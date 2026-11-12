import { main } from '../../../wailsjs/go/models'
import { formatGap } from '../../sim/shared'
import { RaceGoal } from '../../sim/useTeamControl'

const ordinal = (n: number) => {
  const s = ['th', 'st', 'nd', 'rd']
  const v = n % 100
  return `${n}${s[(v - 20) % 10] || s[v] || s[0]}`
}

/**
 * The race debrief: the goal met or missed, how the team's riders finished,
 * and the decisions the player made along the way.
 */
export function Debrief({
  goal,
  breakMade,
  results,
  team,
  decisions,
  onResults,
}: {
  goal: RaceGoal | null
  breakMade: boolean
  results: main.RaceResults
  team: string
  decisions: string[]
  onResults: () => void
}) {
  const ours = results.results.filter((r) => r.team === team).sort((a, b) => a.position - b.position)
  const best = ours[0]
  const goalRider = goal?.kind === 'place' ? results.results.find((r) => r.riderId === goal.riderId) : undefined
  let met = false
  let verdict = ''
  if (goal?.kind === 'place') {
    // The goal counts for the team: any of ours reaching the target place.
    met = !!best && best.position <= goal.target
    verdict = goalRider
      ? `${goalRider.riderName} finished ${ordinal(goalRider.position)}${goalRider.gap > 0 ? ` (+${formatGap(goalRider.gap)})` : ''}.`
      : `${goal.riderName} didn't finish.`
    if (best && goalRider && best.riderId !== goalRider.riderId) {
      verdict += ` Best of ours: ${best.riderName}, ${ordinal(best.position)}.`
    }
  } else if (goal?.kind === 'break') {
    met = breakMade
    verdict = breakMade ? 'We had a rider in the break.' : 'Nobody of ours made the break.'
    if (best) verdict += ` Best of ours: ${best.riderName}, ${ordinal(best.position)}.`
  }
  return (
    <section className={`debrief ${goal ? (met ? 'met' : 'missed') : ''}`}>
      <div className="debrief-head">
        <h2>{goal ? `${met ? '✅ Goal met' : '❌ Goal missed'}: ${goal.text}` : '🏁 Race over'}</h2>
        <button className="primary" onClick={onResults}>Full results →</button>
      </div>
      {verdict && <p className="debrief-verdict">{verdict}</p>}
      <div className="debrief-cols">
        <div>
          <h3>Our riders</h3>
          <ol className="debrief-riders">
            {ours.slice(0, 7).map((r) => (
              <li key={r.riderId}>
                <span>{ordinal(r.position)}</span> {r.riderName}
                {r.gap > 0 && <span className="debrief-gap"> +{formatGap(r.gap)}</span>}
              </li>
            ))}
          </ol>
        </div>
        <div>
          <h3>Your calls</h3>
          {decisions.length === 0 ? (
            <p className="debrief-none">No decisions: the team raced on its own.</p>
          ) : (
            <ul className="debrief-calls">
              {decisions.map((d, i) => (
                <li key={i}>{d.replace(/^🎬\s*/, '')}</li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </section>
  )
}
