#!/usr/bin/env bash
# Start/stop the Wails dev app (Go backend + Vite frontend) and a headless
# Chromium with the DevTools protocol on :9222, for driver.mjs.
# Usage: app.sh start | stop | status     (run from the repo root)
set -u
ROOT=$(git rev-parse --show-toplevel)
RUN=${RUN_DIR:-$HOME/.cache/cycling-sim-run}
mkdir -p "$RUN"
export PATH=$HOME/go_install/go/bin:$HOME/go/bin:$PATH

case "${1:-}" in
start)
  if ! curl -sf http://localhost:34115 >/dev/null; then
    tmux kill-session -t cs-wails 2>/dev/null
    tmux new-session -d -s cs-wails "cd '$ROOT' && wails dev -tags webkit2_41 2>&1 | tee '$RUN/wails.log'"
    timeout 300 bash -c 'until curl -sf http://localhost:34115 >/dev/null; do sleep 2; done' \
      || { echo "wails dev did not come up; see $RUN/wails.log"; exit 1; }
  fi
  if ! curl -sf http://127.0.0.1:9222/json/version >/dev/null; then
    # snap Chromium: keep the profile under $HOME (snap confinement)
    nohup chromium-browser --headless=new --no-sandbox --remote-debugging-port=9222 \
      --user-data-dir="$RUN/profile" about:blank >"$RUN/chrome.log" 2>&1 &
    echo $! >"$RUN/chrome.pid"
    timeout 30 bash -c 'until curl -sf http://127.0.0.1:9222/json/version >/dev/null; do sleep 1; done' \
      || { echo "chromium did not start; see $RUN/chrome.log"; exit 1; }
  fi
  echo "app: http://localhost:34115  cdp: :9222  logs: $RUN"
  ;;
stop)
  tmux kill-session -t cs-wails 2>/dev/null
  # kill by port/pid file, never `pkill -f <pattern>` (it matches this shell)
  lsof -ti:34115,3000,9222 -sTCP:LISTEN 2>/dev/null | xargs -r kill
  [ -f "$RUN/chrome.pid" ] && kill "$(cat "$RUN/chrome.pid")" 2>/dev/null; rm -f "$RUN/chrome.pid"
  echo stopped
  ;;
status)
  curl -s -o /dev/null -w "app %{http_code}\n" http://localhost:34115
  curl -s -o /dev/null -w "cdp %{http_code}\n" http://127.0.0.1:9222/json/version
  ;;
*) echo "usage: $0 start|stop|status"; exit 2 ;;
esac
