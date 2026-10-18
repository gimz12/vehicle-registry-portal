import { promises as fs } from 'node:fs'
import { isAbsolute, join, resolve } from 'node:path'

// Serves the frozen sweep output (eval/results/<date>). Nothing here recomputes a
// metric: it aggregates per-run JSON into ranges across seeds × rates.

type Profile = 'P0' | 'P1' | 'P2' | 'P3'
const PROFILES: Profile[] = ['P0', 'P1', 'P2', 'P3']

interface Cell {
  ruleId: string
  truePositive: number
  falsePositive: number
  falseNegative: number
  evaluated: number
  gapOnly: number
  gapStructural: number
  suppressed: number
}
interface RunFile {
  corpus: string
  seed: number
  rate: number
  profile: Profile
  report: {
    cells: Cell[]
    exposure: {
      mean: number
      median: number
      p95: number
      max: number
      resolvableRefs: number
      linkability: number
      quasiLinkability: number
    }
  }
}

export interface Range { min: number, max: number }
export interface GridCell {
  measured: 'available' | 'partial' | 'suppressed' | 'unstable'
  predicted: 'available' | 'partial' | 'suppressed' | '?'
  agree: boolean
  precision: Range
  recall: Range
  f1: Range
  coverage: Range
  runs: number
}
export interface ResultsPayload {
  date: string
  manifestSha256: string
  seeds: number[]
  rates: number[]
  rules: string[]
  grid: Record<string, Record<string, Record<Profile, GridCell>>> // corpus -> rule -> profile
  agreement: Record<string, { agree: number, total: number }>
  exposure: Record<Profile, {
    mean: Range
    p95: Range
    max: Range
    resolvableRefs: Range
    linkability: Range
    quasiLinkability: Range
  }>
  frontier: { corpus: string, seed: number, rate: number, profile: Profile, meanExposure: number, macroF1: number }[]
  scenarios: { id: string, title: string, source: string, rule: string, outcome: Record<Profile, string> }[]
  latency: string
}

const classOf = (c: Cell) => c.evaluated === 0 ? 'suppressed' : c.gapStructural > 0 ? 'partial' : 'available'
const safeDiv = (a: number, b: number) => (b === 0 ? 0 : a / b)
const range = (values: number[]): Range => ({ min: Math.min(...values), max: Math.max(...values) })

async function walk(dir: string): Promise<string[]> {
  const out: string[] = []
  for (const entry of await fs.readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory())
      out.push(...await walk(path))
    else if (/^P[0-3]\.json$/.test(entry.name))
      out.push(path)
  }
  return out
}

function glyphClass(g: string): GridCell['predicted'] {
  return g === '✓' ? 'available' : g === '△' ? 'partial' : g === '✗' ? 'suppressed' : '?'
}

