// Plays a live race to the end from the live-race screen (start it with
// driver.mjs first): answers each stage plan (LEADER env: the rider to ride
// for) and every decision card, picking the first option whose text contains
// one of the ';'-separated preferences (else the first option). Prints each
// card as a JSON line, screenshots it (<outprefix>-mN.png), then the result.
// usage: LEADER='Jonas Vingegaard' node playtest.mjs /tmp/out 'Let him go;Sit in'
import fs from 'node:fs'
const out = process.argv[2], pickRule = process.argv[3] || 'first'
const list = await (await fetch('http://127.0.0.1:9222/json/list')).json()
const page = list.find((t) => t.type === 'page')
const ws = new WebSocket(page.webSocketDebuggerUrl)
let id = 0; const pending = new Map()
ws.onmessage = (m) => { const d = JSON.parse(m.data); if (d.id && pending.has(d.id)) { pending.get(d.id)(d); pending.delete(d.id) } }
await new Promise((r) => (ws.onopen = r))
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })) })
const ev = async (e) => (await send('Runtime.evaluate', { expression: e, awaitPromise: true, returnByValue: true })).result?.result?.value
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
const next = () => ev(`[...document.querySelectorAll('button')].find(b=>b.innerText.trim()==='Next decision')?.click()`)
await send('Emulation.setDeviceMetricsOverride', { width: 1500, height: 1000, deviceScaleFactor: 1, mobile: false })
const LEADER = JSON.stringify(process.env.LEADER || '@@none@@')
let n = 0
for (let t = 0; t < 7200; t++) {
  if (await ev(`location.pathname.includes('stage-race-gc') || !!document.querySelector('.debrief')`)) { console.log('FINISHED'); break }
  if (await ev(`!!document.querySelector('.stage-plan')`)) {
    const feed = await ev(`(document.querySelector('[class*=feed]')?.innerText || '').slice(0, 1200)`)
    console.log('FEED ' + JSON.stringify(feed))
    await ev(`(() => { const c = document.querySelector('.stage-plan'); const b = [...c.querySelectorAll('button')].find(b => b.innerText.includes(${LEADER})); b?.click() })()`)
    await sleep(400)
    const title = await ev(`(() => { const c = document.querySelector('.stage-plan'); const t = c.querySelector('h2,h3')?.innerText; [...c.querySelectorAll('button')].find(b => b.innerText.trim() === 'Start the stage')?.click(); return t })()`)
    console.log('PLAN ' + title)
    await sleep(1500); await next(); continue
  }
  const m = await ev(`(() => { const c = document.querySelector('.decision-card:not(.stage-plan)'); if (!c) return null;
    return { title: c.querySelector('h2')?.innerText, km: c.querySelector('.decision-km')?.innerText, text: c.querySelector('.decision-text')?.innerText,
      options: [...c.querySelectorAll('.decision-option')].map(b => b.innerText.replace(/\\n/g,' — ')) } })()`)
  if (m) {
    n++
    await send('Page.captureScreenshot', { format: 'png' }).then(r => fs.writeFileSync(`${out}-m${n}.png`, Buffer.from(r.result.data, 'base64')))
    let idx = 0
    const prefs = pickRule.split(';')
    for (const p of prefs) { const i = m.options.findIndex(o => o.toLowerCase().includes(p.toLowerCase())); if (i >= 0) { idx = i; break } }
    m.picked = m.options[idx]
    console.log(JSON.stringify(m))
    await ev(`document.querySelectorAll('.decision-card:not(.stage-plan) .decision-option')[${idx}]?.click()`)
    await sleep(1500); await next(); continue
  }
  if (t % 15 === 0) await next()
  await sleep(1000)
}
await sleep(3000); console.log('RESULT:', await ev(`(document.querySelector('.debrief') || document.querySelector('main'))?.innerText.slice(0,1500)`))
ws.close()
