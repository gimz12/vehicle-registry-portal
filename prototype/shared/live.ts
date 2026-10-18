/**
 * Live-network events: what the portal publishes and what the ledger reports back.
 * The /live page draws nothing that no event reported.
 */
/**
 * Off-ledger events. handoff is custodian to custodian (actor: the sender or fetcher);
 * opened means the Police service decrypted and audited the record itself.
 */
export type OffLedgerWhat = 'seal' | 'index' | 'anchor' | 'shares' | 'open' | 'store' | 'release' | 'fleet' | 'refused'
  | 'handoff' | 'reveal' | 'opened' | 'lookup' | 'match' | 'ownercheck'

export type Phase = 'proposed' | 'endorsed' | 'submitted' | 'committed' | 'refused'

export type LiveEvent =
  | { kind: 'phase', t: number, op: string, actor: string, phase: Phase, txId?: string, endorsers?: string[], detail?: string, transient?: boolean, ms?: number }
  | { kind: 'read', t: number, op: string, actor: string, detail?: string }
  | { kind: 'offledger', t: number, what: OffLedgerWhat, actor: string, detail?: string, service?: 'dmt' | 'police' | 'customs' | 'vet' | 'insurer' }
  | { kind: 'block', t: number, number: number, txCount: number, valid: number, invalid: number, codes: string[] }
  | { kind: 'chaincode', t: number, name: string, txId: string, block: number }
  | { kind: 'metrics', t: number, heights: Record<string, number> }
  | { kind: 'status', t: number, detail: string }

/** Scene node ids. Agencies on the left, one peer per organisation, the ordering service, custody below. */
export const NODES = [
  'CUSTOMS', 'ASSEMBLER', 'DMT', 'VET', 'INSURER', 'POLICE', 'BUYER',
  'PORTAL', 'PEER1', 'PEER2', 'PEER3', 'PEER4', 'PEER5', 'ORDERER',
  'CUSTODY_CUSTOMS', 'CUSTODY_DMT', 'CUSTODY_VET', 'CUSTODY_INSURER', 'CUSTODY_POLICE',
] as const
export type NodeId = typeof NODES[number]
export const PEERS = ['PEER1', 'PEER2', 'PEER3', 'PEER4', 'PEER5'] as const satisfies readonly NodeId[]

const AGENCY: Record<string, NodeId> = {
  BUYER: 'BUYER', DMT: 'DMT', INSURER: 'INSURER', POLICE: 'POLICE', POLICE_APPROVER: 'POLICE',
  CUSTOMS: 'CUSTOMS', ASSEMBLER: 'ASSEMBLER', VET: 'VET',
}
export const agencyNode = (actor: string): NodeId => AGENCY[actor] ?? 'DMT'

/** Org<n>MSP → PEER<n> (network/lib.sh: Org1 DMT, Org2 Police, Org3 Customs, Org4 VET, Org5 insurers). */
const peerOf = (msp: string): NodeId => (PEERS[Number(msp.replace(/\D/g, '')) - 1] ?? 'PEER1')
/** The gateway peer: the acting agency's own organisation's peer (server/utils/fabric.ts identityFor). */
const GATEWAY: Record<string, NodeId> = { POLICE: 'PEER2', CUSTOMS: 'PEER3', VET: 'PEER4', INSURER: 'PEER5' }
export const gatewayPeer = (actor: string): NodeId => GATEWAY[agencyNode(actor)] ?? 'PEER1'

/** One step: a sentence, and the packets that move during it (none, one, or several at once). */
export interface Step { from?: NodeId, to?: NodeId, also?: [NodeId, NodeId][], text: string }

// Captions avoid Fabric jargon: peer = an organisation's copy of the ledger, chaincode = the shared rules.
const PEER_NAME: Record<string, string> = {
  PEER1: 'the DMT’s copy of the ledger', PEER2: 'the Police copy of the ledger', PEER3: 'the Customs copy of the ledger',
  PEER4: 'the emission centres’ copy of the ledger', PEER5: 'the insurers’ copy of the ledger',
}
const PEER_ORG: Record<string, string> = { PEER1: 'DMT', PEER2: 'Police', PEER3: 'Customs', PEER4: 'emission centres’', PEER5: 'insurers’' }
const cap = (t: string) => t.charAt(0).toUpperCase() + t.slice(1)
const list = (xs: string[]) => xs.length < 2 ? (xs[0] ?? '') : `${xs.slice(0, -1).join(', ')} and ${xs[xs.length - 1]}`
const copies = (ps: NodeId[]) => `the ${list(ps.map(p => PEER_ORG[p] ?? p))} ${ps.length === 1 ? 'copy' : 'copies'}`

