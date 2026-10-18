import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { canonicalNIC, deriveHandles, idxEngineOf, vidOf } from '../../server/utils/handles'

// Fixture produced by `go run ./cmd/handle` with k_v = 0x11…, k_own = 0x22…
const fixture = JSON.parse(readFileSync(join(__dirname, '__fixtures__/handles.json'), 'utf8')) as Record<string, string>
const keys = { kv: Buffer.alloc(32, 0x11), kown: Buffer.alloc(32, 0x22) }

describe('keyed identifiers', () => {
  it('derives the same handle, indices and pseudonym as the Go identity package', () => {
    const got = deriveHandles(keys, { chassis: 'nhp10-1234567', engine: '1NZ-A123456', make: 'Toyota', nic: '751234567v' })
    expect(got.vid).toBe(fixture.vid)
    expect(got.idxChassis).toBe(fixture.idxChassis)
    expect(got.idxEngine).toBe(fixture.idxEngine)
    expect(got.pseudonym).toBe(fixture.pseudonym)
  })

  it('canonicalises spelling so one chassis number has one handle', () => {
    expect(vidOf(keys, 'NHP10 1234567')).toBe(fixture.vid)
    expect(vidOf(keys, 'nhp10-1234567')).toBe(fixture.vid)
    expect(vidOf(keys, 'NHP101234568')).not.toBe(fixture.vid)
  })

  it('scopes the engine index by make and refuses null-like identifiers', () => {
    expect(idxEngineOf(keys, 'Toyota', '1NZ-A123456')).not.toBe(idxEngineOf(keys, 'Nissan', '1NZ-A123456'))
    expect(() => vidOf(keys, 'N/A')).toThrow(/canonicalisation/)
    expect(() => vidOf(keys, '0')).toThrow()
    expect(canonicalNIC('197512345678')).toBe('197512345678')
    expect(canonicalNIC('751234567v')).toBe('751234567V')
    expect(() => canonicalNIC('197512345678V')).toThrow(/NIC/)
  })
})
