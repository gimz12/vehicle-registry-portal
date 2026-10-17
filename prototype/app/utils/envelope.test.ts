import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { anchorOf, openOwner, sealOwner } from '../../server/utils/envelope'
import type { StoredEnvelope, StoredShare } from '../../server/utils/envelope'

const vid = 'ab'.repeat(32)
const fixtures = join(__dirname, '__fixtures__/envelope')
const read = <T>(p: string) => JSON.parse(readFileSync(join(fixtures, p), 'utf8')) as T

describe('owner envelope', () => {
  it('opens an envelope sealed by the Go identity package (byte compatibility)', () => {
    const envelope = read<StoredEnvelope>(`owner_record/${vid}.json`)
    const dmt = read<StoredShare>(`custody_dmt/${vid}.json`)
    const police = read<StoredShare>(`custody_police/${vid}.json`)
    const owner = openOwner(envelope, dmt, police, envelope.anchor)
    expect(owner.name).toBe('Fixture Person')
    expect(owner.nic).toBe('199012345678V')
    expect(owner.note).toMatch(/SYNTHETIC/)
  })

  it('refuses without both shares and refuses a tampered or mismatched anchor', () => {
    const envelope = read<StoredEnvelope>(`owner_record/${vid}.json`)
    const dmt = read<StoredShare>(`custody_dmt/${vid}.json`)
    const police = read<StoredShare>(`custody_police/${vid}.json`)
    expect(() => openOwner(envelope, dmt, { vid, share: '' })).toThrow(/both custody shares/)
    expect(() => openOwner(envelope, dmt, police, 'ff'.repeat(32))).toThrow(/committed on the ledger/)
    const tampered = { ...envelope, ciphertext: 'AA' + envelope.ciphertext.slice(2) }
    expect(() => openOwner(tampered, dmt, police)).toThrow(/altered/)
  })

  it('seals in TypeScript with the same anchor rule and round-trips', () => {
    const sealed = sealOwner(vid, { name: 'Round Trip', nic: '1', address: '2', note: 'SYNTHETIC' })
    const nonce = Buffer.from(sealed.envelope.nonce, 'base64')
    const ct = Buffer.from(sealed.envelope.ciphertext, 'base64')
    expect(sealed.envelope.anchor).toBe(anchorOf(vid, 1, nonce, ct))
    expect(openOwner(sealed.envelope, sealed.shareDMT, sealed.sharePolice).name).toBe('Round Trip')
    // Shares are independent of the key: swapping one in from another record must fail.
    const other = sealOwner(vid, { name: 'Other', nic: '', address: '', note: '' })
    expect(() => openOwner(sealed.envelope, other.shareDMT, sealed.sharePolice)).toThrow()
  })
})
