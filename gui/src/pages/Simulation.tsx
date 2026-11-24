import { useParams, useLocation, useNavigate } from 'react-router-dom'
import { useState, useEffect, useRef } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { RaceGroup, formatGap, toDisplayGroups } from '../sim/shared'
import { LiveProgressPayload, RaceGoal, useTeamControl } from '../sim/useTeamControl'
import { Debrief } from '../components/race/Debrief'
import { RaceDashboard } from '../components/race/RaceDashboard'
import { RaceTopBar } from '../components/race/RaceTopBar'
import { GapGraphic, groupMarks } from '../components/race/RoadView'
import './Simulation.css'

interface SimulationProgressEvent extends LiveProgressPayload {
  sessionId?: string
  raceId: number
  kmCovered: number
  kmTotal: number
  elapsedTime?: number
  progress: number
  segmentName: string
  groups: Array<{ name: string; gap: number; riders: string[] }>
}

interface InteractiveCompleteEvent {
  sessionId: string
  raceId: number
  goal?: RaceGoal
  breakMade?: boolean
}

function Simulation() {
  const { raceId } = useParams()
  const location = useLocation()
  const navigate = useNavigate()

  const race = location.state?.race as main.RaceInfo | undefined
  const enableIncidents = location.state?.enableIncidents as boolean ?? false
  const interactiveMode = location.state?.interactiveMode as boolean ?? false
  const initialTeam = location.state?.controlledTeam as string ?? ''
  // A season race: started through the season and recorded into it.
  const season = location.state?.season as { seasonId: number; eventId: number } | undefined
  const seasonPage = season ? `/season/${season.seasonId}` : ''
  const raceName = race?.name || raceId?.replace(/-/g, ' ') || 'Race'
  const raceDistance = race?.distance || 257

  const tc = useTeamControl({ interactiveMode, initialTeam, initialEvents: [`Simulation started: ${raceName}`] })
  const { appendEvents, sessionId, paused } = tc

  const [progress, setProgress] = useState(0)
  const [elapsedTime, setElapsedTime] = useState(0)
  const [simulationComplete, setSimulationComplete] = useState(false)
  const [results, setResults] = useState<main.RaceResults | null>(null)
  const [breakMade, setBreakMade] = useState(false)
  const [liveProgress, setLiveProgress] = useState(false)
  const [groups, setGroups] = useState<RaceGroup[]>([])
  const [currentSegmentName, setCurrentSegmentName] = useState('')
  const [kmTotal, setKmTotal] = useState(0)
  const simulationStarted = useRef(false)
  const lastSegmentName = useRef<string | null>(null)

  // Animate progress while simulation runs (fallback when live updates are unavailable)
  useEffect(() => {
    if (liveProgress || paused || simulationComplete) return
    const interval = setInterval(() => {
      setProgress((p) => (p >= 95 ? p : p + 0.3 * tc.speed))
    }, 100)
    return () => clearInterval(interval)
  }, [paused, tc.speed, simulationComplete, liveProgress])

  // The listener is re-subscribed per session; use a ref so it always calls
  // the current ingestProgress.
  const ingestRef = useRef(tc.ingestProgress)
  ingestRef.current = tc.ingestProgress

  useEffect(() => {
    const off = EventsOn('simulation_progress', (payload: SimulationProgressEvent) => {
      if (!race || payload.raceId !== race.id) return
      if (tc.isForeignSession(payload.sessionId)) return
      setLiveProgress(true)
      setProgress(Math.min(100, payload.progress))
      tc.setRouteKm(Math.max(0, payload.kmCovered || 0))
      if (payload.kmTotal > 0) setKmTotal(payload.kmTotal)
      setElapsedTime(Math.max(0, payload.elapsedTime || 0))
      const nextGroups = toDisplayGroups(payload.groups)
      if (nextGroups.length > 0) setGroups(nextGroups)
      if (payload.segmentName && payload.segmentName !== lastSegmentName.current) {
        lastSegmentName.current = payload.segmentName
        setCurrentSegmentName(payload.segmentName)
      }
      ingestRef.current(payload)
    })
    return () => off?.()
  }, [race, interactiveMode, sessionId])

  useEffect(() => {
    const off = EventsOn('interactive_simulation_complete', async (payload: InteractiveCompleteEvent) => {
      if (!race || !sessionId || payload.sessionId !== sessionId || payload.raceId !== race.id) return
      try {
        setBreakMade(!!payload.breakMade)
        setResults(season ? await App.GetSeasonRaceResults(season.seasonId, season.eventId) : await App.GetRaceResults(race.id))
        setSimulationComplete(true)
      } catch (err) {
        console.error('Failed to load results:', err)
        appendEvents(['❌ Failed to load results'])
      }
    })
    return () => off?.()
  }, [race, sessionId])

  // Start the simulation once.
  useEffect(() => {
    if (simulationStarted.current || !race) return
    simulationStarted.current = true
    const run = async () => {
      try {
        if (interactiveMode) {
          const plan = location.state?.plan ?? { leaderId: 0, break: 'auto', hardOn: [], attackOn: '' }
          const sess = season
            ? await App.StartSeasonRace(season.seasonId, season.eventId, main.RacePlan.createFrom(plan))
            : await App.StartInteractiveSimulation(race.id, enableIncidents, initialTeam, main.RacePlan.createFrom(plan))
          await tc.beginSession(sess)
          return
        }
        setResults(await App.RunSimulation(race.id, enableIncidents))
        setSimulationComplete(true)
      } catch (err) {
        console.error('Simulation failed:', err)
        appendEvents(['❌ Simulation error'])
      }
    }
    run()
  }, [race, enableIncidents])

  // Jump to 100% and show the podium when the simulation completes.
  useEffect(() => {
    if (!simulationComplete || !results) return
    if (tc.skipping) void tc.skipToDecision() // stop fast-forwarding: nothing left to skip to
    setProgress(100)
    tc.setPaused(true)
    const [winner, second, third] = results.results
    if (winner) {
      setGroups([
        { name: 'Winner', gap: '🏆', riders: [winner.riderName] },
        { name: '2nd', gap: '+' + formatGap(second?.gap || 0), riders: [second?.riderName || ''] },
        { name: '3rd', gap: '+' + formatGap(third?.gap || 0), riders: [third?.riderName || ''] },
      ])
      appendEvents([`🏁 ${winner.riderName} wins!`])
    }
  }, [simulationComplete, results])

  const effectiveTotalKm = kmTotal > 0 ? kmTotal : (tc.routeProfile?.totalKm || raceDistance)
  const kmRemaining = Math.max(0, effectiveTotalKm - tc.routeKm)
  const controlsDisabled = simulationComplete || (!interactiveMode && liveProgress)

  return (
    <RaceDashboard
      tc={tc}
      groups={groups}
      interactive={interactiveMode}
      kmRemaining={kmRemaining}
      emptyText="Simulation running. Results will appear here when finished."
      banner={interactiveMode && simulationComplete && results ? (
        <Debrief
          goal={tc.goal}
          breakMade={breakMade}
          results={results}
          team={tc.controlledTeam}
          decisions={tc.calls}
          onResults={() => (season ? navigate(seasonPage, { state: { justRaced: season.eventId } }) : navigate(`/results/${raceId}`, { state: { results } }))}
          resultsLabel={season ? 'Back to the season →' : undefined}
        />
      ) : undefined}
      top={
        <RaceTopBar
          title={raceName}
          subtitle={interactiveMode ? undefined : currentSegmentName || undefined}
          kmRemaining={kmRemaining}
          kmCovered={Math.min(effectiveTotalKm, tc.routeKm || (progress / 100) * effectiveTotalKm)}
          kmTotal={effectiveTotalKm}
          elapsed={elapsedTime}
          profile={tc.routeProfile}
          paused={paused}
          speed={tc.speed}
          playbackDisabled={controlsDisabled}
          onTogglePause={tc.togglePause}
          onSpeedChange={tc.changeSpeed}
          onSkip={interactiveMode ? tc.skipToDecision : undefined}
          decisionLevel={interactiveMode ? tc.decisionLevel : undefined}
          marks={groupMarks(groups, tc)}
          groups={groups}
          road={groups.length > 0 ? <GapGraphic groups={groups} tc={interactiveMode ? tc : undefined} large /> : undefined}
          onDecisionLevel={tc.setDecisionLevel}
          goal={tc.goal?.text}
          skipping={tc.skipping}
          extra={simulationComplete && results && !interactiveMode ? (
            <button className="primary" onClick={() => navigate(`/results/${raceId}`, { state: { results } })}>
              View Results →
            </button>
          ) : undefined}
        />
      }
    />
  )
}

export default Simulation
