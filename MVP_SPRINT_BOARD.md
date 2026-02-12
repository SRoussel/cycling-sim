# MVP Sprint Board: Realtime Interactive Racing

## Sprint Goal
Ship a playable realtime-feel MVP for interactive racing with:
- continuous timeline feel,
- per-rider telemetry visibility,
- controllable rider effort,
- basic sprint-train behavior,
- route profile visualization.

## Duration
- 1 week (5 engineering days + 1 polish/test day)

## Owners (Roles)
- SIM: Simulation Engineer
- BE: Backend/Wails Engineer
- FE: Frontend Engineer
- QA: QA Engineer
- PM: Product/Design Owner

## Ticket Board

| ID | Title | Owner | Estimate | Priority | Depends On | Status |
|---|---|---|---:|---|---|---|
| RT-001 | Telemetry contract and event schemas | BE + FE | 0.5d | P0 | None | Done |
| RT-002 | Emit per-rider telemetry from simulation loop | SIM + BE | 1.0d | P0 | RT-001 | Done |
| RT-003 | Frontend telemetry store + interpolation clock | FE | 1.0d | P0 | RT-001 | Done |
| RT-004 | Rider telemetry cards (speed/power/fatigue/effort/group) | FE | 0.75d | P0 | RT-003 | Done |
| RT-005 | Individual rider effort command API + backend state | BE + SIM | 1.0d | P0 | RT-001 | Done |
| RT-006 | Effort slider UI + command UX safeguards | FE | 0.75d | P0 | RT-005 | Done |
| RT-007 | Sprint train follow-chain + order backend logic | SIM | 1.0d | P1 | RT-002 | Done |
| RT-008 | Sprint train order UI + follow/launch controls | FE | 0.5d | P1 | RT-007 | Done |
| RT-009 | Live elevation/route profile strip with race marker | FE | 1.0d | P1 | RT-003 | Done |
| RT-010 | Perf, tuning, regression, acceptance signoff | QA + FE + SIM | 1.0d | P0 | RT-004, RT-006, RT-008, RT-009 | In Progress |

---

## Ticket Details

### RT-001 — Telemetry contract and event schemas
**Owner:** BE + FE  
**Estimate:** 0.5 day  
**Outcome:** stable event payload contract shared by backend and frontend.

#### Existing events (already in place)
- simulation_progress
  - sessionId?: string
  - raceId: number
  - kmCovered: number
  - kmTotal: number
  - elapsedTime: number
  - progress: number
  - segmentName: string
  - riderGroups?: Record<number, string>
  - groups: Array<{name: string; gap: number; riders: string[]}>
  - events: string[]

- stage_race_progress
  - sessionId?: string
  - stageRaceId: number
  - stageNumber: number
  - stageCount: number
  - stageName: string
  - stageKm: number
  - stageKmTotal: number
  - stageElapsed: number
  - overallKm: number
  - overallKmTotal: number
  - overallElapsed: number
  - progressStage: number
  - progressOverall: number
  - riderGroups?: Record<number, string>
  - groups: Array<{name: string; gap: number; riders: string[]}>
  - events: string[]

#### New fields for MVP (to add)
Add to both progress events:
- riderTelemetry?: Record<number, {
  speedKph: number
  powerWatts: number
  effortPct: number
  fatiguePct: number
  wPrimePct: number
  routeKm: number
  routePct: number
}>
- profile?: {
  totalKm: number
  points: Array<{km: number; elev: number}>
}

---

### RT-002 — Emit per-rider telemetry from simulation loop
**Owner:** SIM + BE  
**Estimate:** 1.0 day  
**Depends:** RT-001

#### Scope
- Compute telemetry per rider at each emitted progress frame.
- Use lightweight/derived values where direct values are unavailable.

#### Exact backend additions
In simulation progress snapshots (internal structs):
- RiderTelemetry map[int64]RiderTelemetryPoint
- RouteProfile *RouteProfile (emitted once, then omitted or cached in FE)

New internal types:
- RiderTelemetryPoint
  - SpeedKph float64
  - PowerWatts float64
  - EffortPct float64
  - FatiguePct float64
  - WPrimePct float64
  - RouteKm float64
  - RoutePct float64
- RouteProfile
  - TotalKm float64
  - Points []ProfilePoint
- ProfilePoint
  - Km float64
  - Elev float64

