---
name: run-cycling-sim
description: Build, run, and drive the cycling-sim app (Wails desktop GUI and Go CLI). Use when asked to start or run the app, click through a live race, take a screenshot of the GUI, check a frontend/team-control change in the real app, or smoke-test the CLI.
---

The GUI is a Wails app (Go backend, React/Vite frontend in `gui/`). Agents drive it headless: `app.sh start` runs `wails dev` (which serves the app with live Go bindings at http://localhost:34115) plus a headless Chromium with DevTools on :9222, and `driver.mjs` sends it line commands (nav / click / waitfor / eval / shot). The CLI is driven directly.

All paths are relative to the repo root.

## Prerequisites

These were already installed on this machine (WSL2 Ubuntu). Nothing extra was needed:
- Go at `~/go_install/go/bin` (app.sh adds it and `~/go/bin` to PATH).
- `wails` CLI at `~/go/bin/wails`.
- Node 22 (the driver uses built-in `fetch`/`WebSocket`, no npm packages).
- `chromium-browser` (snap), `tmux`, `lsof`.
- `gui/node_modules` installed (`cd gui && npm install` if missing).

## Run the GUI (agent path)

```bash
S=.claude/skills/run-cycling-sim
# The GUI opens the real DB (~/.config/CyclingSim) unless told otherwise:
export CYCLING_SIM_DB=$HOME/.cache/cycling-sim-run/gui-check.db
sqlite3 ~/.config/CyclingSim/cycling_sim.db ".backup '$CYCLING_SIM_DB'"
$S/app.sh start      # first start builds Go + Vite: ~1-2 min; prints "app: http://localhost:34115 ..."
mkdir -p /tmp/cs-shots
node $S/driver.mjs <<'EOF'
nav http://localhost:34115/
waitfor Milan-San Remo
click Milan-San Remo
waitfor Watch the race
click Direct a team
wait 1500
click Start the race
waitfor Chase breaks
click Don't chase
wait 3000
shot /tmp/cs-shots/live.png
EOF
$S/app.sh stop
```

What it does: on a scratch copy of the DB, it opens the home page, picks a race, takes charge of a team ("Direct a team"), starts the live race, sets a team order and screenshots it. The driver exits 1 if any `click` or `waitfor` fails. **Look at the screenshot.**

Driver commands, one per line, read from stdin or a file argument:

| Command | What it does |
|---|---|
| `nav <url>` | Navigates and waits 3 s |
| `click <text>` | Clicks the first button/link/label containing the text |
| `waitfor <text>` | Waits up to 30 s for the text to appear |
| `wait <ms>` | Sleeps |
| `eval <js>` | Evaluates JS and prints the result as JSON |
| `scrollto <css>` | Scrolls an element into view |
| `text [css]` | Prints innerText |
| `shot <png>` | Takes a 1500×1000 screenshot |

Logs and the Chromium profile are in `~/.cache/cycling-sim-run/` (`wails.log`, `chrome.log`). `app.sh status` shows whether both ports are up. The app keeps running between driver calls, so you can drive it step by step.

Pages: home `/`, then click a race card for race setup, then Start for `/simulation/:id`. Stage races go through their own cards to `/stage-race/:id` and then `/stage-race-sim/:id`. The live-race panels are in `gui/src/components/race/`, and their state is in `gui/src/sim/useTeamControl.ts`.

## Run the CLI

Use a scratch DB so you don't touch the saved results in the real DB (`~/.config/CyclingSim/cycling_sim.db`, the default; the GUI under `app.sh` uses it too):

```bash
export PATH=$HOME/go_install/go/bin:$PATH
go build -o bin/cycling-sim ./cmd/cycling-sim/
export CYCLING_SIM_DB=$HOME/.cache/cycling-sim-run/smoke.db
bin/cycling-sim seed          # "Seed complete!"
bin/cycling-sim races
bin/cycling-sim run 1 --seed 1   # Milan-San Remo, prints results table
```

## Run (human path)

`wails dev -tags webkit2_41` opens a native window (WSLg shows it). You can also open http://localhost:34115 in any browser while it runs. Neither is usable by an agent headless; use app.sh and the driver.

## Test

```bash
export PATH=$HOME/go_install/go/bin:$PATH
go test ./... -tags webkit2_41
cd gui && npx tsc --noEmit
```

## Gotchas

- **Use :34115, not :3000.** :3000 is bare Vite, where `window.go` is missing, so every backend call fails. :34115 is Wails' dev server and injects the bindings, so the app is fully functional in plain Chromium.
- **Don't deep-link to `/race-setup/:id`.** The page renders but shows no race info or start list, because the race is passed in router state from the home card. Always `nav /` and `click` the race card.
- **The Team Control panel is below the fold** at 1000 px height. `scrollto .team-orders-panel` (or `.team-controls`) before `shot`.
- **After adding a Go method the GUI calls,** update `gui/wailsjs/go/app/App.js` and `App.d.ts` (or run `wails generate module -tags webkit2_41`); pages import them through `gui/src/api.ts`. Wails dev regenerates them, but the typecheck runs before that. Hand-editing them in the existing format works.
- **`wails dev` logs `ERR | Unknown message from front end: runtime:ready`** whenever the browser connects. It's harmless.
- **Never stop processes with `pkill -f` or `pgrep -f` on a pattern** like `remote-debugging-port`: it matched the agent's own shell command and killed it (exit 144). app.sh stops by port and pid file.
- **No `chromium-cli` here.** That's why `driver.mjs` exists: a ~70-line raw DevTools-protocol client.
- **The live race runs at 0.5× by default.** Text such as the rider rows changes under you, so `waitfor` the thing you need rather than sleeping.

## Troubleshooting

- **`ERR_CONNECTION_REFUSED` on :34115 mid-session** (the screenshot shows "This site can't be reached"): `wails dev` had exited ("Development mode exited" at the end of `wails.log`). Run `app.sh stop; app.sh start`.
- **`no page target on :9222`:** Chromium isn't running. Run `app.sh start`.
