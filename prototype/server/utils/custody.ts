import { readFileSync } from 'node:fs'
import type { OwnerPII, SealedOwner, StoredEnvelope, StoredShare } from '~~/server/utils/envelope'
import { loadEnvelope, storeEnvelope } from '~~/server/utils/envelope'
import { agencyHandles, descAnchorOf, loadRegistryKeys, ownerTagOf, pseudonymOf } from '~~/server/utils/handles'
import type { VehicleDescription } from '~~/server/utils/handles'
import type { VehicleDescriptionView } from '~~/shared/disclosure'
import { fleetOf as fleetFromFile, recordOwnership } from '~~/server/utils/ownerIndex'
import { publish } from '~~/server/utils/liveBus'

/**
 * Where each organisation's off-ledger data lives; the only module that knows.
 * services (default): one custody service per organisation; DMT and Police pass key
 * material to each other directly. files (fallback): offchain/dev-store on this host.
 */
export type CustodyOrg = 'dmt' | 'police' | 'customs' | 'vet' | 'insurer'
export const CUSTODY_ORGS: CustodyOrg[] = ['dmt', 'police', 'customs', 'vet', 'insurer']

export interface CustodyConfig {
  offchainDir: string
  registryKeys?: string
  custodyMode: string
  /** "dmt=http://localhost:8101,police=…" */
  custodyUrls: string
  /** portal.env from setup-env.sh: CUSTODY_TOKEN_<ORG> and CUSTODY_TOKEN_<ORG>_KEYING. */
  custodyEnvFile: string
}

export interface LedgerCheck { fn: string, peer: string, status: string, ms: number }

/** A refusal or failure from a custody service, with the service's own HTTP status and words. */
export class CustodyError extends Error {
  constructor(readonly statusCode: number, message: string, readonly org: CustodyOrg) {
    super(message)
  }
}

const ACTOR: Record<CustodyOrg, string> = { dmt: 'DMT', police: 'POLICE', customs: 'CUSTOMS', vet: 'VET', insurer: 'INSURER' }

export const servicesMode = (config: CustodyConfig) => config.custodyMode !== 'files'

const urlOf = (config: CustodyConfig, org: CustodyOrg) => {
  const pair = config.custodyUrls.split(',').map(p => p.split('=')).find(([k]) => k === org)
  if (!pair?.[1])
    throw new CustodyError(500, `no URL configured for the ${org} custody service`, org)
  return pair[1].replace(/\/$/, '')
}

/**
 * A credential by name: "dmt" … "insurer", or a desk's "<org>_keying" token.
 * Service-to-service credentials ("…_TO_…") are refused even if the file has them.
 */
let tokens: Record<string, string> | null = null
function tokenOf(config: CustodyConfig, name: string, org: CustodyOrg): string {
  if (!tokens) {
    tokens = Object.fromEntries(readFileSync(config.custodyEnvFile, 'utf8').split('\n')
      .map(l => l.match(/^CUSTODY_TOKEN_([A-Z_]+)=(\S+)$/))
      .filter((m): m is RegExpMatchArray => !!m && !m[1]!.includes('_TO_'))
      .map(m => [m[1]!.toLowerCase(), m[2]!]))
  }
  const t = tokens[name]
  if (!t)
    throw new CustodyError(500,
      `no ${name} token for the ${org} custody service in ${config.custodyEnvFile}; run offchain/custody/setup-env.sh`,
      org)
  return t
}

export async function custodyCall<T>(
  config: CustodyConfig, org: CustodyOrg, method: 'GET' | 'PUT' | 'POST', path: string, body?: unknown, credential: string = org,
): Promise<T> {
  // Configuration faults (no URL, no token file) are not outages: resolve them first.
  const url = `${urlOf(config, org)}${path}`
  const headers = { 'authorization': `Bearer ${tokenOf(config, credential, org)}`, 'content-type': 'application/json' }
  let res: Response
  try {
    res = await fetch(url, {
      method, headers, body: body === undefined ? undefined : JSON.stringify(body), signal: AbortSignal.timeout(15000),
    })
  }
  catch (error) {
    throw new CustodyError(502, `the ${org} custody service is unreachable: ${(error as Error).message}`, org)
  }
  const reply = await res.json().catch(() => ({})) as T & { message?: string }
  if (!res.ok) {
    const message = reply.message ?? `the ${org} custody service answered ${res.status}`
    // Only a custodian's decision is drawn as a refusal; 401 or 5xx is a fault, not a decision.
    if ([403, 404, 409].includes(res.status))
      publish({ kind: 'offledger', what: 'refused', service: org, actor: ACTOR[org], detail: message })
    throw new CustodyError(res.status, message, org)
  }
  return reply
}

