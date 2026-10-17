import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { canonical, loadRegistryKeys } from '../../server/utils/handles'
import { fleetOf, recordOwnership } from '../../server/utils/ownerIndex'

describe('off-ledger store robustness (CQ-11..14)', () => {
  it('keeps every vehicle when one owner\'s writes race', async () => {
    const store = mkdtempSync(join(tmpdir(), 'oi-'))
    const vids = Array.from({ length: 20 }, (_, i) => String(i).padStart(64, '0'))
    await Promise.all(vids.map(v => recordOwnership(store, 'tag', v)))
    expect(await fleetOf(store, vids[0]!)).toHaveLength(20)
  })

  it('refuses a damaged owner index instead of reading it as empty', async () => {
    const store = mkdtempSync(join(tmpdir(), 'oi-'))
    await recordOwnership(store, 'tag', 'a'.repeat(64))
    writeFileSync(join(store, 'custody_dmt', 'owner_index.json'), '{"owners":')
    await expect(fleetOf(store, 'a'.repeat(64))).rejects.toThrow()
    await expect(recordOwnership(store, 'tag', 'b'.repeat(64))).rejects.toThrow()
    expect(readFileSync(join(store, 'custody_dmt', 'owner_index.json'), 'utf8')).toBe('{"owners":')
  })

  it('never replaces an unreadable key file with fresh keys', async () => {
    const dir = mkdtempSync(join(tmpdir(), 'keys-'))
    writeFileSync(join(dir, 'registry-keys.json'), '{"kv": "00", ')
    await expect(loadRegistryKeys({ offchainDir: dir })).rejects.toThrow()
    expect(readFileSync(join(dir, 'registry-keys.json'), 'utf8')).toBe('{"kv": "00", ')
  })

  it('canonicalises per character, as Go does', () => {
    expect(canonical('WVW-ß-123')).toBe('WVW123')
    expect(canonical('nhp10 1234567')).toBe('NHP101234567')
  })
})
