// Shared types, constants and helpers for the live race views
// (Simulation.tsx and StageRaceSimulation.tsx).

export interface RiderTelemetryPoint {
  speedKph: number
  powerWatts: number
  effortPct: number
  fatiguePct: number
  wPrimePct: number
  routeKm: number
  routePct: number
}

export interface RouteProfile {
  totalKm: number
  points: Array<{ km: number; elev: number }>
}

export interface TeamState {
  sprintTrainOrder: number[]
  followMap: Record<number, number | null>
  sprintTrainPhase: 'idle' | 'following' | 'launch'
  launchingRiderId?: number
}

export const MAX_EVENTS = 150
export const COMMAND_COOLDOWN_MS = 450
export const ACCEPT_PULSE_MS = 360
export const TELEMETRY_INTERP_MS = 220
export const TELEMETRY_HISTORY_MAX = 120
export const EFFORT_MIN_PCT = 60
export const EFFORT_MAX_PCT = 130
export const EFFORT_DEFAULT_PCT = 100
export const EFFORT_DEBOUNCE_MS = 120

export function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t
}

export function interpolateTelemetryMap(
  previous: Record<number, RiderTelemetryPoint>,
  target: Record<number, RiderTelemetryPoint>,
  alpha: number
): Record<number, RiderTelemetryPoint> {
  const out: Record<number, RiderTelemetryPoint> = {}
  const keys = new Set<number>([
    ...Object.keys(previous).map((k) => Number(k)),
    ...Object.keys(target).map((k) => Number(k)),
  ])

  keys.forEach((key) => {
    const from = previous[key]
    const to = target[key]
    if (!from && !to) return
    if (!from && to) {
      out[key] = to
      return
    }
    if (from && !to) {
      out[key] = from
      return
    }
    out[key] = {
      speedKph: lerp(from!.speedKph, to!.speedKph, alpha),
      powerWatts: lerp(from!.powerWatts, to!.powerWatts, alpha),
      effortPct: lerp(from!.effortPct, to!.effortPct, alpha),
      fatiguePct: lerp(from!.fatiguePct, to!.fatiguePct, alpha),
      wPrimePct: lerp(from!.wPrimePct, to!.wPrimePct, alpha),
      routeKm: lerp(from!.routeKm, to!.routeKm, alpha),
      routePct: lerp(from!.routePct, to!.routePct, alpha),
    }
  })

  return out
}

export function clamp01(value: number): number {
  if (value < 0) return 0
  if (value > 1) return 1
  return value
}

export function profilePath(points: Array<{ km: number; elev: number }>): string {
  if (points.length === 0) return ''
  const width = 100
  const height = 28
  const minVisualElevRange = 1200
  const minElev = Math.min(...points.map((p) => p.elev))
  const maxElev = Math.max(...points.map((p) => p.elev))
  const elevRange = Math.max(minVisualElevRange, maxElev-minElev)
  const totalKm = Math.max(0.001, points[points.length - 1].km)

  return points
    .map((p, idx) => {
      const x = (p.km / totalKm) * width
      const y = height - ((p.elev - minElev) / elevRange) * height
      return `${idx === 0 ? 'M' : 'L'}${x.toFixed(2)} ${y.toFixed(2)}`
    })
    .join(' ')
}

export function parseAttackEvent(line: string): { riderId: number | null; team: string | null } | null {
  const lower = line.toLowerCase()
  if (!lower.startsWith('attack:')) return null
  const riderMatch = line.match(/\brider=(\d+)\b/i)
  const teamMatch = line.match(/\bteam=([^\s]+)\b/i)
  return {
    riderId: riderMatch ? Number(riderMatch[1]) : null,
    team: teamMatch ? String(teamMatch[1]) : null,
  }
}

export interface RaceGroup {
  name: string
  gap: string
  riders: string[]
}

export function formatGap(seconds: number): string {
  if (seconds === 0) return '—'
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  if (m > 0) return `${m}:${s.toString().padStart(2, '0')}`
  return `${s}s`
}

export function formatRaceClock(seconds: number): string {
  const safe = Math.max(0, Math.floor(seconds))
  const h = Math.floor(safe / 3600)
  const m = Math.floor((safe % 3600) / 60)
  const s = safe % 60
  return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

/** Converts backend groups (gap in seconds) to display groups. */
export function toDisplayGroups(groups: Array<{ name: string; gap: number; riders: string[] }> | undefined): RaceGroup[] {
  return (groups || []).map((g) => ({
    name: g.name,
    gap: g.gap <= 0 ? '—' : '+' + formatGap(g.gap),
    riders: g.riders,
  }))
}

export function formatCommandLabel(action: string): string {
  switch (action) {
    case 'conserve':
      return 'Conserve'
    case 'chase':
      return 'Chase'
    case 'attack':
      return 'Attack'
    default:
      return 'AI (Hold)'
  }
}

export function groupClassName(groupName: string | undefined): string {
  const g = (groupName || '').toLowerCase()
  if (g.includes('lead') || g.includes('winner')) return 'group-label lead'
  if (g.includes('chaser') || g.includes('chase')) return 'group-label chasers'
  if (g.includes('peloton')) return 'group-label peloton'
  if (g.includes('dropped')) return 'group-label dropped'
  if (g.includes('group')) return 'group-label group'
  return 'group-label unknown'
}

export function groupSortRank(groupName: string | undefined): number {
  const g = (groupName || '').toLowerCase()
  if (g.includes('lead') || g.includes('winner')) return 0
  if (g.includes('chaser') || g.includes('chase')) return 1
  if (g.includes('peloton')) return 2
  if (g.includes('group')) return 3
  if (g.includes('dropped')) return 4
  return 5
}

export function groupBucket(groupName: string | undefined): string {
  const g = (groupName || '').toLowerCase()
  if (g.includes('lead') || g.includes('winner')) return 'Lead'
  if (g.includes('chaser') || g.includes('chase')) return 'Chasers'
  if (g.includes('peloton')) return 'Peloton'
  if (g.includes('group')) return groupName || 'Group'
  if (g.includes('dropped')) return 'Dropped'
  return 'Unknown'
}

export function clampEffortPct(value: number): number {
  return Math.max(EFFORT_MIN_PCT, Math.min(EFFORT_MAX_PCT, Math.round(value)))
}
