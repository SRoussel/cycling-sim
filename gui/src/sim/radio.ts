import { RiderTelemetryPoint } from './shared'

/**
 * Race radio: short messages from the player's riders when their state
 * changes in a way a director would hear about (an emptied reserve, a lost
 * wheel, a bridge, a recovery). Derived from telemetry and road groups only;
 * it never changes the race.
 *
 * Group changes are judged by who a rider was with, not by group names or
 * positions (those shift whenever a move goes clear or groups merge): he lost
 * the wheel when most of his previous group-mates are now ahead of him, and
 * he got across when most of his new group-mates were ahead of him before.
 */

export interface RadioRider {
  id: number
  name: string
}

interface RiderMemory {
  reserve: number // last W' %
  mates: number[] // riders in his group at the last update
  lowSince: boolean // reserve went under LOW and hasn't recovered
  lowSaid: boolean // he said he was on the limit and hasn't said he recovered
  emptySaid: boolean
  heavySaid: boolean
  lastKm: number // km remaining at his last message
  lastReserveKm: number // ... at his last message about his reserve
}

export interface RadioState {
  riders: Record<number, RiderMemory>
  rank: Record<number, number> // every rider's group position at the last update
  lastKm: number // km remaining at the team's last message
}

export const newRadioState = (): RadioState => ({ riders: {}, rank: {}, lastKm: Infinity })

const LOW = 25
const EMPTY = 5
const RECOVERED = 70
const HEAVY_LEGS = 88
// A rider's news about his group at most once per this many km, about his
// reserve (empty, on the limit, better) once per RESERVE_QUIET_KM: domestiques
// empty and refill all day, and hearing it every time is noise.
const QUIET_KM = 5
const RESERVE_QUIET_KM = 25
// The team speaks at most once per this many km (the most important line wins).
const TEAM_QUIET_KM = 3
// Group changes need this many riders to judge by.
const MIN_MATES = 3

/** The surname a teammate would use on the radio ("van Aert", "Pedersen"). */
export function radioName(full: string): string {
  const parts = full.trim().split(/\s+/)
  return parts.length > 1 ? parts.slice(1).join(' ') : full
}

function fmtGap(s: number): string {
  const r = Math.round(Math.max(0, s))
  return r < 60 ? `${r} s` : `${Math.floor(r / 60)}:${String(r % 60).padStart(2, '0')}`
}

/** A group's position on the road: listed groups in order, then "Group N" by N. */
function groupRanker(groups: Array<{ name: string }>): (name: string) => number {
  const order: Record<string, number> = {}
  groups.forEach((g, i) => (order[g.name] = i))
  return (name: string) => {
    if (order[name] !== undefined) return order[name]
    const n = Number(/(\d+)$/.exec(name)?.[1])
    return Number.isFinite(n) ? groups.length + n : groups.length + 1000
  }
}

/**
 * radioMessages compares this update with what the radio remembers and
 * returns new lines (updating the memory). riderGroups covers every rider in
 * the race; groups are the listed road groups front to back with gaps (s).
 */
