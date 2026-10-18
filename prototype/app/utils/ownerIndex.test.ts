import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { agencyHandles, ownerTagOf } from '../../server/utils/handles'
import { assign, fleetIn } from '../../server/utils/ownerIndex'

const fixture = JSON.parse(readFileSync(join(__dirname, '__fixtures__/handles.json'), 'utf8')) as Record<string, string>
const keys = { kv: Buffer.alloc(32, 0x11), kown: Buffer.alloc(32, 0x22) }

describe('owner index (DMT custody)', () => {
  it('derives the same owner tag as the Go identity package', () => {
    expect(ownerTagOf(keys, '751234567v')).toBe(fixture.ownerTag)
  })

  it('groups an owner\'s vehicles and moves a vehicle on transfer', () => {
    let index = { owners: {}, vehicles: {} }
    index = assign(index, 'owner-a', 'v2')
    index = assign(index, 'owner-a', 'v1')
    index = assign(index, 'owner-b', 'v3')
    expect(fleetIn(index, 'v1')).toEqual(['v1', 'v2'])
    index = assign(index, 'owner-b', 'v2') // v2 sold to owner b
    expect(fleetIn(index, 'v1')).toEqual(['v1'])
    expect(fleetIn(index, 'v3')).toEqual(['v2', 'v3'])
    expect(fleetIn(index, 'unknown')).toEqual(['unknown'])
  })
})

describe('agency handle resolver', () => {
  it('returns asset-scope identifiers only, never an owner pseudonym', () => {
    const got = agencyHandles(keys, { chassis: 'nhp10-1234567', engine: '1NZ-A123456', make: 'Toyota' }) as unknown as Record<string, string>
    expect(got.vid).toBe(fixture.vid)
    expect(got.idxEngine).toBe(fixture.idxEngine)
    expect(Object.keys(got).sort()).toEqual(['idxChassis', 'idxEngine', 'vid'])
    // Even if a caller smuggles a NIC in, nothing owner-scoped comes back.
    const smuggled = agencyHandles(keys, { chassis: 'nhp10-1234567', nic: '751234567v' } as never) as unknown as Record<string, string>
    expect(smuggled.pseudonym).toBeUndefined()
    expect(smuggled.ownerTag).toBeUndefined()
  })
})
