/**
 * A colour per team for dots beside riders' names: the main colour of the
 * team's kit where it's distinctive, otherwise one from a palette picked by
 * the team's name (stable across races). Only a visual hint.
 */
const KIT: Array<[RegExp, string]> = [
  [/visma/i, '#ffd200'],
  [/^EF /i, '#ff4fa3'],
  [/UAE/i, '#f5f5f5'],
  [/lidl/i, '#e0192e'],
  [/soudal|quick-?step/i, '#2f80ed'],
  [/jayco/i, '#29b6f6'],
  [/bahrain/i, '#d4a017'],
  [/astana/i, '#00c2c7'],
  [/alpecin/i, '#2b4fbf'],
  [/ineos/i, '#8b1e3f'],
  [/red bull|bora/i, '#e63946'],
  [/movistar/i, '#4f7cff'],
  [/groupama|fdj/i, '#7aa7ff'],
  [/cofidis/i, '#ff3b3b'],
  [/uno-?x/i, '#ff7a00'],
  [/tudor/i, '#c81d25'],
  [/decathlon/i, '#0091d5'],
  [/lotto/i, '#ff5a5f'],
  [/picnic|postnl/i, '#9ca3af'],
  [/pinarello|q36/i, '#9aa5b1'],
  [/arkéa|arkea/i, '#ff6b6b'],
  [/israel/i, '#7dd3fc'],
  [/totalenergies/i, '#38bdf8'],
]

const PALETTE = ['#a78bfa', '#34d399', '#f472b6', '#fbbf24', '#60a5fa', '#f87171', '#2dd4bf', '#c084fc', '#facc15', '#fb923c', '#4ade80', '#e879f9']

export function teamColor(team: string | undefined | null): string {
  if (!team) return '#64748b'
  for (const [re, c] of KIT) if (re.test(team)) return c
  let h = 0
  for (let i = 0; i < team.length; i++) h = (h * 31 + team.charCodeAt(i)) >>> 0
  return PALETTE[h % PALETTE.length]
}
