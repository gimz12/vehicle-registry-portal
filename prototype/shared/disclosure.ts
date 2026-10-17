export const PURPOSES = ['BUYER', 'DMT', 'INSURER', 'POLICE'] as const
export type Purpose = typeof PURPOSES[number]

/** Validates the `?as=` query value; null when it is not one of the four purposes. */
export function parsePurpose(raw: unknown): Purpose | null {
  if (typeof raw !== 'string')
    return null
  const upper = raw.trim().toUpperCase()
  return (PURPOSES as readonly string[]).includes(upper) ? upper as Purpose : null
}

/** Purposes that may read the lifecycle timeline; the others get the derived disclosure only. */
export const HISTORY_PURPOSES: readonly Purpose[] = ['DMT', 'POLICE']

export interface Policy {
  purpose: Purpose
  profile: 'P0' | 'P1' | 'P2' | 'P3'
  reason: string
  caseId?: string
}

export interface DisclosureFinding {
  ruleId: string
  vid: string
  status: 'PASS' | 'FLAGGED' | 'SUPPRESSED' | 'GAP_NOTICE'
  strength: 'INFO' | 'WARNING' | 'ALERT' | ''
  tier: 0 | 1 | 2
  detail: string
  reason: string
  atSeq: number
  subject: 'OWNER' | 'SUBMITTER' | 'SYSTEM'
  /** A gap caused by the privacy view itself rather than by missing evidence. */
  structural: boolean
}

export interface VehicleStatus {
  stolen: boolean
  writtenOff: boolean
  retired: boolean
  /** A finance company is recorded as absolute owner; the lender is never named. */
  encumbered?: boolean
  /** A theft report is on record and has not (yet) led to a flag. */
  theftReportPending?: boolean
  /** The vehicle was flagged stolen and the flag was later cleared. */
  theftCleared?: boolean
  /** The record carries the description Customs committed at import. */
  descCommitted?: boolean
}

/** What a vehicle is, from the DMT's records. Never includes the plate on the buyer page. */
export interface VehicleDescriptionView {
  make: string
  model: string
  year: number
  engineCc: number
  fuel: string
  colour: string
  committed: boolean
  /** The DMT's stored description no longer reproduces the fingerprint on the ledger. */
  mismatch?: boolean
}

export interface VehicleDisclosure {
  vid: string
  profile: 'P0' | 'P1' | 'P2' | 'P3'
  policy?: Policy
  status?: VehicleStatus
  passed: DisclosureFinding[]
  failed: DisclosureFinding[]
  notPerformed: DisclosureFinding[]
}

export interface RawFinding {
  RuleID: string
  VID: string
  Status: DisclosureFinding['status']
  Strength: DisclosureFinding['strength']
  Tier: 0 | 1 | 2
  Detail: string
  Reason: string
  AtSeq: number
  Subject: DisclosureFinding['subject']
  Structural?: boolean
}

export interface RawDisclosure {
  VID: string
  Profile: VehicleDisclosure['profile']
  Passed: RawFinding[]
  Failed: RawFinding[]
  NotPerformed: RawFinding[]
  Policy?: Policy
  Status?: VehicleStatus
}

export function normalizeDisclosure(raw: RawDisclosure): VehicleDisclosure {
  const normalizeFinding = (finding: RawFinding): DisclosureFinding => ({
    ruleId: finding.RuleID,
    vid: finding.VID,
    status: finding.Status,
    strength: finding.Strength,
    tier: finding.Tier,
    detail: finding.Detail,
    reason: finding.Reason,
    atSeq: finding.AtSeq,
    subject: finding.Subject,
    structural: finding.Structural === true,
  })
  return {
    vid: raw.VID,
    profile: raw.Profile,
    policy: raw.Policy,
    status: raw.Status,
    passed: raw.Passed.map(normalizeFinding),
    failed: raw.Failed.map(normalizeFinding),
    notPerformed: raw.NotPerformed.map(normalizeFinding),
  }
}

