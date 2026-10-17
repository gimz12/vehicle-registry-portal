import { promises as fs } from 'node:fs'
import { join } from 'node:path'

interface DemoVehicle { vid: string, scenario: string, title: string, source: string, vehicle: number }

// Lists the Corpus C demo vehicles when the portal runs on cmd/disclose output.
export default defineEventHandler(async (event): Promise<DemoVehicle[]> => {
  const config = useRuntimeConfig(event)
  if (!config.fabric.demoDir)
    return []
  try {
    return JSON.parse(await fs.readFile(join(config.fabric.demoDir, 'index.json'), 'utf8')) as DemoVehicle[]
  }
  catch {
    return []
  }
})
