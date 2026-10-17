import { timingSafeEqual } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { createServer } from 'node:http'
import type { IncomingMessage, ServerResponse } from 'node:http'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import pg from 'pg'
import { anchorOf, openOwner } from '../../server/utils/envelope'
import type { StoredEnvelope, StoredShare } from '../../server/utils/envelope'
import { agencyHandles, canonicalPlate, descAnchorOf, descriptionProblem, ownerTagOf, pseudonymOf, vidOf } from '../../server/utils/handles'
import type { RegistryKeys, VehicleDescription } from '../../server/utils/handles'
import { fabricLedger } from './ledger'
import type { Ledger, LedgerCheck } from './ledger'

/**
* One organisation's custody service (ORG=dmt|police|customs|vet|insurer), each
* with its own PostgreSQL database and bearer token. DMT holds sealed owner
* records, its key half, the owner index and the pseudonymisation keys; Police
* holds its key half; the others hold their internal fields. DMT and Police talk
* directly (PARTNER_URL, PARTNER_TOKEN), never through the portal.
*
* Every route except GET /health needs a bearer token naming the caller (own
* org, or one of CUSTODY_CALLER_TOKENS). Request bodies are never logged.
*/
export type Org = 'dmt' | 'police' | 'customs' | 'vet' | 'insurer'
export interface Db { query: (text: string, params?: unknown[]) => Promise<{ rows: Record<string, unknown>[] }> }
/** The other custodian, reached directly with this service's own credential for it. */
export interface Partner { name: string, url: string, token: string, call?: typeof fetch }
export interface Deps {
  org: Org
  token: string
  db: Db
  ledger?: Ledger
  keys?: RegistryKeys
  /** token -> caller name, for callers other than this organisation's own portal. */
  callers?: Record<string, string>
  partner?: Partner
}
export interface Reply { status: number, body: unknown }

export class HttpError extends Error {
  constructor(readonly status: number, message: string) {
    super(message)
  }
}

const HEX64 = /^[0-9a-f]{64}$/
/** The chaincode's not-found text; anything else is a ledger failure. */
const UNKNOWN = /registry: record not found/
const RECORD = /^[A-Za-z0-9/_.:-]{1,80}$/
const CASE = /^[A-Za-z0-9/_-]{1,64}$/

/** The internal fields each agency keeps, as JSON name → column. */
export const INTERNAL = {
  customs: { table: 'import_record', fields: { declarant: 'declarant', cusdecNo: 'cusdec_no', valuationLkr: 'valuation_lkr' } },
  vet: { table: 'test_record', fields: { technician: 'technician', certificateNo: 'certificate_no' } },
  insurer: {
    table: 'claim_record',
    fields: { policyNo: 'policy_no', claimFile: 'claim_file', assessorContact: 'assessor_contact', reserveLkr: 'reserve_lkr' },
  },
} as const

const b64 = (v: unknown) => Buffer.from(v as Buffer).toString('base64')
const bytes = (s: unknown, what: string) => {
  if (typeof s !== 'string' || !s)
    throw new HttpError(400, `${what} is required (base64)`)
  return Buffer.from(s, 'base64')
}

/** One release_log row; every row names the caller. */
async function logRelease(deps: Deps, item: string, vid: string, caseId: string | null, outcome: string, caller = '') {
  const who = !caller ? '' : /^refused/.test(outcome) ? ` (asked by ${caller})` : ` to ${caller}`
  await deps.db.query('INSERT INTO release_log (item, vid, case_id, outcome) VALUES ($1, $2, $3, $4)', [item, vid, caseId, outcome + who])
}

/** Each route names the callers it admits; anyone else is refused before anything is read. */
function allow(caller: string, ...names: string[]) {
  if (!names.includes(caller))
    throw new HttpError(403, `this route is for ${names.join(' or ')}, not ${caller}`)
}