export function evidenceLabel(tier: number): string {
  if (tier === 2)
    return 'E2 · independently anchored'
  if (tier === 1)
    return 'E1 · accountable attestation'
  return 'E0 · unavailable or self-declared'
}

// ---- lifecycle history ----

export interface RawEvent {
  Seq: number
  RecordID: string
  Type: string
  P: string
  Day: number
  Tier: 0 | 1 | 2
  AttestorID: string
  Odometer: number
  EvidenceAnchor: string
  EvidenceKind: string
  IncidentRef: string
  InsurerID: string
}

export interface RawHistory {
  record: { vehicle: { VID: string, Events: RawEvent[], Stolen: boolean, WrittenOff: boolean, Retired: boolean } }
  policy: Policy
}

export interface LifecycleEvent {
  seq: number
  recordId: string
  type: string
  agency: string
  day: number
  tier: 0 | 1 | 2
  attestorId: string
  pseudonym: string
  odometer: number
  evidenceAnchor: string
  evidenceKind: string
  redacted: boolean
  insurerId: string
}

export interface VehicleHistory {
  vid: string
  policy: Policy
  stolen: boolean
  writtenOff: boolean
  retired: boolean
  events: LifecycleEvent[]
}

/** Writing agency per event type, mirroring the chaincode's authorisation table. */
export function agencyFor(type: string): string {
  switch (type) {
    case 'IMPORT': return 'Customs'
    case 'REGISTRATION':
    case 'TRANSFER': return 'DMT'
    case 'MILEAGE':
    case 'EMISSION': return 'VET'
    case 'ACCIDENT':
    case 'THEFT_REPORT':
    case 'THEFT_FLAG':
    case 'THEFT_CLEAR': return 'Police'
    case 'OWNER_CONFIRMED': return 'DMT'
    case 'CLAIM': return 'Insurer'
    case 'ENCUMBRANCE_SET':
    case 'ENCUMBRANCE_RELEASED': return 'DMT'
    default: return 'Unknown'
  }
}

export function normalizeHistory(raw: RawHistory): VehicleHistory {
  const vehicle = raw.record.vehicle
  return {
    vid: vehicle.VID,
    policy: raw.policy,
    stolen: vehicle.Stolen,
    writtenOff: vehicle.WrittenOff,
    retired: vehicle.Retired,
    events: [...(vehicle.Events ?? [])]
      .sort((a, b) => a.Seq - b.Seq)
      .map(e => ({
        seq: e.Seq,
        recordId: e.RecordID,
        type: e.Type,
        agency: agencyFor(e.Type),
        day: e.Day,
        tier: e.Tier,
        attestorId: e.AttestorID,
        pseudonym: e.P,
        odometer: e.Odometer,
        evidenceAnchor: e.EvidenceAnchor,
        evidenceKind: e.EvidenceKind,
        redacted: e.IncidentRef === 'REDACTED_BY_PROFILE',
        insurerId: e.InsurerID,
      })),
  }
}

// ---- reveal / escalation ----

export const REVEAL_GROUNDS = ['FRAUD_INVESTIGATION', 'THEFT_INVESTIGATION', 'COURT_ORDER', 'DATA_SUBJECT_CONSENT'] as const
export type RevealGround = typeof REVEAL_GROUNDS[number]

export interface RevealRequest {
  id: string
  vid: string
  ruleId: string
  groundCode: string
  groundRef: string
  requestedBy: string
  dmtApproved: boolean
  policeApproved: boolean
  status: 'PENDING' | 'APPROVED' | 'REJECTED'
  reason?: string
  accessLog?: string[]
  /** How many vehicles the owner's fleet held when the DMT approved. */
  fleetSize?: number
}

/** The verdict of a DMT owner-scope predicate (F6a or F6b); never the fleet. */
export interface PredicateResult {
  vid: string
  ruleId: 'F6a' | 'F6b'
  status: DisclosureFinding['status']
  strength: DisclosureFinding['strength']
}

