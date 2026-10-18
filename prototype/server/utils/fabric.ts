import { createPrivateKey } from 'node:crypto'
import { promises as fs } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'
import * as grpc from '@grpc/grpc-js'
import { connect, hash, signers } from '@hyperledger/fabric-gateway'
import type { Contract, Gateway } from '@hyperledger/fabric-gateway'
import type { Agency, PredicateResult, Purpose, RawDisclosure, RawHistory, RevealRequest, VehicleDisclosure, VehicleHistory } from '~~/shared/disclosure'
import { normalizeDisclosure, normalizeHistory } from '~~/shared/disclosure'
import { custodyFleet } from '~~/server/utils/custody'
import { endorsersOf } from '~~/server/utils/endorsers'
import { publish } from '~~/server/utils/liveBus'

export interface FabricRuntimeConfig {
  channelName: string
  chaincodeName: string
  /** peerOrganizations directory of the lab network; empty = the fabric-samples default. */
  organizationsPath: string
  /** Host the peers' ports are published on. */
  peerHost: string
  /** File-backed off-ledger store, used only in custodyMode "files" (and for the demo keys file). */
  offchainDir: string
  /** "services" (per-organisation custody containers) or "files" (fallback); see server/utils/custody.ts. */
  custodyMode: string
  custodyUrls: string
  custodyEnvFile: string
  /** Optional "<k_v hex>:<k_own hex>"; see server/utils/handles.ts. */
  registryKeys: string
  /** When set, disclosures are read from cmd/disclose output instead of Fabric. */
  demoDir: string
  /** "name=url,name=url": operations endpoints whose metrics the live view polls. */
  ops: string
}

export interface Identity {
  mspId: string
  peerEndpoint: string
  peerHostAlias: string
  cryptoPath: string
  user: string
  peer: string
}

const defaultOrganizations = join(homedir(), 'fabric-registry/fabric-samples/test-network/organizations/peerOrganizations')

/** One enrolled user of organisation n on the five-organisation lab network (ports as network/lib.sh). */
export function orgIdentity(config: FabricRuntimeConfig, n: number, user = 'User1'): Identity {
  return {
    mspId: `Org${n}MSP`,
    peerEndpoint: `${config.peerHost || 'localhost'}:${7051 + 2000 * (n - 1)}`,
    peerHostAlias: `peer0.org${n}.example.com`,
    cryptoPath: join(config.organizationsPath || defaultOrganizations, `org${n}.example.com`),
    user: `${user}@org${n}.example.com`,
    peer: `peer0.org${n}.example.com`,
  }
}

/**
 * Which enrolled identity speaks for which purpose. Org1 DMT (also buyer and
 * assembler), Org2 Police, Org3 Customs, Org4 VET, Org5 insurers.
 */
const ORG_OF: Record<string, number> = { POLICE: 2, POLICE_APPROVER: 2, CUSTOMS: 3, VET: 4, INSURER: 5 }
// The officer who requests a case cannot approve it, so approvals use a second Police identity.
const USER_OF: Record<string, string> = { ASSEMBLER: 'assembler1', POLICE_APPROVER: 'police2', CUSTOMS: 'customs1', VET: 'vet1' }
// A claim is filed under the certificate's registry.party: one identity per lab insurer.
const INSURER_USERS: Record<string, string> = { 'insurer-b': 'insurer2', 'insurer-c': 'insurer3' }

export function identityFor(config: FabricRuntimeConfig, purpose: Purpose | Agency | 'POLICE_APPROVER', party = ''): Identity {
  const user = purpose === 'INSURER' ? (INSURER_USERS[party] ?? 'insurer1') : (USER_OF[purpose] ?? 'User1')
  return orgIdentity(config, ORG_OF[purpose] ?? 1, user)
}

async function firstFile(directory: string): Promise<string> {
  const files = (await fs.readdir(directory)).sort()
  if (!files[0])
    throw new Error(`No credential file found in ${directory}`)
  return join(directory, files[0])
}

