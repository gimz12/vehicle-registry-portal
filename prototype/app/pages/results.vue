<script setup lang="ts">
import type { ResultsPayload } from '~~/server/api/results/index.get'

const { data, error } = await useApi<ResultsPayload>('/results')

const profiles = ['P0', 'P1', 'P2', 'P3'] as const
type Profile = typeof profiles[number]

const corpus = ref<'A' | 'B'>('A')
const corpusName: Record<string, string> = {
  A: 'Corpus A, clean conformance data',
  B: 'Corpus B, with frozen noise model',
}

const pct = (v: number) => `${Math.round(v * 100)}`
const rateList = computed(() => (data.value?.rates ?? []).map(r => `${Math.round(r * 100)}%`).join(', '))
const cell = (rule: string, p: Profile) => data.value!.grid[corpus.value]![rule]![p]
type Range = { min: number, max: number }
const fmtRange = (r: Range) => Math.abs(r.max - r.min) < 0.005
  ? pct(r.min)
  : `${pct(r.min)}–${pct(r.max)}`
const fmtNum = (r: Range, d = 1) => Math.abs(r.max - r.min) < 0.05
  ? r.min.toFixed(d)
  : `${r.min.toFixed(d)}–${r.max.toFixed(d)}`

const classWord: Record<string, string> = {
  'available': 'Available',
  'partial': 'Partial',
  'suppressed': 'Suppressed',
  'unstable': 'Unstable',
  '?': 'No prediction',
}
const classFill: Record<string, string> = {
  available: 'bg-[color:var(--viz-good)]/15',
  partial: 'bg-[color:var(--viz-warning)]/25',
  suppressed: 'bg-[color:var(--viz-neutral)]/35',
  unstable: 'bg-alert-bg',
}

// Frontier: exposure (x) against macro F1 (y), one point per profile per run.
const frontierPoints = computed(() => {
  if (!data.value)
    return []
  const byKey = new Map<string, { corpus: string, profile: Profile, xs: number[], ys: number[] }>()
  for (const row of data.value.frontier) {
    const key = `${row.corpus}-${row.profile}`
    const entry = byKey.get(key) ?? { corpus: row.corpus, profile: row.profile, xs: [], ys: [] }
    entry.xs.push(row.meanExposure)
    entry.ys.push(row.macroF1)
    byKey.set(key, entry)
  }
  return [...byKey.values()].map(e => ({
    ...e,
    x: e.xs.reduce((a, b) => a + b, 0) / e.xs.length,
    y: e.ys.reduce((a, b) => a + b, 0) / e.ys.length,
    yMin: Math.min(...e.ys),
    yMax: Math.max(...e.ys),
  }))
})
const fw = 560
const fh = 300
const fm = { l: 44, r: 28, t: 16, b: 36 }
const xMax = computed(() => Math.max(16, ...frontierPoints.value.map(p => p.x)) * 1.05)
const fx = (v: number) => fm.l + (v / xMax.value) * (fw - fm.l - fm.r)
const fy = (v: number) => fm.t + (1 - v) * (fh - fm.t - fm.b)
const linePoints = (c: string) => frontierPoints.value
  .filter(p => p.corpus === c)
  .sort((a, b) => a.x - b.x)
  .map(p => `${fx(p.x)},${fy(p.y)}`)
  .join(' ')

// Cost of ambiguity: P1 precision and recall, Corpus A beside Corpus B.
const ambiguity = computed(() => {
  if (!data.value || !data.value.grid.B)
    return []
  return data.value.rules
    .filter(rule => data.value!.grid.A![rule]!.P1.measured !== 'suppressed')
    .map(rule => ({
      rule,
      a: data.value!.grid.A![rule]!.P1,
      b: data.value!.grid.B![rule]!.P1,
    }))
})
const outcomeChip: Record<string, string> = {
  DETECTED: 'chip-pass', PASS: 'chip-pass', MISSED: 'chip-alert', FALSE_POSITIVE: 'chip-alert',
  GAP: 'chip-warn', SUPPRESSED: 'chip-supp',
}
const outcomeWord: Record<string, string> = {
  DETECTED: 'Detected',
  PASS: 'Passed',
  MISSED: 'Missed',
  FALSE_POSITIVE: 'False positive',
  GAP: 'Gap',
  SUPPRESSED: 'Withheld',
}
</script>