export function radioMessages(
  state: RadioState,
  riders: RadioRider[],
  telemetry: Record<number, RiderTelemetryPoint> | undefined,
  riderGroups: Record<number, string> | undefined,
  groups: Array<{ name: string; gap: number; size?: number }> | undefined,
  kmRemaining: number,
): string[] {
  if (!telemetry || !riderGroups || !groups || groups.length === 0) return []
  const rankOf = groupRanker(groups)
  const rank: Record<number, number> = {}
  const members: Record<string, number[]> = {}
  for (const [idStr, name] of Object.entries(riderGroups)) {
    const id = Number(idStr)
    rank[id] = rankOf(name)
    ;(members[name] ||= []).push(id)
  }
  const prevRank = state.rank
  const gapOf = (name: string) => groups.find((g) => g.name === name)?.gap

  const teamCanSpeak = state.lastKm - kmRemaining >= TEAM_QUIET_KM
  let best: { line: string; priority: number; mem: RiderMemory } | null = null
  for (const r of riders) {
    const tel = telemetry[r.id]
    const gname = riderGroups[r.id]
    if (!tel || gname === undefined) continue
    const mine = rank[r.id]
    const mates = (members[gname] || []).filter((id) => id !== r.id)
    const reserve = tel.wPrimePct
    const legs = tel.legsPct ?? 100
    const mem = state.riders[r.id]
    if (!mem) {
      state.riders[r.id] = { reserve, mates, lowSince: false, lowSaid: false, emptySaid: false, heavySaid: false, lastKm: Infinity, lastReserveKm: Infinity }
      continue
    }
    const who = radioName(r.name)
    const canSpeak = mem.lastKm - kmRemaining >= QUIET_KM
    const reserveNews = mem.lastReserveKm - kmRemaining >= RESERVE_QUIET_KM

    // Of his old group-mates, how many are now ahead of him; of his new
    // ones, how many were ahead of him before.
    const oldStill = mem.mates.filter((id) => rank[id] !== undefined)
    const leftBehind = oldStill.length >= MIN_MATES && oldStill.filter((id) => rank[id] < mine).length > oldStill.length / 2
    const myPrev = prevRank[r.id]
    const newKnown = mates.filter((id) => prevRank[id] !== undefined)
    const joined = myPrev !== undefined && newKnown.length >= MIN_MATES &&
      newKnown.filter((id) => prevRank[id] < myPrev).length > newKnown.length / 2

    let line: string | null = null
    let priority = 0
    if (!canSpeak || !teamCanSpeak) {
      // quiet: just remember
    } else if (leftBehind) {
      // The gap to the group most of his old mates are in now.
      const aheadName = Object.keys(members).find((n) => rankOf(n) < mine && oldStill.some((id) => riderGroups[id] === n))
      const ga = aheadName !== undefined ? gapOf(aheadName) : undefined
      const gb = gapOf(gname)
      const gap = ga !== undefined && gb !== undefined ? ` ${fmtGap(gb - ga)} back.` : ''
      line = reserve < LOW ? `${who}: I'm dropped.${gap} I can't close it.` : `${who}: Lost the wheel.${gap} I'm chasing.`
      priority = 5
    } else if (joined) {
      line = mine === 0 ? `${who}: I'm across, in the front group.` : `${who}: I'm back on, in ${groupWords(gname, mates.length + 1)}.`
      priority = 5
    } else if (!reserveNews) {
      // he's said how he feels recently
    } else if (reserve <= EMPTY && !mem.emptySaid) {
      line = `${who}: I'm empty. Nothing left right now.`
      priority = 4
    } else if (reserve < LOW && !mem.lowSince) {
      line = `${who}: On the limit, I need a few minutes in the wheels.`
      priority = 3
    } else if (reserve >= RECOVERED && mem.lowSaid) {
      line = `${who}: Feeling better, ready when you need me.`
      priority = 2
    } else if (legs < HEAVY_LEGS && !mem.heavySaid) {
      line = `${who}: Legs are getting heavy. Long day.`
      priority = 1
    }

    if (reserve < LOW) mem.lowSince = true
    if (reserve >= RECOVERED) {
      mem.lowSince = false
      mem.emptySaid = false
    }
    mem.reserve = reserve
    mem.mates = mates
    if (line && (!best || priority > best.priority)) {
      best = { line, priority, mem }
    }
  }
  state.rank = rank
  if (!best) return []
  const m = best.mem
  m.lastKm = kmRemaining
  state.lastKm = kmRemaining
  if (best.priority >= 2 && best.priority <= 4) m.lastReserveKm = kmRemaining
  if (best.priority === 4) m.emptySaid = true
  if (best.priority === 3) m.lowSaid = true
  if (best.priority === 2) m.lowSaid = false
  if (best.priority === 1) m.heavySaid = true
  return [`📻 ${best.line}`]
}

/** How a rider names the group he's in. */
function groupWords(name: string, size: number): string {
  const n = name.toLowerCase()
  if (n === 'peloton') return 'the peloton'
  if (n === 'lead') return 'the front group'
  return size === 1 ? 'a group of my own' : `a group of ${size}`
}