/** One gateway connection for an enrolled identity; the caller closes both handles. */
export async function openGateway(identity: Identity): Promise<{ gateway: Gateway, close: () => void }> {
  const tlsRootCert = await fs.readFile(join(identity.cryptoPath, 'peers', identity.peer, 'tls/ca.crt'))
  const certificate = await fs.readFile(await firstFile(join(identity.cryptoPath, 'users', identity.user, 'msp/signcerts')))
  const privateKeyPem = await fs.readFile(await firstFile(join(identity.cryptoPath, 'users', identity.user, 'msp/keystore')))
  const client = new grpc.Client(
    identity.peerEndpoint,
    grpc.credentials.createSsl(tlsRootCert),
    { 'grpc.ssl_target_name_override': identity.peerHostAlias },
  )
  const gateway = connect({
    client,
    identity: { mspId: identity.mspId, credentials: certificate },
    signer: signers.newPrivateKeySigner(createPrivateKey(privateKeyPem)),
    hash: hash.sha256,
    evaluateOptions: () => ({ deadline: Date.now() + 5000 }),
    endorseOptions: () => ({ deadline: Date.now() + 15000 }),
    submitOptions: () => ({ deadline: Date.now() + 5000 }),
    commitStatusOptions: () => ({ deadline: Date.now() + 60000 }),
  })
  const close = () => {
    gateway.close()
    client.close()
  }
  return { gateway, close }
}

async function withContract<T>(
  config: FabricRuntimeConfig,
  identity: Identity,
  contractName: 'VehicleRegistry' | 'FraudDetector' | 'IdentityManager',
  fn: (contract: Contract) => Promise<T>,
): Promise<T> {
  const { gateway, close } = await openGateway(identity)
  try {
    return await fn(gateway.getNetwork(config.channelName).getContract(config.chaincodeName, contractName))
  }
  catch (error) {
    throw new Error(unwrapGatewayError(error), { cause: error })
  }
  finally {
    close()
  }
}

/** contract.submit in its real steps (endorse, order, commit), each reported to the live view. */
async function submitWithPhases(
  contract: Contract,
  actor: string,
  op: string,
  options: { arguments: string[], transientData?: Record<string, string> },
): Promise<Uint8Array> {
  const started = performance.now()
  const ms = () => Math.round(performance.now() - started)
  publish({ kind: 'phase', op, actor, phase: 'proposed', transient: !!options.transientData })
  try {
    const transaction = await contract.newProposal(op, options).endorse()
    publish({ kind: 'phase', op, actor, phase: 'endorsed', txId: transaction.getTransactionId(), endorsers: endorsersOf(transaction), ms: ms() })
    const submitted = await transaction.submit()
    publish({ kind: 'phase', op, actor, phase: 'submitted', txId: submitted.getTransactionId() })
    const status = await submitted.getStatus()
    if (!status.successful)
      throw new Error(`transaction ${status.transactionId} was not committed: status ${status.code}`)
    publish({ kind: 'phase', op, actor, phase: 'committed', txId: status.transactionId, detail: `block ${status.blockNumber}`, ms: ms() })
    return submitted.getResult()
  }
  catch (error) {
    publish({ kind: 'phase', op, actor, phase: 'refused', detail: unwrapGatewayError(error), ms: ms() })
    throw error
  }
}