/** Retries an idempotent store on an outage (5xx or unreachable), never on a refusal. */
async function withRetry<T>(store: () => Promise<T>): Promise<T> {
  for (let attempt = 1; ; attempt++) {
    try {
      return await store()
    }
    catch (error) {
      if (!(error instanceof CustodyError) || error.statusCode < 500 || attempt === 3)
        throw error
      await new Promise(r => setTimeout(r, 400 * attempt))
    }
  }
}

const describe = (checks: LedgerCheck[] = []) => checks.map(c => `${c.fn} ${c.status} on ${c.peer}, ${c.ms} ms`).join('; ')

interface Handoff { to: 'dmt' | 'police', ms: number, ledgerCheck?: LedgerCheck[] }

export interface Handles {
  vid: string
  idxChassis?: string
  idxEngine?: string
  pseudonym?: string
  descAnchor?: string
}

/**
 * Keyed identifiers for a desk write. Services mode asks the DMT service, so the
 * portal never holds k_v or k_own; a keying token is accepted for a chassis, never a NIC.
 */
export interface HandleInput {
  chassis?: string
  engine?: string
  make?: string
  nic?: string
  vid?: string
  /** With a description, the service also returns its keyed anchor. */
  model?: string
  year?: number
  engineCc?: number
  fuel?: string
}

export const deskCredential = (agency: string) => agency === 'DMT' || agency === 'ASSEMBLER' ? 'dmt' : `${agency.toLowerCase()}_keying`

export async function keyedHandles(config: CustodyConfig, input: HandleInput, agency = 'DMT'): Promise<Handles> {
  if (servicesMode(config))
    return custodyCall<Handles>(config, 'dmt', 'POST', '/handles', input, deskCredential(agency))
  const keys = await loadRegistryKeys(config)
  const out: Handles = input.chassis
    ? agencyHandles(keys, { chassis: input.chassis, engine: input.engine, make: input.make })
    : { vid: input.vid ?? '' }
  if (!out.vid)
    throw new CustodyError(400, 'chassis, or vid with nic, is required', 'dmt')
  if (input.nic)
    out.pseudonym = pseudonymOf(keys, input.nic, out.vid)
  if (input.model !== undefined && input.chassis)
    out.descAnchor = descAnchorOf(keys, out.vid, input as VehicleDescription)
  return out
}

// ---- Vehicle identity: plates, descriptions, the buyer's match, the theft owner check ----

const needsServices = (config: CustodyConfig, what: string) => {
  if (!servicesMode(config))
    throw new CustodyError(501, `${what} needs the custody services; the fallback file store does not keep plates or descriptions`, 'dmt')
}

/** An agency desk finds a vehicle by its plate, with its own credential. Never the buyer. */
export async function custodyPlateLookup(
  config: CustodyConfig, plate: string, agency: string, day?: number,
): Promise<string> {
  needsServices(config, 'A plate lookup')
  const reply = await custodyCall<{ vid: string }>(config, 'dmt', 'POST', '/plates/lookup', { plate, day }, deskCredential(agency))
  publish({ kind: 'offledger', what: 'lookup', service: 'dmt', actor: agency, detail: `vehicle ${reply.vid.slice(0, 8)}…` })
  return reply.vid
}

/** What the vehicle is (never its plate). The buyer page asks with its own credential. */
export async function custodyDescription(
  config: CustodyConfig, vid: string, credential: string,
): Promise<VehicleDescriptionView> {
  needsServices(config, 'A vehicle description')
  return custodyCall<VehicleDescriptionView>(config, 'dmt', 'POST', '/vehicles/description', { vid }, credential)
}

/** One bit: does the chassis typed from the car give this handle? */
export async function custodyMatch(
  config: CustodyConfig, vid: string, chassis: string, credential: string,
): Promise<boolean> {
  needsServices(config, 'The chassis match')
  const { match } = await custodyCall<{ match: boolean }>(config, 'dmt', 'POST', '/vehicles/match', { vid, chassis }, credential)
  publish({ kind: 'offledger', what: 'match', service: 'dmt', actor: credential === 'buyer_portal' ? 'BUYER' : 'DMT', detail: match ? 'matches' : 'does not match' })
  return match
}