/** A call to the other custodian; its refusals keep their status. */
async function partnerCall<T>(deps: Deps, method: 'GET' | 'PUT', path: string, body?: unknown): Promise<T & { ms: number }> {
  const p = deps.partner
  if (!p)
    throw new HttpError(500, `no partner custody service is configured for the ${deps.org} service`)
  const started = Date.now()
  let res: Response
  try {
    res = await (p.call ?? fetch)(`${p.url.replace(/\/$/, '')}${path}`, {
      method,
      headers: { 'authorization': `Bearer ${p.token}`, 'content-type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(20000),
    })
  }
  catch (error) {
    throw new HttpError(502, `the ${p.name} custody service is unreachable: ${(error as Error).message}`)
  }
  const reply = await res.json().catch(() => ({})) as T & { message?: string }
  if (!res.ok) {
    // 401 or 5xx at the partner is a fault here, not a decision.
    const status = res.status >= 500 || res.status === 401 ? 502 : res.status
    throw new HttpError(status, `${p.name} custody service: ${reply.message ?? `answered ${res.status}`}`)
  }
  return { ...reply, ms: Date.now() - started }
}

/** The custodian's own ledger check of a case's vehicle and state; every outcome is logged. */
async function requireCase(deps: Deps, item: string, vid: string, caseId: string, allowed: string[], caller = '') {
  if (!CASE.test(caseId)) {
    await logRelease(deps, item, vid, null, 'refused: no case', caller)
    throw new HttpError(403, `the ${item} is released only under a case; none was given`)
  }
  let c
  try {
    c = await deps.ledger!.caseFor(caseId)
  }
  catch (error) {
    const message = (error as Error).message
    const unknown = UNKNOWN.test(message)
    await logRelease(deps, item, vid, caseId, unknown ? 'refused: unknown case' : 'refused: ledger unreachable', caller)
    if (unknown)
      throw new HttpError(404, `case ${caseId} is not on the ledger`)
    throw new HttpError(502, `the ledger could not be read, so nothing is released: ${message}`)
  }
  if (c.vid !== vid || !allowed.includes(c.status)) {
    await logRelease(deps, item, vid, caseId, `refused: ${c.status}${c.vid === vid ? '' : ' for another vehicle'}`, caller)
    throw new HttpError(403, `the ledger shows case ${caseId} as ${c.status} for ${c.vid === vid ? 'this' : 'another'} vehicle; nothing released`)
  }
  return c
}

async function fleetOf(deps: Deps, vid: string): Promise<string[]> {
  const { rows } = await deps.db.query(
    'SELECT vid FROM owner_index WHERE owner_tag = (SELECT owner_tag FROM owner_index WHERE vid = $1) ORDER BY vid', [vid])
  return rows.length ? rows.map(r => String(r.vid)) : [vid]
}

async function health(deps: Deps): Promise<Reply> {
  const started = Date.now()
  let db = false
  let ledger: boolean | undefined
  try {
    await deps.db.query('SELECT 1')
    db = true
  }
  catch { /* reported as db: false */ }
  if (deps.ledger) {
    try {
      await deps.ledger.caseFor('HEALTH/PROBE')
      ledger = true
    }
    catch (error) {
      ledger = UNKNOWN.test((error as Error).message) // the chaincode answered
    }
  }
  return { status: db && ledger !== false ? 200 : 503, body: { org: deps.org, db, ledger, ms: Date.now() - started } }
}

// ---- DMT ----

type Handler = (
  deps: Deps, method: string, parts: string[], query: URLSearchParams, body: Body, caller: string,
) => Promise<Reply>
type Body = Record<string, unknown>

const DESKS = ['police-desk', 'customs-desk', 'vet-desk', 'insurer-desk']
/** The buyer page's own credential: the description and the one-bit match, nothing else. */
const BUYER = 'buyer-portal'
/** Wrong NICs allowed per theft report number before a new report is needed. */
const MAX_REFUSED = 3
/** One owner check at a time per vehicle, so parallel guesses cannot all beat the refusal count. */
// Limitation: in-process queue; one DMT service process in the laboratory.
const checking = new Map<string, Promise<unknown>>()
function oneAtATime<T>(key: string, run: () => Promise<T>): Promise<T> {
  const next = (checking.get(key) ?? Promise.resolve()).catch(() => {}).then(run)
  checking.set(key, next)
  return next.finally(() => {
    if (checking.get(key) === next)
      checking.delete(key)
  })
}

/** A vehicle handle from a JSON body, or 400. Identifiers travel only in bodies, never in logged paths. */
function bodyVid(body: Body): string {
  const vid = String(body.vid ?? '').toLowerCase()
  if (!HEX64.test(vid))
    throw new HttpError(400, 'vid must be a 64-character hexadecimal handle')
  return vid
}

/** The vehicle's own facts on the DMT's peer; a failed read is a 502, never a bare 500. */
async function readFacts(deps: Deps, vid: string, reportRef?: string) {
  try {
    return await deps.ledger!.vehicleFacts(vid, reportRef)
  }
  catch (error) {
    throw new HttpError(502, `the ledger could not be read, so nothing is answered: ${(error as Error).message}`)
  }
}

function bodyDescription(body: Body): VehicleDescription {
  const d = {
    make: String(body.make ?? ''), model: String(body.model ?? ''), year: Number(body.year),
    engineCc: Number(body.engineCc), fuel: String(body.fuel ?? '').toUpperCase(), colour: String(body.colour ?? ''),
  }
  const problem = descriptionProblem(d)
  if (problem)
    throw new HttpError(400, problem)
  return d
}

/** Vehicle identity routes on the DMT service; null when the path is not one of them. */
async function vehicleIdentity(
  deps: Deps, method: string, parts: string[], body: Body, caller: string,
): Promise<Reply | null> {
  const [resource, action] = parts
  if (parts.length !== 2 || !['plates', 'vehicles', 'theft'].includes(resource ?? ''))
    return null

  if (resource === 'plates' && action === 'lookup' && method === 'POST') {
    // Agency desks only: a plate seen on the road must not open a vehicle's history.
    allow(caller, 'dmt', ...DESKS)
    let plate: string
    try {
      plate = canonicalPlate(String(body.plate ?? ''))
    }
    catch (error) {
      throw new HttpError(400, (error as Error).message)
    }
    const day = body.day === undefined || body.day === '' ? null : Number(body.day)
    if (day !== null && (!Number.isInteger(day) || day < 0 || day > 1_000_000))
      throw new HttpError(400, 'day must be a whole number from 0 to 1000000')
    const { rows } = await deps.db.query(
      day === null
        ? 'SELECT vid, from_day, to_day FROM plate_history WHERE plate = $1 AND to_day IS NULL'
        : 'SELECT vid, from_day, to_day FROM plate_history WHERE plate = $1 AND from_day <= $2 AND (to_day IS NULL OR to_day > $2) ORDER BY from_day DESC LIMIT 1',
      day === null ? [plate] : [plate, day])
    if (!rows[0])
      throw new HttpError(404, 'no vehicle carries this plate')
    await logRelease(deps, 'handle from a plate', String(rows[0].vid), null, 'derived', caller)
    return { status: 200, body: { vid: rows[0].vid, fromDay: rows[0].from_day, toDay: rows[0].to_day } }
  }

  if (resource === 'vehicles' && action === 'description' && method === 'POST') {
    allow(caller, 'dmt', ...DESKS, BUYER)
    const vid = bodyVid(body)
    const { rows } = await deps.db.query(
      'SELECT make, model, year, engine_cc, fuel, colour, desc_anchor FROM vehicle_description WHERE vid = $1', [vid])
    if (!rows[0])
      throw new HttpError(404, 'no description is recorded for this vehicle')
    const r = rows[0]
    // Recomputed each time, so an edited row loses the "committed" mark.
    let committed = false
    const facts = await readFacts(deps, vid)
    if (facts.descAnchor) {
      const d = { make: r.make, model: r.model, year: Number(r.year), engineCc: Number(r.engine_cc), fuel: r.fuel }
      committed = facts.descAnchor === descAnchorOf(deps.keys!, vid, d)
    }
    // The stored row no longer reproduces the ledger anchor: it was edited.
    const mismatch = !!facts.descAnchor && !committed
    await logRelease(deps, 'description', vid, null, 'released', caller)
    // The plate is never part of this answer: a handle must not lead to a car on the road.
    return {
      status: 200,
      body: {
        make: r.make, model: r.model, year: Number(r.year), engineCc: Number(r.engine_cc),
        fuel: r.fuel, colour: r.colour, committed, mismatch,
      },
    }
  }

  if (resource === 'vehicles' && action === 'match' && method === 'POST') {
    // One bit, never a digest, so it cannot look up an unknown car.
    allow(caller, 'dmt', ...DESKS, BUYER)
    const vid = bodyVid(body)
    let match = false
    try {
      const derived = Buffer.from(vidOf(deps.keys!, String(body.chassis ?? '')))
      match = derived.length === vid.length && timingSafeEqual(derived, Buffer.from(vid))
    }
    catch { /* an empty or null-like chassis matches nothing */ }
    await logRelease(deps, 'chassis match', vid, null, match ? 'matched' : 'not matched', caller)
    return { status: 200, body: { match } }
  }

  if (resource === 'theft' && action === 'check' && method === 'POST') {
    // Is the theft reporter the owner? MAX_REFUSED wrong NICs per ledger report
    // number limits guessing; the NIC is compared as a keyed tag and never stored.
    allow(caller, 'police-desk')
    const vid = bodyVid(body)
    const reportRef = String(body.reportRef ?? '').trim()
    if (!CASE.test(reportRef))
      throw new HttpError(400, 'reportRef must be 1–64 letters, digits, / _ or -')
    return oneAtATime(vid, () => ownerCheck(deps, vid, reportRef, body, caller))
  }

  if (resource === 'vehicles' && action === 'description' && method === 'PUT') {
    allow(caller, 'dmt')
    const vid = bodyVid(body)
    const d = bodyDescription(body)
    if (!d.colour)
      throw new HttpError(400, 'colour is required')
    const anchor = descAnchorOf(deps.keys!, vid, d)
    if (body.descAnchor !== undefined && body.descAnchor !== '' && body.descAnchor !== anchor)
      throw new HttpError(409, 'the description does not reproduce the anchor given')
    // A description is anchored only if its anchor matches the ledger; otherwise it is kept unanchored.
    const facts = await readFacts(deps, vid)
    if (facts.descAnchor && facts.descAnchor !== anchor)
      throw new HttpError(409, 'the description does not match the one committed on the ledger; nothing stored')
    const { rows } = await deps.db.query(
      `INSERT INTO vehicle_description (vid, make, model, year, engine_cc, fuel, colour, desc_anchor)
       VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
       ON CONFLICT (vid) DO UPDATE SET colour = EXCLUDED.colour, stored_at = now()
       WHERE vehicle_description.make = EXCLUDED.make AND vehicle_description.model = EXCLUDED.model
         AND vehicle_description.year = EXCLUDED.year AND vehicle_description.engine_cc = EXCLUDED.engine_cc
         AND vehicle_description.fuel = EXCLUDED.fuel
       RETURNING vid`,
      [vid, d.make, d.model, d.year, d.engineCc, d.fuel, d.colour, facts.descAnchor ? anchor : null])
    if (!rows.length)
      throw new HttpError(409, 'a different description is already recorded for this vehicle; only the colour may change')
    await logRelease(deps, 'description stored', vid, null, 'stored', caller)
    return { status: 200, body: { stored: true, committed: !!facts.descAnchor } }
  }

  if (resource === 'vehicles' && action === 'plate' && method === 'PUT') {
    allow(caller, 'dmt')
    const vid = bodyVid(body)
    let plate: string
    try {
      plate = canonicalPlate(String(body.plate ?? ''))
    }
    catch (error) {
      throw new HttpError(400, (error as Error).message)
    }
    const fromDay = Number(body.fromDay ?? 0)
    if (!Number.isInteger(fromDay) || fromDay < 0 || fromDay > 1_000_000)
      throw new HttpError(400, 'fromDay must be a whole number from 0 to 1000000')
    const reason = String(body.reason ?? 'registration').slice(0, 80)
    const { rows: current } = await deps.db.query('SELECT plate FROM plate_history WHERE vid = $1 AND to_day IS NULL', [vid])
    if (current[0]?.plate === plate)
      return { status: 200, body: { stored: true, unchanged: true } }
    try {
      // One statement; the partial unique index refuses a plate open on another vehicle.
      await deps.db.query(
        `WITH closed AS (UPDATE plate_history SET to_day = $3 WHERE vid = $1 AND to_day IS NULL RETURNING id)
         INSERT INTO plate_history (plate, vid, from_day, reason) SELECT $2, $1, $3, $4`,
        [vid, plate, fromDay, reason])
    }
    catch (error) {
      if ((error as { code?: string }).code === '23505')
        throw new HttpError(409, 'this plate is already assigned to another live vehicle')
      throw error
    }
    await logRelease(deps, 'plate assigned', vid, null, 'stored', caller)
    return { status: 200, body: { stored: true } }
  }
  throw new HttpError(404, 'not found')
}

/** The owner check itself, run one at a time per vehicle (see oneAtATime). */
async function ownerCheck(deps: Deps, vid: string, reportRef: string, body: Body, caller: string): Promise<Reply> {
  const facts = await readFacts(deps, vid, reportRef)
  if (!facts.theftReported) {
    await logRelease(deps, 'owner confirmation', vid, reportRef, 'refused: no theft report on the ledger', caller)
    throw new HttpError(409, `record the theft report ${reportRef} on the ledger first`)
  }
  let tag: string
  try {
    tag = ownerTagOf(deps.keys!, String(body.nic ?? ''))
  }
  catch (error) {
    throw new HttpError(400, (error as Error).message)
  }
  const { rows: tries } = await deps.db.query(
    `SELECT count(*)::int AS n FROM release_log
     WHERE item = 'owner confirmation' AND vid = $1 AND case_id = $2
       AND outcome LIKE 'refused: not the owner%'`, [vid, reportRef])
  if (Number(tries[0]?.n ?? 0) >= MAX_REFUSED) {
    await logRelease(deps, 'owner confirmation', vid, reportRef, 'refused: too many wrong NICs for this report', caller)
    throw new HttpError(429,
      `${MAX_REFUSED} NICs were already refused for report ${reportRef}; `
      + 'record a new theft report, or open a theft case')
  }
  const { rows } = await deps.db.query('SELECT owner_tag FROM owner_index WHERE vid = $1', [vid])
  const stored = rows[0]?.owner_tag ? Buffer.from(String(rows[0].owner_tag)) : null
  const confirmed = !!stored && stored.length === tag.length && timingSafeEqual(stored, Buffer.from(tag))
  if (!confirmed) {
    await logRelease(deps, 'owner confirmation', vid, reportRef, 'refused: not the owner', caller)
    return { status: 200, body: { confirmed: false, ledgerCheck: [facts.check] } }
  }
  if (facts.confirmed) {
    // A retry after the confirmation committed but the flag did not: nothing new to write.
    await logRelease(deps, 'owner confirmation', vid, reportRef, 'confirmed (already on the ledger)', caller)
    return { status: 200, body: { confirmed: true, ledgerCheck: [facts.check] } }
  }
  let committed
  try {
    // The record id includes the history length, so a report number can be reconfirmed later.
    committed = await deps.ledger!.confirmReporter(vid, reportRef, `owner-confirmed:${reportRef}:${facts.events}`)
  }
  catch (error) {
    const message = (error as Error).message
    const refusal = message.match(/registry: [^"\n]*/)?.[0]
    await logRelease(deps, 'owner confirmation', vid, reportRef, 'refused: not committed', caller)
    throw refusal
      ? new HttpError(409, refusal)
      : new HttpError(502, `the confirmation could not be committed: ${message}`)
  }
  await logRelease(deps, 'owner confirmation', vid, reportRef, 'confirmed', caller)
  return { status: 200, body: { confirmed: true, ledgerCheck: [facts.check, committed.check] } }
}

const dmt: Handler = async (deps, method, parts, query, body, caller) => {
  const identity = await vehicleIdentity(deps, method, parts, body, caller)
  if (identity)
    return identity
  const [resource, vid, sub] = parts
  if (method === 'POST' && resource === 'handles' && !vid) {
    // Any desk may key a chassis; only the DMT may derive an owner pseudonym from a
    // NIC. Derivations are logged with the caller, never the chassis or NIC.
    allow(caller, 'dmt', ...DESKS)
    if (typeof body.nic === 'string' && body.nic && caller !== 'dmt')
      throw new HttpError(403, 'only the DMT derives an owner pseudonym from a NIC; a desk key gives handles from a chassis only')
    let out: Record<string, string>
    try {
      const chassis = typeof body.chassis === 'string' ? body.chassis : ''
      out = chassis
        ? { ...agencyHandles(deps.keys!, { chassis, engine: String(body.engine ?? ''), make: String(body.make ?? '') }) }
        : {}
      const forVid = out.vid ?? (typeof body.vid === 'string' && HEX64.test(body.vid) ? body.vid : '')
      if (!forVid)
        throw new HttpError(400, 'chassis, or vid with nic, is required')
      // With a description, also its keyed anchor.
      if (body.model !== undefined && chassis) {
        const d = bodyDescription(body)
        out.descAnchor = descAnchorOf(deps.keys!, forVid, d)
      }
      if (typeof body.nic === 'string' && body.nic)
        out.pseudonym = pseudonymOf(deps.keys!, body.nic, forVid)
    }
    catch (error) {
      // A null-like chassis or a malformed NIC is the caller's mistake, not a service failure.
      throw error instanceof HttpError ? error : new HttpError(400, (error as Error).message)
    }
    await logRelease(deps, out.pseudonym ? 'pseudonym from a NIC' : 'handle from a chassis', out.vid ?? String(body.vid), null, 'derived', caller)
    return { status: 200, body: out }
  }
  if (resource !== 'records' || !vid || !HEX64.test(vid) || parts.length > 3 || (sub && sub !== 'fleet'))
    throw new HttpError(404, 'not found')

  if (method === 'PUT' && !sub) {
    // Store after the ledger commit; the Police half goes to the Police service from here.
    allow(caller, 'dmt')
    const env = body.envelope as StoredEnvelope | undefined
    const share = body.shareDMT as { share?: string } | undefined
    const policeShare = body.sharePolice as { share?: string } | undefined
    if (!env || env.vid !== vid || env.version !== 1)
      throw new HttpError(400, 'envelope for this vehicle (version 1) is required')
    const nonce = bytes(env.nonce, 'nonce')
    const ciphertext = bytes(env.ciphertext, 'ciphertext')
    const shareBytes = bytes(share?.share, 'shareDMT')
    const policeBytes = bytes(policeShare?.share, 'sharePolice')
    if (nonce.length !== 12 || shareBytes.length !== 32 || policeBytes.length !== 32
      || anchorOf(vid, 1, nonce, ciphertext) !== env.anchor)
      throw new HttpError(400, 'envelope or key half is malformed, or its anchor does not recompute')
    let ownerTag: string
    try {
      ownerTag = ownerTagOf(deps.keys!, String(body.nic ?? ''))
    }
    catch (error) {
      throw new HttpError(400, (error as Error).message)
    }
    const committed = await deps.ledger!.anchorOf(vid)
    if (committed.anchor !== env.anchor)
      throw new HttpError(409, 'the ledger has not committed this owner record\'s anchor; nothing stored')
    // Write-once per owner record: a different half for the same anchor would make the record unopenable.
    const { rows: prior } = await deps.db.query(
      'SELECT r.anchor_sha256, s.share FROM owner_record r JOIN key_share s USING (vid) WHERE vid = $1', [vid])
    if (prior[0] && prior[0].anchor_sha256 === env.anchor && !Buffer.from(prior[0].share as Buffer).equals(shareBytes))
      throw new HttpError(409, 'a different DMT key half is already stored for this owner record; nothing stored')
    // The Police half first: if the Police service refuses, nothing is stored here either.
    const police = await partnerCall<{ ledgerCheck?: LedgerCheck[] }>(deps, 'PUT', `/shares/${vid}`, { share: policeShare!.share, anchor: env.anchor })
    await logRelease(deps, 'Police key half', vid, null, 'handed over to the Police custody service')
    // One statement, so the record, the key half and the index entry are stored together or not at all.
    await deps.db.query(
      `WITH r AS (
         INSERT INTO owner_record (vid, envelope_version, ciphertext, nonce, anchor_sha256) VALUES ($1, 1, $2, $3, $4)
         ON CONFLICT (vid) DO UPDATE
           SET envelope_version = 1, ciphertext = $2, nonce = $3, anchor_sha256 = $4, stored_at = now()
         RETURNING vid),
       s AS (
         INSERT INTO key_share (vid, share) SELECT vid, $5 FROM r
         ON CONFLICT (vid) DO UPDATE SET share = $5 RETURNING vid)
       INSERT INTO owner_index (vid, owner_tag) SELECT vid, $6 FROM s
       ON CONFLICT (vid) DO UPDATE SET owner_tag = $6`,
      [vid, ciphertext, nonce, env.anchor, shareBytes, ownerTag])
    return {
      status: 200,
      body: {
        stored: true,
        tables: ['owner_record', 'key_share', 'owner_index'],
        ledgerCheck: [committed.check],
        handoff: { to: 'police', ms: police.ms, ledgerCheck: police.ledgerCheck ?? [] },
      },
    }
  }

  if (method === 'GET' && sub === 'fleet') {
    // approve: case must be PENDING. read (Police service): case must be APPROVED.
    // predicate: no case to check, so any holder of the DMT token can ask.
    const caseId = query.get('case') ?? ''
    const purpose = query.get('purpose') ?? (caseId ? 'read' : 'predicate')
    const states: Record<string, string[]> = { approve: ['PENDING'], read: ['APPROVED'] }
    if (caseId ? !states[purpose] : purpose !== 'predicate')
      throw new HttpError(400, 'purpose must be approve or read with a case, or predicate without one')
    allow(caller, ...(purpose === 'read' ? ['police-service'] : ['dmt']))
    const checks = caseId ? [(await requireCase(deps, 'fleet', vid, caseId, states[purpose]!, caller)).check] : []
    const fleet = await fleetOf(deps, vid)
    await logRelease(deps, 'fleet', vid, caseId || null, `released: ${purpose}`, caller)
    return { status: 200, body: { fleet, ledgerCheck: checks } }
  }

  if (method === 'GET' && !sub) {
    // Only the Police service, for a reveal; the DMT never fetches its sealed record back.
    allow(caller, 'police-service')
    const caseId = query.get('case') ?? ''
    const approved = await requireCase(deps, 'owner record', vid, caseId, ['APPROVED'], caller)
    const { rows } = await deps.db.query(
      `SELECT r.envelope_version, r.ciphertext, r.nonce, r.anchor_sha256, s.share
       FROM owner_record r JOIN key_share s USING (vid) WHERE vid = $1`, [vid])
    const row = rows[0]
    if (!row) {
      await logRelease(deps, 'owner record', vid, caseId, 'refused: no record', caller)
      throw new HttpError(404, 'no off-ledger owner record exists for this vehicle')
    }
    const version = Number(row.envelope_version)
    if (anchorOf(vid, version, row.nonce as Buffer, row.ciphertext as Buffer) !== row.anchor_sha256) {
      await logRelease(deps, 'owner record', vid, caseId, 'refused: altered', caller)
      throw new HttpError(409, 'owner envelope anchor mismatch: the stored record was altered')
    }
    const committed = await deps.ledger!.anchorOf(vid)
    if (committed.anchor !== row.anchor_sha256) {
      await logRelease(deps, 'owner record', vid, caseId, 'refused: anchor differs from ledger', caller)
      throw new HttpError(409, 'owner envelope does not match the anchor committed on the ledger')
    }
    // The case opens only the owner record that existed when it was approved.
    if (approved.ownerAnchor && approved.ownerAnchor !== row.anchor_sha256) {
      await logRelease(deps, 'owner record', vid, caseId, 'refused: owner changed since approval', caller)
      throw new HttpError(409, 'the owner record has changed since the case was approved (anchor differs); a new case is needed')
    }
    await logRelease(deps, 'owner record', vid, caseId, 'released', caller)
    return {
      status: 200,
      body: {
        envelope: { vid, version, ciphertext: b64(row.ciphertext), nonce: b64(row.nonce), anchor: row.anchor_sha256 },
        shareDMT: { vid, share: b64(row.share) },
        ledgerCheck: [approved.check, committed.check],
      },
    }
  }
  throw new HttpError(404, 'not found')
}

// ---- Police ----

const police: Handler = async (deps, method, parts, query, body, caller) => {
  const [resource, vid] = parts
  if (!['shares', 'reveal', 'fleet'].includes(resource ?? '') || !vid || !HEX64.test(vid) || parts.length !== 2)
    throw new HttpError(404, 'not found')
  const caseId = query.get('case') ?? ''

  if (resource === 'shares' && method === 'PUT') {
    // Handed over by the DMT service after registration or transfer; not by the portal.
    allow(caller, 'dmt-service')
    const share = bytes(body.share, 'share')
    const anchor = String(body.anchor ?? '')
    if (share.length !== 32 || !HEX64.test(anchor))
      throw new HttpError(400, 'a 32-byte key half and the owner record\'s anchor are required')
    // Stored only for the owner record the ledger has committed.
    const committed = await deps.ledger!.anchorOf(vid)
    if (committed.anchor !== anchor)
      throw new HttpError(409, 'the ledger has not committed this owner record\'s anchor; nothing stored')
    // Write-once per owner record: a different half for the same anchor would make the record unopenable.
    const { rows: prior } = await deps.db.query('SELECT share, anchor_sha256 FROM key_share WHERE vid = $1', [vid])
    if (prior[0] && prior[0].anchor_sha256 === anchor && !Buffer.from(prior[0].share as Buffer).equals(share))
      throw new HttpError(409, 'a different Police key half is already stored for this owner record; nothing stored')
    await deps.db.query(
      `INSERT INTO key_share (vid, share, anchor_sha256) VALUES ($1, $2, $3)
       ON CONFLICT (vid) DO UPDATE SET share = $2, anchor_sha256 = $3, stored_at = now()`, [vid, share, anchor])
    await logRelease(deps, 'Police key half', vid, null, 'stored, from the DMT custody service')
    return { status: 200, body: { stored: true, tables: ['key_share'], ledgerCheck: [committed.check] } }
  }

  if (resource === 'shares' && method === 'GET') {
    allow(caller, 'police')
    const approved = await requireCase(deps, 'Police key half', vid, caseId, ['APPROVED'], caller)
    const row = await ownHalf(deps, vid, caseId, approved.ownerAnchor, caller)
    await logRelease(deps, 'Police key half', vid, caseId, 'released', caller)
    return { status: 200, body: { sharePolice: { vid, share: b64(row.share) }, ledgerCheck: [approved.check] } }
  }

  if (resource === 'reveal' && method === 'POST') {
    // The reveal: own ledger check, own half, the DMT's part, decrypt, and the audit
    // entry committed before anything is returned.
    allow(caller, 'police')
    const approved = await requireCase(deps, 'owner identity', vid, caseId, ['APPROVED'], caller)
    const row = await ownHalf(deps, vid, caseId, approved.ownerAnchor, caller)
    let dmtPart
    try {
      dmtPart = await partnerCall<{ envelope: StoredEnvelope, shareDMT: StoredShare, ledgerCheck?: LedgerCheck[] }>(
        deps, 'GET', `/records/${vid}?case=${encodeURIComponent(caseId)}`)
    }
    catch (error) {
      await logRelease(deps, 'owner identity', vid, caseId, `refused: ${(error as Error).message}`, caller)
      throw error
    }
    const committed = await deps.ledger!.anchorOf(vid)
    let owner
    try {
      owner = openOwner(dmtPart.envelope, dmtPart.shareDMT, { vid, share: b64(row.share) }, committed.anchor)
    }
    catch (error) {
      await logRelease(deps, 'owner identity', vid, caseId, 'refused: could not be opened', caller)
      throw new HttpError(409, (error as Error).message)
    }
    let logged
    try {
      logged = await deps.ledger!.recordReveal(caseId, vid)
    }
    catch (error) {
      const message = (error as Error).message
      await logRelease(deps, 'owner identity', vid, caseId, 'refused: the audit entry was not committed', caller)
      const refusal = message.match(/registry: [^"\n]*/)?.[0]
      throw refusal
        ? new HttpError(409, refusal)
        : new HttpError(502, `the audit entry could not be committed, so nothing is shown: ${message}`)
    }
    await logRelease(deps, 'owner identity', vid, caseId, 'opened', caller)
    return {
      status: 200,
      body: {
        vid, caseId, anchor: dmtPart.envelope.anchor, owner, accessLog: logged.accessLog,
        ledgerCheck: [approved.check, committed.check, logged.check],
        handoff: { to: 'dmt', ms: dmtPart.ms, ledgerCheck: dmtPart.ledgerCheck ?? [] },
      },
    }
  }

  if (resource === 'fleet' && method === 'GET') {
    // The owner's vehicle list for a Police read under an approved case, fetched from the DMT service.
    allow(caller, 'police')
    const approved = await requireCase(deps, 'fleet', vid, caseId, ['APPROVED'], caller)
    const dmtFleet = await partnerCall<{ fleet: string[], ledgerCheck?: LedgerCheck[] }>(
      deps, 'GET', `/records/${vid}/fleet?case=${encodeURIComponent(caseId)}&purpose=read`)
    await logRelease(deps, 'fleet', vid, caseId, 'relayed from the DMT custody service', caller)
    return {
      status: 200,
      body: { fleet: dmtFleet.fleet, ledgerCheck: [approved.check], handoff: { to: 'dmt', ms: dmtFleet.ms, ledgerCheck: dmtFleet.ledgerCheck ?? [] } },
    }
  }
  throw new HttpError(404, 'not found')
}

/** The Police half for a case; refused unless it belongs to the approved owner record. */
async function ownHalf(deps: Deps, vid: string, caseId: string, ownerAnchor: string, caller: string) {
  const { rows } = await deps.db.query('SELECT share, anchor_sha256 FROM key_share WHERE vid = $1', [vid])
  if (!rows[0]) {
    await logRelease(deps, 'Police key half', vid, caseId, 'refused: no key half', caller)
    throw new HttpError(404, 'no Police key half exists for this vehicle')
  }
  if (ownerAnchor && rows[0].anchor_sha256 !== ownerAnchor) {
    await logRelease(deps, 'Police key half', vid, caseId, 'refused: owner changed since approval', caller)
    throw new HttpError(409, 'the owner record has changed since the case was approved (anchor differs); a new case is needed')
  }
  return rows[0]
}

// ---- Customs, VET, insurers: internal records ----

const internal: Handler = async (deps, method, parts, query, body) => {
  const spec = INTERNAL[deps.org as keyof typeof INTERNAL]
  const map = spec.fields as Record<string, string>
  const [resource, id] = parts
  if (resource !== 'records' || parts.length > 2)
    throw new HttpError(404, 'not found')
  const columns = Object.values(map)
  const toJson = (row: Record<string, unknown>) => ({
    recordId: row.record_id,
    vid: row.vid,
    // NUMERIC columns arrive from node-pg as strings; give them back as numbers.
    fields: Object.fromEntries(Object.entries(map).map(([k, c]) => [k, c.endsWith('_lkr') && row[c] !== null ? Number(row[c]) : row[c]])),
    storedAt: row.stored_at,
  })
  if (method === 'GET' && !id) {
    const vid = query.get('vid') ?? ''
    if (!HEX64.test(vid))
      throw new HttpError(400, 'vid is required')
    const { rows } = await deps.db.query(`SELECT record_id, vid, ${columns.join(', ')}, stored_at FROM ${spec.table} WHERE vid = $1 ORDER BY stored_at`, [vid])
    return { status: 200, body: { records: rows.map(toJson) } }
  }
  if (!id || !RECORD.test(id))
    throw new HttpError(400, 'a record id is required')
  if (method === 'GET') {
    const { rows } = await deps.db.query(`SELECT record_id, vid, ${columns.join(', ')}, stored_at FROM ${spec.table} WHERE record_id = $1`, [id])
    if (!rows[0])
      throw new HttpError(404, `no internal record ${id}`)
    return { status: 200, body: toJson(rows[0]) }
  }
  if (method === 'PUT') {
    const vid = String(body.vid ?? '')
    const fields = (body.fields ?? {}) as Record<string, unknown>
    if (!HEX64.test(vid))
      throw new HttpError(400, 'vid is required')
    const unknown = Object.keys(fields).filter(k => !(k in map))
    if (unknown.length)
      throw new HttpError(400, `unknown field(s): ${unknown.join(', ')}; allowed: ${Object.keys(map).join(', ')}`)
    const values = Object.keys(map).map((k) => {
      const v = fields[k]
      if (v === undefined || v === '')
        return null
      if ((typeof v !== 'string' && typeof v !== 'number') || String(v).length > 200)
        throw new HttpError(400, `${k} must be text or a number of at most 200 characters`)
      return v
    })
    const sets = columns.map((c, i) => `${c} = $${i + 3}`).join(', ')
    const { rows } = await deps.db.query(
      `INSERT INTO ${spec.table} (record_id, vid, ${columns.join(', ')}) VALUES ($1, $2, ${columns.map((_, i) => `$${i + 3}`).join(', ')})
       ON CONFLICT (record_id) DO UPDATE SET ${sets}, stored_at = now()
       WHERE ${spec.table}.vid = EXCLUDED.vid
       RETURNING record_id`,
      [id, vid, ...values])
    if (!rows.length)
      throw new HttpError(409, `record ${id} already belongs to another vehicle in this service`)
    return { status: 200, body: { stored: true, tables: [spec.table] } }
  }
  throw new HttpError(404, 'not found')
}

/** Which caller a bearer token names. Constant-time per token. */
function callerOf(deps: Deps, authorization: string | undefined): string | null {
  const given = Buffer.from(authorization?.match(/^Bearer (\S+)$/)?.[1] ?? '')
  let found: string | null = null
  for (const [token, name] of [[deps.token, deps.org], ...Object.entries(deps.callers ?? {})]) {
    const want = Buffer.from(token ?? '')
    if (want.length && given.length === want.length && timingSafeEqual(given, want))
      found = name ?? null
  }
  return found
}

/** Routes one request. Exported so the tests can drive it without a socket. */
export async function route(
  deps: Deps, method: string, url: string, authorization: string | undefined, body: Body,
): Promise<Reply> {
  const { pathname, searchParams } = new URL(url, 'http://custody')
  // Record ids such as CUSDEC/2026/0001 arrive percent-encoded as one segment.
  let parts: string[]
  try {
    parts = pathname.split('/').filter(Boolean).map(decodeURIComponent)
  }
  catch {
    return { status: 400, body: { message: 'malformed path' } }
  }
  try {
    if (method === 'GET' && pathname === '/health')
      return await health(deps)
    const caller = callerOf(deps, authorization)
    if (!caller)
      return { status: 401, body: { message: 'a valid bearer token for this organisation\'s service is required' } }
    if (deps.org === 'dmt')
      return await dmt(deps, method, parts, searchParams, body, caller)
    if (deps.org === 'police')
      return await police(deps, method, parts, searchParams, body, caller)
    return await internal(deps, method, parts, searchParams, body, caller)
  }
  catch (error) {
    if (error instanceof HttpError)
      return { status: error.status, body: { message: error.message } }
    return { status: 500, body: { message: `the ${deps.org} custody service failed: ${(error as Error).message}` } }
  }
}

async function readJson(req: IncomingMessage): Promise<Record<string, unknown>> {
  let size = 0
  const chunks: Buffer[] = []
  for await (const chunk of req) {
    size += (chunk as Buffer).length
    if (size > 64 * 1024)
      throw new HttpError(413, 'request too large')
    chunks.push(chunk as Buffer)
  }
  if (!size)
    return {}
  let parsed: unknown
  try {
    parsed = JSON.parse(Buffer.concat(chunks).toString('utf8'))
  }
  catch {
    throw new HttpError(400, 'the body must be JSON')
  }
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed))
    throw new HttpError(400, 'the body must be a JSON object')
  return parsed as Record<string, unknown>
}

export function serve(deps: Deps, port: number) {
  return createServer(async (req: IncomingMessage, res: ServerResponse) => {
    const started = Date.now()
    let reply: Reply
    try {
      reply = await route(deps, req.method ?? 'GET', req.url ?? '/', req.headers.authorization, await readJson(req))
    }
    catch (error) {
      reply = { status: error instanceof HttpError ? error.status : 400, body: { message: (error as Error).message } }
    }
    // After a 413 the rest of the body is unread: close the connection rather than reuse it.
    res.writeHead(reply.status, { 'content-type': 'application/json', ...(reply.status === 413 ? { connection: 'close' } : {}) })
    res.end(JSON.stringify(reply.body))
    // Method, path and status only: never a body, which may hold owner details or key halves.
    console.log(`${deps.org} ${req.method} ${req.url?.split('?')[0]} ${reply.status} ${Date.now() - started}ms`)
  }).listen(port)
}

async function main() {
  const org = process.env.ORG as Org
  if (!['dmt', 'police', 'customs', 'vet', 'insurer'].includes(org))
    throw new Error('ORG must be dmt, police, customs, vet or insurer')
  const pool = new pg.Pool({ connectionString: process.env.DATABASE_URL, max: 5 })
  // An idle client's error (for example a database restart) must not crash the service.
  pool.on('error', error => console.error(`${org} database pool: ${error.message}`))
  const schema = readFileSync(join(dirname(fileURLToPath(import.meta.url)), 'sql', `${org}.sql`), 'utf8')
  for (let attempt = 1; ; attempt++) {
    try {
      await pool.query(schema)
      break
    }
    catch (error) {
      if (attempt >= 30)
        throw error
      await new Promise(r => setTimeout(r, 1000))
    }
  }
  // "police-service=<token>,customs-desk=<token>": the callers other than this organisation's own portal.
  const callers = Object.fromEntries((process.env.CUSTODY_CALLER_TOKENS ?? '').split(',')
    .map(pair => pair.trim().split('='))
    .filter(([name, token]) => name && token && token.length >= 32)
    .map(([name, token]) => [token!, name!]))
  const deps: Deps = { org, token: process.env.CUSTODY_TOKEN ?? '', db: pool, callers }
  if (org === 'dmt' || org === 'police') {
    if (!process.env.PARTNER_URL || !process.env.PARTNER_TOKEN)
      throw new Error('PARTNER_URL and PARTNER_TOKEN are required: the DMT and Police services hand over key halves directly')
    deps.partner = { name: org === 'dmt' ? 'Police' : 'DMT', url: process.env.PARTNER_URL, token: process.env.PARTNER_TOKEN }
  }
  if (org === 'dmt' || org === 'police') {
    deps.ledger = fabricLedger({
      org: org === 'dmt' ? 1 : 2,
      peerEndpoint: process.env.PEER_ENDPOINT ?? (org === 'dmt' ? 'peer0.org1.example.com:7051' : 'peer0.org2.example.com:9051'),
      organizations: process.env.FABRIC_ORGANIZATIONS ?? '/fabric/peerOrganizations',
      channel: process.env.CHANNEL ?? 'registry',
      chaincode: process.env.CHAINCODE ?? 'vehicle-registry',
    })
  }
  if (org === 'dmt') {
    const [kv, kown] = (process.env.REGISTRY_KEYS ?? '').split(':')
    if (!kv || !kown || !HEX64.test(kv) || !HEX64.test(kown))
      throw new Error('REGISTRY_KEYS must be "<k_v hex>:<k_own hex>"')
    deps.keys = { kv: Buffer.from(kv, 'hex'), kown: Buffer.from(kown, 'hex') }
  }
  if (!deps.token)
    throw new Error('CUSTODY_TOKEN is required')
  serve(deps, Number(process.env.PORT ?? 8100))
  console.log(`${org} custody service listening on ${process.env.PORT ?? 8100}`)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href)
  main().catch((error) => {
    console.error(error)
    process.exit(1)
  })
