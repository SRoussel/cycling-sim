import { useParams, useLocation, useNavigate } from 'react-router-dom'
import { useState, useEffect, useRef, useMemo } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import './Simulation.css'

interface RaceGroup {
  name: string
  gap: string
  riders: string[]
}

interface RiderTelemetryPoint {
  speedKph: number
  powerWatts: number
  effortPct: number
  fatiguePct: number
  wPrimePct: number
  routeKm: number
  routePct: number
}

interface RouteProfile {
  totalKm: number
  points: Array<{ km: number; elev: number }>
}

interface TeamState {
  sprintTrainOrder: number[]
  followMap: Record<number, number | null>
  sprintTrainPhase: 'idle' | 'following' | 'launch'
  launchingRiderId?: number
}

interface SimulationProgressEvent {
  sessionId?: string
  raceId: number
  kmCovered: number
  kmTotal: number
  elapsedTime?: number
  progress: number
  segmentName: string
  riderGroups?: Record<number, string>
  riderTelemetry?: Record<number, RiderTelemetryPoint>
  profile?: RouteProfile
  teamState?: TeamState
  groups: Array<{
    name: string
    gap: number
    riders: string[]
  }>
  events: string[]
}

interface InteractiveCompleteEvent {
  sessionId: string
  raceId: number
}

const MAX_EVENTS = 150
const COMMAND_COOLDOWN_MS = 450
const ACCEPT_PULSE_MS = 360
const TELEMETRY_INTERP_MS = 220
const TELEMETRY_HISTORY_MAX = 120
const EFFORT_MIN_PCT = 60
const EFFORT_MAX_PCT = 130
const EFFORT_DEFAULT_PCT = 100
const EFFORT_DEBOUNCE_MS = 120

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t
}