/** The Police desk asks the DMT whether the reporter is the registered owner; on yes the DMT writes OWNER_CONFIRMED. */
export async function custodyTheftCheck(
  config: CustodyConfig, vid: string, nic: string, reportRef: string,
): Promise<boolean> {
  needsServices(config, 'The owner check')
  const reply = await custodyCall<{ confirmed: boolean, ledgerCheck?: LedgerCheck[] }>(
    config, 'dmt', 'POST', '/theft/check', { vid, nic, reportRef }, 'police_keying')
  publish({ kind: 'offledger', what: 'ownercheck', service: 'dmt', actor: 'POLICE', detail: `report ${reportRef}; ${reply.confirmed ? 'owner confirmed' : 'not the owner'}; ${describe(reply.ledgerCheck)}` })
  return reply.confirmed
}

/** After a registration: the DMT keeps the description and the plate in its own database. */
export async function storeVehicleIdentity(
  config: CustodyConfig, vid: string, d: VehicleDescription | null, plate: string | undefined, descAnchor?: string,
): Promise<string[]> {
  if (!servicesMode(config))
    return []
  const stored: string[] = []
  if (d && d.colour) {
    await withRetry(() => custodyCall(config, 'dmt', 'PUT', '/vehicles/description', { vid, ...d, descAnchor }))
    stored.push('description')
  }
  if (plate) {
    await withRetry(() => custodyCall(config, 'dmt', 'PUT', '/vehicles/plate', { vid, plate }))
    stored.push('plate')
  }
  if (stored.length)
    publish({ kind: 'offledger', what: 'store', service: 'dmt', actor: 'DMT', detail: `${stored.join(' and ')} for ${vid.slice(0, 8)}…` })
  return stored
}

/** After the ledger commits: the sealed record and DMT half to the DMT, the Police half to the Police. */
export async function storeOwner(config: CustodyConfig, sealed: SealedOwner, nic: string) {
  const vid = sealed.envelope.vid
  if (!servicesMode(config)) {
    await storeEnvelope(config.offchainDir, sealed)
    publish({ kind: 'offledger', what: 'seal', actor: 'DMT', detail: `vehicle ${vid.slice(0, 8)}… (file store on the portal host)` })
    await recordOwnership(config.offchainDir, ownerTagOf(await loadRegistryKeys(config), nic), vid)
    publish({ kind: 'offledger', what: 'index', actor: 'DMT' })
    return
  }
  // The DMT service hands the Police half on before storing its own rows, so a refusal anywhere stores nothing.
  const dmt = await withRetry(() => custodyCall<{ ledgerCheck?: LedgerCheck[], handoff?: Handoff }>(
    config, 'dmt', 'PUT', `/records/${vid}`, { envelope: sealed.envelope, shareDMT: sealed.shareDMT, sharePolice: sealed.sharePolice, nic }))
  publish({ kind: 'offledger', what: 'store', service: 'dmt', actor: 'DMT', detail: `vehicle ${vid.slice(0, 8)}…; ${describe(dmt.ledgerCheck)}` })
  publish({ kind: 'offledger', what: 'handoff', service: 'police', actor: 'DMT', detail: `Police half of ${vid.slice(0, 8)}…; ${describe(dmt.handoff?.ledgerCheck)}` })
}

/** Both parts for a reveal. Services mode: each custodian checks the case on the ledger itself first. */
export interface OwnerParts { envelope: StoredEnvelope, shareDMT: StoredShare, sharePolice: StoredShare }

/** Files mode only: both parts from the file store on the portal host, which decrypts itself. */
export async function loadOwner(config: CustodyConfig, vid: string, caseId: string): Promise<OwnerParts> {
  const parts = await loadEnvelope(config.offchainDir, vid)
  publish({ kind: 'offledger', what: 'shares', actor: 'POLICE', detail: `case ${caseId} (file store on the portal host)` })
  return parts
}

export interface Revealed { owner: OwnerPII, anchor: string, accessLog: string[] }

/**
 * Services mode: the Police service checks the case, fetches the DMT's part,
 * decrypts and commits IDENTITY_REVEAL before returning. The portal sees only the result.
 */