export default defineEventHandler(async (event): Promise<ResultsPayload> => {
  const config = useRuntimeConfig(event)
  const configured = config.resultsDir || 'eval/results/2026-10-03'
  const dir = isAbsolute(configured) ? configured : resolve(process.cwd(), configured)
  const date = dir.split('/').pop() ?? ''

  let files: string[]
  try {
    files = await walk(dir)
  }
  catch {
    throw createError({
      statusCode: 404,
      statusMessage: `No sweep results found at ${configured}. Run ./eval/reproduce.sh first.`,
    })
  }
  const runs: RunFile[] = await Promise.all(files.map(async f => JSON.parse(await fs.readFile(f, 'utf8')) as RunFile))

  // Prediction table (frozen with the results).
  const predicted: Record<string, Record<Profile, GridCell['predicted']>> = {}
  try {
    const md = await fs.readFile(resolve(dir, '../../predictions.md'), 'utf8')
    for (const line of md.split('\n')) {
      if (!line.startsWith('| F'))
        continue
      const fields = line.split('|')
      const rule = fields[1]!.trim().split(/\s+/)[0]!
      predicted[rule] = {
        P0: glyphClass(fields[3]!.trim()), P1: glyphClass(fields[4]!.trim()),
        P2: glyphClass(fields[5]!.trim()), P3: glyphClass(fields[6]!.trim()),
      }
    }
  }
  catch { /* grid still renders without predictions */ }

  const rules = [...new Set(runs.flatMap(r => r.report.cells.map(c => c.ruleId)))].sort()
  const grid: ResultsPayload['grid'] = {}
  const agreement: ResultsPayload['agreement'] = {}
  for (const corpus of [...new Set(runs.map(r => r.corpus))].sort()) {
    grid[corpus] = {}
    agreement[corpus] = { agree: 0, total: 0 }
    for (const rule of rules) {
      grid[corpus][rule] = {} as Record<Profile, GridCell>
      for (const profile of PROFILES) {
        const cells = runs.filter(r => r.corpus === corpus && r.profile === profile)
          .map(r => r.report.cells.find(c => c.ruleId === rule)!).filter(Boolean)
        const classes = new Set(cells.map(classOf))
        const measured = classes.size === 1 ? [...classes][0]! : 'unstable'
        const pred = predicted[rule]?.[profile] ?? '?'
        const f1 = cells.map((c) => {
          const p = safeDiv(c.truePositive, c.truePositive + c.falsePositive)
          const r = safeDiv(c.truePositive, c.truePositive + c.falseNegative)
          return p + r === 0 ? 0 : 2 * p * r / (p + r)
        })
        let agree = pred === measured
        // △ realised as an accuracy drop inside a decidable cell counts as agreement.
        if (!agree && pred === 'partial' && measured === 'available') {
          const prev = PROFILES[PROFILES.indexOf(profile) - 1]
          const prevF1 = prev ? grid[corpus][rule][prev]?.f1.min : undefined
          if (prevF1 !== undefined && Math.min(...f1) < prevF1)
            agree = true
        }
        agreement[corpus].total++
        if (agree)
          agreement[corpus].agree++
        grid[corpus][rule][profile] = {
          measured,
          predicted: pred,
          agree,
          precision: range(cells.map(c => safeDiv(c.truePositive, c.truePositive + c.falsePositive))),
          recall: range(cells.map(c => safeDiv(c.truePositive, c.truePositive + c.falseNegative))),
          f1: range(f1),
          coverage: range(cells.map(c => safeDiv(c.evaluated, c.evaluated + c.gapOnly))),
          runs: cells.length,
        }
      }
    }
  }

  const exposure = {} as ResultsPayload['exposure']
  for (const profile of PROFILES) {
    const e = runs.filter(r => r.corpus === 'A' && r.profile === profile).map(r => r.report.exposure)
    exposure[profile] = {
      mean: range(e.map(x => x.mean)), p95: range(e.map(x => x.p95)), max: range(e.map(x => x.max)),
      resolvableRefs: range(e.map(x => x.resolvableRefs)), linkability: range(e.map(x => x.linkability)),
      quasiLinkability: range(e.map(x => x.quasiLinkability)),
    }
  }

  const frontier: ResultsPayload['frontier'] = []
  try {
    const csv = (await fs.readFile(join(dir, 'frontier.csv'), 'utf8')).trim().split('\n').slice(1)
    for (const line of csv) {
      const [corpus, seed, rate, profile, meanExposure, , , , macroF1] = line.split(',')
      frontier.push({
        corpus: corpus!, seed: Number(seed), rate: Number(rate), profile: profile as Profile,
        meanExposure: Number(meanExposure), macroF1: Number(macroF1),
      })
    }
  }
  catch { /* optional */ }

  let scenarios: ResultsPayload['scenarios'] = []
  try {
    scenarios = JSON.parse(await fs.readFile(join(dir, 'scenarios.json'), 'utf8'))
  }
  catch { /* optional */ }
  let latency = ''
  try {
    latency = await fs.readFile(join(dir, 'latency.txt'), 'utf8')
  }
  catch { /* optional */ }
  let manifestSha256 = ''
  try {
    const { createHash } = await import('node:crypto')
    const manifest = await fs.readFile(resolve(dir, `../../freeze/${date}/MANIFEST.sha256`))
    manifestSha256 = createHash('sha256').update(manifest).digest('hex')
  }
  catch { /* optional */ }

  return {
    date, manifestSha256,
    seeds: [...new Set(runs.map(r => r.seed))].sort((a, b) => a - b),
    rates: [...new Set(runs.map(r => r.rate))].sort((a, b) => a - b),
    rules, grid, agreement, exposure, frontier, scenarios, latency,
  }
})
