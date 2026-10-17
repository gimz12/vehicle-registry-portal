<script setup lang="ts">
import type { DisclosureFinding, Purpose, VehicleDescriptionView, VehicleDisclosure, VehicleHistory } from '~~/shared/disclosure'
import { HISTORY_PURPOSES, PURPOSES, evidenceLabel, parsePurpose } from '~~/shared/disclosure'

const route = useRoute()
const router = useRouter()
const vid = computed(() => String((route.params as { vid?: string }).vid ?? ''))
const purpose = computed<Purpose>(() => parsePurpose((route.query as { as?: string }).as) ?? 'BUYER')
const caseId = computed(() => String((route.query as { case?: string }).case ?? ''))
const caseDraft = ref(caseId.value)

const purposeLabel: Record<Purpose, { name: string, prepared: string }> = {
  BUYER: { name: 'Buyer', prepared: 'a buyer' },
  DMT: { name: 'DMT officer', prepared: 'the DMT' },
  INSURER: { name: 'Insurer', prepared: 'an insurer' },
  POLICE: { name: 'Police', prepared: 'the Police' },
}

function setPurpose(next: Purpose) {
  router.replace({ query: { ...route.query, as: next, case: next === 'POLICE' && caseDraft.value ? caseDraft.value : undefined } })
}
const purposeModel = computed<Purpose>({ get: () => purpose.value, set: setPurpose })
function applyCase() {
  router.replace({ query: { ...route.query, as: 'POLICE', case: caseDraft.value || undefined } })
}

const disclosureUri = computed(() => {
  const params = new URLSearchParams({ as: purpose.value })
  if (purpose.value === 'POLICE' && caseId.value)
    params.set('case', caseId.value)
  return `/vehicle/${encodeURIComponent(vid.value)}?${params}`
})
const { data, status, error, refresh } = await useApi<VehicleDisclosure>(disclosureUri)

const showHistory = computed(() => HISTORY_PURPOSES.includes(purpose.value))
const historyUri = computed(() => {
  const params = new URLSearchParams({ as: purpose.value })
  if (caseId.value)
    params.set('case', caseId.value)
  return `/vehicle/${encodeURIComponent(vid.value)}/history?${params}`
})
// Keyed on the URL so every purpose switch refetches; buyers and insurers
// skip the call instead of drawing a 403.
const { data: history, error: historyError } = await useAsyncData(
  () => historyUri.value,
  () => showHistory.value ? $api<VehicleHistory>(historyUri.value) : Promise.resolve(null),
)

// The vehicle's description (never its plate) and the buyer's one-bit chassis check.
// A 404 shows nothing; any other failure, such as the rate limit, is shown.
const descriptionProblem = ref('')
const { data: description } = await useAsyncData(
  () => `description:${vid.value}`,
  () => $api<VehicleDescriptionView>(`/vehicle/${encodeURIComponent(vid.value)}/description`).catch((e) => {
    const err = e as { statusCode?: number, data?: { statusMessage?: string } }
    descriptionProblem.value = err.statusCode === 404 ? '' : `What this vehicle is could not be shown: ${err.data?.statusMessage ?? 'the DMT did not answer'}`
    return null
  }),
)
const descriptionLine = computed(() => {
  const d = description.value
  if (!d)
    return ''
  const fuel = d.fuel.charAt(0) + d.fuel.slice(1).toLowerCase()
  const colour = d.colour === 'not recorded' ? '' : d.colour
  return [d.make, d.model, String(d.year), colour, d.engineCc ? `${d.engineCc} cc` : '', fuel].filter(Boolean).join(' · ')
})
const matchChassis = ref('')
const matchBusy = ref(false)
const matchResult = ref<'' | 'yes' | 'no' | 'error'>('')
const matchError = ref('')
async function checkChassis() {
  matchBusy.value = true
  matchResult.value = ''
  matchError.value = ''
  try {
    const r = await $api<{ match: boolean }>(`/vehicle/${encodeURIComponent(vid.value)}/match`, {
      method: 'POST', body: { chassis: matchChassis.value },
    })
    matchResult.value = r.match ? 'yes' : 'no'
  }
  catch (err) {
    const e = err as { data?: { statusMessage?: string }, message?: string }
    matchResult.value = 'error'
    matchError.value = e.data?.statusMessage || e.message || 'The check could not be made'
  }
  finally {
    matchBusy.value = false
  }
}
// The red banner is for facts that should stop or slow a sale; descCommitted is not one.
const statusShown = computed(() => {
  const st = data.value?.status
  return !!st && !!(st.stolen || st.writtenOff || st.encumbered || st.retired
    || st.theftReportPending || st.theftCleared)
})

