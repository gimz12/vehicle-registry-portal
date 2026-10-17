import { CustodyError } from '~~/server/utils/custody'
import { parsePurpose } from '~~/shared/disclosure'
import { queryVehicleDisclosure } from '~~/server/utils/fabric'
import { refuse } from '~~/server/utils/reply'

export default defineEventHandler(async (event) => {
  const vid = getRouterParam(event, 'vid')?.toLowerCase() ?? ''
  if (!/^[a-f0-9]{64}$/.test(vid))
    return refuse(event, 400, 'A 64-character hexadecimal vehicle handle is required')
  const query = getQuery(event)
  const purpose = parsePurpose(query.as ?? 'BUYER')
  if (!purpose)
    return refuse(event, 400, 'as must be BUYER, DMT, INSURER or POLICE')
  const caseId = typeof query.case === 'string' ? query.case.trim() : ''
  if (caseId && !/^[A-Za-z0-9/_-]{1,64}$/.test(caseId))
    return refuse(event, 400, 'case must be 1–64 characters of letters, digits, / _ or -')
  const config = useRuntimeConfig(event)
  try {
    return await queryVehicleDisclosure(config.fabric, vid, purpose, caseId)
  }
  catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown Fabric error'
    // The DMT custodian refused the fleet (its own ledger read) or is unreachable.
    if (error instanceof CustodyError)
      return refuse(event, error.statusCode >= 500 ? 502 : 403,
        `The registry refused this purpose: DMT custody service: ${message}`)
    if (message.includes('not authorised') || message.includes('not an approved reveal')
      || message.includes('unknown purpose') || message.includes('case '))
      return refuse(event, 403, 'The registry refused this purpose: ' + message)
    if (message.includes('not found'))
      return refuse(event, 404, 'Vehicle was not found')
    return refuse(event, 502, 'The registry ledger could not be queried', import.meta.dev ? { message } : undefined)
  }
})
