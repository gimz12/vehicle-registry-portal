<script setup lang="ts">
/** The five record-linkage capabilities and which the current profile exposes; withheld links are struck through. */
const props = defineProps<{ profile: 'P0' | 'P1' | 'P2' | 'P3' }>()

interface Capability {
  id: string
  name: string
  meaning: string
  requiredBy: string
}

const capabilities: Capability[] = [
  { id: 'RECORD', name: 'Record', meaning: 'read one record on its own', requiredBy: 'claim evidence' },
  { id: 'INDEX', name: 'Index', meaning: 'match chassis and engine indices', requiredBy: 'identity, provenance' },
  { id: 'ASSET', name: 'Vehicle', meaning: 'group this vehicle’s history', requiredBy: 'odometer, theft, claims window' },
  { id: 'INCIDENT', name: 'Incident', meaning: 'join a claim to the accident it cites', requiredBy: 'duplicate anchor' },
  { id: 'OWNER', name: 'Owner', meaning: 'group the owner’s other vehicles', requiredBy: 'serial fraud' },
]

const exposedCount: Record<string, number> = { P0: 5, P1: 4, P2: 3, P3: 2 }
const exposed = computed(() => capabilities.map((c, i) => ({ ...c, on: i < exposedCount[props.profile]! })))
const withheld = computed(() => exposed.value.filter(c => !c.on).map(c => c.name.toLowerCase()))
</script>

<template>
  <div
    class="surface rounded-md p-4"
    data-testid="capability-ladder"
  >
    <div class="flex items-baseline justify-between gap-3">
      <h2 class="text-sm font-semibold">
        Record links this view may follow
      </h2>
      <span class="chip-supp">{{ profile }}</span>
    </div>
    <ol class="mt-3 space-y-1.5">
      <li
        v-for="c in exposed"
        :key="c.id"
        class="flex items-start gap-3 text-sm"
        :aria-label="`${c.name}: ${c.on ? 'exposed' : 'withheld'}`"
      >
        <span
          aria-hidden="true"
          class="mt-1.5 size-2.5 shrink-0 rounded-full border"
          :class="c.on ? 'border-accent bg-accent' : 'border-line-strong bg-transparent'"
        />
        <span class="min-w-0 leading-snug">
          <span
            class="block font-medium"
            :class="c.on ? '' : 'text-muted line-through decoration-line-strong'"
          >{{ c.name }}</span>
          <span class="block text-xs text-muted">{{ c.meaning }}</span>
          <span
            v-if="!c.on"
            class="block text-xs text-muted"
          >Withheld, so {{ c.requiredBy }} checks are not performed.</span>
        </span>
      </li>
    </ol>
    <p
      v-if="withheld.length"
      class="help mt-3 border-t border-line pt-3"
    >
      Fewer links means less about the owner can be learned from this report, and fewer frauds can be checked.
    </p>
    <p
      v-else
      class="help mt-3 border-t border-line pt-3"
    >
      Every link is exposed. This view is available only under an approved case.
    </p>
  </div>
</template>
