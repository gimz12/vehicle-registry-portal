<script setup lang="ts">
import type { LifecycleEvent } from '~~/shared/disclosure'

/** Odometer readings against day; a falling segment (rollback, F1a) is drawn in the alert colour. */
const props = defineProps<{ events: LifecycleEvent[] }>()

const readings = computed(() => props.events.filter(e => e.type === 'MILEAGE').sort((a, b) => a.seq - b.seq))
const w = 520
const h = 160
const m = { l: 56, r: 32, t: 12, b: 28 }
const xMax = computed(() => Math.max(1, ...readings.value.map(r => r.day)))
const yMax = computed(() => Math.max(1, ...readings.value.map(r => r.odometer)) * 1.1)
const x = (d: number) => m.l + (d / xMax.value) * (w - m.l - m.r)
const y = (v: number) => m.t + (1 - v / yMax.value) * (h - m.t - m.b)
const segments = computed(() => readings.value.slice(1).map((r, i) => ({
  from: readings.value[i]!, to: r, rollback: r.odometer < readings.value[i]!.odometer,
})))
const ticks = computed(() => [0, 0.5, 1].map(f => Math.round(yMax.value * f / 10000) * 10000))
const km = (v: number) => `${Math.round(v / 1000)}k`
</script>

<template>
  <figure
    v-if="readings.length >= 2"
    class="surface rounded-md p-3"
  >
    <svg
      :viewBox="`0 0 ${w} ${h}`"
      class="w-full max-w-xl"
      role="img"
      aria-labelledby="mileage-title"
    >
      <title id="mileage-title">Odometer readings over time; a falling segment is a rollback</title>
      <g
        v-for="t in ticks"
        :key="t"
      >
        <line
          :x1="m.l"
          :x2="w - m.r"
          :y1="y(t)"
          :y2="y(t)"
          stroke="var(--viz-grid)"
          stroke-width="1"
        />
        <text
          :x="m.l - 8"
          :y="y(t) + 4"
          text-anchor="end"
          class="fill-[color:var(--muted)] text-[11px] tabular-nums"
        >{{ km(t) }} km</text>
      </g>
      <line
        v-for="s in segments"
        :key="s.to.seq"
        :x1="x(s.from.day)"
        :y1="y(s.from.odometer)"
        :x2="x(s.to.day)"
        :y2="y(s.to.odometer)"
        :stroke="s.rollback ? 'var(--alert)' : 'var(--series-1)'"
        stroke-width="2"
        stroke-linecap="round"
      />
      <g
        v-for="r in readings"
        :key="r.seq"
      >
        <circle
          :cx="x(r.day)"
          :cy="y(r.odometer)"
          r="5"
          fill="var(--series-1)"
          stroke="var(--panel)"
          stroke-width="2"
        >
          <title>Day {{ r.day }}: {{ r.odometer.toLocaleString() }} km, {{ r.agency }} {{ r.attestorId }}</title>
        </circle>
        <text
          :x="x(r.day)"
          :y="h - 8"
          :text-anchor="r.day === xMax ? 'end' : r.day === 0 ? 'start' : 'middle'"
          class="fill-[color:var(--muted)] text-[11px] tabular-nums"
        >day {{ r.day }}</text>
      </g>
    </svg>
    <figcaption class="mt-1 text-xs text-muted">
      <template v-if="segments.some(s => s.rollback)">
        A reading below the previous one is drawn in the alert colour.
      </template>
      <template v-else>
        Readings rise over time; no rollback.
      </template>
    </figcaption>
  </figure>
</template>
