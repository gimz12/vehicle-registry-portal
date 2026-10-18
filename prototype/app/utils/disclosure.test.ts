import { describe, expect, it } from 'vitest'
import {
  HISTORY_PURPOSES,
  agencyFor,
  evidenceLabel,
  normalizeDisclosure,
  normalizeHistory,
  parsePurpose,
  validateDesk,
  validateEscalation,
} from '../../shared/disclosure'

describe('buyer disclosure normalization', () => {
  it('preserves explicit empty arrays and normalizes Go field names', () => {
    const result = normalizeDisclosure({
      VID: 'vid-1',
      Profile: 'P1',
      Passed: [],
      Failed: [],
      NotPerformed: [{
        RuleID: 'F6a', VID: 'vid-1', Status: 'SUPPRESSED', Strength: '', Tier: 0,
        Detail: 'Serial odometer fraud', Reason: 'requires OWNER capability', AtSeq: 0,
        Subject: 'SYSTEM',
      }],
    })
    expect(result.notPerformed).toHaveLength(1)
    expect(result.notPerformed[0]?.ruleId).toBe('F6a')
    expect(result.notPerformed[0]?.structural).toBe(false)
    expect(result.failed).toEqual([])
    expect(result.policy).toBeUndefined()
  })

  it('carries the purpose-bound policy and structural gaps through', () => {
    const result = normalizeDisclosure({
      VID: 'vid-1',
      Profile: 'P2',
      Policy: { purpose: 'BUYER', profile: 'P2', reason: 'derived disclosure' },
      Passed: [],
      Failed: [],
      NotPerformed: [{
        RuleID: 'F3a', VID: 'vid-1', Status: 'GAP_NOTICE', Strength: '', Tier: 0,
        Detail: 'claim evidence', Reason: 'removed by the active data view', AtSeq: 4,
        Subject: 'SYSTEM', Structural: true,
      }],
    })
    expect(result.policy?.profile).toBe('P2')
    expect(result.notPerformed[0]?.structural).toBe(true)
  })

  it('does not describe E0 as verified evidence', () => {
    expect(evidenceLabel(0)).toContain('unavailable')
    expect(evidenceLabel(2)).toContain('independently')
  })
})

describe('purpose parsing', () => {
  it('accepts the four purposes case-insensitively and rejects anything else', () => {
    expect(parsePurpose('buyer')).toBe('BUYER')
    expect(parsePurpose(' Police ')).toBe('POLICE')
    expect(parsePurpose('AUDITOR')).toBeNull()
    expect(parsePurpose(undefined)).toBeNull()
    expect(parsePurpose(['BUYER'])).toBeNull()
  })

  it('reserves the lifecycle history for DMT and Police', () => {
    expect(HISTORY_PURPOSES).toEqual(['DMT', 'POLICE'])
    expect(HISTORY_PURPOSES.includes('BUYER')).toBe(false)
  })
})

describe('lifecycle history normalization', () => {
  it('orders events, derives the writing agency and marks redacted references', () => {
    const history = normalizeHistory({
      policy: { purpose: 'DMT', profile: 'P2', reason: 'transaction' },
      record: {
        vehicle: {
          VID: 'vid-1', Stolen: false, WrittenOff: false, Retired: false,
          Events: [
            { Seq: 1, RecordID: 'claim-1', Type: 'CLAIM', P: 'p', Day: 505, Tier: 2, AttestorID: 'a', Odometer: 0, EvidenceAnchor: '', EvidenceKind: '', IncidentRef: 'REDACTED_BY_PROFILE', InsurerID: 'insurer-a' },
            { Seq: 0, RecordID: 'm-1', Type: 'MILEAGE', P: 'p', Day: 0, Tier: 1, AttestorID: 'lane', Odometer: 50000, EvidenceAnchor: '', EvidenceKind: '', IncidentRef: '', InsurerID: '' },
          ],
        },
      },
    })
    expect(history.events.map(e => e.seq)).toEqual([0, 1])
    expect(history.events[0]?.agency).toBe('VET')
    expect(history.events[1]?.agency).toBe('Insurer')
    expect(history.events[1]?.redacted).toBe(true)
    expect(agencyFor('THEFT_FLAG')).toBe('Police')
  })
})

describe('escalation input validation', () => {
  const good = { action: 'create', as: 'POLICE', caseId: 'CID/2026/0007', vid: 'a'.repeat(64), ruleId: 'F3c', ground: 'COURT_ORDER', groundRef: 'B/1234/2026' }

  it('accepts a complete request and a bare approval', () => {
    expect(validateEscalation(good)).toBeNull()
    expect(validateEscalation({ action: 'approve', as: 'DMT', caseId: 'CID/2026/0007' })).toBeNull()
  })

  it('rejects missing legal basis, bad actors and malformed handles', () => {
    expect(validateEscalation({ ...good, groundRef: '  ' })).toMatch(/groundRef/)
    expect(validateEscalation({ ...good, ground: 'CURIOSITY' })).toMatch(/ground must/)
    expect(validateEscalation({ ...good, as: 'BUYER' })).toMatch(/as must/)
    expect(validateEscalation({ ...good, vid: 'short' })).toMatch(/vid/)
    expect(validateEscalation({ ...good, caseId: 'has spaces' })).toMatch(/caseId/)
    expect(validateEscalation(null)).toMatch(/body/)
  })
})

