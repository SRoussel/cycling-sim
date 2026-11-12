// Shared types, constants and helpers for the live race views
// (Simulation.tsx and StageRaceSimulation.tsx).

export interface RiderTelemetryPoint {
  speedKph: number
  powerWatts: number
  effortPct: number
  fatiguePct: number
  wPrimePct: number
  /** Threshold now as a share of the rider's fresh one (fatigue, durability). */
  legsPct?: number
  routeKm: number
  routePct: number
}

export interface RouteSector {
  name: string
  startKm: number
  lengthKm: number
  stars: number
}

export interface RouteProfile {
  totalKm: number
  points: Array<{ km: number; elev: number }>
  sectors?: RouteSector[]
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
      legsPct: lerp(from!.legsPct ?? 100, to!.legsPct ?? 100, alpha),
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

/** Engine event lines look like "type: reason | km=12.3". */
function splitEvent(line: string): { type: string; reason: string; km: number | null } {
  const [body, kmPart] = line.split(' | km=')
  const colon = body.indexOf(': ')
  const type = colon >= 0 ? body.slice(0, colon) : body
  const reason = colon >= 0 ? body.slice(colon + 2) : ''
  const km = kmPart !== undefined && !Number.isNaN(Number(kmPart)) ? Number(kmPart) : null
  return { type: type.trim().toLowerCase(), reason, km }
}

/** Attack events: "attack: rider=581; key=1; name=Tadej Pogačar; team=UAE Team Emirates". */
export function parseAttackEvent(
  line: string,
): { riderId: number | null; team: string | null; name: string | null; key: boolean } | null {
  const ev = splitEvent(line)
  if (ev.type !== 'attack') return null
  const fields: Record<string, string> = {}
  for (const part of ev.reason.split('; ')) {
    const eq = part.indexOf('=')
    if (eq > 0) fields[part.slice(0, eq).trim()] = part.slice(eq + 1).trim()
  }
  return {
    riderId: fields.rider ? Number(fields.rider) : null,
    team: fields.team ?? null,
    name: fields.name ?? null,
    key: fields.key === '1',
  }
}

/** A readable line for the event log (engine lines are formatted; others pass through). */
export function formatEventLine(line: string): string {
  const ev = splitEvent(line)
  const at = ev.km !== null ? `${ev.km.toFixed(1)} km to go · ` : ''
  switch (ev.type) {
    case 'attack': {
      const a = parseAttackEvent(line)
      if (!a) return line
      return `${at}⚡ ${a.name ?? `Rider ${a.riderId}`}${a.team ? ` (${a.team})` : ''} attacks`
    }
    case 'breakaway':
      if (ev.reason === 'breakaway caught') return `${at}🤝 Breakaway caught`
      return `${at}🏃 Breakaway: ${ev.reason.replace(/^breakaway:\s*/, '')}`
    case 'sprint-chase':
      return `${at}🚴 Sprinters' teams chase`
    case 'cobbles':
      return `${at}🪨 Cobbles / technical section`
    case 'summit':
      return `${at}⛰ Climb near the finish`
    case 'final':
      return `${at}🏁 Final 10 km`
  }
  return line
}

export interface RaceGroup {
  name: string
  gap: string
  riders: string[]
  /** Riders in the group (riders lists only the first few names). */
  size?: number
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
export function toDisplayGroups(
  groups: Array<{ name: string; gap: number; riders: string[]; size?: number }> | undefined,
): RaceGroup[] {
  return (groups || []).map((g) => ({
    name: g.name,
    gap: g.gap <= 0 ? '—' : '+' + formatGap(g.gap),
    riders: g.riders,
    size: g.size,
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
  if (/^\d+(st|nd|rd|th)$/.test(g)) return 'Finished'
  return 'Finished'
}

export function clampEffortPct(value: number): number {
  return Math.max(EFFORT_MIN_PCT, Math.min(EFFORT_MAX_PCT, Math.round(value)))
}