<template>
  <article class="space-y-12">
    <header class="border-b border-line pb-5">
      <h1 class="text-2xl font-semibold sm:text-3xl">
        What the experiment found
      </h1>
      <p
        v-if="data"
        class="mt-2 max-w-3xl text-muted"
      >
        Held-out seeds {{ data.seeds.join(', ') }} at fraud rates {{ rateList }},
        1,000 vehicles per run, evaluated under all four linkage profiles. Inputs were frozen on {{ data.date }};
        manifest <span class="font-mono text-sm">{{ data.manifestSha256.slice(0, 16) }}</span>.
      </p>
    </header>

    <div
      v-if="error"
      class="rounded-md border border-alert bg-alert-bg p-6"
      role="alert"
    >
      <h2 class="font-semibold text-alert">
        No results to show
      </h2>
      <p class="mt-2 text-sm">
        {{ error.statusMessage || error.message }}
      </p>
    </div>

    <template v-else-if="data">
      <!-- 1. Prediction grid -->
      <section aria-labelledby="h-grid">
        <div class="flex flex-wrap items-end justify-between gap-4">
          <div>
            <h2
              id="h-grid"
              class="text-xl font-semibold"
            >
              Predicted against measured, rule by rule
            </h2>
            <p class="help mt-1 max-w-2xl">
              Each cell is one fraud rule under one profile. The fill is the measured decidability class;
              the small mark is what the frozen table predicted. Precision and recall are ranges across the nine runs.
            </p>
          </div>
          <div
            class="flex rounded-sm border border-line-strong text-sm"
            role="group"
            aria-label="Corpus"
          >
            <button
              v-for="c in (['A', 'B'] as const)"
              :key="c"
              type="button"
              class="px-3 py-1.5"
              :class="corpus === c ? 'bg-accent text-accent-ink' : 'bg-panel text-ink hover:text-accent'"
              :aria-pressed="corpus === c"
              @click="corpus = c"
            >
              Corpus {{ c }}
            </button>
          </div>
        </div>
        <p class="mt-3 text-sm">
          <span class="font-semibold">{{ corpusName[corpus] }}.</span>
          {{ data.agreement[corpus]?.agree }} of {{ data.agreement[corpus]?.total }} cells agree with the prediction.
        </p>
        <div class="mt-4 overflow-x-auto">
          <table class="w-full border-collapse text-sm">
            <thead>
              <tr class="text-left text-muted">
                <th class="py-2 pr-3 font-medium">
                  Rule
                </th>
                <th
                  v-for="p in profiles"
                  :key="p"
                  class="py-2 pr-2 font-medium"
                >
                  {{ p }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="rule in data.rules"
                :key="rule"
                class="border-t border-line"
              >
                <th class="py-1.5 pr-3 text-left font-mono font-medium">
                  {{ rule }}
                </th>
                <td
                  v-for="p in profiles"
                  :key="p"
                  class="py-1.5 pr-2"
                >
                  <div
                    class="rounded-sm px-2.5 py-2"
                    :class="classFill[cell(rule, p).measured]"
                    :title="`${rule} at ${p}: measured ${cell(rule, p).measured}, predicted ${cell(rule, p).predicted}`"
                  >
                    <div class="flex items-center justify-between gap-2">
                      <span class="font-medium">{{ classWord[cell(rule, p).measured] }}</span>
                      <span
                        class="text-xs"
                        :class="cell(rule, p).agree ? 'text-muted' : 'font-semibold text-alert'"
                      >
                        {{ cell(rule, p).agree ? 'as predicted' : `predicted ${classWord[cell(rule, p).predicted]}` }}
                      </span>
                    </div>
                    <div
                      v-if="cell(rule, p).measured !== 'suppressed'"
                      class="mt-0.5 text-xs text-muted tabular-nums"
                    >
                      P {{ fmtRange(cell(rule, p).precision) }}, R {{ fmtRange(cell(rule, p).recall) }}
                    </div>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- 2. Frontier -->
      <section
        aria-labelledby="h-frontier"
        class="grid gap-8 lg:grid-cols-[1fr_20rem]"
      >
        <div>
          <h2
            id="h-frontier"
            class="text-xl font-semibold"
          >
            Privacy against detection
          </h2>
          <p class="help mt-1 max-w-2xl">
            Records reachable from one vehicle reference on the horizontal axis,
            mean F1 over decidable rules on the vertical. One point per profile, averaged over the nine runs;
            a bar shows the F1 range. Left is more private; up is more detection.
          </p>
          <figure class="mt-4">
            <svg
              :viewBox="`0 0 ${fw} ${fh}`"
              class="w-full max-w-2xl"
              role="img"
              aria-labelledby="frontier-title"
            >
              <title id="frontier-title">
                Exposure versus detection for profiles P0 to P3 on Corpus A and Corpus B
              </title>
              <g
                v-for="t in [0, 0.25, 0.5, 0.75, 1]"
                :key="t"
              >
                <line
                  :x1="fm.l"
                  :x2="fw - fm.r"
                  :y1="fy(t)"
                  :y2="fy(t)"
                  stroke="var(--viz-grid)"
                  stroke-width="1"
                />
                <text
                  :x="fm.l - 8"
                  :y="fy(t) + 4"
                  text-anchor="end"
                  class="fill-[color:var(--muted)] text-[11px] tabular-nums"
                >{{ t.toFixed(2) }}</text>
              </g>
              <line
                :x1="fm.l"
                :x2="fw - fm.r"
                :y1="fy(0)"
                :y2="fy(0)"
                stroke="var(--viz-axis)"
                stroke-width="1"
              />
              <g
                v-for="t in [0, 4, 8, 12, 16]"
                :key="'x' + t"
              >
                <text
                  :x="fx(t)"
                  :y="fh - fm.b + 18"
                  text-anchor="middle"
                  class="fill-[color:var(--muted)] text-[11px] tabular-nums"
                >{{ t }}</text>
              </g>
              <text
                :x="(fm.l + fw - fm.r) / 2"
                :y="fh - 4"
                text-anchor="middle"
                class="fill-[color:var(--muted)] text-[11px]"
              >records reachable from one vehicle reference</text>

              <g
                v-for="c in ['A', 'B']"
                :key="c"
              >
                <polyline
                  :points="linePoints(c)"
                  fill="none"
                  :stroke="c === 'A' ? 'var(--series-1)' : 'var(--series-2)'"
                  stroke-width="2"
                  stroke-linejoin="round"
                  stroke-linecap="round"
                  :stroke-dasharray="c === 'B' ? '4 4' : undefined"
                />
              </g>
              <g
                v-for="p in frontierPoints"
                :key="p.corpus + p.profile"
              >
                <line
                  :x1="fx(p.x)"
                  :x2="fx(p.x)"
                  :y1="fy(p.yMin)"
                  :y2="fy(p.yMax)"
                  :stroke="p.corpus === 'A' ? 'var(--series-1)' : 'var(--series-2)'"
                  stroke-width="2"
                  stroke-linecap="round"
                />
                <circle
                  :cx="fx(p.x)"
                  :cy="fy(p.y)"
                  r="5"
                  :fill="p.corpus === 'A' ? 'var(--series-1)' : 'var(--series-2)'"
                  stroke="var(--panel)"
                  stroke-width="2"
                >
                  <title>
                    Corpus {{ p.corpus }} {{ p.profile }}: {{ p.x.toFixed(1) }} records,
                    F1 {{ p.y.toFixed(2) }} ({{ p.yMin.toFixed(2) }}–{{ p.yMax.toFixed(2) }})
                  </title>
                </circle>
                <text
                  v-if="p.corpus === 'A'"
                  :x="p.x > xMax * 0.8 ? fx(p.x) - 9 : fx(p.x) + 9"
                  :y="fy(p.y) + (p.profile === 'P2' ? 16 : -8)"
                  :text-anchor="p.x > xMax * 0.8 ? 'end' : 'start'"
                  class="fill-[color:var(--ink)] text-[11px] font-medium"
                >{{ p.profile }}</text>
              </g>
            </svg>
            <figcaption class="mt-2 flex flex-wrap gap-5 text-sm text-muted">
              <span class="flex items-center gap-2">
                <span class="inline-block h-0.5 w-5 bg-[color:var(--series-1)]" />Corpus A, clean
              </span>
              <span class="flex items-center gap-2">
                <span class="inline-block w-5 border-t-2 border-dashed border-[color:var(--series-2)]" />Corpus B, noisy
              </span>
            </figcaption>
          </figure>
        </div>
        <dl class="surface divide-y divide-line rounded-md text-sm">
          <div
            v-for="p in profiles"
            :key="p"
            class="p-3"
          >
            <dt class="font-semibold">
              {{ p }}
            </dt>
            <dd class="mt-1 grid grid-cols-2 gap-x-3 gap-y-0.5 text-muted tabular-nums">
              <span>Records reachable</span><span class="text-ink">{{ fmtNum(data.exposure[p].mean) }}</span>
              <span>95th percentile</span><span class="text-ink">{{ fmtNum(data.exposure[p].p95, 0) }}</span>
              <span>Resolvable refs</span><span class="text-ink">{{ fmtNum(data.exposure[p].resolvableRefs, 0) }}</span>
              <span>Owner linkability</span><span class="text-ink">{{ fmtNum(data.exposure[p].linkability, 2) }}</span>
              <span>Quasi-identifier</span>
              <span class="text-ink">{{ fmtNum(data.exposure[p].quasiLinkability, 2) }}</span>
            </dd>
          </div>
        </dl>
      </section>

      <!-- 3. Cost of ambiguity -->
      <section
        v-if="ambiguity.length"
        aria-labelledby="h-noise"
      >
        <h2
          id="h-noise"
          class="text-xl font-semibold"
        >
          The cost of ambiguity
        </h2>
        <p class="help mt-1 max-w-2xl">
          At profile P1, precision and recall on clean data beside the same rule on noisy data.
          Accuracy moves; no rule changes class, which is what hypothesis H4 predicted.
        </p>
        <div class="mt-4 grid gap-x-8 gap-y-3 sm:grid-cols-2">
          <div
            v-for="row in ambiguity"
            :key="row.rule"
            class="grid grid-cols-[3rem_1fr] items-center gap-3 text-sm"
          >
            <span class="font-mono font-medium">{{ row.rule }}</span>
            <div class="space-y-1">
              <div
                v-for="m in (['precision', 'recall'] as const)"
                :key="m"
                class="grid grid-cols-[3.5rem_1fr_6rem] items-center gap-2"
              >
                <span class="text-xs text-muted">{{ m === 'precision' ? 'precision' : 'recall' }}</span>
                <div class="relative h-4">
                  <div
                    class="absolute left-0 top-0 h-[7px] rounded-r-sm bg-[color:var(--series-1)]"
                    :style="{ width: `${row.a[m].min * 100}%` }"
                    :title="`Corpus A ${m} ${fmtRange(row.a[m])}%`"
                  />
                  <div
                    class="absolute left-0 top-[9px] h-[7px] rounded-r-sm bg-[color:var(--series-2)]"
                    :style="{ width: `${row.b[m].min * 100}%` }"
                    :title="`Corpus B ${m} ${fmtRange(row.b[m])}%`"
                  />
                </div>
                <span class="text-xs text-muted tabular-nums">
                  {{ fmtRange(row.a[m]) }} vs {{ fmtRange(row.b[m]) }}
                </span>
              </div>
            </div>
          </div>
        </div>
        <p class="mt-3 flex flex-wrap gap-5 text-sm text-muted">
          <span class="flex items-center gap-2">
            <span class="inline-block size-2.5 rounded-sm bg-[color:var(--series-1)]" />Corpus A, clean
          </span>
          <span class="flex items-center gap-2">
            <span class="inline-block size-2.5 rounded-sm bg-[color:var(--series-2)]" />
            Corpus B, noisy (lowest value across runs)
          </span>
        </p>
      </section>

      <!-- 4. Scenarios -->
      <section
        v-if="data.scenarios.length"
        aria-labelledby="h-scenarios"
      >
        <h2
          id="h-scenarios"
          class="text-xl font-semibold"
        >
          Named cases under each profile
        </h2>
        <p class="help mt-1 max-w-2xl">
          Publicly reported and practitioner-described fraud patterns, replayed through the registry.
          Two rows are deliberate: a documented false positive and a documented evidence gap.
        </p>
        <div class="mt-4 overflow-x-auto">
          <table class="w-full border-collapse text-sm">
            <thead>
              <tr class="text-left text-muted">
                <th class="py-2 pr-3 font-medium">
                  Case
                </th>
                <th class="py-2 pr-3 font-medium">
                  Rule
                </th>
                <th
                  v-for="p in profiles"
                  :key="p"
                  class="py-2 pr-3 font-medium"
                >
                  {{ p }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="s in data.scenarios"
                :key="s.id + s.rule"
                class="border-t border-line"
              >
                <td class="py-2 pr-3">
                  <span class="font-mono text-xs text-muted">{{ s.id.slice(0, 3) }}</span>
                  {{ s.title }}
                  <span class="block text-xs text-muted">{{ s.source }}</span>
                </td>
                <td class="py-2 pr-3 font-mono">
                  {{ s.rule }}
                </td>
                <td
                  v-for="p in profiles"
                  :key="p"
                  class="py-2 pr-3"
                >
                  <span :class="outcomeChip[s.outcome[p]] ?? 'chip-supp'">
                    {{ outcomeWord[s.outcome[p]] ?? s.outcome[p] }}
                  </span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- 5. Latency -->
      <section
        v-if="data.latency"
        aria-labelledby="h-latency"
      >
        <h2
          id="h-latency"
          class="text-xl font-semibold"
        >
          Laboratory latency
        </h2>
        <p class="help mt-1 max-w-2xl">
          Feasibility figures, not a deciding criterion.
          In-process benchmarks for the chaincode service and the signed-log baseline.
        </p>
        <pre
          class="surface mt-4 overflow-x-auto rounded-md p-4 font-mono text-xs leading-relaxed text-muted"
        >{{ data.latency.trim() }}</pre>
      </section>
    </template>
  </article>
</template>