describe('agency desk validation', () => {
  const hex = 'c'.repeat(64)
  it('lets each agency do only its own actions', () => {
    expect(validateDesk({ agency: 'CUSTOMS', action: 'import', recordId: 'import-9', chassisNumber: 'NHP10-1234567', engineNumber: '1NZ-A1', make: 'Toyota' })).toBeNull()
    expect(validateDesk({ agency: 'CUSTOMS', action: 'register' })).toMatch(/CUSTOMS may only/)
    expect(validateDesk({ agency: 'POLICE', action: 'theft', vid: hex, recordId: 'theft-1', reportRef: 'PR/2026/1' })).toBeNull()
    expect(validateDesk({ agency: 'VET', action: 'theft', vid: hex, recordId: 'x' })).toMatch(/VET may only/)
  })
  it('v1.8: a theft needs a report number; the reporter NIC is checked if given', () => {
    expect(validateDesk({ agency: 'POLICE', action: 'theft', vid: hex, recordId: 'theft-1' })).toMatch(/report number/)
    expect(validateDesk({ agency: 'POLICE', action: 'report', vid: hex, recordId: 'r-1' })).toMatch(/report number/)
    expect(validateDesk({ agency: 'POLICE', action: 'report', vid: hex, recordId: 'r-1', reportRef: 'PR/1' })).toBeNull()
    expect(validateDesk({ agency: 'POLICE', action: 'theft', vid: hex, recordId: 't-1', reportRef: 'PR/1', reporterNic: 'nope' })).toMatch(/NIC/)
    expect(validateDesk({ agency: 'POLICE', action: 'theft', vid: hex, recordId: 't-1', reportRef: 'PR/1', reporterNic: '199012345678' })).toBeNull()
    expect(validateDesk({ agency: 'DMT', action: 'report', vid: hex, recordId: 'r-1', reportRef: 'PR/1' })).toMatch(/DMT may only/)
  })
  it('v1.8: a description is optional, but complete and checked when given', () => {
    const imp = { agency: 'CUSTOMS', action: 'import', recordId: 'i-1', chassisNumber: 'NHP10-1234567', engineNumber: '1NZ-A1', make: 'Toyota' }
    expect(validateDesk(imp)).toBeNull()
    expect(validateDesk({ ...imp, model: 'Axio', year: 2016, engineCc: 1496, fuel: 'PETROL' })).toBeNull()
    expect(validateDesk({ ...imp, model: 'Axio' })).toMatch(/year/)
    expect(validateDesk({ ...imp, model: 'Axio', year: 2016, engineCc: 1496, fuel: 'STEAM' })).toMatch(/fuel/)
    expect(validateDesk({ ...imp, model: 'Axio', year: 1800, engineCc: 1496, fuel: 'PETROL' })).toMatch(/year/)
  })
  it('registers from physical identifiers and a valid NIC; transfers need the new owner NIC', () => {
    const base = { agency: 'DMT', action: 'register', recordId: 'reg-1', chassisNumber: 'NHP10-1234567', engineNumber: '1NZ-A1', make: 'Toyota' }
    expect(validateDesk(base)).toMatch(/owner name/)
    expect(validateDesk({ ...base, owner: { name: 'A', nic: 'not-a-nic', address: 'C' } })).toMatch(/NIC must/)
    expect(validateDesk({ ...base, owner: { name: 'A', nic: '751234567V', address: 'C' } })).toBeNull()
    expect(validateDesk({ ...base, chassisNumber: 'N/A' })).toMatch(/chassis/)
    expect(validateDesk({ agency: 'DMT', action: 'transfer', vid: hex, recordId: 't-1' })).toMatch(/new owner/)
    expect(validateDesk({ agency: 'DMT', action: 'transfer', vid: hex, recordId: 't-1', owner: { name: 'B', nic: '197512345678', address: 'x' } })).toBeNull()
    expect(validateDesk({ agency: 'DMT', action: 'retire', vid: hex })).toBeNull()
  })
  it('rejects a reject without a reason', () => {
    expect(validateEscalation({ action: 'reject', as: 'DMT', caseId: 'C1' })).toMatch(/reason/)
    expect(validateEscalation({ action: 'reject', as: 'DMT', caseId: 'C1', reason: 'Order expired' })).toBeNull()
  })

  it('checks an agency’s internal fields before anything reaches the ledger', () => {
    const imp = { agency: 'CUSTOMS', action: 'import', recordId: 'c-1', chassisNumber: 'NHP10-1234567', engineNumber: '1NZ-123456', make: 'Toyota' }
    expect(validateDesk({ ...imp, internal: { declarant: 'X Ltd (synthetic)', valuationLkr: 6850000 } })).toBeNull()
    expect(validateDesk({ ...imp, internal: { valuationLkr: '6,850,000' } })).toMatch(/amount in rupees/)
    expect(validateDesk({ ...imp, internal: { nic: '199011112222' } })).toMatch(/not kept for import/)
    expect(validateDesk({ ...imp, internal: 'abc' })).toMatch(/must be an object/)
    expect(validateDesk({ agency: 'DMT', action: 'retire', vid: 'a'.repeat(64), internal: { declarant: 'X' } })).toMatch(/keeps no internal fields/)
  })
})