/** Who is acting, as a sentence subject. */
const WHO: Record<string, string> = {
  BUYER: 'a buyer', DMT: 'the DMT', INSURER: 'an insurer', POLICE: 'a Police officer', POLICE_APPROVER: 'a second Police officer',
  CUSTOMS: 'Customs', ASSEMBLER: 'the assembler', VET: 'an emission centre',
}
const who = (actor: string) => WHO[actor] ?? actor
/** Whose digital ID signs the request. */
const ID_OF: Record<string, string> = {
  BUYER: 'the buyer’s', DMT: 'the DMT’s', INSURER: 'the insurer’s', POLICE: 'the Police officer’s', POLICE_APPROVER: 'the second Police officer’s',
  CUSTOMS: 'Customs’', ASSEMBLER: 'the assembler’s', VET: 'the emission centre’s',
}
const idOf = (actor: string) => ID_OF[actor] ?? `${actor}’s`
/** What they asked for, in plain words. */
const OP: Record<string, string> = {
  CreateImportAttestation: 'record a vehicle import', RegisterVehicle: 'register a vehicle', TransferVehicle: 'transfer a vehicle to a new owner',
  SetRetiredStatus: 'change whether a vehicle is retired', SetTheftStatus: 'change a vehicle’s stolen status',
  SetWriteOffStatus: 'mark a vehicle as written off', AppendAccident: 'record an accident', AppendClaim: 'record an insurance claim',
  AppendEmission: 'record an emission test', AppendMileage: 'record a mileage reading', SetEncumbrance: 'record a finance agreement',
  ReleaseEncumbrance: 'record that a finance agreement ended', CreateRevealRequest: 'open a case to identify the owner',
  ApproveReveal: 'approve the case', RejectReveal: 'reject the case', EvaluateOwnerPredicate: 'check the owner’s other vehicles',
  RecordIdentityAccess: 'note on the case that the owner was identified',
  AppendTheftReport: 'record a theft report', ConfirmReporter: 'confirm that the reporter is the registered owner', GetDisclosureFor: 'see a vehicle’s record',
  GetVehicleHistoryFor: 'see a vehicle’s history', GetRevealAudit: 'look up a case',
}
const op = (name: string) => OP[name] ?? name
/** The chaincode's refusal without its "registry:" prefix. */
const reason = (detail?: string) => detail?.replace(/^registry:\s*/, '')
/** What the shared rules announced. */
const ANNOUNCED: Record<string, string> = {
  VehicleRegistered: 'a vehicle was registered', ImportAttestationCreated: 'an import was recorded',
  LifecycleEventAppended: 'a new event was added to a vehicle’s history', RetiredStatusChanged: 'a vehicle’s retired status changed',
  WriteOffStatusChanged: 'a vehicle’s written-off status changed', RevealRequested: 'a case to identify an owner was opened',
  RevealDecision: 'a decision was recorded on a case', OwnerScopeAccessed: 'an owner’s vehicles were looked at under a case',
  IdentityRevealed: 'an owner was identified under a case',
}
/** Why the peers rejected a change. */
const INVALID: Record<string, string> = {
  ENDORSEMENT_POLICY_FAILURE: 'not enough organisations signed it',
  MVCC_READ_CONFLICT: 'it clashed with another change made at the same moment',
}
const ORG_OF_MSP = ['DMT', 'Police', 'Customs', 'emission centres', 'insurers']
const orgOfMsp = (msp: string) => ORG_OF_MSP[Number(msp.replace(/\D/g, '')) - 1] ?? msp

/** "GetRevealAudit APPROVED on peer0.org1.example.com, 20 ms" → "the case is approved (20 ms)". */
const ANSWER: Record<string, string> = {
  APPROVED: 'the case is approved', PENDING: 'the case is still waiting for approval', REJECTED: 'the case was rejected',
  ANCHOR: 'the fingerprint is there', NO_ANCHOR: 'there is no fingerprint for this vehicle', COMMITTED: 'the note is saved',
}
export function plainCheck(detail = ''): string {
  return list(detail.split('; ').filter(Boolean).map((part) => {
    const m = part.match(/^\w+ (\w+) on \S+, (\d+) ms$/)
    return m ? `${ANSWER[m[1]!] ?? m[1]!.toLowerCase()} (${m[2]} ms)` : part
  }))
}

