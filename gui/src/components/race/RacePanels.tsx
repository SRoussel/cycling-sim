export function EventLog({ events, emptyText }: { events: string[]; emptyText?: string }) {
  return (
    <section className="card events-panel">
      <h2>Event Log</h2>
      <ul className="events-list">
        {events.length === 0 && emptyText ? <li>{emptyText}</li> : events.map((event, i) => <li key={i}>{event}</li>)}
      </ul>
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
        {paused ? '▶ Play' : '⏸ Pause'}
      </button>
      <select value={speed} onChange={(e) => onSpeedChange(Number(e.target.value))} disabled={disabled}>
        {SPEEDS.map((s) => (
          <option key={s} value={s}>{s}x</option>
        ))}
      </select>
    </div>
  )
}
