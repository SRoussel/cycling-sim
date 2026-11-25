// The one place the frontend touches the Go backend. Pages import calls,
// backend types and events from here, never from wailsjs directly, so the
// transport (Wails bindings today) can change in this file alone.
export * from '../wailsjs/go/app/App'
export { app, director, simulation } from '../wailsjs/go/models'
export { EventsOn } from '../wailsjs/runtime/runtime'