const names = (ps: NodeId[]) => copies(ps)

type Service = NonNullable<Extract<LiveEvent, { kind: 'offledger' }>['service']>
const STORE: Record<Service, NodeId> = {
  dmt: 'CUSTODY_DMT', police: 'CUSTODY_POLICE', customs: 'CUSTODY_CUSTOMS', vet: 'CUSTODY_VET', insurer: 'CUSTODY_INSURER',
}
const STORE_NAME: Record<Service, string> = {
  dmt: 'DMT database', police: 'Police database', customs: 'Customs database',
  vet: 'emission centre’s database', insurer: 'insurer’s database',
}
/** The agency whose own database a service is: each database is drawn connected to its organisation only. */
const OWNER: Record<Service, NodeId> = { dmt: 'DMT', police: 'POLICE', customs: 'CUSTOMS', vet: 'VET', insurer: 'INSURER' }
const OWNER_NAME: Record<Service, string> = { dmt: 'the DMT', police: 'the Police', customs: 'Customs', vet: 'the emission centre', insurer: 'the insurer' }
const caseOf = (detail?: string) => (detail?.split('; ')[0] ?? 'the case').replace(/^case /, 'case ')

/** A database's own ledger check: to its own organisation's copy of the ledger and back. */
function ledgerCheck(service: 'dmt' | 'police', what: string, detail = ''): Step[] {
  const node = STORE[service]
  const peer: NodeId = service === 'dmt' ? 'PEER1' : 'PEER2'
  return [
    {
      from: node, to: peer,
      text: `The ${STORE_NAME[service]} checks ${what} itself. It asks ${PEER_NAME[peer]} directly, not the portal`,
    },
    { from: peer, to: node, text: detail ? `The ledger answers: ${plainCheck(detail)}` : 'The ledger answers' },
  ]
}