export interface EscalationInput {
  action: 'create' | 'approve' | 'reject' | 'predicate'
  as: 'POLICE' | 'DMT'
  caseId?: string
  vid?: string
  ruleId?: string
  ground?: RevealGround
  groundRef?: string
  reason?: string
}

/** Validates an escalation request body; returns an error message or null. */
export function validateEscalation(body: unknown): string | null {
  if (!body || typeof body !== 'object')
    return 'A JSON body is required'
  const input = body as Partial<EscalationInput>
  if (input.action !== 'create' && input.action !== 'approve' && input.action !== 'reject' && input.action !== 'predicate')
    return 'action must be create, approve, reject or predicate'
  if (input.as !== 'POLICE' && input.as !== 'DMT')
    return 'as must be POLICE or DMT'
  if (input.action === 'predicate') {
    // Only the DMT, as custodian of the owner index, evaluates owner-scope predicates.
    if (input.as !== 'DMT')
      return 'only the DMT evaluates owner-scope predicates'
    if (!input.vid || !/^[a-f0-9]{64}$/.test(input.vid))
      return 'vid must be a 64-character hexadecimal handle'
    if (input.ruleId !== 'F6a' && input.ruleId !== 'F6b')
      return 'ruleId must be F6a or F6b'
    return null
  }
  if (!input.caseId || !/^[A-Za-z0-9/_-]{1,64}$/.test(input.caseId))
    return 'caseId must be 1–64 characters of letters, digits, / _ or -'
  if (input.action === 'create') {
    if (!input.vid || !/^[a-f0-9]{64}$/.test(input.vid))
      return 'vid must be a 64-character hexadecimal handle'
    if (!input.ruleId || !/^F\d[a-d]?$/.test(input.ruleId))
      return 'ruleId must name a registry rule such as F3c'
    if (!input.ground || !(REVEAL_GROUNDS as readonly string[]).includes(input.ground))
      return 'ground must be one of ' + REVEAL_GROUNDS.join(', ')
    if (!input.groundRef || input.groundRef.trim().length === 0 || input.groundRef.length > 120)
      return 'groundRef (case number, court order or consent record) is required'
  }
  if (input.action === 'reject' && (!input.reason || input.reason.trim().length === 0 || input.reason.length > 240))
    return 'reason is required when rejecting'
  return null
}

// ---- agency desk (writes) ----

export const AGENCIES = ['CUSTOMS', 'ASSEMBLER', 'DMT', 'VET', 'POLICE', 'INSURER'] as const
export type Agency = typeof AGENCIES[number]

export const DESK_ACTIONS = {
  CUSTOMS: ['import'],
  ASSEMBLER: ['attest'],
  DMT: ['register', 'transfer', 'retire', 'encumber', 'release'],
  VET: ['mileage', 'emission'],
  POLICE: ['report', 'theft', 'clear', 'accident', 'writeoff'],
  INSURER: ['claim', 'writeoff'],
} as const satisfies Record<Agency, readonly string[]>
export type DeskAction = typeof DESK_ACTIONS[Agency][number]

export interface DeskInput {
  agency: Agency
  action: DeskAction
  vid?: string
  recordId?: string
  /** Physical identifiers as printed on the papers; the server derives the keyed indices. */
  chassisNumber?: string
  engineNumber?: string
  make?: string
  provenance?: 'CUSTOMS' | 'ASSEMBLER'
  /** Owner details: the registered owner on 'register', the new owner on 'transfer'. Sealed off-ledger. */
  owner?: { name: string, nic: string, address: string }
  day?: number
  odometer?: number
  vetLaneId?: string
  evidenceAnchor?: string
  evidenceKind?: 'POLICE_REPORT' | 'ASSESSOR_ATTESTATION'
  insurerId?: string
  /** The assessor who attested a claim; empty when no assessor was involved. */
  assessorId?: string
  /** Absolute-owner endorsement or deletion-letter number (encumber / release). */
  reference?: string
  /** Description (import, registration): only its keyed fingerprint goes on the ledger. */
  model?: string
  year?: number
  engineCc?: number
  fuel?: string
  /** DMT registration: the plate and colour, kept in the DMT's database only. */
  plate?: string
  colour?: string
  /** Theft: the Police report number (report, theft) and the reporter's NIC (theft). */
  reportRef?: string
  reporterNic?: string
  flag?: boolean
  /** The agency's own internal fields, kept in its custody service, never on the ledger. */
  internal?: Record<string, string | number>
}

