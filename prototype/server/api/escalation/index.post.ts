import { CustodyError } from '~~/server/utils/custody'
import type { EscalationInput } from '~~/shared/disclosure'
import { validateEscalation } from '~~/shared/disclosure'
import { submitOwnerPredicate, submitRevealApproval, submitRevealRejection, submitRevealRequest } from '~~/server/utils/fabric'
import { refuse } from '~~/server/utils/reply'

// Owner-scope escalation writes to the ledger (the case and its approvals are
// the audit record), so this is the portal's only submit path.
export default defineEventHandler(async (event) => {
  const body = await readBody(event)
  const problem = validateEscalation(body)
  if (problem)
    return refuse(event, 400, problem)
  const input = body as EscalationInput
  const config = useRuntimeConfig(event)
  try {
    if (input.action === 'predicate')
      return await submitOwnerPredicate(config.fabric, input.vid!, input.ruleId as 'F6a' | 'F6b')
    if (input.action === 'create') {
      return await submitRevealRequest(config.fabric, input.as, {
        caseId: input.caseId!, vid: input.vid!, ruleId: input.ruleId!,
        ground: input.ground!, groundRef: input.groundRef!,
      })
    }
    if (input.action === 'reject')
      return await submitRevealRejection(config.fabric, input.as, input.caseId!, input.reason!.trim())
    return await submitRevealApproval(config.fabric, input.as, input.caseId!)
  }
  catch (error) {
    const message = error instanceof Error ? error.message : 'Unknown Fabric error'
    if (error instanceof CustodyError)
      return refuse(event, error.statusCode >= 500 ? 502 : error.statusCode, `DMT custody service: ${message}`)
    const refusal = message.match(/registry: [^"\n]*/)?.[0]
    return refusal ? refuse(event, 409, refusal) : refuse(event, 502, 'The escalation could not be committed: ' + message)
  }
})
