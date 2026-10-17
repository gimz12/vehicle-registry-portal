import type { RevealedIdentity } from '~~/shared/disclosure'
import { queryEnvelopeAnchor, queryRevealAudit, submitIdentityAccess } from '~~/server/utils/fabric'
import { loadEnvelope, openOwner } from '~~/server/utils/envelope'
import { CustodyError, loadOwner, revealOwner, servicesMode } from '~~/server/utils/custody'
import type { Refusal } from '~~/server/utils/reply'
import { refuse } from '~~/server/utils/reply'
import { publish } from '~~/server/utils/liveBus'

// Identity reveal under an approved case, at the Police custody service (or the
// file store in files mode). The audit write always precedes the returned identity.
export default defineEventHandler(async (event): Promise<RevealedIdentity | Refusal> => {
  const body = await readBody(event) as { caseId?: string, vid?: string }
  const caseId = String(body?.caseId ?? '').trim()
  const vid = String(body?.vid ?? '').trim().toLowerCase()
  if (!/^[A-Za-z0-9/_-]{1,64}$/.test(caseId) || !/^[a-f0-9]{64}$/.test(vid))
    return refuse(event, 400, 'caseId and a 64-character vid are required')
  const config = useRuntimeConfig(event)

  if (config.fabric.demoDir) {
    if (caseId !== `DEMO-CASE-${vid.slice(0, 8)}`)
      return refuse(event, 403, 'In demo mode only the pre-approved DEMO-CASE for this vehicle may reveal')
    const { envelope, shareDMT, sharePolice } = await loadEnvelope(config.fabric.demoDir, vid)
    const owner = openOwner(envelope, shareDMT, sharePolice)
    const accessLog = [`OWNER_SCOPE:${vid}`, `IDENTITY_REVEAL:${vid} (demo, not committed)`]
    return { vid, caseId, anchor: envelope.anchor, owner, accessLog }
  }

  let audit
  try {
    audit = await queryRevealAudit(config.fabric, caseId)
  }
  catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error'
    if (message.includes('not found'))
      return refuse(event, 404, `Case ${caseId} does not exist`)
    return refuse(event, 502, 'The case could not be read: ' + message)
  }
  if (audit.status !== 'APPROVED' || audit.vid !== vid)
    return refuse(event, 403, `Case ${caseId} is not an approved reveal for this vehicle`)
  try {
    if (servicesMode(config.fabric)) {
      const { owner, anchor, accessLog } = await revealOwner(config.fabric, vid, caseId)
      return { vid, caseId, anchor, owner, accessLog }
    }
    const onChainAnchor = await queryEnvelopeAnchor(config.fabric, vid, caseId)
    const { envelope, shareDMT, sharePolice } = await loadOwner(config.fabric, vid, caseId)
    const owner = openOwner(envelope, shareDMT, sharePolice, onChainAnchor)
    const logged = await submitIdentityAccess(config.fabric, caseId, vid)
    publish({ kind: 'offledger', what: 'open', actor: 'POLICE', detail: `case ${caseId}` })
    return { vid, caseId, anchor: envelope.anchor, owner, accessLog: logged.accessLog ?? [] }
  }
  catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown error'
    // A custodian's own refusal (403: its ledger check failed; 404; 409: altered) keeps its status and words.
    if (error instanceof CustodyError)
      return refuse(event, error.statusCode >= 500 ? 502 : error.statusCode,
        `${error.org.toUpperCase()} custody service: ${message}`)
    if (message.includes('ENOENT'))
      return refuse(event, 404, 'No off-ledger owner record exists for this vehicle')
    if (message.includes('anchor') || message.includes('shares'))
      return refuse(event, 409, message)
    // The chaincode's own refusal (for example, a case approved for a previous owner record).
    const refusal = message.match(/registry: [^"\n]*/)?.[0]
    if (refusal)
      return refuse(event, 409, refusal)
    return refuse(event, 502, 'The identity could not be revealed: ' + message)
  }
})