/** The steps one event plays, in order: an optional hop and a sentence. Pure, so the page and tests agree. */
export function steps(e: LiveEvent): Step[] {
  switch (e.kind) {
    case 'phase': {
      const gw = gatewayPeer(e.actor)
      switch (e.phase) {
        case 'proposed': return [
          {
            from: agencyNode(e.actor), to: 'PORTAL',
            text: `${cap(who(e.actor))} asks to ${op(e.op)}. `
              + `The portal signs the request with ${idOf(e.actor)} own digital ID`
              + `${e.transient ? ' and attaches the owner’s list of vehicles' : ''}`,
          },
          { from: 'PORTAL', to: gw, text: `The request goes to ${PEER_NAME[gw]}, which runs the shared rules to check it` },
        ]
        case 'endorsed': {
          const signers = e.endorsers?.length ? [...new Set(e.endorsers.map(peerOf))] : [gw]
          const others = signers.filter(p => p !== gw)
          const out: Step[] = []
          if (others.length) {
            out.push({
              also: others.map(p => [gw, p] as [NodeId, NodeId]),
              text: 'A change needs a majority, three of the five organisations, to agree. '
                + `So it also asks ${names(others)}`,
            })
            out.push({
              also: others.map(p => [p, gw] as [NodeId, NodeId]),
              text: 'Each one checks the request against its own copy of the ledger and signs it if the rules allow it',
            })
          }
          out.push({ from: gw, to: 'PORTAL', text: `${signers.length} signature${signers.length === 1 ? '' : 's'} come back to the portal${e.ms ? `, in ${e.ms} ms` : ''}` })
          return out
        }
        case 'submitted': return [
          { from: 'PORTAL', to: gw, text: `The portal sends the signed change back to ${PEER_NAME[gw]}` },
          { from: gw, to: 'ORDERER', text: 'It goes to the ordering service, which puts changes in order and packs them into the next block' },
        ]
        case 'committed': return [
          { also: PEERS.map(p => ['ORDERER', p] as [NodeId, NodeId]), text: 'The new block goes to all five copies of the ledger. Each one checks the signatures and saves it' },
          { from: gw, to: 'PORTAL', text: `${cap(PEER_NAME[gw]!)} tells the portal the change is saved${e.ms ? ` (${e.ms} ms in total)` : ''}` },
        ]
        case 'refused': return [
          { from: gw, to: 'PORTAL', text: `The shared rules say no: ${reason(e.detail) ?? 'this change is not allowed'}. Nothing is saved` },
          { from: 'PORTAL', to: agencyNode(e.actor), text: `${cap(who(e.actor))} sees the reason on screen. The ledger is unchanged` },
        ]
      }
      break
    }
    case 'read': {
      const node = agencyNode(e.actor)
      const peer = gatewayPeer(e.actor)
      return [
        { from: node, to: 'PORTAL', text: `${cap(who(e.actor))} asks to ${op(e.op)}${e.detail ? ` (${e.detail})` : ''}` },
        { from: 'PORTAL', to: peer, text: `The question goes to ${PEER_NAME[peer]}. Reading changes nothing, so one copy is enough` },
        { from: peer, to: 'PORTAL', text: `It answers with only what ${who(e.actor)} is allowed to see` },
        { from: 'PORTAL', to: node, text: `${cap(who(e.actor))} sees the answer. Nothing was saved` },
      ]
    }
    case 'offledger': {
      const node = e.service ? STORE[e.service] : 'CUSTODY_DMT'
      const name = e.service ? STORE_NAME[e.service] : 'database'
      // The agency that owns this database: it is the only one drawn talking to it.
      const owner = e.service ? OWNER[e.service] : 'DMT'
      const ownerName = e.service ? OWNER_NAME[e.service] : 'the agency'
      switch (e.what) {
        // Files mode (fallback): one file store on the portal host; nobody checks the ledger.
        case 'seal': return [{
          also: [['DMT', 'CUSTODY_DMT'], ['CUSTODY_DMT', 'CUSTODY_POLICE']],
          text: 'Fallback mode: the locked owner record and both key halves are saved in a file on the portal computer. Only the record’s fingerprint is on the ledger',
        }]
        case 'index': return [{ from: 'DMT', to: 'CUSTODY_DMT', text: 'Fallback mode: the list of which vehicles belong to which owner is updated in that file' }]
        case 'shares': return [{
          also: [['CUSTODY_DMT', 'CUSTODY_POLICE'], ['CUSTODY_POLICE', 'POLICE']],
          text: 'Fallback mode: both key halves are read from the file on the portal computer. No database checks the case',
        }]
        case 'anchor': return [{ from: 'PEER2', to: 'PORTAL', text: 'The record’s fingerprint is read from the ledger for the approved case' }]
        // Services mode: each organisation's own container, database and password.
        case 'store':
          if (e.service === 'dmt') {
            return [
              { from: 'DMT', to: node, text: 'The DMT sends the locked owner record and both key halves to its own database' },
              ...ledgerCheck('dmt', 'that this record’s fingerprint was saved', e.detail?.split('; ')[1]),
              { text: 'It matches, so the DMT database keeps the record and its own key half. Next it passes the Police half on' },
            ]
          }
          if (e.service === 'police')
            return [{ from: owner, to: node, text: 'The Police key half is saved in the Police database. One half alone cannot open the record' }]
          return [{ from: owner, to: node, text: `${cap(ownerName)} saves its own notes about ${e.detail ?? 'this record'} in its own database. None of this goes on the ledger` }]
        case 'release': return [
          {
            from: owner, to: node,
            text: `${cap(ownerName)} asks its own database for its part, for ${caseOf(e.detail)}`,
          },
          ...ledgerCheck(e.service === 'dmt' ? 'dmt' : 'police', 'the case', e.detail?.split('; ').slice(1).join('; ')),
          {
            from: node, to: owner,
            text: e.service === 'dmt'
              ? 'The case is approved and the fingerprint matches. The DMT database hands over the locked record and its key half'
              : 'The case is approved. The Police database hands over its key half, which alone opens nothing',
          },
        ]
        case 'fleet':
          if (e.service === 'police') {
            return [
              { from: 'POLICE', to: node, text: `For their search, the Police ask their own database for the owner’s list of vehicles, for ${caseOf(e.detail)}` },
              ...ledgerCheck('police', 'the case', e.detail?.split('; ').slice(1).join('; ')),
            ]
          }
          if (e.detail?.startsWith('case')) {
            return [
              { from: 'DMT', to: node, text: `To approve ${caseOf(e.detail)}, the DMT asks its own database for the owner’s list of vehicles` },
              ...ledgerCheck('dmt', 'the case', e.detail.split('; ').slice(1).join('; ')),
              { from: node, to: 'DMT', text: 'The DMT database hands over the list. The DMT sends it to the ledger copies for this check only; it is never saved' },
            ]
          }
          return [
            { from: 'DMT', to: node, text: 'The DMT asks its own database for the owner’s list of vehicles, to run its own check' },
            { from: node, to: 'DMT', text: 'The list comes back. The shared rules decide whether the check is allowed' },
          ]
        // Organisation to organisation: the portal is not in these hops.
        case 'handoff': {
          const [item, ...checks] = (e.detail ?? '').split('; ')
          if (e.actor === 'DMT') {
            return [
              { from: 'CUSTODY_DMT', to: 'CUSTODY_POLICE', text: 'The DMT database passes the Police key half straight to the Police database. The portal is not involved' },
              ...ledgerCheck('police', 'that this record’s fingerprint was saved', checks.join('; ')),
              { text: 'It matches, so the Police database keeps its half. One half alone cannot open the record' },
            ]
          }
          const fleet = item?.startsWith('fleet')
          return [
            {
              from: 'CUSTODY_POLICE', to: 'CUSTODY_DMT',
              text: `The Police database asks the DMT database directly for ${fleet ? 'the owner’s list of vehicles' : 'the locked record and the DMT key half'}, for ${item?.replace(/^fleet, /, '') || 'the case'}`,
            },
            ...ledgerCheck('dmt', 'the case', checks.join('; ')),
            ...(fleet
              ? [
                  { from: 'CUSTODY_DMT', to: 'CUSTODY_POLICE', text: 'The case is approved. The DMT database sends the list to the Police database' } as Step,
                  { from: 'CUSTODY_POLICE', to: 'POLICE', text: 'The Police send the list to the ledger copies for this check only; it is never saved' } as Step,
                ]
              : [{ from: 'CUSTODY_DMT', to: 'CUSTODY_POLICE', text: 'The case is approved and the fingerprint matches. The DMT database sends the locked record and its key half to the Police database' } as Step]),
          ]
        }
        case 'reveal': return [
          { from: 'POLICE', to: node, text: `The Police ask their own database to open the owner’s record, for ${caseOf(e.detail)}` },
          ...ledgerCheck('police', 'the case', e.detail?.split('; ').slice(1).join('; ')),
        ]
        case 'opened': return [
          { text: 'The Police database joins the two key halves, checks the fingerprint once more, and unlocks the record' },
          { from: 'CUSTODY_POLICE', to: 'PEER2', text: 'Before showing anything, it writes a note on the ledger: this record was opened under this case' },
          { from: 'PEER2', to: 'CUSTODY_POLICE', text: 'The note is saved on the ledger, so the opening can always be traced' },
          { from: 'CUSTODY_POLICE', to: 'POLICE', text: 'Only now does the officer see the owner’s details. The portal never held a key' },
        ]
        // Finding a car by plate, the buyer's match, the theft owner check.
        case 'lookup': {
          const desk = agencyNode(e.actor)
          return [
            { from: desk, to: 'CUSTODY_DMT', text: `${cap(who(e.actor))} asks the DMT database which car carries a plate` },
            { from: 'CUSTODY_DMT', to: desk, text: `The DMT database answers with the car's handle (${e.detail ?? ''}); the plate never goes on the ledger` },
          ]
        }
        case 'match': return [
          { from: 'BUYER', to: 'CUSTODY_DMT', text: 'The buyer types the chassis number stamped on the car; the DMT database checks it against this record' },
          { from: 'CUSTODY_DMT', to: 'BUYER', text: `One word comes back: ${e.detail ?? 'matches or does not match'}. No code that could look up another car` },
        ]
        case 'ownercheck': {
          const [ref, verdict, ...checks] = (e.detail ?? '').split('; ')
          const yes = verdict === 'owner confirmed'
          return [
            { from: 'POLICE', to: 'CUSTODY_DMT', text: `The Police desk asks the DMT database whether the person reporting ${ref ?? 'the theft'} is the registered owner` },
            ...ledgerCheck('dmt', 'that the theft report is on the ledger', checks.join('; ')),
            { text: 'The DMT database compares the reporter\u2019s NIC, scrambled with its secret key, with the owner tag it keeps for this car; the NIC is not stored' },
            ...(yes
              ? [
                  { from: 'CUSTODY_DMT', to: 'PEER1', text: 'It matches: the DMT database writes "owner confirmed" to the ledger with the DMT\u2019s own identity' } as Step,
                  { from: 'CUSTODY_DMT', to: 'POLICE', text: 'Yes. The Police can now flag the car stolen; the ledger will accept it' } as Step,
                ]
              : [{ from: 'CUSTODY_DMT', to: 'POLICE', text: 'No: the reporter is not the registered owner. Nothing is written, and the car cannot be flagged on this report' } as Step]),
          ]
        }
        case 'refused': return [{ from: node, to: owner, text: `The ${name} says no: ${e.detail ?? 'nothing is handed over'}` }]
        case 'open': return [{ from: 'PORTAL', to: 'POLICE', text: 'Fallback mode: the portal joins the two halves, unlocks the record, notes the opening on the ledger, and only then shows it' }]
      }
      break
    }
    case 'block': return [{ text: `Block ${e.number} is complete: ${blockSummary(e)}` }]
    case 'chaincode': return [{ text: `The shared rules announce that ${ANNOUNCED[e.name] ?? e.name}. Every organisation hears it` }]
    default: return []
  }
  return []
}

