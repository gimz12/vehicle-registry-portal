import { CustodyError, custodyPlateLookup, keyedHandles } from '~~/server/utils/custody'
import { overLimit } from '~~/server/utils/ratelimit'
import { refuse } from '~~/server/utils/reply'

// Agency-side resolver: chassis (or plate) in, asset-scope keyed identifiers out.
// No NIC in and no pseudonym out, so it is not an ownership oracle; not offered to buyers.
const DESK_AGENCIES = ['DMT', 'ASSEMBLER', 'POLICE', 'CUSTOMS', 'VET', 'INSURER']

export default defineEventHandler(async (event) => {
  type Lookup = { chassis?: string, engine?: string, make?: string, agency?: string, plate?: string }
  const body = await readBody(event) as Lookup
  const chassis = String(body?.chassis ?? '').trim()
  const engine = String(body?.engine ?? '').trim()
  const make = String(body?.make ?? '').trim()
  const plate = String(body?.plate ?? '').trim()
  if (!chassis && !plate)
    return refuse(event, 400, 'a chassis number or a number plate is required')
  const agency = String(body?.agency ?? 'DMT').toUpperCase()
  if (!DESK_AGENCIES.includes(agency))
    return refuse(event, 400, `agency must be one of ${DESK_AGENCIES.join(', ')}`)
  const config = useRuntimeConfig(event)
  if (plate && !chassis) {
    // The portal has no login, so the plate lookup is limited per address too.
    const wait = overLimit(event, 'plate', 60, 600)
    if (wait) {
      setResponseHeader(event, 'retry-after', wait)
      return refuse(event, 429, `Too many plate lookups from this address; try again in ${wait} seconds`)
    }
    try {
      return { vid: await custodyPlateLookup(config.fabric, plate, agency) }
    }
    catch (error) {
      if (error instanceof CustodyError)
        return refuse(event, error.statusCode >= 500 && error.statusCode !== 501 ? 502 : error.statusCode,
          error.message)
      throw error
    }
  }
  try {
    // No NIC is ever passed, so no pseudonym can come back.
    return await keyedHandles(config.fabric, { chassis, engine, make }, agency)
  }
  catch (error) {
    if (error instanceof CustodyError && error.statusCode >= 500)
      return refuse(event, 502, error.message)
    return refuse(event, 400, error instanceof Error ? error.message : 'Invalid identifier')
  }
})
