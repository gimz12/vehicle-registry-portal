import type { H3Event } from 'h3'

/**
 * A refusal is an expected answer, not an exception: returned as a body it stays
 * out of the dev server's error renderer and keeps the `statusMessage` shape.
 */
export interface Refusal { statusCode: number, statusMessage: string, data?: Record<string, unknown> }

export function refuse(
  event: H3Event,
  statusCode: number,
  statusMessage: string,
  data?: Record<string, unknown>,
): Refusal {
  setResponseStatus(event, statusCode, statusMessage)
  return data ? { statusCode, statusMessage, data } : { statusCode, statusMessage }
}
