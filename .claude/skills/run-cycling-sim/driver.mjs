// Drives the cycling-sim GUI in headless Chromium over the DevTools protocol
// (Node 22+: built-in fetch/WebSocket, no npm deps). Start it with app.sh.
//
// Reads one command per line from stdin (or a file argument):
//   nav <url>            navigate (waits 3 s for React to render)
//   click <text>         click the first button/link/label whose text contains <text>
//   waitfor <text>       wait up to 30 s for <text> to appear in the page
//   wait <ms>            sleep
//   eval <js>            evaluate a JS expression, print the JSON result
//   scrollto <selector>  scroll an element into view
//   text [selector]      print innerText (default body, first 1500 chars)
//   shot <file.png>      screenshot (1500x1000 viewport)
// Exits non-zero if a click or waitfor fails.
import fs from 'node:fs'

const src = process.argv[2] ? fs.readFileSync(process.argv[2], 'utf8') : fs.readFileSync(0, 'utf8')
const lines = src.split('\n').map((l) => l.trim()).filter((l) => l && !l.startsWith('#'))

const list = await (await fetch('http://127.0.0.1:9222/json/list')).json()
const page = list.find((t) => t.type === 'page')
if (!page) { console.error('no page target on :9222 (run app.sh start)'); process.exit(1) }
const ws = new WebSocket(page.webSocketDebuggerUrl)
let id = 0
const pending = new Map()
ws.onmessage = (m) => { const d = JSON.parse(m.data); if (d.id && pending.has(d.id)) { pending.get(d.id)(d); pending.delete(d.id) } }
await new Promise((r) => (ws.onopen = r))
const send = (method, params = {}) => new Promise((r) => { const i = ++id; pending.set(i, r); ws.send(JSON.stringify({ id: i, method, params })) })
const evaluate = async (expr) => {
  const r = await send('Runtime.evaluate', { expression: expr, awaitPromise: true, returnByValue: true })
  if (r.result?.exceptionDetails) return { error: r.result.exceptionDetails.exception?.description || 'exception' }
  return r.result?.result?.value
}
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))
await send('Page.enable'); await send('Runtime.enable')
await send('Emulation.setDeviceMetricsOverride', { width: 1500, height: 1000, deviceScaleFactor: 1, mobile: false })

let failed = false
for (const line of lines) {
  const sp = line.indexOf(' ')
  const cmd = sp < 0 ? line : line.slice(0, sp)
  const arg = sp < 0 ? '' : line.slice(sp + 1)
  switch (cmd) {
    case 'nav': await send('Page.navigate', { url: arg }); await sleep(3000); console.log('nav', arg); break
    case 'wait': await sleep(Number(arg)); break
    case 'eval': console.log('eval:', JSON.stringify(await evaluate(arg))); break
    case 'text': console.log(await evaluate(`(document.querySelector(${JSON.stringify(arg || 'body')})?.innerText ?? 'NOT FOUND').slice(0, 1500)`)); break
    case 'scrollto': console.log('scrollto', arg, await evaluate(`(() => { const e = document.querySelector(${JSON.stringify(arg)}); if (!e) return 'NOT FOUND'; e.scrollIntoView({block: 'start'}); return 'ok' })()`)); await sleep(500); break
    case 'click': {
      const r = await evaluate(`(() => { const els = [...document.querySelectorAll('button, a, label, [role=radio]')]; const el = els.find(e => (e.innerText || '').trim().includes(${JSON.stringify(arg)})); if (!el) return 'NOT FOUND'; el.click(); return 'clicked ' + el.tagName })()`)
      console.log('click', arg, '->', r); if (r === 'NOT FOUND') failed = true
      await sleep(500); break
    }
    case 'waitfor': {
      let ok = false
      for (let i = 0; i < 60 && !ok; i++) { ok = await evaluate(`document.body.innerText.includes(${JSON.stringify(arg)})`); if (!ok) await sleep(500) }
      console.log('waitfor', arg, ok ? 'ok' : 'TIMEOUT'); if (!ok) failed = true; break
    }
    case 'shot': { const r = await send('Page.captureScreenshot', { format: 'png' }); fs.writeFileSync(arg, Buffer.from(r.result.data, 'base64')); console.log('shot', arg); break }
    default: console.log('unknown command:', cmd); failed = true
  }
}
ws.close()
process.exit(failed ? 1 : 0)
