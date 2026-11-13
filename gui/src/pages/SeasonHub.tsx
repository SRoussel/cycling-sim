import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import * as App from '../../wailsjs/go/main/App'
import { director, main } from '../../wailsjs/go/models'
import { Jersey } from '../components/race/Jersey'
import { PlanForm, PlanValue } from '../components/race/PlanForm'
import { formatGap } from '../sim/shared'
import './Season.css'

type Tab = 'overview' | 'calendar' | 'team' | 'standings' | 'contracts'

/** Thousand euro → "€1.2M" / "€450k". */
function money(k: number): string {
  return k >= 1000 ? `€${(k / 1000).toFixed(1)}M` : `€${k}k`
}

const CATEGORY: Record<string, { label: string; badge: string }> = {
  monument: { label: 'monument', badge: 'climb' },
  classic: { label: 'classic', badge: 'hilly' },
  grandtour: { label: 'grand tour', badge: 'stage tdf' },
  weeklong: { label: 'week-long', badge: 'stage other' },
}

function CategoryBadge({ category }: { category: string }) {
  const c = CATEGORY[category] ?? { label: category, badge: '' }
  return <span className={`badge ${c.badge}`}>{c.label}</span>
}

/** The 1-100 ratings that shape a rider's racing, for a tooltip. */
function ratingsTitle(r: main.RiderInfo): string {
  return `Climbing ${r.climbing} · Punch ${r.punch} · Repeated efforts ${r.repeatability} · Cobbles ${r.handling} · Endurance ${r.endurance} · Sprint ${r.sprint} · TT ${r.tt}`
}

function eventDates(e: main.SeasonEventInfo): string {
  return e.days > 1 ? `${e.start} – ${e.end}` : e.start
}

/** The season's main screen. */
function SeasonHub() {
  const { seasonId } = useParams()
  const id = Number(seasonId)
  const location = useLocation()
  // The race just raced (back from the race screen, or just simulated): its
  // result opens the overview.
  const [justRaced, setJustRaced] = useState<number>((location.state as { justRaced?: number } | null)?.justRaced ?? 0)
  const [hub, setHub] = useState<main.SeasonHub | null>(null)
  const [tab, setTab] = useState<Tab>('overview')
  const [error, setError] = useState('')

  // Another season: start from its overview.
  useEffect(() => {
    setTab('overview')
    setHub(null)
  }, [id])

  const reload = useCallback(async () => {
    try {
      setHub(await App.GetSeasonHub(id))
    } catch (e) {
      setError(String(e))
    }
  }, [id])
  useEffect(() => {
    reload()
  }, [reload])

  if (error) {
    return (
      <div className="error-message">
        <h2>Season not found</h2>
        <p>{error}</p>
        <Link to="/season"><button className="primary">← Seasons</button></Link>
      </div>
    )
  }
  if (!hub) return <div className="empty-state">Loading the season…</div>
  const s = hub.season
  const offseason = s.phase === 'offseason'

  return (
    <div className="season page-enter" key={id}>
      <header className="season-head">
        <div>
          <h1><Jersey team={s.team} size={20} /> {s.name}</h1>
          <p className="muted">
            {s.team} · {offseason ? `transfer window before ${s.year}` : `${s.year} · ${s.date}`} · {hub.roster} riders
          </p>
        </div>
        <div className="season-money">
          <div title="Your wage budget for the year"><span>Budget</span><b>{money(s.budget)}</b></div>
          <div title="Salaries of the riders under contract now"><span>Wages</span><b className={hub.wages > s.budget ? 'warn' : ''}>{money(hub.wages)}</b></div>
          {!offseason && (
            <div title="Committed for next year: riders under contract beyond this year and riders signed for next year">
              <span>Next year</span><b>{money(hub.nextCommitted)} · {hub.nextRiders} riders</b>
            </div>
          )}
          {!offseason && (
            <div title="The board's confidence in you: objectives met add to it, objectives missed take away">
              <span>Board</span><b className={hub.confidence < 35 ? 'warn' : ''}>{Math.round(hub.confidence)}%</b>
            </div>
          )}
        </div>
      </header>

      <nav className="season-tabs" role="tablist">
        {(['overview', 'calendar', 'team', 'standings', 'contracts'] as Tab[]).map((t) => (
          <button key={t} role="tab" aria-selected={tab === t} className={tab === t ? 'on' : ''} onClick={() => setTab(t)}>
            {t === 'contracts' ? (offseason ? 'Transfer window' : 'Contracts') : t[0].toUpperCase() + t.slice(1)}
          </button>
        ))}
      </nav>

      {tab === 'overview' && <Overview hub={hub} reload={reload} openTab={setTab} justRaced={justRaced} onRaced={setJustRaced} />}
      {tab === 'calendar' && <Calendar hub={hub} />}
      {tab === 'team' && <Team hub={hub} reload={reload} />}
      {tab === 'standings' && <Standings hub={hub} />}
      {tab === 'contracts' && <Contracts hub={hub} reload={reload} />}
    </div>
  )
}

