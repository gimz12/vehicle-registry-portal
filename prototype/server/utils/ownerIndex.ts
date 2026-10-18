import { promises as fs } from 'node:fs'
import { dirname, join } from 'node:path'

/**
 * The DMT's owner index (files mode): owner tag (HMAC(k_own, NIC)) to the vehicles
 * that owner holds, plus the reverse map. Writes follow ledger commits, never precede them.
 */
export interface OwnerIndex {
  owners: Record<string, string[]>
  vehicles: Record<string, string>
}

const indexPath = (store: string) => join(store, 'custody_dmt', 'owner_index.json')

export async function loadOwnerIndex(store: string): Promise<OwnerIndex> {
  let raw: string
  try {
    raw = await fs.readFile(indexPath(store), 'utf8')
  }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code === 'ENOENT')
      return { owners: {}, vehicles: {} }
    throw error
  }
  // A damaged index throws: reading it as empty would let the next write erase it.
  const parsed = JSON.parse(raw) as Partial<OwnerIndex>
  return { owners: parsed.owners ?? {}, vehicles: parsed.vehicles ?? {} }
}

async function saveOwnerIndex(store: string, index: OwnerIndex) {
  await fs.mkdir(dirname(indexPath(store)), { recursive: true })
  const tmp = `${indexPath(store)}.tmp`
  await fs.writeFile(tmp, `${JSON.stringify(index, null, 2)}\n`, { mode: 0o600 })
  await fs.rename(tmp, indexPath(store)) // atomic: a crash leaves the old index, never half a file
}

// Limitation: one in-process queue; enough for one portal process, a file lock if several ever write.
let queue: Promise<unknown> = Promise.resolve()

/** Pure update: the vehicle now belongs to ownerTag, and to no one else. */
export function assign(index: OwnerIndex, ownerTag: string, vid: string): OwnerIndex {
  const owners: Record<string, string[]> = {}
  for (const [tag, vids] of Object.entries(index.owners)) {
    const kept = vids.filter(v => v !== vid)
    if (kept.length > 0)
      owners[tag] = kept
  }
  owners[ownerTag] = [...new Set([...(owners[ownerTag] ?? []), vid])].sort()
  return { owners, vehicles: { ...index.vehicles, [vid]: ownerTag } }
}

/** The vehicle's current owner's fleet; the vehicle alone when the index has no entry. */
export function fleetIn(index: OwnerIndex, vid: string): string[] {
  const tag = index.vehicles[vid]
  return tag && index.owners[tag]?.includes(vid) ? [...index.owners[tag]] : [vid]
}

export function recordOwnership(store: string, ownerTag: string, vid: string): Promise<void> {
  const next = queue.then(async () => saveOwnerIndex(store, assign(await loadOwnerIndex(store), ownerTag, vid)))
  queue = next.catch(() => {}) // one failed write must not block the next
  return next
}

export async function fleetOf(store: string, vid: string): Promise<string[]> {
  return fleetIn(await loadOwnerIndex(store), vid)
}