function interpolateTelemetryMap(
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

function clamp01(value: number): number {
  if (value < 0) return 0
  if (value > 1) return 1
  return value
}

function profilePath(points: Array<{ km: number; elev: number }>): string {
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

function parseAttackEvent(line: string): { riderId: number | null; team: string | null } | null {
  const lower = line.toLowerCase()
  if (!lower.startsWith('attack:')) return null
  const riderMatch = line.match(/\brider=(\d+)\b/i)
  const teamMatch = line.match(/\bteam=([^\s]+)\b/i)
  return {
    riderId: riderMatch ? Number(riderMatch[1]) : null,
    team: teamMatch ? String(teamMatch[1]) : null,
  }
}

function Simulation() {
  const { raceId } = useParams()
  const location = useLocation()
  const navigate = useNavigate()
  const [progress, setProgress] = useState(0)
  const [elapsedTime, setElapsedTime] = useState(0)
  const [isRunning, setIsRunning] = useState(true)
  const [speed, setSpeed] = useState(0.5)
  const [simulationComplete, setSimulationComplete] = useState(false)
  const [results, setResults] = useState<main.RaceResults | null>(null)
  const [liveProgress, setLiveProgress] = useState(false)
  const [listenerReady, setListenerReady] = useState(false)
  const simulationStarted = useRef(false)
  const lastSegmentName = useRef<string | null>(null)
  
  const race = location.state?.race as main.RaceInfo | undefined
  const enableIncidents = location.state?.enableIncidents as boolean ?? false
  const interactiveMode = location.state?.interactiveMode as boolean ?? false
  const initialTeam = location.state?.controlledTeam as string ?? ''
  const raceName = race?.name || raceId?.replace(/-/g, ' ') || 'Race'
  const raceDistance = race?.distance || 257

  const [sessionId, setSessionId] = useState<string | null>(null)
  const [controlledTeam, setControlledTeam] = useState(initialTeam)
  const [teamRiders, setTeamRiders] = useState<main.RiderInfo[]>([])
  const [telemetryByRider, setTelemetryByRider] = useState<Record<number, RiderTelemetryPoint>>({})
  const [routeProfile, setRouteProfile] = useState<RouteProfile | null>(null)
  const [routeKm, setRouteKm] = useState(0)
  const [riderGroups, setRiderGroups] = useState<Record<number, string>>({})
  const [riderPersistentActions, setRiderPersistentActions] = useState<Record<number, string>>({})
  const [riderOneShotQueued, setRiderOneShotQueued] = useState<Record<number, boolean>>({})
  const [riderEffortTargets, setRiderEffortTargets] = useState<Record<number, number>>({})
  const [teamState, setTeamState] = useState<TeamState | null>(null)
  const [sprintTrainOrder, setSprintTrainOrder] = useState<number[]>([])
  const [followMap, setFollowMap] = useState<Record<number, number | null>>({})
  const [acceptedButtons, setAcceptedButtons] = useState<Record<string, boolean>>({})
  const [riderCommandLocked, setRiderCommandLocked] = useState<Record<number, boolean>>({})
  const telemetryDisplayRef = useRef<Record<number, RiderTelemetryPoint>>({})
  const telemetryPrevRef = useRef<Record<number, RiderTelemetryPoint>>({})
  const telemetryTargetRef = useRef<Record<number, RiderTelemetryPoint>>({})
  const telemetryLastUpdateRef = useRef(0)
  const telemetryRafRef = useRef<number | null>(null)
  const telemetryHistoryRef = useRef<Record<number, RiderTelemetryPoint[]>>({})
  const effortDebounceTimersRef = useRef<Record<number, number>>({})

  const [pauseOnRivalAttacks, setPauseOnRivalAttacks] = useState(true)
  const seenAttackEventsRef = useRef<Set<string>>(new Set())
  const [attackAlert, setAttackAlert] = useState<{ riderId: number | null; team: string | null; line: string } | null>(null)
  const [followAttackRiderId, setFollowAttackRiderId] = useState<number | null>(null)

  const [events, setEvents] = useState<string[]>([
    `Simulation started: ${raceName}`,
  ])

  const [groups, setGroups] = useState<RaceGroup[]>([])

  const appendEvents = (lines: string[]) => {
    if (lines.length === 0) return
    setEvents((prev) => {
      const next = [...prev, ...lines]
      if (next.length <= MAX_EVENTS) return next
      return next.slice(next.length - MAX_EVENTS)
    })
  }

  const startTelemetryInterpolation = () => {
    if (telemetryRafRef.current !== null) return

    const tick = () => {
      const elapsedMs = performance.now() - telemetryLastUpdateRef.current
      const alpha = Math.max(0, Math.min(1, elapsedMs / TELEMETRY_INTERP_MS))
      const interpolated = interpolateTelemetryMap(telemetryPrevRef.current, telemetryTargetRef.current, alpha)
      telemetryDisplayRef.current = interpolated
      setTelemetryByRider(interpolated)

      if (alpha < 1) {
        telemetryRafRef.current = requestAnimationFrame(tick)
        return
      }
      telemetryRafRef.current = null
    }

    telemetryRafRef.current = requestAnimationFrame(tick)
  }

  // Animate progress while simulation runs (fallback when live updates are unavailable)
  useEffect(() => {
    if (liveProgress || !isRunning || simulationComplete) return
    const interval = setInterval(() => {
      setProgress((p) => {
        if (simulationComplete || p >= 95) {
          return p
        }
        return p + 0.3 * speed
      })
    }, 100)
    return () => clearInterval(interval)
  }, [isRunning, speed, simulationComplete, liveProgress])

  useEffect(() => {
    const off = EventsOn('simulation_progress', (payload: SimulationProgressEvent) => {
      if (!race || payload.raceId !== race.id) return
      if (interactiveMode && sessionId && payload.sessionId && payload.sessionId !== sessionId) return
      setLiveProgress(true)
      setProgress(Math.min(100, payload.progress))
      setRouteKm(Math.max(0, payload.kmCovered || 0))
      setElapsedTime(Math.max(0, payload.elapsedTime || 0))
      const nextGroups: RaceGroup[] = payload.groups.map((group) => ({
        name: group.name,
        gap: group.gap <= 0 ? '—' : '+' + formatGap(group.gap),
        riders: group.riders,
      }))
      if (payload.riderGroups) {
        setRiderGroups(payload.riderGroups)
      }
      if (payload.riderTelemetry) {
        const now = performance.now()
        telemetryPrevRef.current = telemetryDisplayRef.current
        telemetryTargetRef.current = payload.riderTelemetry
        telemetryLastUpdateRef.current = now

        const history = telemetryHistoryRef.current
        Object.entries(payload.riderTelemetry).forEach(([id, point]) => {
          const riderID = Number(id)
          const series = history[riderID] || []
          series.push(point)
          if (series.length > TELEMETRY_HISTORY_MAX) {
            series.splice(0, series.length - TELEMETRY_HISTORY_MAX)
          }
          history[riderID] = series
        })

        startTelemetryInterpolation()
      }
      if (payload.profile) {
        setRouteProfile(payload.profile)
      }
      if (payload.teamState) {
        setTeamState(payload.teamState)
        setSprintTrainOrder(payload.teamState.sprintTrainOrder || [])
        setFollowMap(payload.teamState.followMap || {})
      }
      if (nextGroups.length > 0) {
        setGroups(nextGroups)
      }
      if (payload.segmentName && payload.segmentName !== lastSegmentName.current) {
        lastSegmentName.current = payload.segmentName
        appendEvents([`Segment: ${payload.segmentName}`])
      }
      if (payload.events && payload.events.length > 0) {
        if (interactiveMode && sessionId && pauseOnRivalAttacks && isRunning) {
          for (const line of payload.events) {
            const parsed = parseAttackEvent(line)
            if (!parsed) continue
            if (seenAttackEventsRef.current.has(line)) continue
            seenAttackEventsRef.current.add(line)

            const isRival = parsed.team ? parsed.team !== controlledTeam : true
            if (!isRival) continue

            setAttackAlert({ riderId: parsed.riderId, team: parsed.team, line })
            const defaultFollower = normalizedSprintOrder[0] ?? teamRiders[0]?.id ?? null
            setFollowAttackRiderId(defaultFollower)
            setIsRunning(false)
            void App.SetInteractivePaused(sessionId, true)
            appendEvents([`⏸ Paused: rival attack detected`])
            break
          }
        }
        appendEvents(payload.events)
      }
      setRiderOneShotQueued({})
    })

    return () => {
      if (off) {
        off()
      }
    }
  }, [race, interactiveMode, sessionId])

  useEffect(() => {
    const off = EventsOn('interactive_simulation_complete', async (payload: InteractiveCompleteEvent) => {
      if (!race) return
      if (!sessionId || payload.sessionId !== sessionId) return
      if (payload.raceId !== race.id) return

      try {
        const res = await App.GetRaceResults(race.id)
        setResults(res)
        setSimulationComplete(true)
      } catch (err) {
        console.error('Failed to load results:', err)
        appendEvents(['❌ Failed to load results'])
      }
    })

    return () => {
      if (off) off()
    }
  }, [race, sessionId])

  useEffect(() => {
    setListenerReady(true)
  }, [])

  useEffect(() => {
    return () => {
      if (telemetryRafRef.current !== null) {
        cancelAnimationFrame(telemetryRafRef.current)
      }
      Object.values(effortDebounceTimersRef.current).forEach((timer) => {
        window.clearTimeout(timer)
      })
      effortDebounceTimersRef.current = {}
    }
  }, [])

  useEffect(() => {
    telemetryDisplayRef.current = telemetryByRider
  }, [telemetryByRider])

  // Run actual simulation in background
  useEffect(() => {
    if (!listenerReady || simulationStarted.current || !race) return
    simulationStarted.current = true
    
    const runSim = async () => {
      try {
        if (interactiveMode) {
          const sess = await App.StartInteractiveSimulation(race.id, enableIncidents, controlledTeam)
          setSessionId(sess.sessionId)
          setControlledTeam(sess.controlledTeam)
          setTeamRiders(sess.teamRiders || [])
          const initialActions: Record<number, string> = {}
          for (const rider of (sess.teamRiders || [])) {
            initialActions[rider.id] = 'hold'
          }
          setRiderPersistentActions(initialActions)
          setRiderOneShotQueued({})
          const initialEfforts: Record<number, number> = {}
          for (const rider of (sess.teamRiders || [])) {
            initialEfforts[rider.id] = EFFORT_DEFAULT_PCT
          }
          setRiderEffortTargets(initialEfforts)
          const initialOrder = (sess.teamRiders || []).map((r) => r.id)
          setSprintTrainOrder(initialOrder)
          setFollowMap({})
          setTeamState({
            sprintTrainOrder: initialOrder,
            followMap: {},
            sprintTrainPhase: 'idle',
          })
          setRiderGroups({})
          setRouteProfile(null)
          setRouteKm(0)
          setTelemetryByRider({})
          setAttackAlert(null)
          setFollowAttackRiderId(null)
          seenAttackEventsRef.current = new Set()
          telemetryDisplayRef.current = {}
          telemetryPrevRef.current = {}
          telemetryTargetRef.current = {}
          telemetryHistoryRef.current = {}
          await App.SetInteractiveSpeed(sess.sessionId, speed)
          appendEvents([`🎮 Controlling: ${sess.controlledTeam}`])
          return
        }

        const res = await App.RunSimulation(race.id, enableIncidents)
        setResults(res)
        setSimulationComplete(true)
      } catch (err) {
        console.error('Simulation failed:', err)
        appendEvents(['❌ Simulation error'])
      }
    }
    runSim()
  }, [race, enableIncidents, listenerReady])

  // Jump to 100% when simulation completes
  useEffect(() => {
    if (simulationComplete && results) {
      setProgress(100)
      setIsRunning(false)
      // Show podium from final results
      const winner = results.results[0]
      const second = results.results[1]
      const third = results.results[2]
      if (winner) {
        setGroups([
          { name: 'Winner', gap: '🏆', riders: [winner.riderName] },
          { name: '2nd', gap: '+' + formatGap(second?.gap || 0), riders: [second?.riderName || ''] },
          { name: '3rd', gap: '+' + formatGap(third?.gap || 0), riders: [third?.riderName || ''] },
        ])
        appendEvents([`🏁 ${winner.riderName} wins!`])
      }
    }
  }, [simulationComplete, results])

  const formatGap = (seconds: number): string => {
    if (seconds === 0) return '—'
    const m = Math.floor(seconds / 60)
    const s = Math.floor(seconds % 60)
    if (m > 0) return `${m}:${s.toString().padStart(2, '0')}`
    return `${s}s`
  }

  const handleViewResults = () => {
    navigate(`/results/${raceId}`, { state: { results } })
  }

  const formatRaceClock = (seconds: number): string => {
    const safe = Math.max(0, Math.floor(seconds))
    const h = Math.floor(safe / 3600)
    const m = Math.floor((safe % 3600) / 60)
    const s = safe % 60
    return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
  }

  const telemetryForRider = (riderID: number): RiderTelemetryPoint | null => {
    return telemetryByRider[riderID] || null
  }

  const clampEffortPct = (value: number): number => Math.max(EFFORT_MIN_PCT, Math.min(EFFORT_MAX_PCT, Math.round(value)))

  const effortTargetForRider = (riderID: number, telemetry: RiderTelemetryPoint | null): number => {
    if (riderEffortTargets[riderID] !== undefined) return riderEffortTargets[riderID]
    if (telemetry) return clampEffortPct(telemetry.effortPct)
    return EFFORT_DEFAULT_PCT
  }

  const handleTogglePause = async () => {
    const nextRunning = !isRunning
    setIsRunning(nextRunning)
    if (interactiveMode && sessionId) {
      await App.SetInteractivePaused(sessionId, !nextRunning)
    }
  }

  const handleSpeedChange = async (nextSpeed: number) => {
    setSpeed(nextSpeed)
    if (interactiveMode && sessionId) {
      await App.SetInteractiveSpeed(sessionId, nextSpeed)
    }
  }

  const formatCommandLabel = (action: string): string => {
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

  const groupClassName = (groupName: string | undefined): string => {
    const g = (groupName || '').toLowerCase()
    if (g.includes('lead') || g.includes('winner')) return 'group-label lead'
    if (g.includes('chaser') || g.includes('chase')) return 'group-label chasers'
    if (g.includes('peloton')) return 'group-label peloton'
    if (g.includes('dropped')) return 'group-label dropped'
    if (g.includes('group')) return 'group-label group'
    return 'group-label unknown'
  }

  const buttonClass = (riderId: number, action: string, mode: 'persistent' | 'oneshot'): string => {
    const key = `${riderId}:${action}:${mode}`
    return acceptedButtons[key] ? 'secondary accepted' : 'secondary'
  }

  const groupSortRank = (groupName: string | undefined): number => {
    const group = (groupName || '').toLowerCase()
    if (group.includes('lead') || group.includes('winner')) return 0
    if (group.includes('chaser') || group.includes('chase')) return 1
    if (group.includes('peloton')) return 2
    if (group.includes('group')) return 3
    if (group.includes('dropped')) return 4
    return 5
  }

  const groupBucket = (groupName: string | undefined): string => {
    const group = (groupName || '').toLowerCase()
    if (group.includes('lead') || group.includes('winner')) return 'Lead'
    if (group.includes('chaser') || group.includes('chase')) return 'Chasers'
    if (group.includes('peloton')) return 'Peloton'
    if (group.includes('group')) return groupName || 'Group'
    if (group.includes('dropped')) return 'Dropped'
    return 'Unknown'
  }

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
      const rawGroup = riderGroups[rider.id]
      const label = groupBucket(rawGroup)
      const key = label
      if (indexByKey[key] === undefined) {
        indexByKey[key] = sections.length
        sections.push({ key, label, riders: [] })
      }
      sections[indexByKey[key]].riders.push(rider)
    }

    return sections
  }, [sortedTeamRiders, riderGroups])

  const teamRiderByID = useMemo(() => {
    const out: Record<number, main.RiderInfo> = {}
    for (const rider of teamRiders) {
      out[rider.id] = rider
    }
    return out
  }, [teamRiders])

  const normalizedSprintOrder = useMemo(() => {
    const seen = new Set<number>()
    const next: number[] = []
    for (const riderID of sprintTrainOrder) {
      if (!teamRiderByID[riderID] || seen.has(riderID)) continue
      seen.add(riderID)
      next.push(riderID)
    }
    for (const rider of teamRiders) {
      if (!seen.has(rider.id)) {
        seen.add(rider.id)
        next.push(rider.id)
      }
    }
    return next
  }, [sprintTrainOrder, teamRiders, teamRiderByID])

  const isRiderLocked = (riderId: number): boolean => riderCommandLocked[riderId] === true

  const sendCommand = async (riderId: number, action: string, mode: 'persistent' | 'oneshot') => {
    if (!interactiveMode || !sessionId) return
    if (isRiderLocked(riderId)) return

    setRiderCommandLocked((prev) => ({ ...prev, [riderId]: true }))
    const key = `${riderId}:${action}:${mode}`

    try {
      await App.SetRiderCommand(sessionId, riderId, action, mode)
      setAcceptedButtons((prev) => ({ ...prev, [key]: true }))
      window.setTimeout(() => {
        setAcceptedButtons((prev) => {
          const next = { ...prev }
          delete next[key]
          return next
        })
      }, ACCEPT_PULSE_MS)

      const riderName = teamRiders.find((r) => r.id === riderId)?.name ?? `Rider ${riderId}`
      const label = mode === 'oneshot' ? 'Attack now' : formatCommandLabel(action)
      appendEvents([`🎮 ${riderName}: ${label}`])

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

  const handleTacticChange = (riderId: number, next: string) => {
    void sendCommand(riderId, next, 'persistent')
  }

  const sendEffort = async (riderID: number, effortPct: number) => {
    if (!interactiveMode || !sessionId) return
    try {
      await App.SetRiderEffort(sessionId, riderID, effortPct)
    } catch (err) {
      console.error('Failed to set rider effort', err)
    }
  }

  const handleEffortChange = (riderID: number, nextEffort: number) => {
    const clamped = clampEffortPct(nextEffort)
    setRiderEffortTargets((prev) => ({ ...prev, [riderID]: clamped }))

    const existing = effortDebounceTimersRef.current[riderID]
    if (existing !== undefined) {
      window.clearTimeout(existing)
    }
    effortDebounceTimersRef.current[riderID] = window.setTimeout(() => {
      delete effortDebounceTimersRef.current[riderID]
      void sendEffort(riderID, clamped)
    }, EFFORT_DEBOUNCE_MS)
  }

  const profileMarkerKm = Math.max(0, Math.min(routeProfile?.totalKm || 0, routeKm))
  const profileMarkerPct = routeProfile && routeProfile.totalKm > 0
    ? clamp01(profileMarkerKm / routeProfile.totalKm) * 100
    : 0
  const routePath = routeProfile ? profilePath(routeProfile.points || []) : ''

  const sprintPhaseLabel = teamState?.sprintTrainPhase || 'idle'

  const sendSprintTrainOrder = async (nextOrder: number[]) => {
    if (!interactiveMode || !sessionId) return
    try {
      await App.SetSprintTrainOrder(sessionId, nextOrder)
    } catch (err) {
      console.error('Failed to set sprint train order', err)
    }
  }

  const moveSprintTrainRider = (riderID: number, direction: -1 | 1) => {
    const order = [...normalizedSprintOrder]
    const idx = order.indexOf(riderID)
    if (idx < 0) return
    const target = idx + direction
    if (target < 0 || target >= order.length) return
    const swap = order[target]
    order[target] = order[idx]
    order[idx] = swap
    setSprintTrainOrder(order)
    void sendSprintTrainOrder(order)
  }

  const handleFollowChange = async (riderID: number, followRiderID: number | null) => {
    if (!interactiveMode || !sessionId) return
    const safeFollow = followRiderID === riderID ? null : followRiderID
    setFollowMap((prev) => ({ ...prev, [riderID]: safeFollow }))
    try {
      await App.SetRiderFollow(sessionId, riderID, safeFollow ?? 0)
    } catch (err) {
      console.error('Failed to set rider follow target', err)
    }
  }

  const handleLaunchSprint = async (riderID: number) => {
    if (!interactiveMode || !sessionId) return
    try {
      await App.LaunchSprint(sessionId, riderID)
      appendEvents([`🚀 ${teamRiderByID[riderID]?.name || `Rider ${riderID}`}: Sprint launch`])
    } catch (err) {
      console.error('Failed to launch sprint', err)
    }
  }

  const handleFollowAttack = async () => {
    if (!interactiveMode || !sessionId) return
    if (!followAttackRiderId) return
    try {
      await App.SetRiderCommand(sessionId, followAttackRiderId, 'chase', 'persistent')
      appendEvents([`🎯 ${teamRiderByID[followAttackRiderId]?.name || `Rider ${followAttackRiderId}`}: Follow attack (Chase)`])
    } finally {
      setAttackAlert(null)
      setIsRunning(true)
      await App.SetInteractivePaused(sessionId, false)
    }
  }

  return (
    <div className="simulation page-enter">
      <header className="sim-header">
        <h1>{raceName}</h1>
        <div className="sim-controls">
          <button onClick={handleTogglePause} disabled={simulationComplete || (!interactiveMode && liveProgress)}>
            {isRunning ? '⏸ Pause' : '▶ Play'}
          </button>
          <select value={speed} onChange={(e) => handleSpeedChange(Number(e.target.value))} disabled={simulationComplete || (!interactiveMode && liveProgress)}>
            <option value={0.25}>0.25x</option>
            <option value={0.5}>0.5x</option>
            <option value={1}>1x</option>
            <option value={2}>2x</option>
            <option value={4}>4x</option>
            <option value={8}>8x</option>
          </select>
        </div>
      </header>

      <section className="card course-progress">
        <h2>Race Progress</h2>
        <div className="progress-bar">
          <div className="progress-fill" style={{ width: `${progress}%` }}></div>
          <div className="progress-marker" style={{ left: `${progress}%` }}>🚴</div>
        </div>
        <div className="progress-info">
          <span>{Math.round(progress * raceDistance / 100)} km</span>
          <span>{formatRaceClock(elapsedTime)}</span>
          <span>{Math.round(progress)}%</span>
          <span>{raceDistance} km</span>
        </div>
      </section>

      <section className="card route-profile-card">
        <h2>Route Profile</h2>
        {!routeProfile || routeProfile.points.length === 0 ? (
          <div className="empty-state">Profile loading…</div>
        ) : (
          <div className="route-profile-wrap">
            <svg className="route-profile-svg" viewBox="0 0 100 28" preserveAspectRatio="none" aria-label="Route profile">
              <path d={routePath} className="route-profile-line" />
            </svg>
            <div className="route-profile-marker" style={{ left: `${profileMarkerPct}%` }} />
            <div className="route-profile-info">
              <span>{profileMarkerKm.toFixed(1)} km</span>
              <span>{routeProfile.totalKm.toFixed(1)} km</span>
            </div>
          </div>
        )}
      </section>

      <div className="sim-grid">
        <section className="card groups-panel">
          <h2>Race Situation</h2>
          {groups.length === 0 ? (
            <div className="empty-state">Simulation running. Results will appear here when finished.</div>
          ) : (
            groups.map((group, i) => (
              <div key={i} className={`group ${group.name.toLowerCase().replace(/\s+/g, '-')}`}>
                <div className="group-header">
                  <span className="group-name">{group.name}</span>
                  <span className="group-gap">{group.gap}</span>
                </div>
                <div className="group-riders">
                  {group.riders.join(', ')}
                </div>
              </div>
            ))
          )}
        </section>

        <section className="card events-panel">
          <h2>Event Log</h2>
          <ul className="events-list">
            {events.map((event, i) => (
              <li key={i}>{event}</li>
            ))}
          </ul>
        </section>

        {interactiveMode && (
          <section className="card team-controls">
            <h2>Team Control</h2>
            {!sessionId ? (
              <div className="empty-state">Starting interactive session...</div>
            ) : teamRiders.length === 0 ? (
              <div className="empty-state">No riders found for {controlledTeam}.</div>
            ) : (
              <>
              <div className="sprint-train-panel">
                <div className="sprint-train-header">
                  <span>Sprint Train</span>
                  <span className={`sprint-phase ${sprintPhaseLabel}`}>{sprintPhaseLabel.toUpperCase()}</span>
                </div>
                <div className="attack-pause-toggle">
                  <label>
                    <input
                      type="checkbox"
                      checked={pauseOnRivalAttacks}
                      onChange={(e) => setPauseOnRivalAttacks(e.target.checked)}
                      disabled={!sessionId}
                    />
                    Pause on rival attacks
                  </label>
                </div>
                {attackAlert && (
                  <div className="attack-alert">
                    <div>
                      ⚠️ Rival attack{attackAlert.team ? ` (${attackAlert.team})` : ''}
                    </div>
                    <div className="attack-alert-actions">
                      <select
                        value={followAttackRiderId ?? ''}
                        onChange={(e) => setFollowAttackRiderId(e.target.value === '' ? null : Number(e.target.value))}
                        disabled={!sessionId}
                      >
                        <option value="">Select rider…</option>
                        {normalizedSprintOrder.map((id) => (
                          <option key={id} value={id}>{teamRiderByID[id]?.name || `Rider ${id}`}</option>
                        ))}
                      </select>
                      <button className="secondary" onClick={handleFollowAttack} disabled={!sessionId || !followAttackRiderId}>Follow attack</button>
                    </div>
                  </div>
                )}
                <div className="sprint-train-order">
                  {normalizedSprintOrder.map((riderID, idx) => (
                    <div key={riderID} className="sprint-train-order-row">
                      <span>{idx + 1}. {teamRiderByID[riderID]?.name || `Rider ${riderID}`}</span>
                      <div>
                        <button className="secondary" onClick={() => moveSprintTrainRider(riderID, -1)} disabled={idx === 0}>↑</button>
                        <button className="secondary" onClick={() => moveSprintTrainRider(riderID, 1)} disabled={idx === normalizedSprintOrder.length - 1}>↓</button>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
              <div className="team-roster">
                {groupedTeamRiders.map((section) => (
                  <div key={section.key} className="team-group-section">
                    <div className={`team-group-header ${groupClassName(section.label)}`}>{section.label}</div>
                    {section.riders.map((r) => (
                      <div key={r.id} className="team-rider">
                        {(() => {
                          const telemetry = telemetryForRider(r.id)
                          return (
                            <>
                        <div className="team-rider-row">
                          <div className="team-rider-main">
                            <div className="team-rider-name">{r.name}</div>
                            <div className="team-rider-meta">
                              <span className={groupClassName(riderGroups[r.id])}>{riderGroups[r.id] || 'Unknown'}</span>
                              <span>{telemetry ? `${Math.round(telemetry.speedKph)}kph` : '—'}</span>
                              <span>{telemetry ? `${Math.round(telemetry.powerWatts)}W` : '—'}</span>
                              <span>{telemetry ? `${Math.round(telemetry.effortPct)}%` : '—'}</span>
                              <span>{formatCommandLabel(riderPersistentActions[r.id] || 'hold')}</span>
                              {riderOneShotQueued[r.id] && <span>Attack queued</span>}
                              {isRiderLocked(r.id) && <span>Cooldown…</span>}
                            </div>
                          </div>

                          <div className="team-rider-controls">
                            <div className="team-rider-effort compact">
                              <label htmlFor={`effort-${r.id}`}>Effort</label>
                              <input
                                id={`effort-${r.id}`}
                                type="range"
                                min={EFFORT_MIN_PCT}
                                max={EFFORT_MAX_PCT}
                                step={1}
                                value={effortTargetForRider(r.id, telemetry)}
                                onChange={(e) => handleEffortChange(r.id, Number(e.target.value))}
                                disabled={!sessionId}
                              />
                              <span>{effortTargetForRider(r.id, telemetry)}%</span>
                            </div>

                            <div className="team-rider-actions compact">
                              <select
                                value={riderPersistentActions[r.id] || 'hold'}
                                onChange={(e) => handleTacticChange(r.id, e.target.value)}
                                disabled={isRiderLocked(r.id)}
                              >
                                <option value="hold">AI (Hold)</option>
                                <option value="conserve">Conserve</option>
                                <option value="chase">Chase</option>
                                <option value="attack">Attack</option>
                              </select>
                              <button className={buttonClass(r.id, 'attack', 'oneshot')} onClick={() => sendCommand(r.id, 'attack', 'oneshot')} disabled={isRiderLocked(r.id)}>Attack now</button>
                              <select
                                value={followMap[r.id] ?? ''}
                                onChange={(e) => handleFollowChange(r.id, e.target.value === '' ? null : Number(e.target.value))}
                                disabled={!sessionId}
                              >
                                <option value="">Follow: None</option>
                                {normalizedSprintOrder
                                  .filter((id) => id !== r.id)
                                  .map((id) => (
                                    <option key={id} value={id}>Follow: {teamRiderByID[id]?.name || `Rider ${id}`}</option>
                                  ))}
                              </select>
                              <button className="secondary" onClick={() => handleLaunchSprint(r.id)} disabled={!sessionId}>Launch</button>
                            </div>
                          </div>
                        </div>
                            </>
                          )
                        })()}
                      </div>
                    ))}
                  </div>
                ))}
              </div>
              </>
            )}
          </section>
        )}
      </div>

      {simulationComplete && results && (
        <div className="race-finished">
          <button className="primary" onClick={handleViewResults}>View Results →</button>
        </div>
      )}
    </div>
  )
}

export default Simulation