#### Definition of done
- Telemetry map is populated for all active riders at each progress event.
- No panics when riders DNF.

---

### RT-003 — Frontend telemetry store + interpolation clock
**Owner:** FE  
**Estimate:** 1.0 day  
**Depends:** RT-001

#### Scope
- Normalized map store keyed by riderId.
- Frame interpolation (requestAnimationFrame) between backend events.
- Hard cap history buffers to prevent growth.

#### State model
- telemetryByRider: Record<number, RiderTelemetryPoint>
- lastEventTs: number
- interpAlpha: number

#### Definition of done
- Smooth updates at visual refresh rate even when event cadence is lower.
- No visible stutter from event bursts.

---

### RT-004 — Rider telemetry cards
**Owner:** FE  
**Estimate:** 0.75 day  
**Depends:** RT-003

#### Scope
For each controlled rider card, display:
- Group
- Speed (kph)
- Power (W)
- Effort (%)
- Fatigue bar (%)
- W′ bar (%)

#### Definition of done
- Values update live during race.
- Bars remain stable and readable during long runs.

---

### RT-005 — Individual rider effort command API + backend state
**Owner:** BE + SIM  
**Estimate:** 1.0 day  
**Depends:** RT-001

#### New command API
Wails method:
- SetRiderEffort(sessionId: string, riderId: number, effortPct: number): Promise<void>

Rules:
- Clamp effortPct in [60, 130]
- Persist until changed or reset
- Integrate with existing action override logic

#### Backend model additions
In interactive session state:
- effort map[int64]float64

In simulation decision/power path:
- apply effort multiplier to target power before fatigue constraints

#### Definition of done
- Rider effort change is visible in telemetry within next frames.

---

### RT-006 — Effort slider UI + command UX safeguards
**Owner:** FE  
**Estimate:** 0.75 day  
**Depends:** RT-005

#### Scope
- Add per-rider effort slider (60% to 130%).
- Show numeric effort value.
- Debounce sends (e.g., 120ms).
- Keep existing cooldown/pulse behavior for discrete commands.

#### Definition of done
- Slider feels responsive and does not spam backend.
- Effort value and telemetry move coherently.

---

### RT-007 — Sprint train follow-chain + order backend logic
**Owner:** SIM  
**Estimate:** 1.0 day  
**Depends:** RT-002

#### Scope
- Add sprint train ordering to interactive session state (ordered rider IDs).
- Add per-rider follow assignment so riders can follow a designated wheel.
- In final-km window (default last 8km):
  - riders with follow targets prioritize staying on their leader's wheel,
  - lead rider sets pace,
  - designated sprinter conserves until explicit launch command.

#### API
- SetSprintTrainOrder(sessionId: string, riderIds: number[]): Promise<void>
- SetRiderFollow(sessionId: string, riderId: number, followRiderId?: number): Promise<void>
- LaunchSprint(sessionId: string, riderId: number): Promise<void>

#### Definition of done
- Distinct follow-chain behavior visible in final-km scenarios.
- Launch command causes selected rider to switch from follow/conserve to sprint effort.

---

### RT-008 — Sprint train order UI + follow/launch controls
**Owner:** FE  
**Estimate:** 0.5 day  
**Depends:** RT-007

#### Scope
- Add sprint train order editor in Team Control panel (drag/reorder or move up/down).
- Add per-rider follow control (select leader rider or none).
- Add explicit Launch button for selected sprinter.
- Status chip states:
  - IDLE
  - FOLLOWING
  - LAUNCH

#### Event field extension
- teamState?: {
  sprintTrainOrder: number[]
  followMap: Record<number, number | null>
  sprintTrainPhase: "idle" | "following" | "launch"
  launchingRiderId?: number
}

---

### RT-009 — Live elevation/route profile strip
**Owner:** FE  
**Estimate:** 1.0 day  
**Depends:** RT-003

#### Scope
- Add compact profile chart in simulation pages.
- Draw course profile from profile.points.
- Overlay marker at current kmCovered (or overallKm for stage race).

#### Definition of done
- Marker moves continuously with timeline.
- Chart remains performant throughout stage race.

---

### RT-010 — Perf, tuning, regression, acceptance
**Owner:** QA + FE + SIM  
**Estimate:** 1.0 day  
**Depends:** RT-004, RT-006, RT-008, RT-009