function blockSummary(e: Extract<LiveEvent, { kind: 'block' }>): string {
  const changes = `${e.txCount} change${e.txCount === 1 ? '' : 's'}`
  if (!e.invalid)
    return `${changes}, all accepted`
  const why = list([...new Set(e.codes.map(c => INVALID[c] ?? c))])
  return `${changes}, ${e.invalid} rejected because ${why}`
}

/** The links packets travel for one event, in order (simultaneous packets in step order). */
export const hops = (e: LiveEvent): [NodeId, NodeId][] =>
  steps(e).flatMap(s => [...(s.from && s.to ? [[s.from, s.to] as [NodeId, NodeId]] : []), ...(s.also ?? [])])

/** One plain-English line per event, for the log beside the scene. */
export function describeEvent(e: LiveEvent): string {
  switch (e.kind) {
    case 'phase':
      switch (e.phase) {
        case 'proposed': return `${cap(who(e.actor))} asks to ${op(e.op)}${e.transient ? ', with the owner’s list of vehicles' : ''}`
        case 'endorsed': return `signed by ${e.endorsers?.length ? list([...new Set(e.endorsers.map(orgOfMsp))]) : 'the organisations'}${e.ms ? ` in ${e.ms} ms` : ''}`
        case 'submitted': return 'sent to the ordering service'
        case 'committed': return `saved on all five copies of the ledger${e.ms ? ` (${e.ms} ms in total)` : ''}`
        case 'refused': return `refused by the shared rules: ${reason(e.detail) ?? 'not allowed'}`
      }
      break
    case 'read': return `${cap(who(e.actor))} asks to ${op(e.op)}${e.detail ? ` (${e.detail})` : ''}; nothing saved`
    case 'offledger': {
      const name = e.service ? STORE_NAME[e.service] : 'database'
      switch (e.what) {
        case 'seal': return 'fallback mode: owner record locked and saved in a file on the portal computer'
        case 'index': return 'fallback mode: owner’s vehicle list updated in that file'
        case 'anchor': return 'record fingerprint read from the ledger'
        case 'shares': return 'fallback mode: both key halves read from the file on the portal computer'
        case 'store': return `${cap(name)} saved its part`
        case 'release': return `${cap(name)} checked the ledger and handed over its part (${plainCheck(e.detail?.split('; ').slice(1).join('; '))})`
        case 'fleet': return `${cap(name)} returned the owner’s list of vehicles`
        case 'refused': return `${cap(name)} said no: ${e.detail ?? ''}`
        case 'handoff': return e.actor === 'DMT'
          ? 'DMT database passed the Police key half straight to the Police database'
          : `Police database fetched ${e.detail?.startsWith('fleet') ? 'the owner’s list of vehicles' : 'the DMT’s part'} straight from the DMT database`
        case 'reveal': return 'Police asked their database to open the owner’s record'
        case 'opened': return 'owner’s record unlocked at the Police database; the opening is noted on the ledger'
        case 'lookup': return `${cap(who(e.actor))} found a car by its plate at the DMT database (${e.detail ?? ''})`
        case 'match': return `buyer's chassis check: ${e.detail ?? ''}`
        case 'ownercheck': return `DMT owner check for a theft report: ${(e.detail ?? '').split('; ').slice(0, 2).join(', ')}`
        case 'open': return 'fallback mode: record unlocked by the portal and shown to the Police'
      }
      break
    }
    case 'block': return `block ${e.number}: ${blockSummary(e)}`
    case 'chaincode': return `announced: ${ANNOUNCED[e.name] ?? e.name} (block ${e.block})`
    case 'metrics': return `blocks on each copy: ${Object.entries(e.heights).map(([k, v]) => `${k} ${v}`).join(', ')}`
    case 'status': return e.detail
  }
  return ''
}
