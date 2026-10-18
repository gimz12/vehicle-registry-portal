import { createHmac, randomBytes } from 'node:crypto'
import { promises as fs } from 'node:fs'
import { join } from 'node:path'

/**
 * TypeScript twin of the Go identity package's keyed identifiers, byte for byte:
 * canonicalise (A–Z, 0–9), then HMAC-SHA256 over length-prefixed parts "len:part|".
 * k_v: handle and indices. k_own: owner pseudonym and owner tag (never sent to a client or the ledger).
 */

export interface RegistryKeys { kv: Buffer, kown: Buffer }

const NULLISH = new Set(['', '0', 'NA', 'NONE', 'NULL', 'UNKNOWN'])

export function canonical(s: string): string {
  // Per character, as Go's strings.ToUpper does: JavaScript turns ß into "SS",
  // Go keeps one rune (then dropped), so a multi-character expansion is dropped.
  return [...s]
    .map((c) => {
      const u = c.toUpperCase()
      return u.length === 1 ? u : ''
    })
    .join('')
    .replace(/[^A-Z0-9]/g, '')
}

export function canonicalIdentifier(s: string): string {
  const value = canonical(s)
  if (NULLISH.has(value))
    throw new Error('identifier is empty after canonicalisation')
  return value
}

export function canonicalNIC(s: string): string {
  const value = canonical(s)
  if (/^\d{12}$/.test(value) || /^\d{9}[VX]$/.test(value))
    return value
  throw new Error('NIC must be 12 digits or 9 digits followed by V/X')
}

function mac(key: Buffer, ...parts: string[]): string {
  const h = createHmac('sha256', key)
  for (const p of parts)
    h.update(`${Buffer.byteLength(p)}:${p}|`)
  return h.digest('hex')
}

export const vidOf = (keys: RegistryKeys, chassis: string) => mac(keys.kv, 'vid', canonicalIdentifier(chassis))
export const idxChassisOf = (keys: RegistryKeys, chassis: string) => mac(keys.kv, 'chassis', canonicalIdentifier(chassis))
export function idxEngineOf(keys: RegistryKeys, make: string, engine: string) {
  return mac(keys.kv, 'eng', canonicalIdentifier(make), canonicalIdentifier(engine))
}
export const pseudonymOf = (keys: RegistryKeys, nic: string, vid: string) => mac(keys.kown, 'p', canonicalNIC(nic), vid)
/** Server-internal: keys the owner index. Never returned by any route. */
export const ownerTagOf = (keys: RegistryKeys, nic: string) => mac(keys.kown, 'owner', canonicalNIC(nic))

/** What a vehicle is, as Customs declares it and the DMT records it. Not personal data. */
export interface VehicleDescription {
  make: string
  model: string
  year: number
  engineCc: number
  fuel: string
  colour?: string
}
export const FUELS = ['PETROL', 'DIESEL', 'HYBRID', 'ELECTRIC', 'OTHER'] as const

/** Checks a description's fields; returns a reason, or null if it is well-formed. */
export function descriptionProblem(d: Partial<VehicleDescription>): string | null {
  if (!d.make || canonical(String(d.make)).length < 2)
    return 'make is required'
  if (!d.model || canonical(String(d.model)).length < 1)
    return 'model is required'
  if (!Number.isInteger(Number(d.year)) || Number(d.year) < 1950 || Number(d.year) > 2100)
    return 'year must be a whole number from 1950 to 2100'
  if (!Number.isInteger(Number(d.engineCc)) || Number(d.engineCc) < 0 || Number(d.engineCc) > 20000)
    return 'engine capacity must be a whole number of cc from 0 to 20000 (0 for electric)'
  if (!FUELS.includes(canonical(String(d.fuel ?? '')) as typeof FUELS[number]))
    return `fuel must be one of ${FUELS.join(', ')}`
  return null
}

/** Description fingerprint, keyed and bound to the vehicle. Colour is left out: a car may be repainted legally. */
export function descAnchorOf(keys: RegistryKeys, vid: string, d: VehicleDescription): string {
  return mac(keys.kv, 'desc', vid, canonicalIdentifier(d.make), canonicalIdentifier(d.model),
    String(Number(d.year)), String(Number(d.engineCc)), canonical(d.fuel))
}

/** A number plate as stored and looked up: letters and digits only, 3–12 of them. */
export function canonicalPlate(s: string): string {
  const value = canonical(s)
  if (value.length < 3 || value.length > 12)
    throw new Error('a number plate has 3 to 12 letters and digits')
  return value
}

export interface AgencyHandles { vid: string, idxChassis: string, idxEngine?: string }

/** Asset-scope identifiers only: nothing from a NIC, so it cannot act as an ownership oracle. */
export function agencyHandles(
  keys: RegistryKeys,
  input: { chassis: string, engine?: string, make?: string },
): AgencyHandles {
  const out: AgencyHandles = { vid: vidOf(keys, input.chassis), idxChassis: idxChassisOf(keys, input.chassis) }
  if (input.engine && input.make)
    out.idxEngine = idxEngineOf(keys, input.make, input.engine)
  return out
}

export interface DerivedHandles { vid: string, idxChassis: string, idxEngine: string, pseudonym?: string }

export interface PhysicalIdentifiers { chassis: string, engine: string, make: string, nic?: string }

export function deriveHandles(keys: RegistryKeys, input: PhysicalIdentifiers): DerivedHandles {
  const vid = vidOf(keys, input.chassis)
  const out: DerivedHandles = {
    vid,
    idxChassis: idxChassisOf(keys, input.chassis),
    idxEngine: idxEngineOf(keys, input.make, input.engine),
  }
  if (input.nic)
    out.pseudonym = pseudonymOf(keys, input.nic, vid)
  return out
}

/**
 * Files-mode keys: NUXT_FABRIC_REGISTRY_KEYS="<kv hex>:<kown hex>", else a key file in the dev store.
 * Limitation: the portal holds the keys here; a deployment keeps them in an HSM.
 */
let cached: RegistryKeys | null = null
export async function loadRegistryKeys(config: { offchainDir: string, registryKeys?: string }): Promise<RegistryKeys> {
  if (cached)
    return cached
  if (config.registryKeys) {
    const [kv, kown] = config.registryKeys.split(':')
    if (!kv || !kown || kv.length !== 64 || kown.length !== 64)
      throw new Error('NUXT_FABRIC_REGISTRY_KEYS must be "<32-byte hex>:<32-byte hex>"')
    cached = { kv: Buffer.from(kv, 'hex'), kown: Buffer.from(kown, 'hex') }
    return cached
  }
  const path = join(config.offchainDir, 'registry-keys.json')
  // 'wx' stops two first requests creating two key pairs. Any other failure throws:
  // fresh keys would silently re-map every handle already on the ledger.
  try {
    const fresh = { kv: randomBytes(32).toString('hex'), kown: randomBytes(32).toString('hex') }
    await fs.mkdir(config.offchainDir, { recursive: true })
    await fs.writeFile(path, `${JSON.stringify(fresh, null, 2)}\n`, { mode: 0o600, flag: 'wx' })
  }
  catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'EEXIST')
      throw error
  }
  const stored = JSON.parse(await fs.readFile(path, 'utf8')) as { kv: string, kown: string }
  if (!/^[0-9a-f]{64}$/.test(stored.kv) || !/^[0-9a-f]{64}$/.test(stored.kown))
    throw new Error(`${path} must hold 32-byte hex kv and kown`)
  cached = { kv: Buffer.from(stored.kv, 'hex'), kown: Buffer.from(stored.kown, 'hex') }
  return cached
}
