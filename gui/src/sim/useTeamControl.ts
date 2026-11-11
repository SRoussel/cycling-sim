// useTeamControl owns everything the live race views share: the event log,
// pause/speed, interactive session lifecycle, per-rider telemetry (with
// interpolation), route profile, and all team-control commands.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import {
  ACCEPT_PULSE_MS,
  COMMAND_COOLDOWN_MS,
  EFFORT_DEBOUNCE_MS,
  EFFORT_DEFAULT_PCT,
  MAX_EVENTS,
  RiderTelemetryPoint,
  RouteProfile,
  TELEMETRY_HISTORY_MAX,
  TELEMETRY_INTERP_MS,
  TeamState,
  clampEffortPct,
  formatCommandLabel,
  groupBucket,
  groupSortRank,
  interpolateTelemetryMap,
  parseAttackEvent,
} from './shared'

export type CommandMode = 'persistent' | 'oneshot'

/** Fields common to race and stage-race progress events. */
export interface LiveProgressPayload {
  riderGroups?: Record<number, string>
  riderTelemetry?: Record<number, RiderTelemetryPoint>
  profile?: RouteProfile
  teamState?: TeamState
  events?: string[]
}

export interface AttackAlert {
  riderId: number | null
  team: string | null
  line: string
}

export function useTeamControl(opts: { interactiveMode: boolean; initialTeam: string; initialEvents?: string[] }) {
  const { interactiveMode } = opts

  // ── Event log ─────────────────────────────────────────────
  const [events, setEvents] = useState<string[]>(opts.initialEvents ?? [])
  const appendEvents = useCallback((lines: string[]) => {
    if (lines.length === 0) return
    setEvents((prev) => {
      const next = [...prev, ...lines]
      return next.length <= MAX_EVENTS ? next : next.slice(next.length - MAX_EVENTS)
    })
  }, [])

  // ── Session & playback ────────────────────────────────────
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [paused, setPaused] = useState(false)
  const [speed, setSpeed] = useState(0.5)

  // Stop the backend session when it is replaced or the page unmounts, so an
  // abandoned interactive race doesn't keep running and overwrite saved results.
  useEffect(() => {
    if (!sessionId) return
    return () => {
      void App.StopInteractiveSession(sessionId)
    }
  }, [sessionId])

  const active = interactiveMode && sessionId !== null

  const setBackendPaused = async (next: boolean) => {
    setPaused(next)
    if (active) await App.SetInteractivePaused(sessionId!, next)
  }
  const togglePause = () => setBackendPaused(!paused)
  const changeSpeed = async (next: number) => {
    setSpeed(next)
    if (active) await App.SetInteractiveSpeed(sessionId!, next)
  }

  // ── Team & rider state ────────────────────────────────────
  const [controlledTeam, setControlledTeam] = useState(opts.initialTeam)
  const [teamRiders, setTeamRiders] = useState<main.RiderInfo[]>([])
  const [riderGroups, setRiderGroups] = useState<Record<number, string>>({})
  const [riderPersistentActions, setRiderPersistentActions] = useState<Record<number, string>>({})
  const [riderOneShotQueued, setRiderOneShotQueued] = useState<Record<number, boolean>>({})
  const [riderEffortTargets, setRiderEffortTargets] = useState<Record<number, number>>({})
  const [teamState, setTeamState] = useState<TeamState | null>(null)
  const [sprintTrainOrder, setSprintTrainOrder] = useState<number[]>([])
  const [followMap, setFollowMap] = useState<Record<number, number | null>>({})
  const [acceptedButtons, setAcceptedButtons] = useState<Record<string, boolean>>({})
  const [riderCommandLocked, setRiderCommandLocked] = useState<Record<number, boolean>>({})
  const [pauseOnRivalAttacks, setPauseOnRivalAttacks] = useState(true)
  // Standing team orders (chase / break / pace), see SetTeamOrder.
  const [teamOrders, setTeamOrders] = useState<TeamOrders>(DEFAULT_TEAM_ORDERS)
  const [attackAlert, setAttackAlert] = useState<AttackAlert | null>(null)
  const [followAttackRiderId, setFollowAttackRiderId] = useState<number | null>(null)
  const seenAttackEventsRef = useRef<Set<string>>(new Set())
  const effortDebounceTimersRef = useRef<Record<number, number>>({})

  // ── Route ─────────────────────────────────────────────────
  const [routeProfile, setRouteProfile] = useState<RouteProfile | null>(null)
  const [routeKm, setRouteKm] = useState(0)

  // ── Telemetry with interpolation ──────────────────────────
  const [telemetryByRider, setTelemetryByRider] = useState<Record<number, RiderTelemetryPoint>>({})
  const telemetryDisplayRef = useRef<Record<number, RiderTelemetryPoint>>({})
  const telemetryPrevRef = useRef<Record<number, RiderTelemetryPoint>>({})
  const telemetryTargetRef = useRef<Record<number, RiderTelemetryPoint>>({})
  const telemetryLastUpdateRef = useRef(0)
  const telemetryRafRef = useRef<number | null>(null)
  const telemetryHistoryRef = useRef<Record<number, RiderTelemetryPoint[]>>({})

  const startTelemetryInterpolation = () => {
    if (telemetryRafRef.current !== null) return
    const tick = () => {
      const elapsedMs = performance.now() - telemetryLastUpdateRef.current
      const alpha = Math.max(0, Math.min(1, elapsedMs / TELEMETRY_INTERP_MS))
      const interpolated = interpolateTelemetryMap(telemetryPrevRef.current, telemetryTargetRef.current, alpha)
      telemetryDisplayRef.current = interpolated
      setTelemetryByRider(interpolated)
      telemetryRafRef.current = alpha < 1 ? requestAnimationFrame(tick) : null
    }
    telemetryRafRef.current = requestAnimationFrame(tick)
  }

  useEffect(() => {
    return () => {
      if (telemetryRafRef.current !== null) cancelAnimationFrame(telemetryRafRef.current)
      Object.values(effortDebounceTimersRef.current).forEach((t) => window.clearTimeout(t))
      effortDebounceTimersRef.current = {}
    }
  }, [])

  // ── Derived ───────────────────────────────────────────────
  const sortedTeamRiders = useMemo(() => {
    return [...teamRiders].sort((a, b) => {
      const rankA = groupSortRank(riderGroups[a.id])
      const rankB = groupSortRank(riderGroups[b.id])
      if (rankA !== rankB) return rankA - rankB
      return a.name.localeCompare(b.name)
    })
  }, [teamRiders, riderGroups])

  const groupedTeamRiders = useMemo(() => {
    const sections: Array<{ key: string; label: string; riders: main.RiderInfo[] }> = []
    const indexByKey: Record<string, number> = {}
    for (const rider of sortedTeamRiders) {
      const label = groupBucket(riderGroups[rider.id])
      if (indexByKey[label] === undefined) {
        indexByKey[label] = sections.length
        sections.push({ key: label, label, riders: [] })
      }
      sections[indexByKey[label]].riders.push(rider)
    }
    return sections
  }, [sortedTeamRiders, riderGroups])

  const teamRiderByID = useMemo(() => {
    const out: Record<number, main.RiderInfo> = {}
    for (const rider of teamRiders) out[rider.id] = rider
    return out
  }, [teamRiders])

  const normalizedSprintOrder = useMemo(() => {
    const seen = new Set<number>()
    const next: number[] = []
    for (const id of sprintTrainOrder) {
      if (!teamRiderByID[id] || seen.has(id)) continue
      seen.add(id)
      next.push(id)
    }
    for (const rider of teamRiders) {
      if (!seen.has(rider.id)) {
        seen.add(rider.id)
        next.push(rider.id)
      }
    }
    return next
  }, [sprintTrainOrder, teamRiders, teamRiderByID])

  const riderName = (id: number) => teamRiderByID[id]?.name || `Rider ${id}`
  const sprintPhaseLabel = teamState?.sprintTrainPhase || 'idle'
  const telemetryForRider = (id: number): RiderTelemetryPoint | null => telemetryByRider[id] || null
  const effortTargetForRider = (id: number, telemetry: RiderTelemetryPoint | null): number => {
    if (riderEffortTargets[id] !== undefined) return riderEffortTargets[id]
    if (telemetry) return clampEffortPct(telemetry.effortPct)
    return EFFORT_DEFAULT_PCT
  }
  const isRiderLocked = (id: number): boolean => riderCommandLocked[id] === true
  const isAccepted = (id: number, action: string, mode: CommandMode): boolean => acceptedButtons[`${id}:${action}:${mode}`] === true
  const buttonClass = (id: number, action: string, mode: CommandMode): string =>
    isAccepted(id, action, mode) ? 'secondary accepted' : 'secondary'

  // ── Session start ─────────────────────────────────────────
  const beginSession = async (sess: main.InteractiveSessionInfo) => {
    const riders = sess.teamRiders || []
    setSessionId(sess.sessionId)
    setControlledTeam(sess.controlledTeam)
    setTeamRiders(riders)
    setRiderPersistentActions(Object.fromEntries(riders.map((r) => [r.id, 'hold'])))
    setRiderOneShotQueued({})
    setRiderEffortTargets(Object.fromEntries(riders.map((r) => [r.id, EFFORT_DEFAULT_PCT])))
    const order = riders.map((r) => r.id)
    setSprintTrainOrder(order)
    setFollowMap({})
    setTeamState({ sprintTrainOrder: order, followMap: {}, sprintTrainPhase: 'idle' })
    setRiderGroups({})
    setRouteProfile(null)
    setRouteKm(0)
    setTelemetryByRider({})
    setAttackAlert(null)
    setFollowAttackRiderId(null)
    setTeamOrders(DEFAULT_TEAM_ORDERS)
    seenAttackEventsRef.current = new Set()
    telemetryDisplayRef.current = {}
    telemetryPrevRef.current = {}
    telemetryTargetRef.current = {}
    telemetryHistoryRef.current = {}
    await App.SetInteractiveSpeed(sess.sessionId, speed)
    appendEvents([`🎮 Controlling: ${sess.controlledTeam}`])
  }

  // ── Progress ingestion ────────────────────────────────────
  // Event listeners are subscribed once per session, so they read changing
  // values through this ref instead of a stale closure.
  const latest = useRef({ pauseOnRivalAttacks, paused, controlledTeam, normalizedSprintOrder, teamRiders, sessionId })
  latest.current = { pauseOnRivalAttacks, paused, controlledTeam, normalizedSprintOrder, teamRiders, sessionId }

  /** Returns true when the payload belongs to a different session. */
  const isForeignSession = (payloadSessionId?: string) =>
    interactiveMode && !!latest.current.sessionId && !!payloadSessionId && payloadSessionId !== latest.current.sessionId

  const ingestProgress = (payload: LiveProgressPayload) => {
    if (payload.riderGroups) setRiderGroups(payload.riderGroups)
    if (payload.riderTelemetry) {
      telemetryPrevRef.current = telemetryDisplayRef.current
      telemetryTargetRef.current = payload.riderTelemetry
      telemetryLastUpdateRef.current = performance.now()
      const history = telemetryHistoryRef.current
      for (const [id, point] of Object.entries(payload.riderTelemetry)) {
        const series = history[Number(id)] || []
        series.push(point)
        if (series.length > TELEMETRY_HISTORY_MAX) series.splice(0, series.length - TELEMETRY_HISTORY_MAX)
        history[Number(id)] = series
      }
      startTelemetryInterpolation()
    }
    if (payload.profile) setRouteProfile(payload.profile)
    if (payload.teamState) {
      setTeamState(payload.teamState)
      setSprintTrainOrder(payload.teamState.sprintTrainOrder || [])
      setFollowMap(payload.teamState.followMap || {})
    }
    if (payload.events && payload.events.length > 0) {
      const cur = latest.current
      if (interactiveMode && cur.sessionId && cur.pauseOnRivalAttacks && !cur.paused) {
        for (const line of payload.events) {
          const parsed = parseAttackEvent(line)
          if (!parsed || seenAttackEventsRef.current.has(line)) continue
          seenAttackEventsRef.current.add(line)
          const isRival = parsed.team ? parsed.team !== cur.controlledTeam : true
          if (!isRival) continue
          setAttackAlert({ riderId: parsed.riderId, team: parsed.team, line })
          setFollowAttackRiderId(cur.normalizedSprintOrder[0] ?? cur.teamRiders[0]?.id ?? null)
          setPaused(true)
          void App.SetInteractivePaused(cur.sessionId, true)
          appendEvents(['⏸ Paused: rival attack detected'])
          break
        }
      }
      appendEvents(payload.events)
    }
    setRiderOneShotQueued({})
  }

  // ── Commands ──────────────────────────────────────────────
  const sendCommand = async (riderId: number, action: string, mode: CommandMode) => {
    if (!active || isRiderLocked(riderId)) return
    setRiderCommandLocked((prev) => ({ ...prev, [riderId]: true }))
    const key = `${riderId}:${action}:${mode}`
    try {
      await App.SetRiderCommand(sessionId!, riderId, action, mode)
      setAcceptedButtons((prev) => ({ ...prev, [key]: true }))
      window.setTimeout(() => {
        setAcceptedButtons((prev) => {
          const next = { ...prev }
          delete next[key]
          return next
        })
      }, ACCEPT_PULSE_MS)
      const label = mode === 'oneshot' ? 'Attack now' : formatCommandLabel(action)
      appendEvents([`🎮 ${riderName(riderId)}: ${label}`])
      if (mode === 'persistent') {
        setRiderPersistentActions((prev) => ({ ...prev, [riderId]: action }))
      } else {
        setRiderOneShotQueued((prev) => ({ ...prev, [riderId]: true }))
      }
    } catch (err) {
      console.error('Failed to send command', err)
    } finally {
      window.setTimeout(() => {
        setRiderCommandLocked((prev) => {
          const next = { ...prev }
          delete next[riderId]
          return next
        })
      }, COMMAND_COOLDOWN_MS)
    }
  }

  const handleTacticChange = (riderId: number, next: string) => void sendCommand(riderId, next, 'persistent')

  const handleEffortChange = (riderID: number, nextEffort: number) => {
    const clamped = clampEffortPct(nextEffort)
    setRiderEffortTargets((prev) => ({ ...prev, [riderID]: clamped }))
    const existing = effortDebounceTimersRef.current[riderID]
    if (existing !== undefined) window.clearTimeout(existing)
    effortDebounceTimersRef.current[riderID] = window.setTimeout(() => {
      delete effortDebounceTimersRef.current[riderID]
      if (!active) return
      App.SetRiderEffort(sessionId!, riderID, clamped).catch((err) => console.error('Failed to set rider effort', err))
    }, EFFORT_DEBOUNCE_MS)
  }

  const moveSprintTrainRider = (riderID: number, direction: -1 | 1) => {
    const order = [...normalizedSprintOrder]
    const idx = order.indexOf(riderID)
    const target = idx + direction
    if (idx < 0 || target < 0 || target >= order.length) return
    ;[order[idx], order[target]] = [order[target], order[idx]]
    setSprintTrainOrder(order)
    if (active) {
      App.SetSprintTrainOrder(sessionId!, order).catch((err) => console.error('Failed to set sprint train order', err))
    }
  }

  const handleFollowChange = async (riderID: number, followRiderID: number | null) => {
    if (!active) return
    const safeFollow = followRiderID === riderID ? null : followRiderID
    setFollowMap((prev) => ({ ...prev, [riderID]: safeFollow }))
    try {
      await App.SetRiderFollow(sessionId!, riderID, safeFollow ?? 0)
    } catch (err) {
      console.error('Failed to set rider follow target', err)
    }
  }

  const handleTeamOrder = async (kind: TeamOrderKind, value: TeamOrderValue) => {
    if (!active) return
    try {
      await App.SetTeamOrder(sessionId!, kind, value)
      setTeamOrders((prev) => ({ ...prev, [kind]: value }))
      appendEvents([`📣 Team order: ${TEAM_ORDERS[kind].label}: ${TEAM_ORDERS[kind].options[value].label}`])
    } catch (err) {
      console.error('Failed to set team order', err)
    }
  }

  const handleLaunchSprint = async (riderID: number) => {
    if (!active) return
    try {
      await App.LaunchSprint(sessionId!, riderID)
      appendEvents([`🚀 ${riderName(riderID)}: Sprint launch`])
    } catch (err) {
      console.error('Failed to launch sprint', err)
    }
  }

  const handleFollowAttack = async () => {
    if (!active || !followAttackRiderId) return
    try {
      await App.SetRiderCommand(sessionId!, followAttackRiderId, 'chase', 'persistent')
      appendEvents([`🎯 ${riderName(followAttackRiderId)}: Follow attack (Chase)`])
    } finally {
      setAttackAlert(null)
      await setBackendPaused(false)
    }
  }

  return {
    interactiveMode,
    // log
    events,
    appendEvents,
    // session & playback
    sessionId,
    paused,
    setPaused,
    speed,
    togglePause,
    changeSpeed,
    beginSession,
    isForeignSession,
    ingestProgress,
    // route
    routeProfile,
    routeKm,
    setRouteKm,
    // team
    controlledTeam,
    teamRiders,
    riderGroups,
    riderPersistentActions,
    riderOneShotQueued,
    followMap,
    sprintPhaseLabel,
    pauseOnRivalAttacks,
    setPauseOnRivalAttacks,
    teamOrders,
    attackAlert,
    followAttackRiderId,
    setFollowAttackRiderId,
    sortedTeamRiders,
    groupedTeamRiders,
    teamRiderByID,
    normalizedSprintOrder,
    riderName,
    telemetryForRider,
    effortTargetForRider,
    isRiderLocked,
    isAccepted,
    buttonClass,
    // commands
    sendCommand,
    handleTacticChange,
    handleEffortChange,
    moveSprintTrainRider,
    handleFollowChange,
    handleLaunchSprint,
    handleFollowAttack,
    handleTeamOrder,
  }
}