/** The gateway hides the chaincode's reason ("registry: ...") in `details`; surface it. */
function unwrapGatewayError(error: unknown): string {
  const e = error as { message?: string, details?: { message?: string }[] }
  const detail = e.details?.map(d => d.message ?? '').find(m => m.length > 0)
  const text = detail ?? e.message ?? 'Unknown Fabric error'
  const registry = text.match(/registry: [^"\n]*/)?.[0]
  return registry ?? text.replace(/^chaincode response \d+, /, '')
}

const decode = <T>(bytes: Uint8Array): T => JSON.parse(new TextDecoder().decode(bytes)) as T

async function readDemo<T>(config: FabricRuntimeConfig, vid: string, file: string): Promise<T> {
  const path = join(config.demoDir, vid, `${file}.json`)
  try {
    return JSON.parse(await fs.readFile(path, 'utf8')) as T
  }
  catch {
    throw new Error(`demo: ${file} not found for this vehicle`)
  }
}

export async function queryVehicleDisclosure(
  config: FabricRuntimeConfig,
  vid: string,
  purpose: Purpose,
  caseId = '',
): Promise<VehicleDisclosure> {
  if (config.demoDir) {
    const file = purpose === 'POLICE' && caseId ? 'POLICE-OWNER' : purpose
    return normalizeDisclosure(await readDemo<RawDisclosure>(config, vid, file))
  }
  // An owner-scope read writes the case's access log, so it is submitted; evaluate would discard the audit write.
  const ownerScope = purpose === 'POLICE' && caseId !== ''
  // The fleet goes to the peers as transient data. Fetched first so a custodian's refusal keeps its own status.
  const fleet = ownerScope ? await custodyFleet(config, vid, caseId, 'read') : []
  return withContract(config, identityFor(config, purpose), 'FraudDetector', async (contract) => {
    const started = performance.now()
    const result = ownerScope
      ? await submitWithPhases(contract, purpose, 'GetDisclosureFor', {
          arguments: [purpose, vid, caseId],
          transientData: { fleet: JSON.stringify(fleet) },
        })
      : await contract.evaluateTransaction('GetDisclosureFor', purpose, vid, caseId)
    if (!ownerScope)
      publish({ kind: 'read', op: 'GetDisclosureFor', actor: purpose, detail: `vehicle ${vid.slice(0, 8)}…` })
    // End-to-end gateway round trip, logged as a lab latency figure.
    console.info(`fabric GetDisclosureFor ${purpose}${ownerScope ? ' (submit)' : ''} ${Math.round(performance.now() - started)} ms`)
    return normalizeDisclosure(decode<RawDisclosure>(result))
  })
}

export async function queryVehicleHistory(
  config: FabricRuntimeConfig,
  vid: string,
  purpose: Purpose,
  caseId = '',
): Promise<VehicleHistory> {
  if (config.demoDir)
    return normalizeHistory(await readDemo<RawHistory>(config, vid, `history-${purpose}`))
  return withContract(config, identityFor(config, purpose), 'VehicleRegistry', async (contract) => {
    const result = await contract.evaluateTransaction('GetVehicleHistoryFor', purpose, vid, caseId)
    publish({ kind: 'read', op: 'GetVehicleHistoryFor', actor: purpose, detail: `vehicle ${vid.slice(0, 8)}…` })
    return normalizeHistory(decode<RawHistory>(result))
  })
}

interface RawReveal {
  id: string
  vid: string
  ruleId: string
  groundCode: string
  groundRef: string
  requestedBy: string
  dmtApproved: boolean
  policeApproved: boolean
  status: RevealRequest['status']
  reason?: string
  accessLog?: string[]
  fleetSize?: number
}

export async function submitRevealRequest(
  config: FabricRuntimeConfig,
  as: 'POLICE' | 'DMT',
  input: { caseId: string, vid: string, ruleId: string, ground: string, groundRef: string },
): Promise<RevealRequest> {
  if (config.demoDir)
    throw new Error('demo mode is read-only; escalation needs the live Fabric network')
  return withContract(config, identityFor(config, as), 'IdentityManager', async (contract) => {
    const result = await submitWithPhases(contract, as, 'CreateRevealRequest', { arguments: [input.caseId, input.vid, input.ruleId, input.ground, input.groundRef] })
    return decode<RawReveal>(result)
  })
}

export async function submitRevealApproval(
  config: FabricRuntimeConfig,
  as: 'POLICE' | 'DMT',
  caseId: string,
): Promise<RevealRequest> {
  if (config.demoDir)
    throw new Error('demo mode is read-only; escalation needs the live Fabric network')
  if (as === 'POLICE') {
    return withContract(config, identityFor(config, 'POLICE_APPROVER'), 'IdentityManager', async (contract) => {
      return decode<RawReveal>(await submitWithPhases(contract, 'POLICE_APPROVER', 'ApproveReveal', { arguments: [caseId] }))
    })
  }
  // The DMT's approval attests the owner's fleet from its custody index.
  const pending = await queryRevealAudit(config, caseId, 'DMT')
  const fleet = await custodyFleet(config, pending.vid, caseId, 'approve')
  return withContract(config, identityFor(config, 'DMT'), 'IdentityManager', async (contract) => {
    const result = await submitWithPhases(contract, 'DMT', 'ApproveReveal', { arguments: [caseId], transientData: { fleet: JSON.stringify(fleet) } })
    return decode<RawReveal>(result)
  })
}

/** DMT owner-scope predicate (F6a or F6b): the chaincode returns and records the verdict only. */
export async function submitOwnerPredicate(config: FabricRuntimeConfig, vid: string, ruleId: 'F6a' | 'F6b'): Promise<PredicateResult> {
  if (config.demoDir)
    throw new Error('demo mode is read-only; the owner-scope predicate needs the live Fabric network')
  const fleet = await custodyFleet(config, vid, '', 'predicate')
  return withContract(config, identityFor(config, 'DMT'), 'FraudDetector', async (contract) => {
    const result = await submitWithPhases(contract, 'DMT', 'EvaluateOwnerPredicate', { arguments: [vid, ruleId], transientData: { fleet: JSON.stringify(fleet) } })
    return decode<PredicateResult>(result)
  })
}

export async function queryRevealAudit(config: FabricRuntimeConfig, caseId: string, as: 'POLICE' | 'DMT' = 'POLICE'): Promise<RevealRequest> {
  if (config.demoDir)
    throw new Error('demo mode has no reveal audit')
  // The audit read is gated to the two custodians in chaincode.
  return withContract(config, identityFor(config, as), 'IdentityManager', async (contract) => {
    const result = await contract.evaluateTransaction('GetRevealAudit', caseId)
    publish({ kind: 'read', op: 'GetRevealAudit', actor: as, detail: `case ${caseId}` })
    return decode<RawReveal>(result)
  })
}

export async function submitRevealRejection(
  config: FabricRuntimeConfig,
  as: 'POLICE' | 'DMT',
  caseId: string,
  reason: string,
): Promise<RevealRequest> {
  if (config.demoDir)
    throw new Error('demo mode is read-only; escalation needs the live Fabric network')
  return withContract(config, identityFor(config, as), 'IdentityManager', async (contract) => {
    const result = await submitWithPhases(contract, as, 'RejectReveal', { arguments: [caseId, reason] })
    return decode<RawReveal>(result)
  })
}

/** Writes the identity-reveal entry on the case's access log (Police identity). */
export async function submitIdentityAccess(
  config: FabricRuntimeConfig,
  caseId: string,
  vid: string,
): Promise<RevealRequest> {
  return withContract(config, identityFor(config, 'POLICE'), 'IdentityManager', async (contract) => {
    const result = await submitWithPhases(contract, 'POLICE', 'RecordIdentityAccess', { arguments: [caseId, vid] })
    return decode<RawReveal>(result)
  })
}

/** The on-chain envelope anchor, read as Police under the case. */
export async function queryEnvelopeAnchor(config: FabricRuntimeConfig, vid: string, caseId: string): Promise<string> {
  return withContract(config, identityFor(config, 'POLICE'), 'VehicleRegistry', async (contract) => {
    const result = await contract.evaluateTransaction('GetVehicleHistoryFor', 'POLICE', vid, caseId)
    publish({ kind: 'offledger', what: 'anchor', actor: 'POLICE', detail: `case ${caseId}` })
    const view = decode<{ record: { envelopeAnchor: string } }>(result)
    return view.record.envelopeAnchor
  })
}

/** Agency desk write under the agency's own certificate; the chaincode's refusal text is the error. */
export async function submitAsAgency(
  config: FabricRuntimeConfig,
  agency: Agency,
  contractName: 'VehicleRegistry' | 'FraudDetector' | 'IdentityManager',
  fn: string,
  args: string[],
  party = '',
): Promise<unknown> {
  if (config.demoDir)
    throw new Error('demo mode is read-only; agency writes need the live Fabric network')
  return withContract(config, identityFor(config, agency, party), contractName, async (contract) => {
    const result = await submitWithPhases(contract, agency, fn, { arguments: args })
    return result.length ? decode<unknown>(result) : null
  })
}