const HEX64 = /^[a-f0-9]{64}$/i
const RECORD = /^[A-Za-z0-9/_.:-]{1,80}$/
const REPORT = /^[A-Za-z0-9/_-]{1,64}$/
export const FUEL_TYPES = ['PETROL', 'DIESEL', 'HYBRID', 'ELECTRIC', 'OTHER'] as const

/** A description is optional, but if any of its fields is given, all are needed and checked. */
function descriptionProblem(d: Partial<DeskInput>): string | null {
  const given = [d.model, d.year, d.engineCc, d.fuel].some(v => v !== undefined && v !== '')
  if (!given)
    return null
  if (!d.model || canonicalId(d.model).length < 1)
    return 'model is required with a description'
  if (!Number.isInteger(d.year) || d.year! < 1950 || d.year! > 2100)
    return 'year must be a whole number from 1950 to 2100'
  if (!Number.isInteger(d.engineCc) || d.engineCc! < 0 || d.engineCc! > 20000)
    return 'engine capacity must be a whole number of cc from 0 to 20000 (0 for electric)'
  if (!d.fuel || !(FUEL_TYPES as readonly string[]).includes(d.fuel.toUpperCase()))
    return `fuel must be one of ${FUEL_TYPES.join(', ')}`
  return null
}
const canonicalId = (s: string) => s.toUpperCase().replace(/[^A-Z0-9]/g, '')
const isNIC = (s: string) => /^\d{12}$/.test(canonicalId(s)) || /^\d{9}[VX]$/.test(canonicalId(s))

