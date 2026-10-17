import { describe, expect, it } from 'vitest'
import { describeEvent, hops, plainCheck, steps } from '~~/shared/live'

describe('live network events', () => {
  it('routes a Customs write through its own peer, two more endorsers, the orderers and all five peers', () => {
    expect(hops({ kind: 'phase', t: 0, op: 'CreateImportAttestation', actor: 'CUSTOMS', phase: 'proposed' }))
      .toEqual([['CUSTOMS', 'PORTAL'], ['PORTAL', 'PEER3']])
    expect(hops({ kind: 'phase', t: 0, op: 'CreateImportAttestation', actor: 'CUSTOMS', phase: 'endorsed', endorsers: ['Org3MSP', 'Org1MSP', 'Org2MSP'] }))
      .toEqual([['PEER3', 'PEER1'], ['PEER3', 'PEER2'], ['PEER1', 'PEER3'], ['PEER2', 'PEER3'], ['PEER3', 'PORTAL']])
    expect(hops({ kind: 'phase', t: 0, op: 'CreateImportAttestation', actor: 'CUSTOMS', phase: 'submitted' }))
      .toEqual([['PORTAL', 'PEER3'], ['PEER3', 'ORDERER']])
    expect(hops({ kind: 'phase', t: 0, op: 'CreateImportAttestation', actor: 'CUSTOMS', phase: 'committed' }))
      .toEqual([['ORDERER', 'PEER1'], ['ORDERER', 'PEER2'], ['ORDERER', 'PEER3'], ['ORDERER', 'PEER4'], ['ORDERER', 'PEER5'], ['PEER3', 'PORTAL']])
  })

  it('sends each agency to its own organisation’s peer', () => {
    const gateway = (actor: string) => hops({ kind: 'read', t: 0, op: 'GetDisclosureFor', actor })[1]
    expect(['DMT', 'BUYER', 'POLICE', 'CUSTOMS', 'VET', 'INSURER'].map(a => gateway(a)![1]))
      .toEqual(['PEER1', 'PEER1', 'PEER2', 'PEER3', 'PEER4', 'PEER5'])
  })

  it('keeps owner data off the ledger path and reads off any block', () => {
    // Each database is reached from its own organisation's desk only, never drawn from the portal.
    expect(hops({ kind: 'offledger', t: 0, what: 'store', service: 'police', actor: 'POLICE' })).toEqual([['POLICE', 'CUSTODY_POLICE']])
    expect(hops({ kind: 'offledger', t: 0, what: 'store', service: 'customs', actor: 'CUSTOMS' })).toEqual([['CUSTOMS', 'CUSTODY_CUSTOMS']])
    expect(hops({ kind: 'read', t: 0, op: 'GetDisclosureFor', actor: 'BUYER' })).not.toContainEqual(['PORTAL', 'ORDERER'])
    expect(hops({ kind: 'read', t: 0, op: 'GetDisclosureFor', actor: 'POLICE' })[1]).toEqual(['PORTAL', 'PEER2'])
  })

  it('narrates every hop and keeps a block as a narration-only step', () => {
    const proposed = steps({ kind: 'phase', t: 0, op: 'RegisterVehicle', actor: 'DMT', phase: 'proposed', transient: true })
    expect(proposed).toHaveLength(2)
    expect(proposed[0]!.text).toContain('owner’s list of vehicles')
    expect(proposed.every(s => s.text.length > 20)).toBe(true)
    const block = steps({ kind: 'block', t: 0, number: 5, txCount: 1, valid: 1, invalid: 0, codes: [] })
    expect(block).toEqual([{ text: 'Block 5 is complete: 1 change, all accepted' }])
  })

  it('draws each custodian’s own ledger check before it releases anything', () => {
    expect(hops({ kind: 'offledger', t: 0, what: 'release', service: 'police', actor: 'POLICE', detail: 'case X; GetRevealAudit APPROVED on peer0.org2.example.com, 20 ms' }))
      .toEqual([['POLICE', 'CUSTODY_POLICE'], ['CUSTODY_POLICE', 'PEER2'], ['PEER2', 'CUSTODY_POLICE'], ['CUSTODY_POLICE', 'POLICE']])
    expect(hops({ kind: 'offledger', t: 0, what: 'release', service: 'dmt', actor: 'DMT', detail: 'case X' })[1]).toEqual(['CUSTODY_DMT', 'PEER1'])
    expect(hops({ kind: 'offledger', t: 0, what: 'refused', service: 'dmt', actor: 'DMT', detail: 'pending' })).toEqual([['CUSTODY_DMT', 'DMT']])
  })

  it('draws the two custodians handing key material to each other without the portal', () => {
    const store = hops({ kind: 'offledger', t: 0, what: 'handoff', service: 'police', actor: 'DMT', detail: 'Police half of ab…; GetVehicleHistoryFor ANCHOR on peer0.org2.example.com, 9 ms' })
    expect(store).toEqual([['CUSTODY_DMT', 'CUSTODY_POLICE'], ['CUSTODY_POLICE', 'PEER2'], ['PEER2', 'CUSTODY_POLICE']])
    const fetch = hops({ kind: 'offledger', t: 0, what: 'handoff', service: 'dmt', actor: 'POLICE', detail: 'case X; GetRevealAudit APPROVED on peer0.org1.example.com, 9 ms' })
    expect(fetch).toEqual([['CUSTODY_POLICE', 'CUSTODY_DMT'], ['CUSTODY_DMT', 'PEER1'], ['PEER1', 'CUSTODY_DMT'], ['CUSTODY_DMT', 'CUSTODY_POLICE']])
    expect([...store, ...fetch].some(([a, b]) => a === 'PORTAL' || b === 'PORTAL')).toBe(false)
    // The reveal ends at the Police service, which commits the audit entry before the identity leaves it.
    expect(hops({ kind: 'offledger', t: 0, what: 'opened', service: 'police', actor: 'POLICE', detail: 'case X' }))
      .toEqual([['CUSTODY_POLICE', 'PEER2'], ['PEER2', 'CUSTODY_POLICE'], ['CUSTODY_POLICE', 'POLICE']])
  })

  it('never draws a services-mode custody packet through the portal', () => {
    const events = [
      { what: 'store', service: 'dmt', actor: 'DMT' }, { what: 'store', service: 'vet', actor: 'VET' },
      { what: 'fleet', service: 'dmt', actor: 'DMT', detail: 'case X; c' }, { what: 'fleet', service: 'dmt', actor: 'DMT', detail: 'DMT owner-scope predicate' },
      { what: 'fleet', service: 'police', actor: 'POLICE', detail: 'case X' }, { what: 'reveal', service: 'police', actor: 'POLICE', detail: 'case X' },
      { what: 'handoff', service: 'dmt', actor: 'POLICE', detail: 'fleet, case X' }, { what: 'opened', service: 'police', actor: 'POLICE' },
      { what: 'refused', service: 'insurer', actor: 'INSURER' },
      { what: 'lookup', service: 'dmt', actor: 'POLICE', detail: 'vehicle 8c1a95cc…' },
      { what: 'match', service: 'dmt', actor: 'BUYER', detail: 'matches' },
      { what: 'ownercheck', service: 'dmt', actor: 'POLICE', detail: 'report PR/1; owner confirmed; GetVehicleHistoryFor REPORTED on peer0.org1.example.com, 9 ms' },
    ] as const
    for (const e of events)
      expect(hops({ kind: 'offledger', t: 0, ...e }).some(([a, b]) => a === 'PORTAL' || b === 'PORTAL')).toBe(false)
  })

  it('turns a ledger answer into plain words', () => {
    expect(plainCheck('GetRevealAudit APPROVED on peer0.org2.example.com, 20 ms')).toBe('the case is approved (20 ms)')
    expect(plainCheck('GetRevealAudit PENDING on peer0.org1.example.com, 9 ms; GetVehicleHistoryFor ANCHOR on peer0.org1.example.com, 4 ms'))
      .toBe('the case is still waiting for approval (9 ms) and the fingerprint is there (4 ms)')
  })

  it('v1.8: a yes from the owner check is written to the ledger by the DMT database, a no writes nothing', () => {
    const yes = hops({ kind: 'offledger', t: 0, what: 'ownercheck', service: 'dmt', actor: 'POLICE', detail: 'report PR/1; owner confirmed; x' })
    expect(yes).toContainEqual(['CUSTODY_DMT', 'PEER1'])
    expect(yes.at(-1)).toEqual(['CUSTODY_DMT', 'POLICE'])
    const no = hops({ kind: 'offledger', t: 0, what: 'ownercheck', service: 'dmt', actor: 'POLICE', detail: 'report PR/1; not the owner; x' })
    expect(no.filter(([a, b]) => a === 'CUSTODY_DMT' && b === 'PEER1')).toHaveLength(1) // only the ledger read, no write
    expect(hops({ kind: 'offledger', t: 0, what: 'match', service: 'dmt', actor: 'BUYER', detail: 'matches' }))
      .toEqual([['BUYER', 'CUSTODY_DMT'], ['CUSTODY_DMT', 'BUYER']])
  })

  it('describes an invalid block honestly', () => {
    expect(describeEvent({ kind: 'block', t: 0, number: 410, txCount: 1, valid: 0, invalid: 1, codes: ['ENDORSEMENT_POLICY_FAILURE'] }))
      .toBe('block 410: 1 change, 1 rejected because not enough organisations signed it')
  })
})
