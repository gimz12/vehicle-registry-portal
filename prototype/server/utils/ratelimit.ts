import type { H3Event } from 'h3'

/**
 * A sliding-window allowance per address (and optional scope), so one client cannot
 * sweep chassis numbers, descriptions or plates. Returns the seconds to wait, or 0.
 */
// Limitation: in-memory, per process; a shared store if the portal is ever scaled out.
const buckets = new Map<string, number[]>()
const MAX_KEYS = 10_000

export function overLimit(event: H3Event, name: string, limit: number, windowSeconds: number, scope = ''): number {
  const ip = getRequestIP(event) ?? 'unknown'
  const key = `${name}:${ip}:${scope}`
  const now = Date.now()
  const windowMs = windowSeconds * 1000
  if (buckets.size > MAX_KEYS) {
    for (const [k, times] of buckets) {
      if (!times.length || now - times.at(-1)! >= windowMs)
        buckets.delete(k)
    }
  }
  const recent = (buckets.get(key) ?? []).filter(t => now - t < windowMs)
  if (recent.length >= limit) {
    buckets.set(key, recent)
    return Math.ceil((recent[0]! + windowMs - now) / 1000)
  }
  recent.push(now)
  buckets.set(key, recent)
  return 0
}
