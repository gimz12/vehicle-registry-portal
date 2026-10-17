import { createCipheriv, createDecipheriv, createHash, randomBytes, timingSafeEqual } from 'node:crypto'
import { promises as fs } from 'node:fs'
import { join } from 'node:path'

/**
 * TypeScript twin of identity.SealOwner / OpenOwner (Go), byte-compatible:
 * AES-256-GCM, per-record key = shareDMT XOR sharePolice, 12-byte nonce,
 * additional data "vehicle-owner-envelope:v1:<vid>", anchor = sha256(version
 * byte || vid || nonce || ciphertext||tag). The on-chain anchor must equal the
 * stored anchor before anything is decrypted.
 */

export interface StoredEnvelope {
  vid: string
  version: number
  ciphertext: string // base64, ciphertext followed by the 16-byte GCM tag (Go layout)
  nonce: string // base64
  anchor: string
}
export interface StoredShare { vid: string, share: string }
export interface OwnerPII { name: string, nic: string, address: string, note: string }

const aad = (vid: string, version: number) => Buffer.from(`vehicle-owner-envelope:v${version}:${vid}`)

export function anchorOf(vid: string, version: number, nonce: Buffer, ciphertext: Buffer): string {
  return createHash('sha256')
    .update(Buffer.from([version]))
    .update(Buffer.from(vid))
    .update(nonce)
    .update(ciphertext)
    .digest('hex')
}

export interface SealedOwner { envelope: StoredEnvelope, shareDMT: StoredShare, sharePolice: StoredShare }

export function sealOwner(vid: string, pii: OwnerPII): SealedOwner {
  const recordKey = randomBytes(32)
  const dmt = randomBytes(32)
  const police = Buffer.alloc(32)
  for (let i = 0; i < 32; i++)
    police[i] = recordKey[i]! ^ dmt[i]!
  const nonce = randomBytes(12)
  const cipher = createCipheriv('aes-256-gcm', recordKey, nonce)
  cipher.setAAD(aad(vid, 1))
  const body = Buffer.concat([cipher.update(Buffer.from(JSON.stringify(pii))), cipher.final()])
  const ciphertext = Buffer.concat([body, cipher.getAuthTag()])
  recordKey.fill(0)
  return {
    envelope: { vid, version: 1, ciphertext: ciphertext.toString('base64'), nonce: nonce.toString('base64'), anchor: anchorOf(vid, 1, nonce, ciphertext) },
    shareDMT: { vid, share: dmt.toString('base64') },
    sharePolice: { vid, share: police.toString('base64') },
  }
}

export function openOwner(
  envelope: StoredEnvelope,
  shareDMT: StoredShare,
  sharePolice: StoredShare,
  onChainAnchor?: string,
): OwnerPII {
  const nonce = Buffer.from(envelope.nonce, 'base64')
  const ciphertext = Buffer.from(envelope.ciphertext, 'base64')
  const expected = Buffer.from(anchorOf(envelope.vid, envelope.version, nonce, ciphertext), 'hex')
  const stored = Buffer.from(envelope.anchor, 'hex')
  if (stored.length !== expected.length || !timingSafeEqual(stored, expected))
    throw new Error('owner envelope anchor mismatch: the off-ledger record was altered')
  if (onChainAnchor !== undefined) {
    const chain = Buffer.from(onChainAnchor, 'hex')
    if (chain.length !== expected.length || !timingSafeEqual(chain, expected))
      throw new Error('owner envelope does not match the anchor committed on the ledger')
  }
  const a = Buffer.from(shareDMT.share, 'base64')
  const b = Buffer.from(sharePolice.share, 'base64')
  if (a.length !== 32 || b.length !== 32)
    throw new Error('both custody shares are required to reveal')
  const key = Buffer.alloc(32)
  for (let i = 0; i < 32; i++)
    key[i] = a[i]! ^ b[i]!
  const decipher = createDecipheriv('aes-256-gcm', key, nonce)
  decipher.setAAD(aad(envelope.vid, envelope.version))
  decipher.setAuthTag(ciphertext.subarray(ciphertext.length - 16))
  const plain = Buffer.concat([decipher.update(ciphertext.subarray(0, ciphertext.length - 16)), decipher.final()])
  key.fill(0)
  return JSON.parse(plain.toString('utf8')) as OwnerPII
}

// ---- file-backed development store (mirrors offchain/schema.sql) ----

async function readJSON<T>(path: string): Promise<T> {
  return JSON.parse(await fs.readFile(path, 'utf8')) as T
}

export async function loadEnvelope(store: string, vid: string) {
  return {
    envelope: await readJSON<StoredEnvelope>(join(store, 'owner_record', `${vid}.json`)),
    shareDMT: await readJSON<StoredShare>(join(store, 'custody_dmt', `${vid}.json`)),
    sharePolice: await readJSON<StoredShare>(join(store, 'custody_police', `${vid}.json`)),
  }
}

export async function storeEnvelope(store: string, sealed: SealedOwner) {
  const writes: [string, unknown][] = [
    [join(store, 'owner_record', `${sealed.envelope.vid}.json`), sealed.envelope],
    [join(store, 'custody_dmt', `${sealed.envelope.vid}.json`), sealed.shareDMT],
    [join(store, 'custody_police', `${sealed.envelope.vid}.json`), sealed.sharePolice],
  ]
  for (const [path, value] of writes) {
    await fs.mkdir(join(path, '..'), { recursive: true })
    await fs.writeFile(path, `${JSON.stringify(value, null, 2)}\n`, { mode: 0o600 })
  }
}
