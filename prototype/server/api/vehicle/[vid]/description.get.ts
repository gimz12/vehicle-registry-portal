import { CustodyError, custodyDescription } from '~~/server/utils/custody'
import { overLimit } from '~~/server/utils/ratelimit'
import { refuse } from '~~/server/utils/reply'

// What the vehicle is, from the DMT's database, for the buyer page. Never the
// plate: a handle must not lead to a car on the road.
export default defineEventHandler(async (event) => {
  const vid = getRouterParam(event, 'vid')?.toLowerCase() ?? ''
  if (!/^[a-f0-9]{64}$/.test(vid))
    return refuse(event, 400, 'A 64-character hexadecimal vehicle handle is required')
  const wait = overLimit(event, 'description', 120, 600)
  if (wait) {
    setResponseHeader(event, 'retry-after', wait)
    return refuse(event, 429, `Too many lookups from this address; try again in ${wait} seconds`)
  }
  const config = useRuntimeConfig(event)
  try {
    const { plate: _plate, ...description } = await custodyDescription(config.fabric, vid, 'buyer_portal') as unknown as Record<string, unknown>
    return description
  }
  catch (error) {
    if (error instanceof CustodyError)
      return refuse(event, error.statusCode >= 500 && error.statusCode !== 501 ? 502 : error.statusCode, error.message)
    throw error
  }
})
