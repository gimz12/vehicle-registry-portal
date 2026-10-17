import { queryRevealAudit } from '~~/server/utils/fabric'
import { refuse } from '~~/server/utils/reply'

// Case ids may contain slashes (CID/2026/0007), so the id travels as a query
// parameter rather than a path segment.
export default defineEventHandler(async (event) => {
  const caseId = String(getQuery(event).case ?? '').trim()
  if (!/^[A-Za-z0-9/_-]{1,64}$/.test(caseId))
    return refuse(event, 400, 'case must be 1–64 characters of letters, digits, / _ or -')
  const config = useRuntimeConfig(event)
  try {
    return await queryRevealAudit(config.fabric, caseId)
  }
  catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown Fabric error'
    if (message.includes('not found'))
      return refuse(event, 404, 'Case was not found')
    return refuse(event, 502, 'The reveal audit could not be read: ' + message)
  }
})
