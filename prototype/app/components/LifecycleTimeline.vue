<script setup lang="ts">
import type { LifecycleEvent, VehicleHistory } from '~~/shared/disclosure'
import { evidenceLabel } from '~~/shared/disclosure'

const props = defineProps<{ history: VehicleHistory }>()

const flags = computed(() => [
  props.history.stolen ? 'flagged stolen' : '',
  props.history.writtenOff ? 'written off' : '',
  props.history.retired ? 'retired' : '',
].filter(Boolean))

const typeName: Record<string, string> = {
  IMPORT: 'Import cleared',
  REGISTRATION: 'First registration',
  TRANSFER: 'Ownership transfer',
  MILEAGE: 'Odometer reading',
  EMISSION: 'Emission test',
  ACCIDENT: 'Accident report',
  CLAIM: 'Insurance claim',
  THEFT_REPORT: 'Theft report recorded',
  OWNER_CONFIRMED: 'Owner confirmed by the DMT',
  THEFT_FLAG: 'Flagged stolen',
  THEFT_CLEAR: 'Theft report cleared',
  ENCUMBRANCE_SET: 'Finance recorded',
  ENCUMBRANCE_RELEASED: 'Finance released',
}

function tierClass(tier: number): string {
  if (tier === 2)
    return 'chip-pass'
  if (tier === 1)
    return 'chip-warn'
  return 'chip-supp'
}

function describe(e: LifecycleEvent): string {
  switch (e.type) {
    case 'MILEAGE': return `${e.odometer.toLocaleString()} km`
    case 'CLAIM':
      if (e.redacted)
        return 'Incident reference withheld by this view'
      if (!e.evidenceAnchor)
        return 'No evidence reference at source'
      return `Cites ${e.evidenceKind.toLowerCase().replace('_', ' ')} ${e.evidenceAnchor.slice(0, 40)}`
    case 'ACCIDENT': return `Report ${e.evidenceAnchor.slice(0, 40)}`
    default: return ''
  }
}
</script>

<template>
  <div class="space-y-4">
    <MileageChart :events="history.events" />
    <div
      v-if="flags.length"
      class="mb-3 flex flex-wrap gap-2"
    >
      <span
        v-for="flag in flags"
        :key="flag"
        class="chip-alert"
      >{{ flag }}</span>
    </div>
    <ol class="relative border-l-2 border-line-strong pl-6">
      <li
        v-for="e in history.events"
        :key="e.seq"
        class="relative pb-5 last:pb-0"
        :data-redacted="e.redacted || undefined"
      >
        <span
          aria-hidden="true"
          class="absolute -left-[31px] top-1.5 size-3 rounded-full border-2 border-paper"
          :class="e.redacted ? 'bg-supp' : 'bg-accent'"
        />
        <div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
          <p class="font-medium">
            {{ typeName[e.type] ?? e.type }}
            <span class="font-normal text-muted"> by {{ e.agency }}</span>
          </p>
          <p class="text-sm text-muted tabular-nums">
            day {{ e.day }}
          </p>
        </div>
        <p
          v-if="describe(e)"
          class="text-sm"
          :class="e.redacted ? 'text-muted' : ''"
        >
          {{ describe(e) }}
        </p>
        <p class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted">
          <span
            :class="tierClass(e.tier)"
            :title="evidenceLabel(e.tier)"
          >E{{ e.tier }}</span>
          <span v-if="e.insurerId">{{ e.insurerId }}</span>
          <span class="font-mono">record {{ e.recordId.slice(0, 24) }}</span>
          <span class="font-mono">owner {{ e.pseudonym ? e.pseudonym.slice(0, 10) + '…' : 'not shown' }}</span>
        </p>
      </li>
    </ol>
  </div>
</template>
