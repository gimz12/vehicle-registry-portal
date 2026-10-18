import { CustodyError, custodyMatch } from '~~/server/utils/custody'
import { overLimit } from '~~/server/utils/ratelimit'
import { refuse } from '~~/server/utils/reply'

// One bit: does the chassis typed from the car give this handle? Never a code, so
// it cannot look up an unknown car. Limited per address and vehicle.
export default defineEventHandler(async (event) => {
  const vid = getRouterParam(event, 'vid')?.toLowerCase() ?? ''
  if (!/^[a-f0-9]{64}$/.test(vid))
    return refuse(event, 400, 'A 64-character hexadecimal vehicle handle is required')
  const body = await readBody(event) as { chassis?: string }
  const chassis = String(body?.chassis ?? '').trim()
  if (!chassis)
    return refuse(event, 400, 'the chassis number stamped on the car is required')
  const wait = overLimit(event, 'match', 10, 600, vid)
  if (wait) {
    setResponseHeader(event, 'retry-after', wait)
    return refuse(event, 429, `Too many chassis checks on this vehicle from this address; try again in ${wait} seconds`)
  }
  const config = useRuntimeConfig(event)
  try {
    return { match: await custodyMatch(config.fabric, vid, chassis, 'buyer_portal') }
  }
  catch (error) {
    if (error instanceof CustodyError)
      return refuse(event, error.statusCode >= 500 && error.statusCode !== 501 ? 502 : error.statusCode, error.message)
    throw error
  }
})
