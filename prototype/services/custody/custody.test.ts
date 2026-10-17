// @vitest-environment node
import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { openOwner } from '../../server/utils/envelope'
import type { StoredEnvelope, StoredShare } from '../../server/utils/envelope'
import { descAnchorOf, ownerTagOf, vidOf } from '../../server/utils/handles'
import { route } from './main'
import type { Db, Deps } from './main'
import { reportState } from './ledger'
import type { Ledger } from './ledger'

// The custody gates against a fake database and a fake ledger.
const vid = 'ab'.repeat(32)
const fixtures = join(__dirname, '../../app/utils/__fixtures__/envelope')
const read = <T>(p: string) => JSON.parse(readFileSync(join(fixtures, p), 'utf8')) as T
const envelope = read<StoredEnvelope>(`owner_record/${vid}.json`)
const shareDMT = read<StoredShare>(`custody_dmt/${vid}.json`)
const sharePolice = read<StoredShare>(`custody_police/${vid}.json`)
const TOKEN = 't'.repeat(64)
const auth = `Bearer ${TOKEN}`
// The other callers each service admits (CUSTODY_CALLER_TOKENS).
const POLICE_SVC = 'p'.repeat(64)
const DMT_SVC = 'd'.repeat(64)
const CUSTOMS_DESK = 'c'.repeat(64)
const POLICE_DESK = 'e'.repeat(64)
const BUYER = 'b'.repeat(64)
const callers = {
  [POLICE_SVC]: 'police-service', [DMT_SVC]: 'dmt-service', [CUSTOMS_DESK]: 'customs-desk',
  [POLICE_DESK]: 'police-desk', [BUYER]: 'buyer-portal',
}
const as = (token: string) => `Bearer ${token}`

function fakeDb(rows: Record<string, unknown>[] = []): Db & { writes: string[] } {
  const writes: string[] = []
  return {
    writes,
    query: async (text) => {
      if (/^\s*SELECT/i.test(text))
        return { rows }
      if (/RETURNING record_id/.test(text)) {
        writes.push('INSERT internal')
        return { rows: [{ record_id: 'r' }] }
      }
      writes.push(text.trim().split(/\s+/).slice(0, 3).join(' '))
      return { rows: [] }
    },
  }
}
const check = { fn: 'GetRevealAudit', peer: 'peer0.orgN.example.com', ms: 1 }
interface FakeCase { status?: string, vid?: string, error?: string, ownerAnchor?: string }
interface FakeFacts { theftReported?: boolean, confirmed?: boolean, descAnchor?: string, confirmError?: string }
let confirmations: string[] = []
function fakeLedger(c: FakeCase, anchor = envelope.anchor, facts: FakeFacts = {}): Ledger {
  return {
    vehicleFacts: async (_vid, reportRef) => ({
      theftReported: !!reportRef && !!facts.theftReported, confirmed: !!facts.confirmed, events: 5, descAnchor: facts.descAnchor ?? '',
      check: { ...check, fn: 'GetVehicleHistoryFor', status: 'READ' },
    }),
    confirmReporter: async (_vid, reportRef, recordId) => {
      if (facts.confirmError)
        throw new Error(facts.confirmError)
      expect(recordId).toBe(`owner-confirmed:${reportRef}:5`)
      confirmations.push(reportRef)
      return { check: { ...check, fn: 'ConfirmReporter', status: 'COMMITTED' } }
    },
    caseFor: async () => {
      if (c.error)
        throw new Error(c.error)
      return { status: c.status!, vid: c.vid ?? vid, ownerAnchor: c.ownerAnchor ?? '', check: { ...check, status: c.status! } }
    },
    anchorOf: async () => ({ anchor, check: { ...check, fn: 'GetVehicleHistoryFor', status: 'ANCHOR' } }),
    recordReveal: async () => {
      reveals.push('IDENTITY_REVEAL')
      return { accessLog: [`IDENTITY_REVEAL:${vid}:${anchor}`], check: { ...check, fn: 'RecordIdentityAccess', status: 'COMMITTED' } }
    },
  }
}
let reveals: string[] = []
/** The other custodian, answering every call with one status and body, and recording what it was asked. */
function fakePartner(status: number, reply: unknown) {
  const calls: { method: string, url: string, body?: unknown }[] = []
  const call = (async (url: string, init: RequestInit) => {
    calls.push({ method: String(init.method), url, body: init.body ? JSON.parse(String(init.body)) : undefined })
    return new Response(JSON.stringify(reply), { status, headers: { 'content-type': 'application/json' } })
  }) as unknown as typeof fetch
  return { partner: { name: 'Partner', url: 'http://partner', token: 'x'.repeat(64), call }, calls }
}
const recordRow = (ciphertext = envelope.ciphertext) => ({
  envelope_version: 1, ciphertext: Buffer.from(ciphertext, 'base64'), nonce: Buffer.from(envelope.nonce, 'base64'),
  anchor_sha256: envelope.anchor, share: Buffer.from(shareDMT.share, 'base64'),
})
const deps = (over: Partial<Deps>): Deps => ({ org: 'police', token: TOKEN, db: fakeDb(), callers, ...over })