/** Validates an agency-desk submission; returns an error message or null. */
export function validateDesk(body: unknown): string | null {
  if (!body || typeof body !== 'object')
    return 'A JSON body is required'
  const d = body as Partial<DeskInput>
  if (!d.agency || !(AGENCIES as readonly string[]).includes(d.agency))
    return 'agency must be one of ' + AGENCIES.join(', ')
  const allowed = DESK_ACTIONS[d.agency] as readonly string[]
  if (!d.action || !allowed.includes(d.action))
    return `${d.agency} may only: ${allowed.join(', ')}`
  const needVid = !['import', 'attest', 'register'].includes(d.action)
  if (needVid && (!d.vid || !HEX64.test(d.vid)))
    return 'vid must be a 64-character hexadecimal handle'
  const needRecord = !['retire', 'writeoff', 'register'].includes(d.action) // registration id is set by the chaincode
  if (needRecord && (!d.recordId || !RECORD.test(d.recordId)))
    return 'recordId is required (letters, digits, / _ . : -)'
  switch (d.action) {
    case 'import':
    case 'attest':
    case 'register':
      if (!d.chassisNumber || canonicalId(d.chassisNumber).length < 5)
        return 'chassis number is required'
      if (!d.engineNumber || canonicalId(d.engineNumber).length < 3 || !d.make || canonicalId(d.make).length < 2)
        return 'engine number and make are required'
      if (d.action === 'register') {
        if (!d.owner?.name || !d.owner?.nic || !d.owner?.address)
          return 'owner name, NIC and address are required (synthetic values)'
        if (!isNIC(d.owner.nic))
          return 'NIC must be 12 digits or 9 digits followed by V or X'
        if (d.provenance && d.provenance !== 'CUSTOMS' && d.provenance !== 'ASSEMBLER')
          return 'provenance must be CUSTOMS or ASSEMBLER'
        if (d.plate !== undefined && d.plate !== '' && (canonicalId(d.plate).length < 3 || canonicalId(d.plate).length > 12))
          return 'a number plate has 3 to 12 letters and digits'
        if (d.colour !== undefined && String(d.colour).length > 40)
          return 'colour is at most 40 characters'
      }
      {
        const problem = descriptionProblem(d)
        if (problem)
          return problem
      }
      break
    case 'transfer':
      if (!d.owner?.name || !d.owner?.nic || !d.owner?.address)
        return 'new owner name, NIC and address are required (synthetic values)'
      if (!isNIC(d.owner.nic))
        return 'NIC must be 12 digits or 9 digits followed by V or X'
      break
    case 'mileage':
      if (typeof d.odometer !== 'number' || d.odometer < 0 || d.odometer > 5_000_000)
        return 'odometer must be a number of kilometres'
      if (typeof d.day !== 'number' || d.day < 0)
        return 'day must be a non-negative number'
      break
    case 'emission':
    case 'accident':
      if (typeof d.day !== 'number' || d.day < 0)
        return 'day must be a non-negative number'
      break
    case 'claim':
      if (typeof d.day !== 'number' || d.day < 0)
        return 'day must be a non-negative number'
      if (!d.insurerId || !RECORD.test(d.insurerId))
        return 'insurerId is required'
      if (d.evidenceAnchor && !RECORD.test(d.evidenceAnchor))
        return 'evidenceAnchor must be a record id'
      if (d.assessorId && !RECORD.test(d.assessorId))
        return 'assessorId must be letters, digits, / _ . : -'
      break
    case 'encumber':
    case 'release':
      if (!d.reference || !RECORD.test(d.reference))
        return 'the absolute-owner endorsement or deletion-letter reference is required'
      break
    case 'report':
      if (!d.reportRef || !REPORT.test(d.reportRef))
        return 'the Police report number is required (letters, digits, / _ -)'
      break
    case 'theft':
      if (!d.reportRef || !REPORT.test(d.reportRef))
        return 'the Police report number (or an approved theft case id) is required'
      if (d.reporterNic && !isNIC(d.reporterNic))
        return 'the reporter\'s NIC must be 12 digits or 9 digits followed by V or X'
      break
    case 'retire':
    case 'writeoff':
    case 'clear':
      break
  }
  return validateInternal(d)
}

/**
 * Internal fields per desk action (mirrors services/custody/main.ts INTERNAL). Checked
 * before the ledger write, so a bad value cannot leave a committed record without its internal one.
 */
export const INTERNAL_FIELDS: Partial<Record<DeskAction, string[]>> = {
  import: ['declarant', 'cusdecNo', 'valuationLkr'],
  mileage: ['technician', 'certificateNo'],
  emission: ['technician', 'certificateNo'],
  claim: ['policyNo', 'claimFile', 'assessorContact', 'reserveLkr'],
}
const AMOUNT = /^\d{1,12}(\.\d{1,2})?$/

function validateInternal(d: Partial<DeskInput>): string | null {
  if (d.internal === undefined)
    return null
  const allowed = INTERNAL_FIELDS[d.action!]
  if (!allowed)
    return `${d.action} keeps no internal fields`
  if (!d.internal || typeof d.internal !== 'object' || Array.isArray(d.internal))
    return 'internal must be an object of field values'
  for (const [key, value] of Object.entries(d.internal)) {
    if (!allowed.includes(key))
      return `internal field ${key} is not kept for ${d.action}; allowed: ${allowed.join(', ')}`
    if ((typeof value !== 'string' && typeof value !== 'number') || String(value).length > 200)
      return `internal field ${key} must be text or a number of at most 200 characters`
    if (key.endsWith('Lkr') && !AMOUNT.test(String(value)))
      return `internal field ${key} must be an amount in rupees, digits only (for example 6850000)`
  }
  return null
}

export interface DeskResult {
  ok: boolean
  action: DeskAction
  agency: Agency
  message: string
  vid?: string
  payload?: unknown
}

// ---- identity reveal ----

export interface RevealedIdentity {
  vid: string
  caseId: string
  anchor: string
  owner: { name: string, nic: string, address: string, note: string }
  accessLog: string[]
}
