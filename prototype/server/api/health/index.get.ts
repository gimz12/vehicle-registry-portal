const appStartTime = new Date().toISOString()

export default defineEventHandler(() => {
  return {
    up_since: appStartTime,
  }
})