describe('custody service', () => {
  it('needs this organisation’s token for everything but /health', async () => {
    const d = deps({ ledger: fakeLedger({ status: 'APPROVED' }), db: fakeDb([{ share: Buffer.alloc(32) }]) })
    expect((await route(d, 'GET', `/shares/${vid}?case=C`, undefined, {})).status).toBe(401)
    expect((await route(d, 'GET', `/shares/${vid}?case=C`, 'Bearer wrong', {})).status).toBe(401)
    expect((await route(d, 'GET', '/health', undefined, {})).status).toBe(200)
    expect((await route(d, 'GET', `/shares/${vid}?case=C`, auth, {})).status).toBe(200)
  })

  it('releases the Police half only when its own ledger read shows the case approved for this vehicle', async () => {
    const at = (c: Parameters<typeof fakeLedger>[0]) => {
      const db = fakeDb([{ share: Buffer.from(sharePolice.share, 'base64') }])
      return route(deps({ db, ledger: fakeLedger(c) }), 'GET', `/shares/${vid}?case=C`, auth, {}).then(r => ({ ...r, writes: db.writes }))
    }
    expect((await at({ status: 'PENDING' })).status).toBe(403)
    expect((await at({ status: 'REJECTED' })).status).toBe(403)
    expect((await at({ status: 'APPROVED', vid: 'cd'.repeat(32) })).status).toBe(403)
    expect((await at({ error: 'chaincode response 500, registry: record not found' })).status).toBe(404)
    expect((await at({ error: 'chaincode vehicle-registry not found' })).status).toBe(502) // not an unknown case
    expect((await at({ error: 'connection refused' })).status).toBe(502)
    const ok = await at({ status: 'APPROVED' })
    expect(ok.status).toBe(200)
    expect((await at({ status: 'PENDING' })).writes).toEqual(['INSERT INTO release_log']) // refusals are logged too
    const noCase = await route(deps({ ledger: fakeLedger({ status: 'APPROVED' }) }), 'GET', `/shares/${vid}`, auth, {})
    expect(noCase.status).toBe(403)
  })

  it('releases the DMT part byte for byte, and refuses a record altered at rest or not matching the ledger', async () => {
    const get = (row: Record<string, unknown>, ledgerAnchor = envelope.anchor) => route(
      deps({ org: 'dmt', db: fakeDb([row]), ledger: fakeLedger({ status: 'APPROVED' }, ledgerAnchor) }),
      'GET', `/records/${vid}?case=C`, as(POLICE_SVC), {})
    const ok = await get(recordRow())
    expect(ok.status).toBe(200)
    const body = ok.body as { envelope: StoredEnvelope, shareDMT: StoredShare }
    expect(openOwner(body.envelope, body.shareDMT, sharePolice, envelope.anchor).name).toBe('Fixture Person')
    const altered = await get(recordRow(`AA${envelope.ciphertext.slice(2)}`))
    expect(altered.status).toBe(409)
    expect((altered.body as { message: string }).message).toMatch(/anchor/)
    expect((await get(recordRow(), 'ff'.repeat(32))).status).toBe(409)
    // The DMT's own token (the portal acting as DMT) never fetches the sealed record back.
    const own = await route(deps({ org: 'dmt', db: fakeDb([recordRow()]), ledger: fakeLedger({ status: 'APPROVED' }) }),
      'GET', `/records/${vid}?case=C`, auth, {})
    expect(own.status).toBe(403)
  })

  it('opens only the owner record the case was approved for (v1.7)', async () => {
    const at = (ownerAnchor: string) => route(
      deps({ org: 'dmt', db: fakeDb([recordRow()]), ledger: fakeLedger({ status: 'APPROVED', ownerAnchor }) }),
      'GET', `/records/${vid}?case=C`, as(POLICE_SVC), {})
    expect((await at(envelope.anchor)).status).toBe(200)
    const changed = await at('ee'.repeat(32))
    expect(changed.status).toBe(409)
    expect((changed.body as { message: string }).message).toMatch(/changed since the case was approved/)
    const police = (ownerAnchor: string) => route(
      deps({ db: fakeDb([{ share: Buffer.from(sharePolice.share, 'base64'), anchor_sha256: envelope.anchor }]), ledger: fakeLedger({ status: 'APPROVED', ownerAnchor }) }),
      'GET', `/shares/${vid}?case=C`, auth, {})
    expect((await police(envelope.anchor)).status).toBe(200)
    expect((await police('ee'.repeat(32))).status).toBe(409)
  })

  it('stores the Police half only from the DMT service, for the anchor the ledger has committed, once', async () => {
    const put = (anchor: string, onLedger = envelope.anchor, token = DMT_SVC, stored: Record<string, unknown>[] = []) =>
      route(deps({ ledger: fakeLedger({}, onLedger), db: fakeDb(stored) }), 'PUT', `/shares/${vid}`, as(token), { share: sharePolice.share, anchor })
    expect((await put(envelope.anchor)).status).toBe(200)
    expect((await put(envelope.anchor, 'ff'.repeat(32))).status).toBe(409)
    expect((await put('')).status).toBe(400)
    expect((await put(envelope.anchor, envelope.anchor, TOKEN)).status).toBe(403) // the portal acting as Police
    expect((await put(envelope.anchor, envelope.anchor, CUSTOMS_DESK)).status).toBe(403)
    // Write-once: the same bytes again pass (a retry), a different half for the same record is refused.
    const same = [{ share: Buffer.from(sharePolice.share, 'base64'), anchor_sha256: envelope.anchor }]
    expect((await put(envelope.anchor, envelope.anchor, DMT_SVC, same)).status).toBe(200)
    const other = [{ share: Buffer.alloc(32, 9), anchor_sha256: envelope.anchor }]
    expect((await put(envelope.anchor, envelope.anchor, DMT_SVC, other)).status).toBe(409)
  })

  it('releases a fleet only for the purpose the case state allows', async () => {
    const fleet = (q: string, status: string, token = TOKEN) => route(
      deps({ org: 'dmt', db: fakeDb([{ vid }]), ledger: fakeLedger({ status }) }), 'GET', `/records/${vid}/fleet${q}`, as(token), {})
    expect((await fleet('?case=C&purpose=approve', 'PENDING')).status).toBe(200)
    expect((await fleet('?case=C&purpose=approve', 'APPROVED')).status).toBe(403)
    expect((await fleet('?case=C&purpose=read', 'PENDING', POLICE_SVC)).status).toBe(403)
    expect((await fleet('?case=C&purpose=read', 'APPROVED', POLICE_SVC)).status).toBe(200)
    expect((await fleet('?purpose=read', 'APPROVED')).status).toBe(400)
    // Each purpose has one caller: the Police service reads, the DMT approves and runs its predicate.
    expect((await fleet('?case=C&purpose=read', 'APPROVED', TOKEN)).status).toBe(403)
    expect((await fleet('?case=C&purpose=approve', 'PENDING', POLICE_SVC)).status).toBe(403)
    expect((await fleet('?purpose=predicate', 'APPROVED', POLICE_SVC)).status).toBe(403)
  })

  it('refuses malformed requests as the caller’s error', async () => {
    const d = deps({ org: 'dmt', keys: { kv: Buffer.alloc(32, 1), kown: Buffer.alloc(32, 2) } })
    expect((await route(d, 'POST', '/handles', TOKEN, { chassis: 'X1' })).status).toBe(401) // "Bearer " is required
    expect((await route(d, 'POST', '/handles', auth, { chassis: 'NONE' })).status).toBe(400)
    expect((await route(d, 'POST', '/handles', auth, { vid, nic: 'abc' })).status).toBe(400)
    expect((await route(d, 'GET', `/records/${vid}/fleet/x`, auth, {})).status).toBe(404)
  })

  it('gives a desk key handles from a chassis, and owner pseudonyms only to the DMT', async () => {
    const d = deps({ org: 'dmt', keys: { kv: Buffer.alloc(32, 1), kown: Buffer.alloc(32, 2) } })
    const desk = await route(d, 'POST', '/handles', as(CUSTOMS_DESK), { chassis: 'KDH200-0001', engine: '1KD-1', make: 'Toyota' })
    expect(desk.status).toBe(200)
    expect((desk.body as { vid: string }).vid).toMatch(/^[0-9a-f]{64}$/)
    expect((await route(d, 'POST', '/handles', as(CUSTOMS_DESK), { vid, nic: '199012345678' })).status).toBe(403)
    expect((await route(d, 'POST', '/handles', as(POLICE_SVC), { chassis: 'KDH200-0001' })).status).toBe(403)
    const own = await route(d, 'POST', '/handles', auth, { vid, nic: '199012345678' })
    expect(own.status).toBe(200)
    expect((own.body as { pseudonym: string }).pseudonym).toMatch(/^[0-9a-f]{64}$/)
  })

  it('stores a sealed record only once the ledger holds its anchor, handing the Police half to the Police service first', async () => {
    const put = (ledgerAnchor: string, police = fakePartner(200, { stored: true, ledgerCheck: [] }), token = TOKEN) => {
      const db = fakeDb()
      return route(deps({ org: 'dmt', db, ledger: fakeLedger({}, ledgerAnchor), partner: police.partner, keys: { kv: Buffer.alloc(32, 1), kown: Buffer.alloc(32, 2) } }),
        'PUT', `/records/${vid}`, as(token), { envelope, shareDMT, sharePolice, nic: '199012345678' })
        .then(r => ({ ...r, writes: db.writes, calls: police.calls }))
    }
    const early = await put('ff'.repeat(32))
    expect(early.status).toBe(409)
    expect(early.writes).toEqual([])
    expect(early.calls).toEqual([])
    const stored = await put(envelope.anchor)
    expect(stored.status).toBe(200)
    expect(stored.calls).toEqual([{ method: 'PUT', url: `http://partner/shares/${vid}`, body: { share: sharePolice.share, anchor: envelope.anchor } }])
    expect(stored.writes).toEqual(['INSERT INTO release_log', 'WITH r AS'])
    // The Police service refuses: nothing is stored at the DMT either, and the refusal keeps its status.
    const refused = await put(envelope.anchor, fakePartner(409, { message: 'the ledger has not committed this anchor' }))
    expect(refused.status).toBe(409)
    expect((refused.body as { message: string }).message).toMatch(/^Partner custody service: /)
    expect(refused.writes).toEqual([])
    expect((await put(envelope.anchor, undefined, POLICE_SVC)).status).toBe(403)
  })

  it('reveals at the Police service: own ledger check and half, the DMT part fetched directly, audit committed first', async () => {
    const reveal = (dmt: ReturnType<typeof fakePartner>, token = TOKEN, c: FakeCase = { status: 'APPROVED' }) => {
      reveals = []
      const db = fakeDb([{ share: Buffer.from(sharePolice.share, 'base64'), anchor_sha256: envelope.anchor }])
      return route(deps({ db, ledger: fakeLedger(c), partner: dmt.partner }), 'POST', `/reveal/${vid}?case=C`, as(token), {})
    }
    const dmt = fakePartner(200, { envelope, shareDMT, ledgerCheck: [] })
    const ok = await reveal(dmt)
    expect(ok.status).toBe(200)
    const body = ok.body as { owner: { name: string }, accessLog: string[] }
    expect(body.owner.name).toBe('Fixture Person')
    expect(body.accessLog).toEqual([`IDENTITY_REVEAL:${vid}:${envelope.anchor}`])
    expect(reveals).toEqual(['IDENTITY_REVEAL'])
    expect(dmt.calls[0]!.url).toBe(`http://partner/records/${vid}?case=C`)
    // The DMT service refuses: nothing is opened or logged on the case.
    const refused = await reveal(fakePartner(409, { message: 'owner envelope anchor mismatch' }))
    expect(refused.status).toBe(409)
    expect(reveals).toEqual([])
    // A pending case is refused here before the DMT is asked at all.
    const pending = fakePartner(200, { envelope, shareDMT })
    expect((await reveal(pending, TOKEN, { status: 'PENDING' })).status).toBe(403)
    expect(pending.calls).toEqual([])
    expect((await reveal(fakePartner(200, {}), DMT_SVC)).status).toBe(403)
  })

  it('keeps only the agency’s own listed internal fields', async () => {
    const d = deps({ org: 'customs' })
    expect((await route(d, 'PUT', '/records/cusdec-1', auth, { vid, fields: { declarant: 'X', nic: '1' } })).status).toBe(400)
    expect((await route(d, 'PUT', '/records/cusdec-1', auth, { vid, fields: { declarant: 'X Ltd (synthetic)' } })).status).toBe(200)
    expect((await route(d, 'PUT', `/records/${encodeURIComponent('CUSDEC/2026/0001')}`, auth, { vid, fields: { declarant: 'X' } })).status).toBe(200)
    expect((await route(d, 'PUT', '/records/CUSDEC/2026/0001', auth, { vid, fields: { declarant: 'X' } })).status).toBe(404)
  })

  describe('v1.8 vehicle identity (DMT service)', () => {
    const keys = { kv: Buffer.alloc(32, 1), kown: Buffer.alloc(32, 2) }
    const chassis = 'NZE161-3150001'
    const realVid = vidOf(keys, chassis)
    const nic = '199012345678'
    const dmtDeps = (rows: Record<string, unknown>[] = [], facts: FakeFacts = {}) =>
      deps({ org: 'dmt', keys, db: fakeDb(rows), ledger: fakeLedger({}, envelope.anchor, facts) })

    it('finds a vehicle by plate for agency desks only, and keeps the plate out of the path', async () => {
      const d = dmtDeps([{ vid: realVid, from_day: 0, to_day: null }])
      const ok = await route(d, 'POST', '/plates/lookup', as(POLICE_DESK), { plate: 'cab-1234' })
      expect(ok.status).toBe(200)
      expect((ok.body as { vid: string }).vid).toBe(realVid)
      expect((await route(d, 'POST', '/plates/lookup', as(BUYER), { plate: 'CAB-1234' })).status).toBe(403)
      expect((await route(d, 'POST', '/plates/lookup', as(POLICE_DESK), { plate: 'X' })).status).toBe(400)
      expect((await route(dmtDeps([]), 'POST', '/plates/lookup', auth, { plate: 'CAB-1234' })).status).toBe(404)
    })

    it('describes a vehicle to the buyer page without its plate', async () => {
      const row = { make: 'Toyota', model: 'Axio', year: 2016, engine_cc: 1496, fuel: 'PETROL', colour: 'white', desc_anchor: null }
      const r = await route(dmtDeps([row]), 'POST', '/vehicles/description', as(BUYER), { vid: realVid })
      expect(r.status).toBe(200)
      expect(r.body).toMatchObject({ make: 'Toyota', model: 'Axio', year: 2016, committed: false })
      expect(JSON.stringify(r.body)).not.toMatch(/plate/i)
      // "committed" is recomputed from the stored fields: an edited row loses it.
      const anchor = descAnchorOf(keys, realVid, { make: 'Toyota', model: 'Axio', year: 2016, engineCc: 1496, fuel: 'PETROL' })
      const kept = await route(dmtDeps([row], { descAnchor: anchor }), 'POST', '/vehicles/description', as(BUYER), { vid: realVid })
      expect(kept.body).toMatchObject({ committed: true })
      const edited = await route(dmtDeps([{ ...row, model: 'Premio', desc_anchor: anchor }], { descAnchor: anchor }), 'POST', '/vehicles/description', as(BUYER), { vid: realVid })
      expect(edited.body).toMatchObject({ committed: false, mismatch: true })
      expect((await route(dmtDeps([row]), 'POST', '/vehicles/description', as(POLICE_SVC), { vid: realVid })).status).toBe(403)
    })

    it('answers the chassis match with one bit, never a digest', async () => {
      const yes = await route(dmtDeps(), 'POST', '/vehicles/match', as(BUYER), { vid: realVid, chassis: 'nze161 3150001' })
      expect(yes.body).toEqual({ match: true })
      const no = await route(dmtDeps(), 'POST', '/vehicles/match', as(BUYER), { vid: realVid, chassis: 'RU1-4410071' })
      expect(no.body).toEqual({ match: false })
      expect(JSON.stringify(no.body)).not.toMatch(/[0-9a-f]{64}/)
      expect((await route(dmtDeps(), 'POST', '/vehicles/match', as(BUYER), { vid: realVid, chassis: '' })).body).toEqual({ match: false })
      expect((await route(dmtDeps(), 'POST', '/vehicles/match', as(BUYER), { vid: 'x', chassis: chassis })).status).toBe(400)
    })

    it('confirms a theft reporter only after a report is on the ledger, and only for the owner of that car', async () => {
      const ownerRow = [{ owner_tag: ownerTagOf(keys, nic) }]
      confirmations = []
      const noReport = await route(dmtDeps(ownerRow, { theftReported: false }), 'POST', '/theft/check', as(POLICE_DESK), { vid: realVid, nic, reportRef: 'PR/1' })
      expect(noReport.status).toBe(409)
      const stranger = await route(dmtDeps(ownerRow, { theftReported: true }), 'POST', '/theft/check', as(POLICE_DESK), { vid: realVid, nic: '200012345678', reportRef: 'PR/1' })
      expect(stranger.body).toMatchObject({ confirmed: false })
      expect(confirmations).toEqual([])
      const owner = await route(dmtDeps(ownerRow, { theftReported: true }), 'POST', '/theft/check', as(POLICE_DESK), { vid: realVid, nic, reportRef: 'PR/1' })
      expect(owner.body).toMatchObject({ confirmed: true })
      expect(confirmations).toEqual(['PR/1'])
      expect(JSON.stringify(owner.body)).not.toContain(ownerTagOf(keys, nic))
      expect((await route(dmtDeps(ownerRow, { theftReported: true }), 'POST', '/theft/check', as(CUSTOMS_DESK), { vid: realVid, nic, reportRef: 'PR/1' })).status).toBe(403)
      expect((await route(dmtDeps(ownerRow, { theftReported: true }), 'POST', '/theft/check', auth, { vid: realVid, nic, reportRef: 'PR/1' })).status).toBe(403)
    })

    it('stops a NIC search after three refusals per report, and retries a committed confirmation without writing again', async () => {
      const ownerTag = ownerTagOf(keys, nic)
      const counted = (refused: number) => ({
        ...dmtDeps([], { theftReported: true }),
        db: { query: async (text: string) => ({ rows: /count\(\*\)/.test(text) ? [{ n: refused }] : [{ owner_tag: ownerTag }] }) },
      })
      confirmations = []
      const fourth = await route(counted(3), 'POST', '/theft/check', as(POLICE_DESK), { vid: realVid, nic, reportRef: 'PR/1' })
      expect(fourth.status).toBe(429)
      expect(confirmations).toEqual([])
      expect((await route(counted(2), 'POST', '/theft/check', as(POLICE_DESK), { vid: realVid, nic, reportRef: 'PR/1' })).body).toMatchObject({ confirmed: true })
      expect(confirmations).toEqual(['PR/1'])
      const retry = await route(dmtDeps([{ owner_tag: ownerTag }], { theftReported: true, confirmed: true }), 'POST', '/theft/check', as(POLICE_DESK), { vid: realVid, nic, reportRef: 'PR/1' })
      expect(retry.body).toMatchObject({ confirmed: true })
      expect(confirmations).toEqual(['PR/1'])
    })

    it('counts wrong NICs one request at a time, so parallel guesses cannot pass the cap', async () => {
      let refused = 0
      const db = { query: async (text: string, params?: unknown[]) => {
        if (/count\(\*\)/.test(text))
          return { rows: [{ n: refused }] }
        if (/INSERT INTO release_log/.test(text) && String(params?.[3]).startsWith('refused: not the owner'))
          refused++
        return { rows: [{ owner_tag: ownerTagOf(keys, nic) }] }
      } }
      const d = { ...dmtDeps([], { theftReported: true }), db }
      const replies = await Promise.all([1, 2, 3, 4, 5, 6].map(n =>
        route(d, 'POST', '/theft/check', as(POLICE_DESK), { vid: realVid, nic: `20001234567${n}`, reportRef: 'PR/9' })))
      expect(replies.map(r => r.status).sort()).toEqual([200, 200, 200, 429, 429, 429])
      expect(refused).toBe(3)
    })

    it('reads a report the way the chaincode does: newer than the last transfer or clearance', () => {
      const ev = (Type: string, EvidenceAnchor = '') => ({ Type, EvidenceAnchor })
      expect(reportState([ev('THEFT_REPORT', 'PR/1')], 'PR/1')).toEqual({ reported: true, confirmed: false })
      expect(reportState([ev('THEFT_REPORT', 'PR/1'), ev('OWNER_CONFIRMED', 'PR/1')], 'PR/1')).toEqual({ reported: true, confirmed: true })
      expect(reportState([ev('THEFT_REPORT', 'PR/1'), ev('TRANSFER')], 'PR/1')).toEqual({ reported: false, confirmed: false })
      expect(reportState([ev('THEFT_REPORT', 'PR/1'), ev('OWNER_CONFIRMED', 'PR/1'), ev('THEFT_CLEAR')], 'PR/1').confirmed).toBe(false)
      expect(reportState([ev('THEFT_REPORT', 'PR/2')], 'PR/1').reported).toBe(false)
    })

    it('computes the description fingerprint byte for byte (vector checked with Python hmac)', () => {
      const d = { make: 'Toyota', model: 'Corolla Axio', year: 2016, engineCc: 1496, fuel: 'petrol' }
      expect(descAnchorOf(keys, 'a'.repeat(64), d)).toBe('c068b36f9f0f86185921c6fbc681f43f849bdf7964a87b22b44aafa370d67a5a')
    })

    it('stores a description only if it reproduces the ledger anchor, and only the colour may change later', async () => {
      const d = { vid: realVid, make: 'Toyota', model: 'Axio', year: 2016, engineCc: 1496, fuel: 'petrol', colour: 'white' }
      const anchor = descAnchorOf(keys, realVid, { ...d, fuel: 'PETROL' })
      const kept = await route(dmtDeps([], { descAnchor: anchor }), 'PUT', '/vehicles/description', auth, d) // fake db returns no row
      expect(kept.status).toBe(409)
      expect(JSON.stringify(kept.body)).toMatch(/only the colour may change/)
      const wrong = await route(dmtDeps([], { descAnchor: 'f'.repeat(64) }), 'PUT', '/vehicles/description', auth, d)
      expect(wrong.status).toBe(409)
      expect(JSON.stringify(wrong.body)).toMatch(/committed on the ledger/)
      expect((await route(dmtDeps([], { descAnchor: anchor }), 'PUT', '/vehicles/description', as(BUYER), d)).status).toBe(403)
      expect((await route(dmtDeps([], {}), 'PUT', '/vehicles/description', auth, { ...d, year: 1800 })).status).toBe(400)
    })

    it('refuses a plate already open on another vehicle', async () => {
      const taken = {
        ...dmtDeps(),
        db: { query: async (text: string) => {
          if (/^\s*SELECT/i.test(text))
            return { rows: [] }
          throw Object.assign(new Error('duplicate key'), { code: '23505' })
        } },
      }
      expect((await route(taken, 'PUT', '/vehicles/plate', auth, { vid: realVid, plate: 'CAB-1234' })).status).toBe(409)
      expect((await route(dmtDeps(), 'PUT', '/vehicles/plate', as(POLICE_DESK), { vid: realVid, plate: 'CAB-1234' })).status).toBe(403)
    })
  })
})
