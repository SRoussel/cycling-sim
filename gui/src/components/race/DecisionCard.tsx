import { TeamControl } from '../../sim/useTeamControl'

/** The open decision: what's happening and the choices, one click each. */
export function DecisionCard({ tc }: { tc: TeamControl }) {
  const m = tc.moment
  if (!m) return null
  return (
    <section className={`decision-card kind-${m.kind}`} role="dialog" aria-label={m.title}>
      <div className="decision-head">
        <h2>{m.title}</h2>
        <span className="decision-km">{m.kmRemaining.toFixed(1)} km to go</span>
      </div>
      <p className="decision-text">{m.text}</p>
      <div className="decision-options">
        {m.options.map((o, i) => (
          <button key={o.id} className={`decision-option${i === 0 ? ' first' : ''}`} onClick={() => tc.resolveMoment(o.id)}>
            <span className="decision-label">{o.label}</span>
            {o.help && <span className="decision-help">{o.help}</span>}
          </button>
        ))}
      </div>
    </section>
  )
}
