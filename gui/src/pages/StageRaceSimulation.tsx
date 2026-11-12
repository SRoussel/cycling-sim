import { useParams, useNavigate, Link, useLocation } from 'react-router-dom'
import { useEffect, useRef, useState } from 'react'
import * as App from '../../wailsjs/go/main/App'
import { main } from '../../wailsjs/go/models'
import { EventsOn } from '../../wailsjs/runtime/runtime'
import { RaceGroup, formatRaceClock, toDisplayGroups } from '../sim/shared'
import { LiveProgressPayload, RaceGoal, useTeamControl } from '../sim/useTeamControl'
import { RaceDashboard } from '../components/race/RaceDashboard'
import { RaceTopBar } from '../components/race/RaceTopBar'
import './Simulation.css'
import './RaceSetup.css'

interface StageRaceProgressEvent extends LiveProgressPayload {
  sessionId?: string
  stageRaceId: number
  stageNumber: number
  stageCount: number
  stageName: string
  stageKm: number
  stageKmTotal: number
  stageElapsed?: number
  overallKm: number
  overallKmTotal: number
  overallElapsed?: number
  progressStage: number
  progressOverall: number
  groups: Array<{ name: string; gap: number; riders: string[] }>
}

interface InteractiveStageRaceCompleteEvent {
  sessionId: string
  stageRaceId: number
  goal?: RaceGoal
}

function StageRaceSimulation() {
  const { stageRaceId } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const interactiveMode = location.state?.interactiveMode as boolean ?? false
  const initialTeam = location.state?.controlledTeam as string ?? ''

  const tc = useTeamControl({ interactiveMode, initialTeam, stageRace: true })
  const { appendEvents, sessionId } = tc

  const [stageRace, setStageRace] = useState<main.StageRaceInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [failed, setFailed] = useState(false)
  const [progress, setProgress] = useState(0)
  const [stageProgress, setStageProgress] = useState(0)
  const [stageElapsed, setStageElapsed] = useState(0)
  const [overallElapsed, setOverallElapsed] = useState(0)
  const [currentStageName, setCurrentStageName] = useState<string | null>(null)
  const [currentStageNumber, setCurrentStageNumber] = useState<number | null>(null)
  const [groups, setGroups] = useState<RaceGroup[]>([])
  const simulationStarted = useRef(false)
  const lastStageName = useRef<string | null>(null)

  useEffect(() => {
    App.GetStageRaces()
      .then((all) => {
        const sr = all.find((s: main.StageRaceInfo) => s.id === parseInt(stageRaceId || '0'))
        if (sr) setStageRace(sr)
      })
      .finally(() => setLoading(false))
  }, [stageRaceId])

  const ingestRef = useRef(tc.ingestProgress)
  ingestRef.current = tc.ingestProgress

  useEffect(() => {
    const off = EventsOn('stage_race_progress', (payload: StageRaceProgressEvent) => {
      if (!stageRace || payload.stageRaceId !== stageRace.id) return
      if (tc.isForeignSession(payload.sessionId)) return
      setProgress(Math.min(100, payload.progressOverall))
      setStageProgress(Math.min(100, payload.progressStage))
      tc.setRouteKm(Math.max(0, payload.stageKm || 0))
      setStageElapsed(Math.max(0, payload.stageElapsed || 0))
      setOverallElapsed(Math.max(0, payload.overallElapsed || 0))
      setCurrentStageName(payload.stageName)
      setCurrentStageNumber(payload.stageNumber)
      const nextGroups = toDisplayGroups(payload.groups)
      if (nextGroups.length > 0) setGroups(nextGroups)
      if (payload.stageName && payload.stageName !== lastStageName.current) {
        lastStageName.current = payload.stageName
        appendEvents([`Stage ${payload.stageNumber}: ${payload.stageName}`])
      }
      ingestRef.current(payload)
    })
    return () => off?.()
  }, [stageRace, interactiveMode, sessionId])

  useEffect(() => {
    const off = EventsOn('interactive_stage_race_complete', (payload: InteractiveStageRaceCompleteEvent) => {
      if (!stageRace || !sessionId || payload.sessionId !== sessionId || payload.stageRaceId !== stageRace.id) return
      navigate(`/stage-race-gc/${stageRaceId}`, { state: { gcGoal: payload.goal ?? null, team: tc.controlledTeam } })
    })
    return () => off?.()
  }, [stageRace, sessionId, navigate, stageRaceId, tc.controlledTeam])

  useEffect(() => {
    if (!stageRace || simulationStarted.current) return
    simulationStarted.current = true
    const run = async () => {
      try {
        if (interactiveMode) {
          await tc.beginSession(await App.StartInteractiveStageRace(stageRace.id, initialTeam))
          return
        }
        await App.RunStageRace(stageRace.id)
        navigate(`/stage-race-gc/${stageRaceId}`)
      } catch (err) {
        console.error('Stage race failed:', err)
        setFailed(true)
        appendEvents(['❌ Stage race failed'])
      }
    }
    run()
  }, [stageRace, navigate, stageRaceId])

  if (loading || !stageRace) {
    return (
      <div className="race-setup">
        <div className="loading">Loading stage race...</div>
      </div>
    )
  }

  const stageTotalKm = tc.routeProfile?.totalKm
    || (stageProgress > 0 ? (tc.routeKm * 100) / stageProgress : 0)
  const stageKmRemaining = Math.max(0, stageTotalKm - tc.routeKm)

  if (failed) {
    return (
      <div className="race-setup">
        <section className="card">
          <h2>Stage Race Halted</h2>
          <p className="empty-state">The stage race stopped. Check logs for details.</p>
          <Link to={`/stage-race/${stageRaceId}`}>
            <button className="primary">← Back to Setup</button>
          </Link>
        </section>
      </div>
    )
  }

  return (
    <RaceDashboard
      tc={tc}
      groups={groups}
      interactive={interactiveMode}
      kmRemaining={stageKmRemaining}
      emptyText="Stage is running. Live groups will appear here."
      top={
        <RaceTopBar
          title={stageRace.name}
          subtitle={currentStageNumber ? `Stage ${currentStageNumber}${currentStageName ? ` · ${currentStageName}` : ''}` : 'Preparing stage…'}
          kmRemaining={stageKmRemaining}
          kmCovered={tc.routeKm}
          kmTotal={stageTotalKm}
          elapsed={stageElapsed}
          profile={tc.routeProfile}
          paused={tc.paused}
          speed={tc.speed}
          playbackDisabled={!interactiveMode || !sessionId}
          onTogglePause={tc.togglePause}
          onSpeedChange={tc.changeSpeed}
          goal={[tc.gcGoal?.text, tc.goal ? `Today: ${tc.goal.text}` : ''].filter(Boolean).join(' · ') || undefined}
          onSkip={interactiveMode ? tc.skipToDecision : undefined}
          skipping={tc.skipping}
          extra={
            <div className="dash-stat" title={`Overall ${formatRaceClock(overallElapsed)}`}>
              <span className="dash-stat-value">{Math.round(progress)}%</span>
              <span className="dash-stat-label">of {stageRace.stageCount} stages</span>
            </div>
          }
        />
      }
    />
  )
}

export default StageRaceSimulation
