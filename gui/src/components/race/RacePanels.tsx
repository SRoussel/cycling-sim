const EMOJI = /[\p{Extended_Pictographic}\u{1F3FB}-\u{1F3FF}\u{FE0F}\u{200D}]+\s*/gu

/** A feed line split into its km to go (in the margin) and its text, with
 * the backend's emoji markers dropped (radio and decisions keep a class). */
function feedLine(event: string): { km: string; text: string; cls?: string } {
  const cls = event.includes('📻') ? 'radio' : event.includes('🎬') ? 'decision' : undefined
  const m = event.match(/^([\d.]+) km to go · (.*)$/s)
  const text = (m ? m[2] : event).replace(EMOJI, '').trim()
  return { km: m ? m[1] : '', text, cls }
}

export function EventLog({ events, emptyText }: { events: string[]; emptyText?: string }) {
  return (
    <section className="card events-panel">
      <h2>Race feed</h2>
      <ol className="events-list">
        {events.length === 0 && emptyText ? <li><span /><span>{emptyText}</span></li> : events.map((event, i) => {
          const l = feedLine(event)
          return <li key={i} className={l.cls}><span className="ev-km">{l.km}</span><span>{l.text}</span></li>
        })}
      </ol>
    </section>
  )
}

const SPEEDS = [0.25, 0.5, 1, 2, 4, 8]

export function PlaybackControls({
  paused,
  speed,
  disabled,
  onTogglePause,
  onSpeedChange,
}: {
  paused: boolean
  speed: number
  disabled: boolean
  onTogglePause: () => void
  onSpeedChange: (speed: number) => void
}) {
  return (
    <div className="sim-controls">
      <button onClick={onTogglePause} disabled={disabled}>
        {paused ? 'Play' : 'Pause'}
      </button>
      <select value={speed} onChange={(e) => onSpeedChange(Number(e.target.value))} disabled={disabled}>
        {SPEEDS.map((s) => (
          <option key={s} value={s}>{s}x</option>
        ))}
      </select>
    </div>
  )
}