function Overview({ hub, reload, openTab, justRaced, onRaced }: {
  hub: main.SeasonHub
  reload: () => Promise<void>
  openTab: (t: Tab) => void
  justRaced: number
  onRaced: (eventId: number) => void
}) {
  const s = hub.season
  const offseason = s.phase === 'offseason'
  const [busy, setBusy] = useState(false)
  const [yearNews, setYearNews] = useState<main.NewsInfo[] | null>(null)

  const endYear = async () => {
    setBusy(true)
    try {
      setYearNews(await App.EndSeasonYear(s.id))
      await reload()
    } finally {
      setBusy(false)
    }
  }
  const startYear = async () => {
    setBusy(true)
    try {
      await App.StartSeasonYear(s.id)
      setYearNews(null)
      await reload()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="season-grid">
      <div className="season-main">
        {justRaced > 0 && <RaceSummary hub={hub} eventId={justRaced} onClose={() => onRaced(0)} />}
        {hub.next && !offseason && <NextRace hub={hub} reload={reload} onRaced={onRaced} />}
        {hub.yearOver && (
          <section className="card next-race">
            <h2>The {s.year} season is over</h2>
            <p className="muted">Close the year: the board gives its verdict and sets next year's budget, contracts run out, riders age, some retire and young riders turn pro. Then the transfer window opens.</p>
            <p className="muted">Riders whose contracts end now leave unless you've renewed them (Contracts).</p>
            <div className="actions">
              <button className="secondary" onClick={() => openTab('contracts')}>Contracts first</button>
              <button className="primary" onClick={endYear} disabled={busy}>{busy ? 'Closing the year…' : `Close ${s.year}`}</button>
            </div>
          </section>
        )}
        {offseason && (
          <section className="card next-race">
            <h2>Transfer window</h2>
            <p className="muted">
              Sign free agents before the {s.year} season. When you start the season the other teams sign the rest, and the calendar, programmes and the board's objectives are drawn up.
            </p>
            {yearNews && (
              <ul className="news compact">
                {yearNews.filter((n) => n.mine).map((n, i) => <li key={i} className="mine">{n.text}</li>)}
              </ul>
            )}
            <div className="actions">
              <button className="secondary" onClick={() => openTab('contracts')}>Free agents</button>
              <button className="primary" onClick={startYear} disabled={busy}>{busy ? 'Starting…' : `Start ${s.year}`}</button>
            </div>
          </section>
        )}
        {!offseason && <Objectives hub={hub} />}
        <section className="card">
          <h2>News</h2>
          <ul className="news">
            {hub.news.map((n, i) => (
              <li key={i} className={n.mine ? 'mine' : ''}><span className="muted">{n.date}</span> {n.text}</li>
            ))}
          </ul>
        </section>
      </div>
      <aside className="season-side">
        <section className="card">
          <h2>Team ranking{offseason ? ` ${s.year - 1}` : ''}</h2>
          <TeamTable rows={hub.teams.slice(0, 12)} mineRow={hub.teams.find((t) => t.mine)} />
        </section>
        <section className="card">
          <h2>Recent races</h2>
          <ul className="recent">
            {hub.events.filter((e) => e.done).slice(-6).reverse().map((e) => (
              <li key={e.id}>
                <span className="race-name">{e.name}</span>
                <span className="muted">{e.podium[0] ?? ''}</span>
                {e.mine && <span className="mine-result">{e.mine}</span>}
              </li>
            ))}
            {hub.events.every((e) => !e.done) && <li className="muted">No races yet.</li>}
          </ul>
        </section>
      </aside>
    </div>
  )
}

function Objectives({ hub }: { hub: main.SeasonHub }) {
  return (
    <section className="card">
      <h2>The board's objectives</h2>
      <ul className="objectives">
        {hub.objectives.map((o, i) => {
          const state = o.met ? 'met' : o.decided ? 'missed' : 'open'
          let now = ''
          if (o.kind === 'team_rank' && o.current) now = `${ordinal(o.current)} now`
          if (o.kind === 'wins') now = `${o.current} of ${o.target}`
          if (o.kind === 'place' && o.current) now = `best ${ordinal(o.current)}`
          if (o.kind === 'stage_win' && o.decided) now = o.met ? 'won' : 'no stage'
          return (
            <li key={i} className={state}>
              <span className="obj-mark">{o.met ? '✓' : o.decided ? '✗' : '·'}</span>
              <span>{o.text}</span>
              <span className="muted">{now}</span>
            </li>
          )
        })}
      </ul>
      <div className="confidence" title="Board confidence">
        <i style={{ width: `${hub.confidence}%` }} className={hub.confidence < 35 ? 'low' : ''} />
      </div>
    </section>
  )
}

function ordinal(n: number): string {
  const s = n % 100 >= 11 && n % 100 <= 13 ? 'th' : ({ 1: 'st', 2: 'nd', 3: 'rd' } as Record<number, string>)[n % 10] ?? 'th'
  return `${n}${s}`
}

/** The next race: pick the squad, then race it live or simulate it. */
function NextRace({ hub, reload, onRaced }: { hub: main.SeasonHub; reload: () => Promise<void>; onRaced: (eventId: number) => void }) {
  const navigate = useNavigate()
  const s = hub.season
  const next = hub.next!
  const [squad, setSquad] = useState<main.SquadInfo | null>(null)
  const [picked, setPicked] = useState<number[]>([])
  const [busy, setBusy] = useState(false)
  const [planning, setPlanning] = useState(false)
  const [plan, setPlan] = useState<PlanValue>({ leaderId: 0, break: 'auto', hardOn: [], attackOn: '' })

  useEffect(() => {
    let live = true
    App.GetSeasonSquad(s.id, next.id).then((sq) => {
      if (!live) return
      setSquad(sq)
      setPicked(sq.riders.filter((r) => r.planned && r.available).map((r) => r.id))
      setPlanning(false)
    })
    return () => {
      live = false
    }
  }, [s.id, next.id])

  const toggle = async (rid: number) => {
    if (!squad) return
    const on = picked.includes(rid)
    if (!on && picked.length >= squad.size) return
    const nextPicks = on ? picked.filter((x) => x !== rid) : [...picked, rid]
    setPicked(nextPicks)
    await App.SetSeasonSquad(s.id, next.id, nextPicks)
  }

  const simulate = async () => {
    setBusy(true)
    try {
      await App.SimulateSeasonEvent(s.id, next.id)
      onRaced(next.id)
      await reload()
    } finally {
      setBusy(false)
    }
  }

  const raceLive = () => {
    const season = { seasonId: s.id, eventId: next.id }
    if (next.kind === 'stagerace') {
      navigate(`/stage-race-sim/${next.refId}`, { state: { interactiveMode: true, controlledTeam: s.team, season, eventName: next.name } })
      return
    }
    if (!planning) {
      setPlanning(true)
      return
    }
    navigate(`/simulation/${next.refId}`, {
      state: { race: squad?.race, interactiveMode: true, controlledTeam: s.team, plan, season },
    })
  }

  const pickedRiders = useMemo(() => (squad?.riders ?? []).filter((r) => picked.includes(r.id)), [squad, picked])

  return (
    <section className="card next-race">
      <div className="next-head">
        <div>
          <h2>Next: {next.name}</h2>
          <p className="muted"><CategoryBadge category={next.category} /> {eventDates(next)} · {next.days} {next.days > 1 ? 'days' : 'day'}</p>
        </div>
        <div className="actions">
          <button className="secondary" onClick={simulate} disabled={busy || picked.length === 0}>{busy ? 'Racing…' : 'Simulate'}</button>
          <button className="primary" onClick={raceLive} disabled={busy || picked.length === 0}>
            {next.kind === 'oneday' && planning ? 'Start the race →' : 'Race it live'}
          </button>
        </div>
      </div>

      {planning && squad && (
        <div className="race-plan inline">
          <h3>Race plan</h3>
          <PlanForm riders={pickedRiders as unknown as main.RiderInfo[]} points={squad.points as director.PlanPoint[]} value={plan} onChange={setPlan} />
        </div>
      )}

      {!squad ? (
        <div className="empty-state">Loading the squad…</div>
      ) : (
        <>
          <p className="squad-count">
            Squad {picked.length}/{squad.size}
            <span className="muted"> · riders sorted by how well the race suits them; fresh = threshold kept with recent racing, form = seasonal form on the day</span>
          </p>
          <table className="squad">
            <thead>
              <tr>
                <th />
                <th>Rider</th>
                <th>Type</th>
                <th title="How well the race suits him (standard deviations above the field)">Suits</th>
                <th title="Seasonal form on the day (100% = at his peak)">Form</th>
                <th title="Threshold kept with his recent racing load (100% = fresh)">Fresh</th>
                <th>Morale</th>
                <th>Days</th>
                <th>Next in his programme</th>
              </tr>
            </thead>
            <tbody>
              {squad.riders.map((r) => {
                const on = picked.includes(r.id)
                return (
                  <tr key={r.id} className={`${on ? 'picked' : ''}${r.available ? '' : ' unavailable'}`} onClick={() => r.available && toggle(r.id)}>
                    <td><input type="checkbox" checked={on} disabled={!r.available || (!on && picked.length >= squad.size)} readOnly /></td>
                    <td className="rider-cell" title={ratingsTitle(r)}>
                      <b>{r.name}</b> <span className="muted">{r.age}</span>
                      {!r.available && <span className="why"> {r.why}</span>}
                    </td>
                    <td className="muted">{r.specialty}</td>
                    <td><FitBar fit={r.fit} /></td>
                    <td className={r.formPct < 98 ? 'warn' : ''}>{r.formPct.toFixed(0)}%</td>
                    <td className={r.loadPct < 98 ? 'warn' : ''}>{r.loadPct.toFixed(0)}%</td>
                    <td className={r.morale < 45 ? 'warn' : ''}>{Math.round(r.morale)}</td>
                    <td>{r.raceDays}</td>
                    <td className="muted programme">{r.programme.filter((p) => p !== next.name).slice(0, 2).join(', ')}</td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </>
      )}
    </section>
  )
}

/** A raced event's result: the top ten (or final GC), our riders, stage winners. */
function RaceSummary({ hub, eventId, onClose }: { hub: main.SeasonHub; eventId: number; onClose: () => void }) {
  const [rows, setRows] = useState<main.SeasonResultRow[] | null>(null)
  useEffect(() => {
    App.GetSeasonEventResults(hub.season.id, eventId).then(setRows)
  }, [hub.season.id, eventId])
  const e = hub.events.find((x) => x.id === eventId)
  if (!e || !rows) return null
  const finals = rows.filter((r) => r.kind !== 'stage')
  const top = finals.filter((r) => r.position <= 10)
  const ours = finals.filter((r) => r.mine && r.position > 10).slice(0, 4)
  return (
    <section className="card race-summary">
      <div className="card-head">
        <h2>{e.name}{e.days > 1 ? ': final GC' : ''}</h2>
        <button className="link" onClick={onClose}>Close</button>
      </div>
      <p className="muted small">
        {e.mine ? <>Our best: <span className="mine-result">{e.mine}</span></> : 'No top results for us'}
        {e.points > 0 && ` · ${e.points} points`}
      </p>
      <EventResults rows={[...top, ...ours, ...rows.filter((r) => r.kind === 'stage')]} />
    </section>
  )
}

function FitBar({ fit }: { fit: number }) {
  const w = Math.max(0, Math.min(100, ((fit + 1) / 4) * 100))
  return (
    <span className="fitbar" title={`${fit.toFixed(1)} σ above the field`}>
      <i style={{ width: `${w}%` }} className={fit >= 2 ? 'top' : fit >= 1 ? 'good' : ''} />
    </span>
  )
}

function TeamTable({ rows, mineRow }: { rows: main.StandingRow[]; mineRow?: main.StandingRow }) {
  const shown = mineRow && !rows.some((r) => r.mine) ? [...rows, mineRow] : rows
  return (
    <table className="standings">
      <tbody>
        {shown.map((t) => (
          <tr key={t.team} className={t.mine ? 'mine' : ''}>
            <td className="pos">{t.pos}</td>
            <td><Jersey team={t.team} /> {t.team}</td>
            <td className="num">{t.points}</td>
            <td className="num muted">{t.wins} W</td>
          </tr>
        ))}
        {shown.length === 0 && <tr><td className="muted">No points yet.</td></tr>}
      </tbody>
    </table>
  )
}

function Calendar({ hub }: { hub: main.SeasonHub }) {
  const [open, setOpen] = useState<number | null>(null)
  const [rows, setRows] = useState<main.SeasonResultRow[]>([])
  const show = async (e: main.SeasonEventInfo) => {
    if (!e.done) return
    if (open === e.id) {
      setOpen(null)
      return
    }
    setRows(await App.GetSeasonEventResults(hub.season.id, e.id))
    setOpen(e.id)
  }
  return (
    <section className="card">
      <h2>{hub.season.phase === 'offseason' ? `${hub.season.year - 1} calendar` : `${hub.season.year} calendar`}</h2>
      <ul className="calendar">
        {hub.events.map((e) => (
          <li key={e.id} className={`${e.done ? 'done' : ''}${hub.next?.id === e.id ? ' next' : ''}`}>
            <button className="cal-row" onClick={() => show(e)} disabled={!e.done}>
              <span className="cal-date">{eventDates(e)}</span>
              <span className="race-name">{e.name}</span>
              <CategoryBadge category={e.category} />
              <span className="muted cal-podium">{e.done ? e.podium.join(' · ') : hub.next?.id === e.id ? 'next' : ''}</span>
              <span className="mine-result">{e.mine}{e.points > 0 ? ` · ${e.points} pts` : ''}</span>
            </button>
            {open === e.id && <EventResults rows={rows} />}
          </li>
        ))}
      </ul>
    </section>
  )
}

function EventResults({ rows }: { rows: main.SeasonResultRow[] }) {
  const finals = rows.filter((r) => r.kind !== 'stage')
  const stages = Array.from(new Set(rows.filter((r) => r.kind === 'stage').map((r) => r.stage)))
  return (
    <div className="event-results">
      <table>
        <tbody>
          {finals.map((r) => (
            <tr key={`${r.kind}-${r.riderId}`} className={r.mine ? 'mine' : ''}>
              <td className="pos">{r.position}</td>
              <td>{r.name}</td>
              <td className="muted"><Jersey team={r.team} /> {r.team}</td>
              <td className="num">{r.position === 1 ? '' : `+${formatGap(r.gap)}`}</td>
              <td className="num muted">{r.points ? `${r.points} pts` : ''}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {stages.length > 0 && (
        <ul className="stage-wins">
          {stages.map((st) => {
            const win = rows.find((r) => r.kind === 'stage' && r.stage === st && r.position === 1)
            const ours = rows.filter((r) => r.kind === 'stage' && r.stage === st && r.mine && r.position <= 10)
            return (
              <li key={st}>
                <span className="muted">Stage {st}</span> {win?.name}
                {ours.length > 0 && <span className="mine-result"> · us: {ours.map((o) => `${ordinal(o.position)} ${o.name}`).join(', ')}</span>}
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

function Team({ hub, reload }: { hub: main.SeasonHub; reload: () => Promise<void> }) {
  const [roster, setRoster] = useState<main.SeasonRiderInfo[]>([])
  const load = useCallback(async () => setRoster(await App.GetSeasonRoster(hub.season.id)), [hub.season.id])
  useEffect(() => {
    load()
  }, [load])
  const year = hub.season.year
  return (
    <section className="card">
      <h2>{hub.season.team}: {roster.length} riders · wages {money(roster.reduce((a, r) => a + r.salary, 0))}</h2>
      <table className="roster">
        <thead>
          <tr>
            <th>Rider</th>
            <th>Age</th>
            <th>Type</th>
            <th title="Threshold power">FTP</th>
            <th>W/kg</th>
            <th title="Peak sprint power">Sprint</th>
            <th title="Short climbs (1-5 min): how far above threshold he can go">Punch</th>
            <th title="How fast he recovers between efforts within a race">Repeat</th>
            <th title="Riding the cobbles and gravel">Cobbles</th>
            <th title="How little he fades over a long, hard day">Endur.</th>
            <th>Salary</th>
            <th title="Last year of his contract">Contract</th>
            <th>Morale</th>
            <th>Days</th>
            <th>Pts</th>
            <th>Wins</th>
            <th>Programme</th>
          </tr>
        </thead>
        <tbody>
          {roster.map((r) => (
            <tr key={r.id}>
              <td><b>{r.name}</b></td>
              <td>{r.age}</td>
              <td className="muted">{r.specialty}</td>
              <td>{r.ftp} W</td>
              <td>{r.wkg.toFixed(2)}</td>
              <td>{r.sprintW} W</td>
              <td>{r.punch}</td>
              <td>{r.repeatability}</td>
              <td>{r.handling}</td>
              <td>{r.endurance}</td>
              <td>{money(r.salary)}</td>
              <td className={r.contractEnd <= year && !r.nextTeam ? 'warn' : ''}>
                {r.nextTeam ? (r.nextTeam === hub.season.team ? `renewed to ${r.contractEnd > year ? r.contractEnd : ''}` : `→ ${r.nextTeam}`) : r.contractEnd}
              </td>
              <td className={r.morale < 45 ? 'warn' : ''}>{Math.round(r.morale)}</td>
              <td>{r.raceDays}</td>
              <td>{r.points || ''}</td>
              <td>{r.wins || ''}</td>
              <td className="muted programme">{r.programme.slice(0, 4).join(', ')}{r.programme.length > 4 ? ` +${r.programme.length - 4}` : ''}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <p className="muted small">Programmes are the team's plan for each rider; change a race's squad on the overview before it starts.</p>
      <button className="link" onClick={() => { load(); reload() }}>Refresh</button>
    </section>
  )
}

function Standings({ hub }: { hub: main.SeasonHub }) {
  return (
    <div className="season-cols">
      <section className="card">
        <h2>Riders</h2>
        <table className="standings">
          <tbody>
            {hub.riders.map((r) => (
              <tr key={r.riderId} className={r.mine ? 'mine' : ''}>
                <td className="pos">{r.pos}</td>
                <td>{r.name}</td>
                <td className="muted"><Jersey team={r.team} /> {r.team}</td>
                <td className="num">{r.points}</td>
                <td className="num muted">{r.wins} W</td>
              </tr>
            ))}
            {hub.riders.length === 0 && <tr><td className="muted">No points yet.</td></tr>}
          </tbody>
        </table>
      </section>
      <section className="card">
        <h2>Teams</h2>
        <TeamTable rows={hub.teams} />
      </section>
    </div>
  )
}

/** Renewals and signings (in the season: riders whose contracts end, for
 * next year; in the off-season: free agents, joining now). */
function Contracts({ hub, reload }: { hub: main.SeasonHub; reload: () => Promise<void> }) {
  const s = hub.season
  const offseason = s.phase === 'offseason'
  const [roster, setRoster] = useState<main.SeasonRiderInfo[]>([])
  const [market, setMarket] = useState<main.SeasonRiderInfo[]>([])
  const [reply, setReply] = useState<{ ok: boolean; message: string } | null>(null)
  const [filter, setFilter] = useState('')
  const load = useCallback(async () => {
    const [r, m] = await Promise.all([App.GetSeasonRoster(s.id), App.GetSeasonMarket(s.id)])
    setRoster(r)
    setMarket(m)
  }, [s.id])
  useEffect(() => {
    load()
  }, [load])

  const offer = async (r: main.SeasonRiderInfo, salary: number, years: number) => {
    const res = await App.OfferContract(s.id, r.id, salary, years)
    setReply(res)
    if (res.ok) {
      await load()
      await reload()
    }
  }
  const autoRenew = async () => {
    const lines = await App.AutoRenewContracts(s.id)
    setReply({ ok: true, message: lines.length ? lines.join(' ') : 'Nobody to renew.' })
    await load()
    await reload()
  }

  const expiring = roster.filter((r) => r.contractEnd <= s.year && !r.nextTeam)
  const shown = market.filter((r) => !filter || r.specialty === filter)

  return (
    <div className="contracts">
      {reply && <div className={`reply ${reply.ok ? 'ok' : 'no'}`}>{reply.message}</div>}
      {!offseason && (
        <section className="card">
          <div className="card-head">
            <h2>Your riders out of contract after {s.year} ({expiring.length})</h2>
            {expiring.length > 0 && <button className="secondary" onClick={autoRenew}>Renew the best (assistant)</button>}
          </div>
          <p className="muted small">Next year: {money(hub.nextCommitted)} committed for {hub.nextRiders} riders; budget {money(s.budget)} (next year's follows the board's verdict). Riders you don't renew leave at the end of the year.</p>
          <OfferTable riders={expiring} onOffer={offer} action="Renew" />
        </section>
      )}
      <section className="card">
        <div className="card-head">
          <h2>{offseason ? 'Free agents (join now)' : `Other teams' riders out of contract after ${s.year} (join next year)`}</h2>
          <select value={filter} onChange={(e) => setFilter(e.target.value)}>
            <option value="">All types</option>
            {['Climber', 'Puncheur', 'Cobbles', 'Sprinter', 'All-rounder'].map((t) => <option key={t} value={t}>{t}</option>)}
          </select>
        </div>
        <p className="muted small">They sign at their asking salary or more if you have the budget and a place (30 riders at most). Riders leaving a team want 15% more.</p>
        <OfferTable riders={shown} onOffer={offer} action="Sign" showTeam />
      </section>
    </div>
  )
}

function OfferTable({ riders, onOffer, action, showTeam }: {
  riders: main.SeasonRiderInfo[]
  onOffer: (r: main.SeasonRiderInfo, salary: number, years: number) => void
  action: string
  showTeam?: boolean
}) {
  if (riders.length === 0) return <div className="empty-state">Nobody.</div>
  return (
    <table className="roster market">
      <thead>
        <tr>
          <th>Rider</th>
          <th>Age</th>
          <th>Type</th>
          {showTeam && <th>Team</th>}
          <th>W/kg</th>
          <th title="How highly the market rates him (his best races)">Rating</th>
          <th>Pts</th>
          <th>Now</th>
          <th>Asks</th>
          <th>Offer</th>
        </tr>
      </thead>
      <tbody>
        {riders.map((r) => (
          <OfferRow key={r.id} r={r} onOffer={onOffer} action={action} showTeam={showTeam} />
        ))}
      </tbody>
    </table>
  )
}

function OfferRow({ r, onOffer, action, showTeam }: {
  r: main.SeasonRiderInfo
  onOffer: (r: main.SeasonRiderInfo, salary: number, years: number) => void
  action: string
  showTeam?: boolean
}) {
  const [salary, setSalary] = useState(r.ask)
  const [years, setYears] = useState(2)
  return (
    <tr>
      <td title={ratingsTitle(r)}><b>{r.name}</b></td>
      <td>{r.age}</td>
      <td className="muted">{r.specialty}</td>
      {showTeam && <td className="muted">{r.team ? <><Jersey team={r.team} /> {r.team}</> : 'free'}</td>}
      <td>{r.wkg.toFixed(2)}</td>
      <td><FitBar fit={r.value} /></td>
      <td>{r.points || ''}</td>
      <td className="muted">{r.salary ? money(r.salary) : ''}</td>
      <td>{money(r.ask)}</td>
      <td className="offer">
        <input type="number" min={100} step={50} value={salary} onChange={(e) => setSalary(Number(e.target.value))} title="Salary (thousand euro a year)" />
        <select value={years} onChange={(e) => setYears(Number(e.target.value))} title="Years">
          {[1, 2, 3, 4].map((y) => <option key={y} value={y}>{y} yr</option>)}
        </select>
        <button className="secondary" onClick={() => onOffer(r, salary, years)}>{action}</button>
      </td>
    </tr>
  )
}

export default SeasonHub
