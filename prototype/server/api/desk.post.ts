import type { DeskInput, DeskResult } from '~~/shared/disclosure'
import { validateDesk } from '~~/shared/disclosure'
import { submitAsAgency } from '~~/server/utils/fabric'
import type { SealedOwner } from '~~/server/utils/envelope'
import { sealOwner } from '~~/server/utils/envelope'
import {
  CustodyError, custodyPlateLookup, custodyTheftCheck, keyedHandles, servicesMode, storeInternal, storeOwner,
  storeVehicleIdentity,
} from '~~/server/utils/custody'
import { loadRegistryKeys, ownerTagOf } from '~~/server/utils/handles'
import { loadOwnerIndex } from '~~/server/utils/ownerIndex'
import type { Handles } from '~~/server/utils/custody'
import type { Refusal } from '~~/server/utils/reply'
import { overLimit } from '~~/server/utils/ratelimit'
import { refuse } from '~~/server/utils/reply'

// One route for every agency write. The chaincode decides; this route shapes the
// arguments and returns the refusal text.
export default defineEventHandler(async (event): Promise<DeskResult | Refusal> => {
  const body = await readBody(event)
  const problem = validateDesk(body)
  if (problem)
    return refuse(event, 400, problem)
  const d = body as DeskInput
  const config = useRuntimeConfig(event)
  let vid = d.vid?.toLowerCase()
  // A bad chassis or NIC is the officer's error. Only the DMT's credential may send a NIC.
  const keyed = (input: Parameters<typeof keyedHandles>[1]): Promise<Handles> =>
    keyedHandles(config.fabric, input, d.agency)

  // Only the description's keyed anchor reaches the ledger; the fields stay with the DMT.
  const desc = d.model
    ? { model: d.model, year: Number(d.year), engineCc: Number(d.engineCc), fuel: String(d.fuel).toUpperCase() }
    : {}
  let descAnchor: string | undefined

  let fn = ''
  let args: string[] = []
  let message = ''
  // Stored only after the ledger commits: a record whose anchor was refused would break every later reveal.
  let sealed: SealedOwner | null = null
  // The owner's NIC, from which the DMT custodian keys its owner index after the commit.
  let ownerNic: string | null = null
  try {
    switch (d.action) {
      case 'import':
      case 'attest': {
      // The same keyed indices the DMT computes at registration; that is the F5 match.
        const h = await keyed({ chassis: d.chassisNumber!, engine: d.engineNumber!, make: d.make!, ...desc })
        vid = h.vid
        fn = 'CreateImportAttestation'
        const source = d.action === 'attest' ? 'ASSEMBLER' : 'CUSTOMS'
        args = [JSON.stringify({
          recordId: d.recordId, idxChassis: h.idxChassis, idxEngine: h.idxEngine, source,
          ...(h.descAnchor ? { descAnchor: h.descAnchor } : {}),
        })]
        message = `${source === 'CUSTOMS' ? 'Import clearance' : 'Assembler attestation'} recorded; handle ${h.vid.slice(0, 12)}…`
        break
      }
      case 'register': {
      // Owner details are sealed off-ledger; only the anchor is committed.
        // A plate already on another live vehicle is refused before the ledger write.
        if (d.plate && servicesMode(config.fabric)) {
          const taken = await custodyPlateLookup(config.fabric, d.plate, 'DMT').catch((error: unknown) => {
            if (error instanceof CustodyError && error.statusCode === 404)
              return null
            throw error
          })
          if (taken)
            return refuse(event, 409, 'This number plate is already assigned to another live vehicle; nothing was written')
        }
        const h = await keyed({
          chassis: d.chassisNumber!, engine: d.engineNumber!, make: d.make!, nic: d.owner!.nic, ...desc,
        })
        vid = h.vid
        descAnchor = h.descAnchor
        sealed = sealOwner(vid, { ...d.owner!, note: 'SYNTHETIC RECORD. No real person is described.' })
        ownerNic = d.owner!.nic
        fn = 'RegisterVehicle'
        args = [JSON.stringify({
          vid, pseudonym: h.pseudonym, idxChassis: h.idxChassis, idxEngine: h.idxEngine,
          provenance: d.provenance ?? 'CUSTOMS', envelopeAnchor: sealed.envelope.anchor,
          ...(descAnchor ? { descAnchor } : {}),
        })]
        message = `Vehicle registered as ${vid.slice(0, 12)}…; owner record sealed off-ledger`
        break
      }
      case 'transfer': {
      // New pseudonym and sealed record; the previous owner's envelope is replaced.
        sealed = sealOwner(vid!, { ...d.owner!, note: 'SYNTHETIC RECORD. No real person is described.' })
        ownerNic = d.owner!.nic
        fn = 'TransferVehicle'
        args = [JSON.stringify({
          recordId: d.recordId, vid,
          pseudonym: (await keyed({ vid, nic: d.owner!.nic })).pseudonym,
          envelopeAnchor: sealed.envelope.anchor,
        })]
        message = 'Ownership transferred; new owner record sealed off-ledger, previous record replaced'
        break
      }
      case 'retire':
        fn = 'SetRetiredStatus'
        args = [vid!, String(d.flag ?? true)]
        message = d.flag === false ? 'Vehicle returned to the active register' : 'Vehicle retired from the register'
        break
      case 'encumber':
        fn = 'SetEncumbrance'
        args = [JSON.stringify({ recordId: d.recordId, vid, reference: d.reference })]
        message = 'Vehicle recorded as under finance; transfers are refused until the release is lodged'
        break
      case 'release':
        fn = 'ReleaseEncumbrance'
        args = [JSON.stringify({ recordId: d.recordId, vid, reference: d.reference })]
        message = 'Finance release recorded; the vehicle may be transferred'
        break
      case 'mileage':
        fn = 'AppendMileage'
        args = [JSON.stringify({
          recordId: d.recordId, vid, day: d.day, odometer: d.odometer, vetLaneId: d.vetLaneId || 'lane-1',
        })]
        message = 'Odometer reading recorded'
        break
      case 'emission':
        fn = 'AppendEmission'
        args = [JSON.stringify({ recordId: d.recordId, vid, day: d.day, vetLaneId: d.vetLaneId || 'lane-1' })]
        message = 'Emission certificate recorded (not used by any fraud rule)'
        break
      case 'report':
        // A report alone flags nothing and blocks nothing.
        fn = 'AppendTheftReport'
        args = [JSON.stringify({ recordId: d.recordId, vid, reference: d.reportRef })]
        message = `Theft report ${d.reportRef} recorded; the vehicle is not flagged until the owner is confirmed`
        break
      case 'theft':
        // With a NIC the DMT first confirms the reporter is the owner on the ledger; without
        // one the report must already be confirmed or name an approved theft case.
        if (d.reporterNic) {
          // The portal has no login: the owner check is limited per address as well.
          const wait = overLimit(event, 'ownercheck', 30, 600)
          if (wait)
            return refuse(event, 429, `Too many owner checks from this address; try again in ${wait} seconds`)
          let confirmed: boolean
          try {
            confirmed = servicesMode(config.fabric)
              ? await custodyTheftCheck(config.fabric, vid!, d.reporterNic, d.reportRef!)
              : await filesModeOwnerCheck(config.fabric, vid!, d.reporterNic, d.reportRef!)
          }
          catch (error) {
            if (error instanceof CustodyError && error.statusCode < 500)
              return refuse(event, error.statusCode, `DMT custody service: ${error.message}`)
            const refusal = error instanceof Error ? error.message.match(/registry: [^"\n]*/)?.[0] : undefined
            if (refusal)
              return refuse(event, 409, refusal)
            throw error
          }
          if (!confirmed)
            return refuse(event, 403, 'The DMT did not confirm the reporter as the registered owner of this vehicle; nothing was flagged')
        }
        fn = 'SetTheftStatus'
        args = [JSON.stringify({ recordId: d.recordId, vid, reference: d.reportRef }), 'true']
        message = d.reporterNic
          ? 'The DMT confirmed the reporter as the owner; vehicle flagged stolen and transfers are now refused'
          : 'Vehicle flagged stolen; transfers are now refused'
        break
      case 'clear':
        fn = 'SetTheftStatus'
        args = [JSON.stringify({ recordId: d.recordId, vid }), 'false']
        message = 'Theft report cleared'
        break
      case 'accident':
        fn = 'AppendAccident'
        args = [JSON.stringify({ recordId: d.recordId, vid, day: d.day })]
        message = 'Accident report filed'
        break
      case 'claim':
        fn = 'AppendClaim'
        args = [JSON.stringify({
          recordId: d.recordId, vid, day: d.day, insurerId: d.insurerId,
          evidenceAnchor: d.evidenceAnchor || '',
          evidenceKind: d.evidenceAnchor ? (d.evidenceKind ?? 'POLICE_REPORT') : '',
          // Empty when no assessor attested, so such claims never look like a shared assessor to F6b.
          attestorId: d.assessorId || '',
        })]
        message = 'Claim filed'
        break
      case 'writeoff':
        fn = 'SetWriteOffStatus'
        args = [vid!, String(d.flag ?? true)]
        message = d.flag === false ? 'Write-off resolved' : 'Vehicle written off; transfers are refused until resolved'
        break
    }
  }
  catch (error) {
    if (error instanceof CustodyError && error.statusCode >= 500)
      return refuse(event, 502, `Nothing was written: ${error.message}`)
    return refuse(event, 400, error instanceof Error ? error.message : 'Invalid identifier')
  }
  let payload: unknown
  try {
    payload = await submitAsAgency(config.fabric, d.agency, 'VehicleRegistry', fn, args, d.insurerId)
  }
  catch (error) {
    const raw = error instanceof Error ? error.message : 'Unknown Fabric error'
    // Surface the chaincode's own refusal text ("registry: ...") when present.
    const refusal = raw.match(/registry: [^"\n]*/)?.[0]
    // 409 only for the chaincode's own refusal; demo mode or a network failure is not one.
    return refusal ? refuse(event, 409, refusal, { agency: d.agency, action: d.action }) : refuse(event, 502, raw)
  }
  // After the commit the ledger write stands: each store is tried on its own and failures are reported.
  const failed: string[] = []
  const attempt = async (what: string, store: () => Promise<unknown>) => {
    try {
      return await store()
    }
    catch (error) {
      failed.push(`${what} (${error instanceof Error ? error.message : 'unknown error'})`)
    }
  }
  if (sealed && vid && ownerNic)
    await attempt('the owner record', () => storeOwner(config.fabric, sealed, ownerNic))
  // After a registration the DMT keeps the description and the plate in its own database.
  if (d.action === 'register' && vid && (d.model || d.plate)) {
    const kept: string[] = []
    if (d.model) {
      const description = { make: d.make!, ...desc, colour: d.colour || 'not recorded' } as Parameters<typeof storeVehicleIdentity>[2]
      const stored = await attempt('the description', () => storeVehicleIdentity(config.fabric, vid, description, undefined, descAnchor))
      kept.push(...(stored as string[] | undefined ?? []))
    }
    if (d.plate) {
      const stored = await attempt('the plate', () => storeVehicleIdentity(config.fabric, vid, null, d.plate, descAnchor))
      kept.push(...(stored as string[] | undefined ?? []))
    }
    if (kept.length)
      message += `; ${kept.join(' and ')} kept in the DMT's database`
  }
  // The agency's own internal fields go to its own service after the commit; the ledger never sees them.
  const owner = { import: 'customs', mileage: 'vet', emission: 'vet', claim: 'insurer' } as const
  const org = owner[d.action as keyof typeof owner]
  let internalStored = false
  if (org && d.internal && vid && d.recordId)
    internalStored = !!(await attempt(`the ${org} internal record`,
      () => storeInternal(config.fabric, org, d.recordId!, vid, d.internal!)))
  if (failed.length)
    return refuse(event, 502, `The ledger committed but ${failed.join('; ')} could not be stored off-ledger. Do not resubmit; the DMT can store it again later`)
  if (internalStored)
    message += `; internal record kept in the ${org === 'vet' ? 'emission centre' : org === 'customs' ? 'Customs' : 'insurer'} service`
  return { ok: true, action: d.action, agency: d.agency, message, vid, payload }
})

/** Files mode: the portal compares owner tags itself and, on yes, confirms as the DMT. */
async function filesModeOwnerCheck(
  config: Parameters<typeof submitAsAgency>[0], vid: string, nic: string, reportRef: string,
): Promise<boolean> {
  const keys = await loadRegistryKeys(config)
  const index = await loadOwnerIndex(config.offchainDir)
  if (index.vehicles[vid] !== ownerTagOf(keys, nic))
    return false
  await submitAsAgency(config, 'DMT', 'VehicleRegistry', 'ConfirmReporter',
    [JSON.stringify({ recordId: `owner-confirmed:${reportRef}:${Date.now()}`, vid, reference: reportRef })])
  return true
}
