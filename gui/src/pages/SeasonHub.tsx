import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router-dom'
import * as App from '../api'
import { director, app } from '../api'
import { Jersey } from '../components/race/Jersey'
import { RiderLink } from '../components/RiderProfile'
import { PlanForm, PlanValue } from '../components/race/PlanForm'
import { formatGap } from '../sim/shared'
import './Season.css'
import { teamColor } from '../sim/teamColors'

type Tab = 'overview' | 'calendar' | 'team' | 'standings' | 'contracts'

/** The season of the page (rider panels compare within its field). */
function useSeasonId(): number {
  return Number(useParams().seasonId)
}

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
function ratingsTitle(r: app.RiderInfo): string {
  return `Climbing ${r.climbing} · Punch ${r.punch} · Repeated efforts ${r.repeatability} · Cobbles ${r.handling} · Endurance ${r.endurance} · Sprint ${r.sprint} · TT ${r.tt}`
}

function eventDates(e: app.SeasonEventInfo): string {
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
  const [hub, setHub] = useState<app.SeasonHub | null>(null)
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
        <div className="season-title">
          <span className="muted">{s.name} · {offseason ? `transfer window before ${s.year}` : `${s.year} · ${s.date}`} · {hub.roster} riders</span>
          <h1><i className="team-bar" style={{ background: teamColor(s.team) }} />{s.team}</h1>
        </div>
        <div className="season-money">
          <div title="Your wage budget for the year"><span>Budget</span><b>{money(s.budget)}</b></div>
          <div title="Salaries of the riders under contract now"><span>Wages</span><b className={hub.wages > s.budget ? 'warn' : ''}>{money(hub.wages)}</b></div>
          {!offseason && (
            <div title="Committed for next year: riders under contract beyond this year and riders signed for next year">
              <span>Next year · {hub.nextRiders} riders</span><b>{money(hub.nextCommitted)}</b>
            </div>
          )}
          {!offseason && (
            <div title="The board's confidence in you: objectives met add to it, objectives missed take away">
              <span>Board</span><b className={hub.confidence < 35 ? 'warn' : 'accent'}>{Math.round(hub.confidence)}%</b>
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
  hub: app.SeasonHub
  reload: () => Promise<void>
  openTab: (t: Tab) => void
  justRaced: number
  onRaced: (eventId: number) => void
}) {
  const s = hub.season
  const offseason = s.phase === 'offseason'
  const [busy, setBusy] = useState(false)
  const [yearNews, setYearNews] = useState<app.NewsInfo[] | null>(null)

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
    <>
    {!offseason && <SeasonStrip hub={hub} />}
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
    </>
  )
}

/** The year at a glance: every event on a January–October line, Grand Tours
 * as bars, our wins in yellow, today marked. */
function SeasonStrip({ hub }: { hub: app.SeasonHub }) {
  const year = hub.season.year
  const t0 = Date.UTC(year, 0, 1)
  const t1 = Date.UTC(year, 9, 31)
  const W = 1000
  // Event dates come as "8 Mar" (the season's year), today as "6 Apr 2026".
  const x = (d: string) => {
    const t = Date.parse(/\d{4}/.test(d) ? `${d} UTC` : `${d} ${year} UTC`)
    return isNaN(t) ? -1 : Math.max(0, Math.min(W, ((t - t0) / (t1 - t0)) * W))
  }
  const today = x(hub.season.date)
  const won = (e: app.SeasonEventInfo) => e.mine.startsWith('1st')
  const months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct']
  const big = hub.events.filter((e) => e.days >= 21)
  return (
    <section className="season-strip" aria-label="The season">
      <svg viewBox={`0 -26 ${W} 80`} width="100%" role="img" aria-label={`${hub.events.filter((e) => e.done).length} of ${hub.events.length} races done, ${hub.events.filter(won).length} won`}>
        <line x1={0} x2={W} y1={10} y2={10} className="ss-line" />
        {today >= 0 && <line x1={0} x2={today} y1={10} y2={10} className="ss-done" />}
        {big.map((e) => (
          <g key={e.id}>
            <rect x={x(e.start)} y={3} width={Math.max(8, x(e.end) - x(e.start))} height={14} rx={7} className={`ss-gt${won(e) ? ' won' : ''}`} />
            <text x={(x(e.start) + x(e.end)) / 2} y={-6} textAnchor="middle" className="ss-label">{e.name}</text>
          </g>
        ))}
        {hub.events.filter((e) => e.days < 21).map((e) => (
          <circle key={e.id} cx={x(e.start)} cy={10} r={won(e) ? 5 : 3} className={`ss-ev${e.done ? ' done' : ''}${won(e) ? ' won' : ''}${hub.next?.id === e.id ? ' next' : ''}`}>
            <title>{`${e.name}${e.mine ? `: ${e.mine}` : ''}`}</title>
          </circle>
        ))}
        {today >= 0 && (
          <g>
            <line x1={today} x2={today} y1={-4} y2={24} className="ss-today" />
            <text x={today} y={36} textAnchor="middle" className="ss-today-label">today</text>
          </g>
        )}
        {months.map((m) => (
          <text key={m} x={x(`1 ${m}`)} y={50} className="ss-month">{m}</text>
        ))}
      </svg>
    </section>
  )
}

