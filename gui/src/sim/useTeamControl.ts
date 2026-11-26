// useTeamControl owns everything the live race views share: the event log,
// pause/speed, interactive session lifecycle, per-rider telemetry (with
// interpolation), route profile, and all team-control commands.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { RadioState, newRadioState, radioMessages } from './radio'
import * as App from '../api'
import { director, app } from '../api'
import {
  ACCEPT_PULSE_MS,
  COMMAND_COOLDOWN_MS,
  EFFORT_DEBOUNCE_MS,
  EFFORT_DEFAULT_PCT,
  MAX_EVENTS,
  RiderTelemetryPoint,
  RouteProfile,
  TELEMETRY_HISTORY_MAX,
  TeamState,
  clampEffortPct,
  formatCommandLabel,
  groupBucket,
  groupSortRank,
  parseAttackEvent,
  formatEventLine,
} from './shared'

export type CommandMode = 'persistent' | 'oneshot'

/** Fields common to race and stage-race progress events. */
export interface LiveProgressPayload {
  riderGroups?: Record<number, string>
  riderTelemetry?: Record<number, RiderTelemetryPoint>
  profile?: RouteProfile
  teamState?: TeamState
  events?: string[]
  moment?: Moment
  goal?: RaceGoal
  gcGoal?: RaceGoal
  gc?: GCLine[]
  stagePlan?: StagePlanRequest
  team?: LiveTeam
  groups?: Array<{ name: string; gap: number; size?: number }>
  kmTotal?: number
  kmCovered?: number
  stageKmTotal?: number
  stageKm?: number
}

/** A decision moment from the director (internal/director): the race is
 * paused until the player picks an option. */
export interface MomentOption {
  id: string
  label: string
  help: string
}
export interface Moment {
  id: string
  kind: string
  title: string
  text: string
  kmRemaining: number
  options: MomentOption[]
}

/** Today's goal for the player's team (internal/director/goal.go). */
export interface RaceGoal {
  kind: 'place' | 'break' | string
  riderId: number
  riderName: string
  target: number
  text: string
}

/** Who set a team order or rider command (internal: interactive_live.go). */
export interface LiveOrder {
  value: TeamOrderValue
  source?: 'you' | 'plan' | 'decision' | string
  note?: string
  kmLeft?: number
}
export interface LiveRider {
  action: string
  source: string
  note?: string
  kmLeft?: number
}
export interface LiveTeam {
  orders: Record<string, LiveOrder>
  riders: Record<number, LiveRider>
}

/** The race waits for the player's plan of the coming stage. */
export interface StagePlanRequest {
  stage: number
  name: string
  points: director.PlanPoint[]
  /** How the last stage went (absent before the first). */
  report?: StageReport
}

/** A finished stage for the player: top five and ours, GC top ten and ours. */
export interface StageReport {
  stage: number
  name: string
  top: ReportLine[]
  ours: ReportLine[]
  gc: ReportLine[]
  ourGc: ReportLine[]
}

export interface ReportLine {
  riderId: number
  name: string
  team: string
  pos: number
  /** Seconds behind the stage winner / the GC leader. */
  gap: number
  /** GC places gained today (negative: lost). */
  move: number
}

/** A rider's place on GC before the stage (pos 0: none yet) and live. */
export interface GCLine {
  riderId: number
  name: string
  team: string
  pos: number
  gap: number
  livePos: number
  liveGap: number
}

/** How deep a rider digs to hold his group, and how hard he works. */
export type DigLevel = 'any' | 'reserve' | 'own'
export type WorkPace = 'auto' | 'tempo' | 'threshold' | 'allout'
export interface RiderStyle {
  dig: DigLevel
  pace: WorkPace
}
export const DEFAULT_STYLE: RiderStyle = { dig: 'any', pace: 'auto' }

/** How often the race stops: never, at the moments that decide it, or all. */
export type DecisionLevel = 'off' | 'key' | 'all'