#### Automated regression status (2026-02-11)
- ✅ `go test ./...` passed
- ✅ `go build ./...` passed
- ✅ `gui: npm run build` passed
- ⏳ Manual interactive run-through checklist still pending (one-day + stage-race live sessions)

#### Manual QA run script (interactive signoff)
1. **Launch app**
  - Start Wails app in dev mode and open the Simulation flow.
  - Confirm no startup errors in console/log output.

2. **One-day race interactive run**
  - Navigate: Home → Race Setup → enable Interactive Mode → start race.
  - Verify team roster loads with group labels, telemetry, effort sliders, and sprint train panel.
  - Reorder sprint train (↑/↓), set follow targets, launch sprint for one rider.
  - Pass criteria:
    - `Sprint Train` phase chip transitions (`IDLE`/`FOLLOWING`/`LAUNCH`) during race updates.
    - Follow selections persist and do not reset unexpectedly.
    - Launch command visibly changes rider behavior/telemetry within subsequent updates.

3. **Command stress test (one-day)**
  - Rapidly click action buttons and move effort sliders for 3–5 riders for ~30s.
  - Reorder sprint train and change follow targets during this period.
  - Pass criteria:
    - No UI crashes/freezes.
    - No unbounded log spam or obvious memory-growth symptoms.
    - Cooldown/pulse and slider debounce remain responsive.

4. **Route profile verification (one-day)**
  - Observe route profile marker through early/mid/late race.
  - Pass criteria:
    - Marker advances continuously with race progress.
    - Marker position remains within profile bounds (0–total km).

5. **Stage race interactive run**
  - Navigate: Home → Stage Race Setup → enable Interactive Mode → start stage race.
  - Repeat reorder/follow/launch interactions in at least one stage.
  - Pass criteria:
    - Stage and overall progress clocks update coherently.
    - Stage route profile marker tracks current stage km.
    - Team controls remain functional across stage transitions.

6. **DNF and resilience spot-check**
  - Let simulation run until at least one incident/DNF scenario appears (or run multiple starts).
  - Pass criteria:
    - Telemetry panels and group rendering remain stable.
    - No broken UI state when riders disappear from active race state.

7. **Final acceptance**
  - Record outcome for each checklist item (Pass/Fail + short note).
  - Mark RT-010 `Done` only if all critical pass criteria are met.

#### Perf targets
- UI frame updates remain smooth with telemetry enabled.
- No unbounded state growth in event or telemetry buffers.

#### Test checklist
- One-day race interactive full run
- Stage race interactive full run
- Command stress test (rapid action and slider changes)
- Final-km sprint train scenario
- DNF riders do not break telemetry rendering

#### Acceptance criteria
- Command latency perceived as immediate.
- Rider cards always show coherent group + telemetry.
- Route profile and marker are correct and stable.

---

## API Change Log (for implementation PRs)

### New Wails methods
- SetRiderEffort(sessionId: string, riderId: number, effortPct: number): Promise<void>
- SetSprintTrainOrder(sessionId: string, riderIds: number[]): Promise<void>
- SetRiderFollow(sessionId: string, riderId: number, followRiderId?: number): Promise<void>
- LaunchSprint(sessionId: string, riderId: number): Promise<void>

### Event payload additions
- simulation_progress
  - riderTelemetry?: Record<number, RiderTelemetryPoint>
  - profile?: RouteProfile
  - teamState?: TeamState

- stage_race_progress
  - riderTelemetry?: Record<number, RiderTelemetryPoint>
  - profile?: RouteProfile
  - teamState?: TeamState

Where:
- RiderTelemetryPoint
  - speedKph: number
  - powerWatts: number
  - effortPct: number
  - fatiguePct: number
  - wPrimePct: number
  - routeKm: number
  - routePct: number
- RouteProfile
  - totalKm: number
  - points: Array<{km: number; elev: number}>
- TeamState
  - sprintTrainOrder: number[]
  - followMap: Record<number, number | null>
  - sprintTrainPhase: "idle" | "following" | "launch"
  - launchingRiderId?: number

---

## Implementation Order (Critical Path)
1. RT-001 → RT-002 → RT-003
2. RT-005 → RT-006
3. RT-004
4. RT-007 → RT-008
5. RT-009
6. RT-010

---

## Out-of-Scope for MVP
- Full map renderer
- Multiplayer/network mode
- New physics engine
- Advanced AI sprint-train tactics beyond basic leadout phases
