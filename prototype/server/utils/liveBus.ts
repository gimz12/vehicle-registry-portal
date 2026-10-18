import type { LiveEvent } from '~~/shared/live'

// In-process fan-out from the Fabric helpers to every open /api/live stream.
// Limitation: one process, one bus; a message broker if the portal is ever scaled out.
type Listener = (e: LiveEvent) => void
const listeners = new Set<Listener>()
const recent: LiveEvent[] = []

// Omit distributed over the union, so each event kind keeps its own fields.
type Publishable = LiveEvent extends infer E ? E extends LiveEvent ? Omit<E, 't'> & { t?: number } : never : never

export function publish(e: Publishable) {
  const event = { t: Date.now(), ...e } as LiveEvent
  recent.push(event)
  if (recent.length > 60)
    recent.shift()
  for (const fn of listeners)
    fn(event)
}

export function subscribe(fn: Listener): () => void {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export const recentEvents = () => [...recent]
