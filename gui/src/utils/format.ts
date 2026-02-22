/**
 * Format a seconds value as H:MM:SS
 */
export function formatTime(seconds: number): string {
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`
}

/**
 * Format a gap in seconds as "+M:SS" or "+Xs", or "—" for zero
 */
export function formatGap(seconds: number): string {
  if (seconds === 0) return '—'
  const m = Math.floor(seconds / 60)
  const s = Math.floor(seconds % 60)
  if (m > 0) {
    return `+${m}:${s.toString().padStart(2, '0')}`
  }
  return `+${s}s`
}

/**
 * Format watts as "NNN W"
 */
export function formatPower(watts: number): string {
  return `${Math.round(watts)} W`
}

/**
 * Format a decimal ratio (0–1) as a percentage string "NN%"
 */
export function formatPct(ratio: number): string {
  return `${Math.round(ratio * 100)}%`
}