const total = computed(() => data.value
  ? data.value.passed.length + data.value.failed.length + data.value.notPerformed.length
  : 0)
const share = (n: number) => total.value ? `${(n / total.value) * 100}%` : '0%'
const summaryLabel = computed(() => {
  if (!data.value)
    return ''
  const d = data.value
  return `${d.failed.length} found something, ${d.passed.length} passed, ${d.notPerformed.length} not performed`
})

const verdict = computed(() => {
  if (!data.value)
    return ''
  const alerts = data.value.failed.filter(f => f.strength === 'ALERT').length
  const warnings = data.value.failed.length - alerts
  if (alerts)
    return `${alerts} alert${alerts > 1 ? 's' : ''}${warnings ? ` and ${warnings} warning${warnings > 1 ? 's' : ''}` : ''} on this vehicle`
  if (warnings)
    return `${warnings} warning${warnings > 1 ? 's' : ''}, no alerts`
  return 'No issue found in the checks that could be performed'
})

const groups = computed(() => data.value
  ? [
      {
        key: 'failed',
        title: 'Checks that found something',
        intro: 'A reportable condition in the records this view may read.',
        findings: data.value.failed,
      },
      { key: 'passed', title: 'Checks passed', intro: 'Enough evidence, nothing found.', findings: data.value.passed },
      {
        key: 'notPerformed',
        title: 'Checks not performed',
        intro: 'Withheld by the privacy profile, or without enough evidence. Not a clean result.',
        findings: data.value.notPerformed,
      },
    ]
  : [])

function chipClass(finding: DisclosureFinding): string {
  if (finding.status === 'FLAGGED')
    return finding.strength === 'ALERT' ? 'chip-alert' : 'chip-warn'
  if (finding.status === 'PASS')
    return 'chip-pass'
  return 'chip-supp'
}
function statusText(finding: DisclosureFinding): string {
  if (finding.status === 'FLAGGED')
    return finding.strength === 'ALERT' ? 'Alert' : 'Warning'
  if (finding.status === 'PASS')
    return 'Passed'
  if (finding.status === 'SUPPRESSED')
    return 'Withheld'
  return finding.structural ? 'Link removed' : 'No evidence'
}
</script>

