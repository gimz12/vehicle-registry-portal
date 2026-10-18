import { createPrivateKey } from 'node:crypto'
import { promises as fs } from 'node:fs'
import { join } from 'node:path'
import * as grpc from '@grpc/grpc-js'
import { connect, hash, signers } from '@hyperledger/fabric-gateway'

/**
* The custodian's own view of the ledger, through its own peer and enrolled
* identity, so nothing here trusts the portal.
*/
export interface LedgerCheck { fn: string, peer: string, status: string, ms: number }
export interface Ledger {
  /** The case's status and vehicle as the ledger records them; throws "not found" for an unknown case. */
  caseFor: (caseId: string) => Promise<{ status: string, vid: string, ownerAnchor: string, check: LedgerCheck }>
  /** The envelope anchor committed for the vehicle, read as this custodian's own purpose. */
  anchorOf: (vid: string) => Promise<{ anchor: string, check: LedgerCheck }>
  /** Commits IDENTITY_REVEAL on the case's access log (Police identity); returns the log. */
  recordReveal: (caseId: string, vid: string) => Promise<{ accessLog: string[], check: LedgerCheck }>
  /** The vehicle as the DMT reads it: whether this report exists, and the description anchor. */
  vehicleFacts: (vid: string, reportRef?: string) =>
  Promise<{ theftReported: boolean, confirmed: boolean, events: number, descAnchor: string, check: LedgerCheck }>
  /** Commits OWNER_CONFIRMED for the report (DMT identity). */
  confirmReporter: (vid: string, reportRef: string, recordId: string) => Promise<{ check: LedgerCheck }>
}

export interface LedgerConfig {
  org: number // 1 = DMT, 2 = Police (network/lib.sh)
  peerEndpoint: string // e.g. peer0.org2.example.com:9051 from inside the fabric_test network
  organizations: string // mounted peerOrganizations directory
  channel: string
  chaincode: string
}

async function firstFile(directory: string): Promise<string> {
  const files = (await fs.readdir(directory)).sort()
  if (!files[0])
    throw new Error(`no credential file in ${directory}`)
  return join(directory, files[0])
}

const reason = (error: unknown) => {
  const details = (error as { details?: { message?: string }[] }).details?.map(d => d.message).filter(Boolean)
  return details?.length ? details.join('; ') : (error as Error).message
}

/** Mirrors service.go reportedSince and confirmedSince. */
export function reportState(events: { Type?: string, EvidenceAnchor?: string }[], ref: string) {
  let reported = false
  let confirmed = false
  for (const e of events) {
    if (e.Type === 'TRANSFER' || e.Type === 'THEFT_CLEAR')
      reported = confirmed = false
    else if (ref && e.EvidenceAnchor === ref && e.Type === 'THEFT_REPORT')
      reported = true
    else if (ref && e.EvidenceAnchor === ref && e.Type === 'OWNER_CONFIRMED')
      confirmed = true
  }
  return { reported: !!ref && reported, confirmed: !!ref && reported && confirmed }
}

export function fabricLedger(cfg: LedgerConfig): Ledger {
  const peer = `peer0.org${cfg.org}.example.com`
  const base = join(cfg.organizations, `org${cfg.org}.example.com`)
  const user = join(base, 'users', `User1@org${cfg.org}.example.com`, 'msp')

  async function transact(submit: boolean, contract: string, fn: string, ...args: string[]): Promise<{ value: unknown, check: Omit<LedgerCheck, 'status'> }> {
    const started = Date.now()
    // Credentials first, so a missing file cannot leave an open gRPC client behind.
    const tlsRoot = await fs.readFile(join(base, 'peers', peer, 'tls/ca.crt'))
    const credentials = await fs.readFile(await firstFile(join(user, 'signcerts')))
    const key = createPrivateKey(await fs.readFile(await firstFile(join(user, 'keystore'))))
    const client = new grpc.Client(cfg.peerEndpoint, grpc.credentials.createSsl(tlsRoot))
    const gateway = connect({
      client,
      identity: { mspId: `Org${cfg.org}MSP`, credentials },
      signer: signers.newPrivateKeySigner(key),
      hash: hash.sha256,
      evaluateOptions: () => ({ deadline: Date.now() + 5000 }),
      endorseOptions: () => ({ deadline: Date.now() + 15000 }),
      submitOptions: () => ({ deadline: Date.now() + 5000 }),
      commitStatusOptions: () => ({ deadline: Date.now() + 60000 }),
    })
    try {
      const target = gateway.getNetwork(cfg.channel).getContract(cfg.chaincode, contract)
      const bytes = submit ? await target.submitTransaction(fn, ...args) : await target.evaluateTransaction(fn, ...args)
      return { value: JSON.parse(Buffer.from(bytes).toString('utf8')), check: { fn, peer, ms: Date.now() - started } }
    }
    catch (error) {
      throw new Error(reason(error), { cause: error })
    }
    finally {
      gateway.close()
      client.close()
    }
  }

  return {
    async caseFor(caseId) {
      const { value, check } = await transact(false, 'IdentityManager', 'GetRevealAudit', caseId)
      const c = value as { status?: string, vid?: string, ownerAnchor?: string }
      return { status: c.status ?? '', vid: c.vid ?? '', ownerAnchor: c.ownerAnchor ?? '', check: { ...check, status: c.status ?? '' } }
    },
    async anchorOf(vid) {
      const { value, check } = await transact(false, 'VehicleRegistry', 'GetVehicleHistoryFor', cfg.org === 2 ? 'POLICE' : 'DMT', vid, '')
      const anchor = (value as { record?: { envelopeAnchor?: string } }).record?.envelopeAnchor ?? ''
      return { anchor, check: { ...check, status: anchor ? 'ANCHOR' : 'NO_ANCHOR' } }
    },
    async vehicleFacts(vid, reportRef = '') {
      const { value, check } = await transact(false, 'VehicleRegistry', 'GetVehicleHistoryFor', 'DMT', vid, '')
      type Ev = { Type?: string, EvidenceAnchor?: string }
      const record = (value as { record?: { descAnchor?: string, vehicle?: { Events?: Ev[] } } }).record
      const events = record?.vehicle?.Events ?? []
      const { reported: theftReported, confirmed } = reportState(events, reportRef)
      return {
        theftReported, confirmed, events: events.length, descAnchor: record?.descAnchor ?? '',
        check: { ...check, status: theftReported ? 'REPORTED' : 'READ' },
      }
    },
    async confirmReporter(vid, reportRef, recordId) {
      const { check } = await transact(true, 'VehicleRegistry', 'ConfirmReporter',
        JSON.stringify({ recordId, vid, reference: reportRef }))
      return { check: { ...check, status: 'COMMITTED' } }
    },
    async recordReveal(caseId, vid) {
      const { value, check } = await transact(true, 'IdentityManager', 'RecordIdentityAccess', caseId, vid)
      const accessLog = (value as { accessLog?: string[] }).accessLog ?? []
      return { accessLog, check: { ...check, status: 'COMMITTED' } }
    },
  }
}