// Fast-forward speed for "skip to the next decision".
const SKIP_SPEED = 16
const DECISIONS_KEY = 'cyclingSim.decisions'
// A rider's attacks within this distance show as one line in the log.
const ATTACK_LOG_GAP_KM = 5

export function useTeamControl(opts: { interactiveMode: boolean; initialTeam: string; initialEvents?: string[]; stageRace?: boolean }) {
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
  // ── Race radio ────────────────────────────────────────────
  const radioRef = useRef<RadioState>(newRadioState())
  const [goal, setGoal] = useState<RaceGoal | null>(null)
  const [gcGoal, setGcGoal] = useState<RaceGoal | null>(null)
  const [gc, setGc] = useState<GCLine[]>([])
  const [stagePlan, setStagePlan] = useState<StagePlanRequest | null>(null)
  // The team's orders and rider commands as the race applies them: the
  // panel shows this, whoever set them (you, the plan, a decision).
  const [live, setLive] = useState<LiveTeam | null>(null)
  // The player's calls this race (decisions and stage plans), for the
  // debrief: the log keeps only its last MAX_EVENTS lines.
  const [calls, setCalls] = useState<string[]>([])
  // Every starter's team by name (road groups list names), for team colours.
  const [teamByName, setTeamByName] = useState<Record<string, string>>({})
  // Every starter by id (team, watts, weight): for the group sheet.
  const [riderById, setRiderById] = useState<Record<number, app.RiderInfo>>({})
  // togglePause is defined before the stage plan's submit: read both by ref.
  const stagePlanRef = useRef<StagePlanRequest | null>(null)
  stagePlanRef.current = stagePlan
  const submitStagePlanRef = useRef<((p: { leaderId: number; break: string; hardOn: string[]; attackOn: string }) => Promise<void>) | null>(null)

  // ── Decision moments ──────────────────────────────────────
  const [moment, setMoment] = useState<Moment | null>(null)
  const [decisionLevel, setDecisionLevelState] = useState<DecisionLevel>(() => {
    try {
      const v = window.localStorage.getItem(DECISIONS_KEY)
      return v === 'off' || v === 'key' ? v : 'all'
    } catch {
      return 'all'
    }
  })
  // Speed to return to after "skip to the next decision" (null: not skipping).
  const [skipFrom, setSkipFrom] = useState<number | null>(null)

  const togglePause = () => {
    if (paused && stagePlanRef.current) {
      // Play on an open stage plan: the team decides.
      return submitStagePlanRef.current?.({ leaderId: 0, break: 'auto', hardOn: [], attackOn: '' })
    }
    if (paused && moment) setMoment(null) // resuming dismisses the decision
    return setBackendPaused(!paused)
  }
  const changeSpeed = async (next: number) => {
    setSpeed(next)
    setSkipFrom(null)
    if (active) await App.SetInteractiveSpeed(sessionId!, next)
  }
  const skipToDecision = async () => {
    if (!active) return
    if (skipFrom !== null) {
      await changeSpeed(skipFrom)
      return
    }
    setSkipFrom(speed)
    if (active) await App.SetInteractiveSpeed(sessionId!, SKIP_SPEED)
    if (paused) await setBackendPaused(false)
  }
  const setDecisionLevel = (level: DecisionLevel) => {
    setDecisionLevelState(level)
    try {
      window.localStorage.setItem(DECISIONS_KEY, level)
    } catch {
      /* per-viewer convenience only */
    }
    if (active) void App.SetDecisionLevel(sessionId!, level)
  }
  /** Send the coming stage's plan (empty: the team decides) and start it. */
  const submitStagePlan = async (plan: { leaderId: number; break: string; hardOn: string[]; attackOn: string }) => {
    if (!active || !stagePlan) return
    const req = stagePlan
    setStagePlan(null)
    await App.SetStagePlan(sessionId!, app.RacePlan.createFrom(plan))
    setPaused(false)
    setTeamOrders((prev) => ({ ...prev, break: plan.break as TeamOrderValue }))
    const bits = [
      plan.leaderId ? `ride for ${riderName(plan.leaderId)}` : '',
      plan.break === 'yes' ? 'a rider in the break' : plan.break === 'no' ? 'nobody in the break' : '',
      plan.hardOn.length ? `hard on ${plan.hardOn.length} point${plan.hardOn.length > 1 ? 's' : ''}` : '',
      plan.attackOn ? 'a planned attack' : '',
    ].filter(Boolean)
    const line = `📋 Stage ${req.stage} plan: ${bits.length ? bits.join(', ') : 'the team decides'}`
    appendEvents([line])
    setCalls((c) => [...c, line])
  }

  submitStagePlanRef.current = submitStagePlan

  const resolveMoment = async (optionId: string) => {
    if (!active || !moment) return
    const m = moment
    setMoment(null)
    const label = await App.ResolveMoment(sessionId!, m.id, optionId)
    setPaused(false)
    if (label) {
      const line = `🎬 ${m.title.replace(/^\S+\s/, '')}: ${label}`
      appendEvents([line])
      setCalls((c) => [...c, line])
    }
  }

  // ── Team & rider state ────────────────────────────────────
  const [controlledTeam, setControlledTeam] = useState(opts.initialTeam)
  const [teamRiders, setTeamRiders] = useState<app.RiderInfo[]>([])
  const [riderGroups, setRiderGroups] = useState<Record<number, string>>({})
  const [riderPersistentActions, setRiderPersistentActions] = useState<Record<number, string>>({})
  const [riderOneShotQueued, setRiderOneShotQueued] = useState<Record<number, boolean>>({})
  const [riderEffortTargets, setRiderEffortTargets] = useState<Record<number, number>>({})
  // How each rider spends himself: how deep he digs to hold his group, and
  // his pace when he does the work (engine: Dig, WorkPace).
  const [riderStyles, setRiderStyles] = useState<Record<number, RiderStyle>>({})
  const [teamState, setTeamState] = useState<TeamState | null>(null)
  const [sprintTrainOrder, setSprintTrainOrder] = useState<number[]>([])
  const [followMap, setFollowMap] = useState<Record<number, number | null>>({})
  const [acceptedButtons, setAcceptedButtons] = useState<Record<string, boolean>>({})
  const [riderCommandLocked, setRiderCommandLocked] = useState<Record<number, boolean>>({})
  // With 250 m decision steps a rider who keeps attacking logs an attack
  // every few hundred metres: the log shows one line per attack, a rider's
  // next only after ATTACK_LOG_GAP_KM.
  const lastAttackKmRef = useRef<Record<number, number>>({})
  // Race conditions alternate when two hold at once (the final 10 km and a
  // climb near the finish): "once per race" conditions show once, the rest
  // at most once per ATTACK_LOG_GAP_KM.
  const lastConditionKmRef = useRef<Record<string, number>>({})
  const isNewAttack = (line: string): boolean => {
    const km = Number(line.split(' | km=')[1])
    const kind = line.slice(0, line.indexOf(':')).trim().toLowerCase()
    if (['final', 'summit', 'cobbles', 'sprint-chase'].includes(kind)) {
      const prev = lastConditionKmRef.current[kind]
      if (prev !== undefined && (kind === 'final' || kind === 'summit' || Number.isNaN(km) || prev - km < ATTACK_LOG_GAP_KM)) return false
      lastConditionKmRef.current[kind] = km
      return true
    }
    const a = parseAttackEvent(line)
    if (!a || a.riderId === null) return true
    const prev = lastAttackKmRef.current[a.riderId]
    if (prev !== undefined && !Number.isNaN(km) && prev - km < ATTACK_LOG_GAP_KM) return false
    if (!Number.isNaN(km)) lastAttackKmRef.current[a.riderId] = km
    return true
  }
  // Standing team orders (chase / break / pace), see SetTeamOrder.
  const [teamOrders, setTeamOrders] = useState<TeamOrders>(DEFAULT_TEAM_ORDERS)
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
    const sections: Array<{ key: string; label: string; riders: app.RiderInfo[] }> = []
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
    const out: Record<number, app.RiderInfo> = {}
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
  const beginSession = async (sess: app.InteractiveSessionInfo) => {
    const riders = sess.teamRiders || []
    setSessionId(sess.sessionId)
    setControlledTeam(sess.controlledTeam)
    setTeamRiders(riders)
    setRiderPersistentActions(Object.fromEntries(riders.map((r) => [r.id, 'hold'])))
    setRiderOneShotQueued({})
    setRiderEffortTargets(Object.fromEntries(riders.map((r) => [r.id, EFFORT_DEFAULT_PCT])))
    setRiderStyles({})
    const order = riders.map((r) => r.id)
    setSprintTrainOrder(order)
    setFollowMap({})
    setTeamState({ sprintTrainOrder: [], followMap: {}, sprintTrainPhase: 'idle' })
    setRiderGroups({})
    setRouteProfile(null)
    setRouteKm(0)
    setTelemetryByRider({})
    setMoment(null)
    setGoal(null)
    setGcGoal(null)
    setGc([])
    setStagePlan(null)
    setLive(null)
    setCalls([])
    setTeamByName(Object.fromEntries((sess.allRiders || []).map((r) => [r.name, r.team])))
    setRiderById(Object.fromEntries((sess.allRiders || []).map((r) => [r.id, r])))
    radioRef.current = newRadioState()
    setTeamOrders(DEFAULT_TEAM_ORDERS)
    lastAttackKmRef.current = {}
    lastConditionKmRef.current = {}
    telemetryDisplayRef.current = {}
    telemetryPrevRef.current = {}
    telemetryTargetRef.current = {}
    telemetryHistoryRef.current = {}
    await App.SetInteractiveSpeed(sess.sessionId, speed)
    await App.SetDecisionLevel(sess.sessionId, decisionLevel)
    appendEvents([`🎮 Controlling: ${sess.controlledTeam}`])
  }

  // ── Progress ingestion ────────────────────────────────────
  // Event listeners are subscribed once per session, so they read changing
  // values through this ref instead of a stale closure.
  const latest = useRef({ paused, sessionId, skipFrom, teamRiders, controlledTeam })
  latest.current = { paused, sessionId, skipFrom, teamRiders, controlledTeam }

  /** Returns true when the payload belongs to a different session. */
  const isForeignSession = (payloadSessionId?: string) =>
    interactiveMode && !!latest.current.sessionId && !!payloadSessionId && payloadSessionId !== latest.current.sessionId

  const ingestProgress = (payload: LiveProgressPayload) => {
    if (payload.riderGroups) setRiderGroups(payload.riderGroups)
    if (payload.goal && interactiveMode) setGoal((g) => (g?.text === payload.goal!.text ? g : payload.goal!)) // a new one each stage
    if (payload.gcGoal && interactiveMode) setGcGoal((g) => g ?? payload.gcGoal!)
    if (payload.gc && payload.gc.length > 0) setGc(payload.gc)
    if (payload.team && interactiveMode) {
      const t = payload.team
      setLive(t)
      setTeamOrders((prev) => {
        const next = { ...prev }
        for (const k of TEAM_ORDER_KINDS) if (t.orders[k]) next[k] = t.orders[k].value
        return next
      })
      setRiderPersistentActions(() => {
        const next: Record<number, string> = {}
        for (const r of latest.current.teamRiders) next[r.id] = t.riders[r.id]?.action ?? 'hold'
        return next
      })
    }
    if (payload.stagePlan && interactiveMode) {
      setStagePlan(payload.stagePlan)
      setPaused(true)
      const cur = latest.current
      if (cur.skipFrom !== null && cur.sessionId) {
        setSpeed(cur.skipFrom)
        setSkipFrom(null)
        void App.SetInteractiveSpeed(cur.sessionId, cur.skipFrom)
      }
    }
    if (interactiveMode) {
      const total = payload.stageKmTotal ?? payload.kmTotal ?? 0
      const done = payload.stageKm ?? payload.kmCovered ?? 0
      const lines = radioMessages(radioRef.current, latest.current.teamRiders, payload.riderTelemetry, payload.riderGroups, payload.groups, Math.max(0, total - done))
      if (lines.length > 0) appendEvents(lines)
    }
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
      // One render per update: the bars and numbers ease with CSS
      // transitions. (A per-frame interpolation loop re-rendered the whole
      // page ~60 times a second for every rider, which crawled under WSL's
      // software rendering.)
      telemetryDisplayRef.current = payload.riderTelemetry
      setTelemetryByRider(payload.riderTelemetry)
    }
    if (payload.profile) setRouteProfile(payload.profile)
    if (payload.teamState) {
      setTeamState(payload.teamState)
      setSprintTrainOrder(payload.teamState.sprintTrainOrder || [])
      setFollowMap(payload.teamState.followMap || {})
    }
    if (payload.events && payload.events.length > 0) {
      // In a directed race the feed keeps what the player acts on: the race
      // favourites' attacks and our own riders' (not every leader's), and
      // none of the route lines the top bar already shows.
      const team = latest.current.controlledTeam
      const relevant = (line: string) => {
        if (!interactiveMode) return true
        const kind = line.slice(0, line.indexOf(':')).trim()
        if (kind === 'cobbles' || kind === 'summit' || kind === 'final') return false
        const a = parseAttackEvent(line)
        return !a || a.fav || a.team === team
      }
      appendEvents(payload.events.filter(isNewAttack).filter(relevant).map(formatEventLine))
    }
    if (payload.moment && interactiveMode) {
      // The session paused itself at this moment.
      const cur = latest.current
      setMoment(payload.moment)
      setPaused(true)
      appendEvents([`⏸ ${payload.moment.title}`])
      if (cur.skipFrom !== null && cur.sessionId) {
        setSpeed(cur.skipFrom)
        setSkipFrom(null)
        void App.SetInteractiveSpeed(cur.sessionId, cur.skipFrom)
      }
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

  const setRiderStyle = (riderID: number, style: RiderStyle) => {
    setRiderStyles((prev) => ({ ...prev, [riderID]: style }))
    if (active) {
      App.SetRiderStyle(sessionId!, riderID, style.dig, style.pace).catch((err) => console.error('Failed to set rider style', err))
    }
  }
  const styleOf = (riderID: number): RiderStyle => riderStyles[riderID] ?? DEFAULT_STYLE

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

  // Who the lead-out rides for (null: the team's fastest finisher in each group).
  const trainSprinter: number | null = teamState?.sprintTrainOrder?.[0] ?? null
  const setTrainSprinter = (riderID: number | null) => {
    const order = riderID ? [riderID] : []
    setTeamState((prev) => ({ ...(prev ?? { followMap: {}, sprintTrainPhase: 'idle' }), sprintTrainOrder: order }))
    if (active) {
      App.SetSprintTrainOrder(sessionId!, order).catch((err) => console.error('Failed to set the lead-out sprinter', err))
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
      appendEvents([`📣 Team order: ${TEAM_ORDERS[kind].label}: ${TEAM_ORDERS[kind].options[value]?.label ?? value}`])
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
    moment,
    resolveMoment,
    goal,
    gcGoal,
    gc,
    stagePlan,
    submitStagePlan,
    live,
    calls,
    teamByName,
    riderById,
    telemetryByRider,
    decisionLevel,
    setDecisionLevel,
    skipping: skipFrom !== null,
    skipToDecision,
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
    teamOrders,
    stageRace: opts.stageRace ?? false,
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
    setRiderStyle,
    styleOf,
    trainSprinter,
    setTrainSprinter,
    handleFollowChange,
    handleLaunchSprint,
    handleTeamOrder,
  }
}

export type TeamControl = ReturnType<typeof useTeamControl>

// Team orders: standing orders for the player's team (see SetTeamOrder).
export type TeamOrderKind = 'chase' | 'break' | 'pace' | 'protect' | 'leadout' | 'ambition'
export type TeamOrderValue = 'auto' | 'yes' | 'no'
export type TeamOrders = Record<TeamOrderKind, TeamOrderValue>

export const DEFAULT_TEAM_ORDERS: TeamOrders = { chase: 'auto', break: 'auto', pace: 'auto', protect: 'auto', leadout: 'auto', ambition: 'auto' }

type OrderSpec = {
  label: string
  stageRaceOnly?: boolean
  options: Partial<Record<TeamOrderValue, { label: string; help: string }>>
}

export const TEAM_ORDER_KINDS: TeamOrderKind[] = ['chase', 'break', 'pace', 'protect', 'leadout', 'ambition']

export const TEAM_ORDERS: Record<TeamOrderKind, OrderSpec> = {
  chase: {
    label: 'Chase breaks',
    options: {
      auto: { label: 'Team decides', help: 'Your domestiques chase when the team has a reason to: a sprint, a leader to protect, a stage to win.' },
      yes: { label: 'Chase', help: 'Your domestiques ride to bring the groups ahead back before the finish.' },
      no: { label: "Don't chase", help: 'Your domestiques sit in and leave the chasing to other teams.' },
    },
  },
  break: {
    label: 'Riders in the break',
    options: {
      auto: { label: 'Team decides', help: 'Riders the team can spare today may go with moves.' },
      yes: { label: 'Get in the break', help: 'Your two strongest domestiques attack until one of them is up the road (not in the finale).' },
      no: { label: 'Nobody in breaks', help: "Your domestiques don't go in breaks; leaders still attack when it pays." },
    },
  },
  pace: {
    label: 'Set the pace',
    options: {
      auto: { label: 'Team decides', help: 'Your domestiques set the pace when a hard tempo would drop the fast finishers.' },
      yes: { label: 'Drive the group', help: 'Your domestiques ride a hard tempo at the front for your leader.' },
      no: { label: 'Sit in', help: 'Your domestiques never drive the group.' },
    },
  },
  protect: {
    label: 'Wait for a dropped leader',
    options: {
      auto: { label: 'Team decides', help: "Domestiques shelter the leader in their group, but nobody waits if the leader is dropped." },
      yes: { label: 'Wait and pace back', help: 'If your leader is dropped, domestiques up the road wait and those with them pace them back to the bunch.' },
      no: { label: 'Nobody waits', help: 'Riders up the road race for themselves if the leader is dropped.' },
    },
  },
  leadout: {
    label: 'Sprint lead-out',
    options: {
      auto: { label: 'Team decides', help: 'Domestiques ride for your leader or sprinter and empty themselves in the last 3 km.' },
      yes: { label: 'Sprint train', help: 'From 8 km out your riders follow the train order below; the last one sprints.' },
      no: { label: 'Every man for himself', help: 'Every rider races the finale for his own result.' },
    },
  },
  ambition: {
    label: 'Ride for the stage',
    stageRaceOnly: true,
    options: {
      auto: { label: 'Team decides', help: 'Each morning the team decides whether to hunt the stage, defending your GC place either way.' },
      yes: { label: 'Hunt the stage', help: "Your team brings the break back so your leader can race for the stage." },
      no: { label: 'GC only', help: "No hunting the stage and no chasing for your sprinter: everything for the GC." },
    },
  },
}