<template>
  <article class="space-y-8">
    <header class="flex flex-wrap items-end justify-between gap-4 border-b border-line pb-5">
      <div class="min-w-0">
        <NuxtLink
          to="/"
          class="text-sm text-accent hover:underline"
        >
          Check another vehicle
        </NuxtLink>
        <h1 class="mt-3 text-2xl font-semibold sm:text-3xl">
          Vehicle disclosure
        </h1>
        <p class="handle mt-1">
          {{ vid }}
        </p>
        <p
          v-if="descriptionLine"
          class="mt-2 font-medium"
          data-testid="vehicle-description"
        >
          {{ descriptionLine }}
          <span class="ml-1 text-sm font-normal text-muted">
            {{ description?.committed ? '· as committed by Customs at import'
              : description?.mismatch ? '· does not match what Customs committed at import: do not rely on it'
                : '· as recorded by the DMT' }}
          </span>
        </p>
        <p
          v-else-if="descriptionProblem"
          class="mt-2 text-sm text-muted"
          role="status"
        >
          {{ descriptionProblem }}
        </p>
        <form
          class="mt-3 flex flex-col gap-2 sm:flex-row sm:items-center"
          data-testid="chassis-match"
          @submit.prevent="checkChassis"
        >
          <label
            for="matchChassis"
            class="text-sm text-muted"
          >Chassis number stamped on the car</label>
          <input
            id="matchChassis"
            v-model="matchChassis"
            class="field sm:max-w-xs"
            placeholder="read it from the car, not the papers"
            spellcheck="false"
          >
          <button
            type="submit"
            class="btn-secondary"
            :disabled="matchBusy || !matchChassis.trim()"
          >
            Check
          </button>
        </form>
        <p
          v-if="matchResult === 'yes'"
          class="mt-1 text-sm font-medium text-pass"
          role="status"
        >
          Matches this record.
        </p>
        <p
          v-else-if="matchResult === 'no'"
          class="mt-1 text-sm font-medium text-alert"
          role="alert"
        >
          Does not match: this record is for a different car. Do not rely on it.
        </p>
        <p
          v-else-if="matchResult === 'error'"
          class="mt-1 text-sm text-muted"
          role="status"
        >
          {{ matchError }}
        </p>
      </div>
      <div
        v-if="data"
        class="sm:text-right"
      >
        <p class="text-sm text-muted">
          Prepared for {{ purposeLabel[purpose].prepared }}
        </p>
        <p
          class="text-lg font-semibold"
          data-testid="verdict"
        >
          {{ verdict }}
        </p>
      </div>
    </header>

    <div
      v-if="data?.status && statusShown"
      class="rounded-md border border-alert bg-alert-bg p-4"
      role="alert"
      data-testid="status-banner"
    >
      <p class="font-semibold text-alert">
        Register status
      </p>
      <ul class="mt-1 space-y-0.5 text-sm">
        <li v-if="data.status.stolen">
          This vehicle is currently reported stolen by the Police.
          Ownership transfers are refused until the report is cleared.
        </li>
        <li v-if="data.status.writtenOff">
          This vehicle is written off by an insurer. Ownership transfers are refused until the write-off is resolved.
        </li>
        <li v-if="data.status.encumbered">
          A finance company is recorded as this vehicle's absolute owner. The DMT will refuse a transfer until the
          company's release is recorded, so ask the seller for the release letter before paying.
        </li>
        <li v-if="data.status.retired">
          This vehicle is retired from the register and cannot be transferred.
        </li>
        <li v-if="data.status.theftReportPending">
          A theft report on this vehicle is being verified. It is not flagged stolen; ask the seller about it.
        </li>
        <li v-if="data.status.theftCleared">
          This vehicle was reported stolen and the report was later cleared by the Police.
        </li>
      </ul>
    </div>

    <div class="grid gap-8 lg:grid-cols-[18rem_1fr]">
      <aside class="space-y-4 lg:sticky lg:top-6 lg:self-start">
        <div class="surface rounded-md p-4">
          <label
            for="purpose"
            class="label"
          >Who is asking</label>
          <select
            id="purpose"
            v-model="purposeModel"
            class="field"
            data-testid="purpose-select"
          >
            <option
              v-for="p in PURPOSES"
              :key="p"
              :value="p"
            >
              {{ purposeLabel[p].name }}
            </option>
          </select>
          <p
            v-if="data?.policy"
            class="help mt-3 first-letter:uppercase"
            data-testid="policy-reason"
          >
            {{ data.policy.reason }}
          </p>

          <form
            v-if="purpose === 'POLICE'"
            class="mt-4 border-t border-line pt-4"
            @submit.prevent="applyCase"
          >
            <label
              for="case"
              class="label"
            >Approved case</label>
            <div class="flex gap-2">
              <input
                id="case"
                v-model="caseDraft"
                class="field"
                placeholder="CID/2026/0007"
                autocomplete="off"
              >
              <button
                type="submit"
                class="btn-secondary"
              >
                Apply
              </button>
            </div>
            <NuxtLink
              :to="caseId ? `/police/${vid}?case=${encodeURIComponent(caseId)}` : `/police/${vid}`"
              class="mt-3 inline-block text-sm text-accent hover:underline"
            >
              {{ caseId ? 'Back to the case' : 'Request owner-scope access for a case' }}
            </NuxtLink>
          </form>
        </div>

        <CapabilityLadder
          v-if="data"
          :profile="data.profile"
        />
      </aside>

      <div class="min-w-0 space-y-8">
        <div
          v-if="status === 'pending'"
          class="surface rounded-md p-6 text-muted"
          aria-live="polite"
        >
          Reading the consortium ledger
        </div>

        <div
          v-else-if="error"
          class="rounded-md border border-alert bg-alert-bg p-6"
          role="alert"
        >
          <h2 class="font-semibold text-alert">
            This disclosure could not be produced
          </h2>
          <p class="mt-2 text-sm">
            {{ error.statusMessage || error.message }}
          </p>
          <button
            class="btn-secondary mt-4"
            @click="refresh()"
          >
            Try again
          </button>
        </div>

        <template v-else-if="data">
          <section aria-label="Summary of checks">
            <div
              class="flex h-3 overflow-hidden rounded-sm bg-line"
              role="img"
              :aria-label="summaryLabel"
            >
              <span
                class="bg-alert"
                :style="{ width: share(data.failed.length) }"
              />
              <span
                class="bg-pass"
                :style="{ width: share(data.passed.length) }"
              />
              <span
                class="bg-supp"
                :style="{ width: share(data.notPerformed.length) }"
              />
            </div>
            <dl class="mt-3 grid grid-cols-3 gap-4 text-sm">
              <div>
                <dt class="flex items-center gap-2 text-muted">
                  <span class="size-2 rounded-full bg-alert" />Found something
                </dt>
                <dd class="text-2xl font-semibold tabular-nums">
                  {{ data.failed.length }}
                </dd>
              </div>
              <div>
                <dt class="flex items-center gap-2 text-muted">
                  <span class="size-2 rounded-full bg-pass" />Passed
                </dt>
                <dd class="text-2xl font-semibold tabular-nums">
                  {{ data.passed.length }}
                </dd>
              </div>
              <div>
                <dt class="flex items-center gap-2 text-muted">
                  <span class="size-2 rounded-full bg-supp" />Not performed
                </dt>
                <dd class="text-2xl font-semibold tabular-nums">
                  {{ data.notPerformed.length }}
                </dd>
              </div>
            </dl>
          </section>

          <section
            v-for="group in groups"
            :key="group.key"
            :aria-labelledby="`h-${group.key}`"
          >
            <h2
              :id="`h-${group.key}`"
              class="text-lg font-semibold"
            >
              {{ group.title }}
            </h2>
            <p class="help mt-0.5">
              {{ group.intro }}
            </p>
            <p
              v-if="group.findings.length === 0"
              class="mt-3 border-t border-line pt-3 text-sm text-muted"
            >
              None.
            </p>
            <ul
              v-else
              class="mt-3 divide-y divide-line border-y border-line"
            >
              <li
                v-for="finding in group.findings"
                :key="`${finding.ruleId}-${finding.atSeq}-${finding.status}`"
                class="grid gap-x-4 gap-y-1 py-3 sm:grid-cols-[3.25rem_1fr_auto]"
              >
                <span class="font-mono text-sm text-muted">{{ finding.ruleId }}</span>
                <span class="min-w-0">
                  <span class="block font-medium first-letter:uppercase">{{ finding.detail }}</span>
                  <span
                    v-if="finding.reason"
                    class="block text-sm text-muted first-letter:uppercase"
                  >{{ finding.reason }}</span>
                  <span class="block text-xs text-muted">{{ evidenceLabel(finding.tier) }}</span>
                </span>
                <span
                  :class="chipClass(finding)"
                  class="self-start justify-self-start sm:justify-self-end"
                >{{ statusText(finding) }}</span>
              </li>
            </ul>
          </section>

          <section
            v-if="showHistory"
            aria-labelledby="h-history"
          >
            <h2
              id="h-history"
              class="text-lg font-semibold"
            >
              Lifecycle on the ledger
            </h2>
            <p class="help mt-0.5">
              Committed events in ledger order, as this purpose may read them.
              Buyers and insurers receive the disclosure only.
            </p>
            <p
              v-if="historyError"
              class="mt-3 rounded-md border border-alert bg-alert-bg p-4 text-sm text-alert"
            >
              {{ historyError.statusMessage || historyError.message }}
            </p>
            <LifecycleTimeline
              v-else-if="history"
              class="mt-4"
              :history="history"
            />
          </section>

          <p class="help border-t border-line pt-4">
            This report identifies the vehicle by a keyed handle and never names its owner.
            Checks not performed are listed so that silence is not read as a clean record.
          </p>
        </template>
      </div>
    </div>
  </article>
</template>
