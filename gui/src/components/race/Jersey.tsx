import { teamColor } from '../../sim/teamColors'

/** A small cycling jersey in a team's colour (used sparingly: GC, podium, our team). */
export function Jersey({ team, size = 12 }: { team: string; size?: number }) {
  const c = teamColor(team)
  return (
    <svg className="jersey" width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <title>{team}</title>
      <path
        d="M8 2.5 L3 5.5 L1 11 L5 12.5 L6 10 L6 21.5 L18 21.5 L18 10 L19 12.5 L23 11 L21 5.5 L16 2.5 Q12 6 8 2.5 Z"
        fill={c}
        stroke="rgba(0,0,0,0.55)"
        strokeWidth="1.2"
        strokeLinejoin="round"
      />
      <path d="M6 15 L18 15" stroke="rgba(0,0,0,0.25)" strokeWidth="1.6" />
    </svg>
  )
}