export async function revealOwner(config: CustodyConfig, vid: string, caseId: string): Promise<Revealed> {
  const reply = await custodyCall<Revealed & { ledgerCheck?: LedgerCheck[], handoff?: Handoff }>(
    config, 'police', 'POST', `/reveal/${vid}?case=${encodeURIComponent(caseId)}`)
  const [caseCheck, anchorCheck, audit] = reply.ledgerCheck ?? []
  publish({ kind: 'offledger', what: 'reveal', service: 'police', actor: 'POLICE', detail: `case ${caseId}; ${describe(caseCheck ? [caseCheck] : [])}` })
  publish({ kind: 'offledger', what: 'handoff', service: 'dmt', actor: 'POLICE', detail: `case ${caseId}; ${describe(reply.handoff?.ledgerCheck)}` })
  publish({ kind: 'offledger', what: 'opened', service: 'police', actor: 'POLICE', detail: `case ${caseId}; ${describe([anchorCheck, audit].filter((c): c is LedgerCheck => !!c))}` })
  return { owner: reply.owner, anchor: reply.anchor, accessLog: reply.accessLog ?? [] }
}

/**
 * The owner's fleet from the DMT's owner index. With a case the DMT service checks
 * it on the ledger; without one (the DMT's own predicate) it relies on the token.
 */
export async function custodyFleet(config: CustodyConfig, vid: string, caseId: string, purpose: 'approve' | 'read' | 'predicate'): Promise<string[]> {
  if (!servicesMode(config))
    return fleetFromFile(config.offchainDir, vid)
  // The Police read goes through the Police service, which fetches the list from the DMT service itself.
  if (purpose === 'read') {
    const reply = await custodyCall<{ fleet: string[], ledgerCheck?: LedgerCheck[], handoff?: Handoff }>(
      config, 'police', 'GET', `/fleet/${vid}?case=${encodeURIComponent(caseId)}`)
    publish({ kind: 'offledger', what: 'fleet', service: 'police', actor: 'POLICE', detail: `case ${caseId}; ${describe(reply.ledgerCheck)}` })
    publish({ kind: 'offledger', what: 'handoff', service: 'dmt', actor: 'POLICE', detail: `fleet, case ${caseId}; ${describe(reply.handoff?.ledgerCheck)}` })
    return reply.fleet
  }
  const query = caseId ? `?case=${encodeURIComponent(caseId)}&purpose=${purpose}` : `?purpose=${purpose}`
  const { fleet, ledgerCheck } = await custodyCall<{ fleet: string[], ledgerCheck?: LedgerCheck[] }>(
    config, 'dmt', 'GET', `/records/${vid}/fleet${query}`)
  publish({ kind: 'offledger', what: 'fleet', service: 'dmt', actor: 'DMT', detail: caseId ? `case ${caseId}; ${describe(ledgerCheck)}` : 'DMT owner-scope predicate' })
  return fleet
}

/** An agency's own internal fields for a committed record; kept only in services mode. */
export async function storeInternal(config: CustodyConfig, org: 'customs' | 'vet' | 'insurer', recordId: string, vid: string, fields: Record<string, unknown>): Promise<boolean> {
  if (!servicesMode(config) || !Object.values(fields).some(v => v !== '' && v !== undefined))
    return false
  await custodyCall(config, org, 'PUT', `/records/${encodeURIComponent(recordId)}`, { vid, fields })
  publish({ kind: 'offledger', what: 'store', service: org, actor: ACTOR[org], detail: `record ${recordId}` })
  return true
}

export interface CustodyHealth { mode: 'services' | 'files', up: number, services: Partial<Record<CustodyOrg, { up: boolean, db?: boolean, ledger?: boolean, ms?: number }>> }

export async function custodyHealth(config: CustodyConfig): Promise<CustodyHealth> {
  if (!servicesMode(config))
    return { mode: 'files', up: 0, services: {} }
  const entries = await Promise.all(CUSTODY_ORGS.map(async (org) => {
    try {
      const res = await fetch(`${urlOf(config, org)}/health`, { signal: AbortSignal.timeout(1500) })
      const h = await res.json() as { db?: boolean, ledger?: boolean, ms?: number }
      return [org, { up: res.ok, ...h }] as const
    }
    catch {
      return [org, { up: false }] as const
    }
  }))
  return { mode: 'services', up: entries.filter(([, h]) => h.up).length, services: Object.fromEntries(entries) }
}