export type TeamControl = ReturnType<typeof useTeamControl>

// Team orders: standing orders for the player's team (see SetTeamOrder).
export type TeamOrderKind = 'chase' | 'break' | 'pace'
export type TeamOrderValue = 'auto' | 'yes' | 'no'
export type TeamOrders = Record<TeamOrderKind, TeamOrderValue>

export const DEFAULT_TEAM_ORDERS: TeamOrders = { chase: 'auto', break: 'auto', pace: 'auto' }

export const TEAM_ORDER_KINDS: TeamOrderKind[] = ['chase', 'break', 'pace']

export const TEAM_ORDERS: Record<TeamOrderKind, { label: string; options: Record<TeamOrderValue, { label: string; help: string }> }> = {
  chase: {
    label: 'Chasing',
    options: {
      auto: { label: 'Team decides', help: 'Your domestiques chase when the team has a reason to: a sprint, a leader to protect, a stage to win.' },
      yes: { label: 'Chase', help: 'Your domestiques ride to bring the groups ahead back before the finish.' },
      no: { label: "Don't chase", help: 'Your domestiques sit in and leave the chasing to other teams.' },
    },
  },
  break: {
    label: 'Breakaway',
    options: {
      auto: { label: 'Riders decide', help: 'Riders the team can spare today may go with moves.' },
      yes: { label: 'Get in the break', help: 'Your two strongest domestiques attack until one of them is up the road (not in the finale).' },
      no: { label: 'Stay together', help: 'Your domestiques don\'t go in breaks; leaders still attack when it pays.' },
    },
  },
  pace: {
    label: 'Pace',
    options: {
      auto: { label: 'Team decides', help: 'Your domestiques set the pace when a hard tempo would drop the fast finishers.' },
      yes: { label: 'Set the pace', help: 'Your domestiques ride a tempo for your leader at the front of their group.' },
      no: { label: 'Sit in', help: 'Your domestiques never drive the group.' },
    },
  },
}
