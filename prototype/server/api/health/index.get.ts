import { custodyHealth } from '~~/server/utils/custody'

const appStartTime = new Date().toISOString()

// Portal liveness plus each custody service's health. Used by /live and the demo scripts.
export default defineEventHandler(async (event) => {
  return {
    up_since: appStartTime,
    custody: await custodyHealth(useRuntimeConfig(event).fabric),
  }
})