function Objectives({ hub }: { hub: app.SeasonHub }) {
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
              <span className="obj-mark" aria-label={state}>{o.met ? <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round" strokeLinejoin="round"><path d="m5 12 5 5 9-10" /></svg> : o.decided ? <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.6" strokeLinecap="round"><path d="M6 6l12 12M18 6 6 18" /></svg> : <i className="obj-open" />}</span>
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
function NextRace({ hub, reload, onRaced }: { hub: app.SeasonHub; reload: () => Promise<void>; onRaced: (eventId: number) => void }) {
  const sid = useSeasonId()
  const navigate = useNavigate()
  const s = hub.season
  const next = hub.next!
  const [squad, setSquad] = useState<app.SquadInfo | null>(null)
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
          <span className="next-kicker">Next · {eventDates(next)} · {next.days} {next.days > 1 ? 'days' : 'day'}</span>
          <h2>{next.name}</h2>
          <p className="muted"><CategoryBadge category={next.category} /></p>
        </div>
        <div className="actions">
          <button className="secondary" onClick={simulate} disabled={busy || picked.length === 0}>{busy ? 'Racing…' : 'Simulate'}</button>
          <button className="primary" onClick={raceLive} disabled={busy || picked.length === 0}>
            {next.kind === 'oneday' && planning ? 'Start the race' : 'Race it live'}
          </button>
        </div>
      </div>

      {planning && squad && (
        <div className="race-plan inline">
          <h3>Race plan</h3>
          <PlanForm riders={pickedRiders as unknown as app.RiderInfo[]} points={squad.points as director.PlanPoint[]} value={plan} onChange={setPlan} />
        </div>
      )}

      {!squad ? (
        <div className="empty-state">Loading the squad…</div>
      ) : (
        <>
          <div className="squad-count">
            <b>Squad {picked.length} of {squad.size}</b>
            <span className="muted">Riders sorted by how well the race suits them. Click a rider to pick or drop him.</span>
          </div>
          <div className="squad-wrap">
            <table className="squad">
              <thead>
                <tr>
                  <th aria-label="Picked" />
                  <th>Rider</th>
                  <th title="How well the race suits him, against the field">Suits the race</th>
                  <th className="num" title="Seasonal form on the day (100% = at his peak)">Form</th>
                  <th className="num" title="Threshold kept with his recent racing load (100% = fresh)">Fresh</th>
                  <th className="num">Morale</th>
                  <th className="num">Days</th>
                  <th>Next in his programme</th>
                </tr>
              </thead>
              <tbody>
                {squad.riders.map((r) => {
                  const on = picked.includes(r.id)
                  const full = !on && picked.length >= squad.size
                  return (
                    <tr
                      key={r.id}
                      className={`${on ? 'picked' : ''}${r.available ? '' : ' unavailable'}${full && r.available ? ' full' : ''}`}
                      onClick={() => r.available && toggle(r.id)}
                      aria-selected={on}
                    >
                      <td><span className={`pick-mark${on ? ' on' : ''}`} aria-hidden="true">{on && <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="3.4" strokeLinecap="round" strokeLinejoin="round"><path d="m5 12 5 5 9-10" /></svg>}</span></td>
                      <td className="rider-cell" title={ratingsTitle(r)}>
                        <RiderLink id={r.id} name={r.name} seasonId={sid} bold />
                        <small className="sub">{r.age} · {r.specialty}{!r.available && <span className="why"> · {r.why}</span>}</small>
                      </td>
                      <td><FitBar fit={r.fit} /></td>
                      <td className={`num${r.formPct < 98 ? ' warn' : ''}`}>{r.formPct.toFixed(0)}%</td>
                      <td className={`num${r.loadPct < 98 ? ' warn' : ''}`}>{r.loadPct.toFixed(0)}%</td>
                      <td className={`num${r.morale < 45 ? ' warn' : ''}`}>{Math.round(r.morale)}</td>
                      <td className="num">{r.raceDays}</td>
                      <td className="programme-cell">{r.programme.filter((p) => p !== next.name).slice(0, 3).join(', ') || <span className="muted">—</span>}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </>
      )}
    </section>
  )
}

/** A raced event's result: the top ten (or final GC), our riders, stage winners. */
function RaceSummary({ hub, eventId, onClose }: { hub: app.SeasonHub; eventId: number; onClose: () => void }) {
  const [rows, setRows] = useState<app.SeasonResultRow[] | null>(null)
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
  const word = fit >= 2 ? 'ideal' : fit >= 1 ? 'good' : fit >= 0 ? 'fair' : 'poor'
  return (
    <span className="fit" title={`${fit.toFixed(1)} σ above the field`}>
      <span className="fitbar"><i style={{ width: `${w}%` }} className={fit >= 2 ? 'top' : fit >= 1 ? 'good' : ''} /></span>
      <span className="fit-word">{word}</span>
    </span>
  )
}

function TeamTable({ rows, mineRow }: { rows: app.StandingRow[]; mineRow?: app.StandingRow }) {
  const shown = mineRow && !rows.some((r) => r.mine) ? [...rows, mineRow] : rows
  return (
    <table className="standings">
      <tbody>
        {shown.map((t) => (
          <tr key={t.team} className={t.mine ? 'mine' : ''}>
            <td className="pos">{t.pos}</td>
            <td><Jersey team={t.team} /> {t.team}</td>
            <td className="num">{t.points}</td>
            <td className="num muted wins">{t.wins} {t.wins === 1 ? 'win' : 'wins'}</td>
          </tr>
        ))}
        {shown.length === 0 && <tr><td className="muted">No points yet.</td></tr>}
      </tbody>
    </table>
  )
}

function Calendar({ hub }: { hub: app.SeasonHub }) {
  const [open, setOpen] = useState<number | null>(null)
  const [rows, setRows] = useState<app.SeasonResultRow[]>([])
  const show = async (e: app.SeasonEventInfo) => {
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

function EventResults({ rows }: { rows: app.SeasonResultRow[] }) {
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

function Team({ hub, reload }: { hub: app.SeasonHub; reload: () => Promise<void> }) {
  const sid = useSeasonId()
  const [roster, setRoster] = useState<app.SeasonRiderInfo[]>([])
  const load = useCallback(async () => setRoster(await App.GetSeasonRoster(hub.season.id)), [hub.season.id])
  useEffect(() => {
    load()
  }, [load])
  const year = hub.season.year
  return (
    <section className="card">
      <h2>{hub.season.team}: {roster.length} riders · wages {money(roster.reduce((a, r) => a + r.salary, 0))}</h2>
      <div className="roster-wrap">
        <table className="roster">
          <thead>
            <tr>
              <th>Rider</th>
              <th className="num" title="Threshold power and per kg">Power</th>
              <th title="Peak sprint power, punch (short climbs), cobbles, endurance: each against the riders of the season">Strengths</th>
              <th title="Racing days, points and wins this season">Season</th>
              <th title="Morale (0–100)">Morale</th>
              <th>Programme</th>
              <th className="num">Contract</th>
            </tr>
          </thead>
          <tbody>
            {roster.map((r) => (
              <tr key={r.id}>
                <td>
                  <RiderLink id={r.id} name={r.name} seasonId={sid} bold />
                  <small className="sub">{r.age} · {r.specialty}</small>
                </td>
                <td className="num">{r.ftp} W<small className="sub">{r.wkg.toFixed(2)} W/kg</small></td>
                <td>
                  <span className="strengths">
                    <Strength label="Spr" value={(r.sprintW - 900) / 9} title={`Sprint ${r.sprintW} W`} />
                    <Strength label="Pun" value={r.punch} title={`Punch ${r.punch}`} />
                    <Strength label="Cob" value={r.handling} title={`Cobbles ${r.handling}`} />
                    <Strength label="End" value={r.endurance} title={`Endurance ${r.endurance}`} />
                  </span>
                </td>
                <td className="season-cell">{r.raceDays} days{r.points ? ` · ${r.points} pts` : ''}{r.wins ? <b> · {r.wins} {r.wins === 1 ? 'win' : 'wins'}</b> : ''}</td>
                <td className={r.morale < 45 ? 'warn' : ''}>{Math.round(r.morale)}</td>
                <td className="programme-cell">{r.programme.length ? r.programme.join(', ') : <span className="muted">—</span>}</td>
                <td className={`num${r.contractEnd <= year && !r.nextTeam ? ' warn' : ''}`}>
                  {r.nextTeam ? (r.nextTeam === hub.season.team ? `renewed${r.contractEnd > year ? ` to ${r.contractEnd}` : ''}` : `→ ${r.nextTeam}`) : `to ${r.contractEnd}`}
                  <small className="sub">{money(r.salary)}</small>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="muted small">Programmes are the team's plan for each rider; change a race's squad on the overview before it starts.</p>
      <button className="link" onClick={() => { load(); reload() }}>Refresh</button>
    </section>
  )
}

/** A small strength bar (rating 0–100). */
function Strength({ label, value, title }: { label: string; value: number; title: string }) {
  const w = Math.max(0, Math.min(100, value))
  return (
    <span className="strength" title={title}>
      <span className="strength-label">{label}</span>
      <span className="strength-track"><i style={{ width: `${w}%` }} /></span>
    </span>
  )
}

function Standings({ hub }: { hub: app.SeasonHub }) {
  const sid = useSeasonId()
  return (
    <div className="season-cols">
      <section className="card">
        <h2>Riders</h2>
        <table className="standings">
          <tbody>
            {hub.riders.map((r) => (
              <tr key={r.riderId} className={r.mine ? 'mine' : ''}>
                <td className="pos">{r.pos}</td>
                <td><RiderLink id={r.riderId} name={r.name} seasonId={sid} /></td>
                <td className="muted"><Jersey team={r.team} /> {r.team}</td>
                <td className="num">{r.points}</td>
                <td className="num muted wins">{r.wins} {r.wins === 1 ? 'win' : 'wins'}</td>
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
function Contracts({ hub, reload }: { hub: app.SeasonHub; reload: () => Promise<void> }) {
  const s = hub.season
  const offseason = s.phase === 'offseason'
  const [roster, setRoster] = useState<app.SeasonRiderInfo[]>([])
  const [market, setMarket] = useState<app.SeasonRiderInfo[]>([])
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

  const offer = async (r: app.SeasonRiderInfo, salary: number, years: number) => {
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
  riders: app.SeasonRiderInfo[]
  onOffer: (r: app.SeasonRiderInfo, salary: number, years: number) => void
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
  r: app.SeasonRiderInfo
  onOffer: (r: app.SeasonRiderInfo, salary: number, years: number) => void
  action: string
  showTeam?: boolean
}) {
  const sid = useSeasonId()
  const [salary, setSalary] = useState(r.ask)
  const [years, setYears] = useState(2)
  return (
    <tr>
      <td title={ratingsTitle(r)}><RiderLink id={r.id} name={r.name} seasonId={sid} bold /></td>
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
