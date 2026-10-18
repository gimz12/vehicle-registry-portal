<script setup lang="ts">
interface DemoVehicle { vid: string, scenario: string, title: string, source: string, vehicle: number }

const vehicleHandle = ref('')
const smokeVehicle = '3333333333333333333333333333333333333333333333333333333333333333'
const { data: demo } = await useApi<DemoVehicle[]>('/demo')

const grouped = computed(() => {
  const out = new Map<string, DemoVehicle[]>()
  for (const v of demo.value ?? [])
    out.set(v.source, [...(out.get(v.source) ?? []), v])
  return [...out.entries()]
})
const sourceName: Record<string, string> = {
  COPA: 'Parliamentary Committee on Public Accounts',
  CIABOC: 'Bribery and Corruption Commission findings',
  DMT: 'Practitioner consultation',
  Design: 'Boundary and negative cases',
}
</script>

<template>
  <div class="space-y-14">
    <section class="grid items-start gap-10 lg:grid-cols-[1fr_20rem]">
      <div class="max-w-xl">
        <h1 class="text-3xl font-semibold sm:text-4xl">
          Check a vehicle before you pay
        </h1>
        <p class="mt-4 text-lg text-muted">
          Enter the handle printed on the seller's registry report. You will see every check the registry made,
          what it found, and every check it could not make for you.
        </p>
        <form
          class="mt-8 flex flex-col gap-2 sm:flex-row"
          @submit.prevent="navigateTo(`/vehicle/${vehicleHandle.toLowerCase()}`)"
        >
          <label
            for="handle"
            class="sr-only"
          >Vehicle handle</label>
          <input
            id="handle"
            v-model="vehicleHandle"
            required
            minlength="64"
            maxlength="64"
            pattern="[A-Fa-f0-9]{64}"
            placeholder="64-character vehicle handle"
            autocomplete="off"
            spellcheck="false"
            class="field font-mono text-sm"
          >
          <button
            type="submit"
            class="btn-primary shrink-0 py-2.5"
          >
            Check vehicle
          </button>
        </form>
        <p class="help mt-3">
          The handle is a keyed digest of the chassis number, printed on the Certificate of Registration.
          Ask the seller for it. The registry offers no lookup by chassis number or number plate; the DMT's
          1919 service describes a vehicle by its plate.
        </p>
        <button
          v-if="!demo?.length"
          class="btn-quiet mt-4 px-0"
          type="button"
          @click="navigateTo(`/vehicle/${smokeVehicle}`)"
        >
          Open the seeded smoke-test vehicle
        </button>
      </div>

      <dl class="surface rounded-md divide-y divide-line text-sm">
        <div class="p-4">
          <dt class="font-semibold">
            What a disclosure shows
          </dt>
          <dd class="mt-1 text-muted">
            Twelve fraud checks across odometer, theft, insurance claims, identity and provenance,
            each marked as found, passed, or not performed.
          </dd>
        </div>
        <div class="p-4">
          <dt class="font-semibold">
            Why some checks are not performed
          </dt>
          <dd class="mt-1 text-muted">
            A buyer's view may not link this vehicle to its owner's other vehicles or to incident files.
            Checks that need those links are declared, not hidden.
          </dd>
        </div>
        <div class="p-4">
          <dt class="font-semibold">
            Who else reads the same ledger
          </dt>
          <dd class="mt-1 text-muted">
            DMT officers, insurers and the Police each receive the view their task needs.
            Owner-level linkage requires an approved case.
          </dd>
        </div>
      </dl>
    </section>

    <section
      v-if="demo?.length"
      aria-labelledby="h-demo"
    >
      <h2
        id="h-demo"
        class="text-xl font-semibold"
      >
        Scenario vehicles
      </h2>
      <p class="help mt-1 max-w-2xl">
        Named fraud patterns encoded as ledger histories.
        Open one, then change who is asking to see the disclosure change with the view.
      </p>
      <div class="mt-5 grid gap-6 md:grid-cols-2">
        <section
          v-for="[source, vehicles] in grouped"
          :key="source"
          :aria-label="sourceName[source] ?? source"
        >
          <h3 class="border-b border-line-strong pb-1 text-sm font-semibold">
            {{ sourceName[source] ?? source }}
          </h3>
          <ul class="divide-y divide-line">
            <li
              v-for="v in vehicles"
              :key="v.vid"
            >
              <NuxtLink
                :to="`/vehicle/${v.vid}`"
                class="flex items-baseline gap-3 py-2.5 text-sm hover:text-accent"
              >
                <span class="font-mono text-xs text-muted">{{ v.scenario.slice(0, 3) }}</span>
                <span>
                  {{ v.title }}
                  <span
                    v-if="v.vehicle > 0"
                    class="text-muted"
                  >, vehicle {{ v.vehicle + 1 }}</span>
                </span>
              </NuxtLink>
            </li>
          </ul>
        </section>
      </div>
    </section>
  </div>
</template>
