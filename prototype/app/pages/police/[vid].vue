<script setup lang="ts">
import type { EscalationInput, PredicateResult, RevealGround, RevealRequest, RevealedIdentity, VehicleDisclosure } from '~~/shared/disclosure'
import { REVEAL_GROUNDS } from '~~/shared/disclosure'

const route = useRoute()
const vid = computed(() => String((route.params as { vid?: string }).vid ?? ''))

// Police view at P1: the live owner-attributed alerts that can ground a request.
const { data: disclosure } = await useApi<VehicleDisclosure>(`/vehicle/${encodeURIComponent(vid.value)}?as=POLICE`)
// F6a/F6b are undecidable at P1; the DMT's owner-scope verdict, if an alert, can ground a request.
const predicateResults = ref<PredicateResult[]>([])
const predicateBusy = ref(false)
const eligible = computed(() => [
  ...(disclosure.value?.failed ?? []).filter(f => f.strength === 'ALERT' && f.subject === 'OWNER'),
  ...predicateResults.value
    .filter(p => p.status === 'FLAGGED' && p.strength === 'ALERT')
    .map(p => ({ ruleId: p.ruleId, atSeq: 0, detail: 'serial pattern confirmed by the DMT owner-scope check' })),
])
function verdict(p: PredicateResult): string {
  if (p.status === 'FLAGGED' && p.strength === 'ALERT')
    return 'pattern found at alert strength: it can ground a request'
  if (p.status === 'FLAGGED')
    return `pattern found, ${p.strength.toLowerCase()} only: it cannot ground a request`
  if (p.status === 'PASS')
    return 'no pattern across the owner\'s vehicles'
  return 'not decidable on the evidence held'
}

const caseId = ref(String((route.query as { case?: string }).case ?? ''))
const ruleId = ref('')
const ground = ref<RevealGround>('FRAUD_INVESTIGATION')
const groundRef = ref('')
const request = ref<RevealRequest | null>(null)
const busy = ref(false)
const problem = ref('')

const groundName: Record<RevealGround, string> = {
  FRAUD_INVESTIGATION: 'Fraud investigation',
  THEFT_INVESTIGATION: 'Theft investigation',
  COURT_ORDER: 'Court order',
  DATA_SUBJECT_CONSENT: 'Consent of the data subject',
}

watch(eligible, (list) => {
  if (!ruleId.value && list[0])
    ruleId.value = list[0].ruleId
}, { immediate: true })

async function checkPredicate(rule: 'F6a' | 'F6b') {
  predicateBusy.value = true
  problem.value = ''
  try {
    const r = await $api<PredicateResult>('/escalation', {
      method: 'POST', body: { action: 'predicate', as: 'DMT', vid: vid.value, ruleId: rule },
    })
    predicateResults.value = [...predicateResults.value.filter(p => p.ruleId !== rule), r]
  }
  catch (error) {
    const e = error as { data?: { statusMessage?: string }, message?: string }
    problem.value = e.data?.statusMessage || e.message || 'The owner-scope check failed'
  }
  finally {
    predicateBusy.value = false
  }
}

// The case card and any refusal sit below step 4; bring the outcome into view.
async function showOutcome() {
  await nextTick()
  document.querySelector(problem.value ? '[data-outcome="problem"]' : '[data-testid="case-state"]')
    ?.scrollIntoView({ behavior: 'smooth', block: 'center' })
}

async function send(input: EscalationInput) {
  busy.value = true
  problem.value = ''
  try {
    request.value = await $api<RevealRequest>('/escalation', { method: 'POST', body: input })
  }
  catch (error) {
    const e = error as { data?: { statusMessage?: string }, message?: string }
    problem.value = e.data?.statusMessage || e.message || 'The request failed'
  }
  finally {
    busy.value = false
    showOutcome()
  }
}

const create = () => send({
  action: 'create', as: 'POLICE', caseId: caseId.value, vid: vid.value,
  ruleId: ruleId.value, ground: ground.value, groundRef: groundRef.value,
})
const approve = (as: 'DMT' | 'POLICE') => send({ action: 'approve', as, caseId: caseId.value })
const rejectReason = ref('')
const reject = (as: 'DMT' | 'POLICE') => send({ action: 'reject', as, caseId: caseId.value, reason: rejectReason.value })

// Step 4: open the off-ledger owner record under the approved case.
const identity = ref<RevealedIdentity | null>(null)
const revealing = ref(false)
async function reveal() {
  revealing.value = true
  problem.value = ''
  try {
    identity.value = await $api<RevealedIdentity>('/reveal', { method: 'POST', body: { caseId: caseId.value, vid: vid.value } })
    if (request.value)
      request.value = { ...request.value, accessLog: identity.value.accessLog }
  }
  catch (error) {
    const e = error as { data?: { statusMessage?: string }, message?: string }
    problem.value = e.data?.statusMessage || e.message || 'The identity could not be revealed'
  }
  finally {
    revealing.value = false
    if (problem.value)
      showOutcome()
  }
}

async function reload() {
  busy.value = true
  problem.value = ''
  try {
    request.value = await $api<RevealRequest>(`/escalation?case=${encodeURIComponent(caseId.value)}`)
  }
  catch (error) {
    const e = error as { data?: { statusMessage?: string }, message?: string }
    problem.value = e.data?.statusMessage || e.message || 'The case could not be read'
  }
  finally {
    busy.value = false
  }
}

// Returning from the owner-scope disclosure with ?case= restores the case.
onMounted(() => {
  if (caseId.value)
    reload()
})

const step = computed(() => {
  if (!request.value)
    return 1
  if (identity.value)
    return 4
  return request.value.status === 'APPROVED' ? 3 : 2
})
const statusName = computed(() => {
  if (!request.value)
    return ''
  return request.value.status === 'APPROVED' ? 'Approved' : request.value.status === 'REJECTED' ? 'Rejected' : 'Pending'
})
const statusChip = computed(() => {
  if (!request.value)
    return ''
  return request.value.status === 'APPROVED' ? 'chip-pass' : request.value.status === 'REJECTED' ? 'chip-alert' : 'chip-warn'
})
</script>

<template>
  <article class="mx-auto max-w-3xl space-y-8">
    <header class="border-b border-line pb-5">
      <NuxtLink
        :to="`/vehicle/${vid}?as=POLICE`"
        class="text-sm text-accent hover:underline"
      >
        Back to the Police disclosure
      </NuxtLink>
      <h1 class="mt-3 text-2xl font-semibold sm:text-3xl">
        Owner-scope access for a case
      </h1>
      <p class="handle mt-1">
        {{ vid }}
      </p>
      <p class="mt-4 max-w-2xl text-muted">
        Linking this vehicle to its owner's other vehicles is never available by default. It is granted for one case
        and one vehicle after the DMT and the Police both approve a request that names a live owner-attributed alert and
        a legal basis issued outside the registry. Every owner-scope read under the case is written to its access log.
      </p>
    </header>

    <ol class="space-y-6">
      <li class="grid gap-4 sm:grid-cols-[2.5rem_1fr]">
        <span
          class="grid size-8 place-items-center rounded-full text-sm font-semibold"
          :class="step >= 1 ? 'bg-accent text-accent-ink' : 'bg-panel-2 text-muted'"
        >1</span>
        <form
          class="surface space-y-4 rounded-md p-5"
          @submit.prevent="create"
        >
          <h2 class="font-semibold">
            Request, as Police
          </h2>
          <p
            v-if="eligible.length === 0 && request"
            class="text-sm text-muted"
          >
            Case {{ request.id }} rests on {{ request.ruleId }}; its approvals and access log are shown below.
          </p>
          <div
            v-else-if="eligible.length === 0"
            class="space-y-3 rounded-sm bg-supp-bg p-4 text-sm"
          >
            <p class="font-medium">
              This vehicle has no alert that could justify owner-level access.
            </p>
            <p class="text-muted">
              A request must rest on an alert the registry has already raised about this vehicle and
              that points at its owner. Warnings do not count. Alerts that qualify:
            </p>
            <ul class="list-inside list-disc text-muted">
              <li>F3c: two claims citing the same police report</li>
              <li>F2: the vehicle is currently flagged stolen</li>
              <li>
                F6b: claims on the owner's vehicles sharing one assessor, once the DMT's owner-scope check below
                confirms it at alert strength
              </li>
            </ul>
            <p class="text-muted">
              F6a, serial odometer rollback, cannot qualify on the live network: emission-centre readings are single
              attestations, so the pattern stays a warning.
            </p>
          </div>
          <div
            v-else
            class="grid gap-4 sm:grid-cols-2"
          >
            <div>
              <label
                for="caseId"
                class="label"
              >Case number</label>
              <input
                id="caseId"
                v-model="caseId"
                required
                pattern="[A-Za-z0-9\/_\-]{1,64}"
                placeholder="CID/2026/0007"
                autocomplete="off"
                class="field"
              >
            </div>
            <div>
              <label
                for="rule"
                class="label"
              >Alert the request rests on</label>
              <select
                id="rule"
                v-model="ruleId"
                required
                class="field"
              >
                <option
                  v-for="f in eligible"
                  :key="f.ruleId + f.atSeq"
                  :value="f.ruleId"
                >
                  {{ f.ruleId }}: {{ f.detail }}
                </option>
              </select>
            </div>
            <div>
              <label
                for="ground"
                class="label"
              >Legal basis</label>
              <select
                id="ground"
                v-model="ground"
                class="field"
              >
                <option
                  v-for="g in REVEAL_GROUNDS"
                  :key="g"
                  :value="g"
                >
                  {{ groundName[g] }}
                </option>
              </select>
            </div>
            <div>
              <label
                for="groundRef"
                class="label"
              >Reference for that basis</label>
              <input
                id="groundRef"
                v-model="groundRef"
                required
                maxlength="120"
                placeholder="Magistrate's Court Colombo B/1234/2026"
                class="field"
              >
            </div>
          </div>
          <div class="space-y-3 rounded-sm border border-line p-4 text-sm">
            <p class="font-medium">
              Serial fraud across the owner's vehicles
            </p>
            <p class="text-muted">
              These checks cannot run at the Police level, because the owner's other vehicles are unlinkable without the
              DMT's owner index. The DMT can run one check over the owner's fleet and return only the verdict. Each
              check is recorded on the ledger.
            </p>
            <div class="flex flex-wrap gap-2">
              <button
                type="button"
                class="btn-secondary"
                :disabled="predicateBusy"
                @click="checkPredicate('F6b')"
              >
                Ask the DMT: shared assessor (F6b)
              </button>
              <button
                type="button"
                class="btn-secondary"
                :disabled="predicateBusy"
                @click="checkPredicate('F6a')"
              >
                Ask the DMT: serial rollback (F6a)
              </button>
            </div>
            <ul
              v-if="predicateResults.length"
              class="space-y-1"
            >
              <li
                v-for="p in predicateResults"
                :key="p.ruleId"
              >
                <span class="font-medium">{{ p.ruleId }}</span>: {{ verdict(p) }}
              </li>
            </ul>
          </div>
          <button
            v-if="eligible.length > 0"
            type="submit"
            :disabled="busy"
            class="btn-primary"
          >
            Submit request
          </button>
        </form>
      </li>

      <li class="grid gap-4 sm:grid-cols-[2.5rem_1fr]">
        <span
          class="grid size-8 place-items-center rounded-full text-sm font-semibold"
          :class="step >= 2 ? 'bg-accent text-accent-ink' : 'bg-panel-2 text-muted'"
        >2</span>
        <div class="surface space-y-3 rounded-md p-5">
          <h2 class="font-semibold">
            Approve, as both agencies
          </h2>
          <p class="help">
            One approval alone leaves the request pending.
            Both are recorded on the ledger with the approving organisation.
          </p>
          <div class="flex flex-wrap gap-2">
            <button
              type="button"
              :disabled="busy || !caseId"
              class="btn-secondary"
              @click="approve('DMT')"
            >
              Approve as DMT
            </button>
            <button
              type="button"
              :disabled="busy || !caseId"
              class="btn-secondary"
              @click="approve('POLICE')"
            >
              Approve as Police
            </button>
            <button
              type="button"
              :disabled="busy || !caseId"
              class="btn-quiet"
              @click="reload"
            >
              Reload case
            </button>
          </div>
          <details class="text-sm">
            <summary class="cursor-pointer text-accent">
              Reject instead
            </summary>
            <div class="mt-3 grid gap-2 sm:grid-cols-[1fr_auto_auto]">
              <input
                v-model="rejectReason"
                class="field"
                maxlength="240"
                placeholder="Reason, recorded on the case (e.g. court order expired)"
                aria-label="Rejection reason"
              >
              <button
                type="button"
                :disabled="busy || !caseId || !rejectReason.trim()"
                class="btn-secondary"
                @click="reject('DMT')"
              >
                Reject as DMT
              </button>
              <button
                type="button"
                :disabled="busy || !caseId || !rejectReason.trim()"
                class="btn-secondary"
                @click="reject('POLICE')"
              >
                Reject as Police
              </button>
            </div>
          </details>
        </div>
      </li>

      <li class="grid gap-4 sm:grid-cols-[2.5rem_1fr]">
        <span
          class="grid size-8 place-items-center rounded-full text-sm font-semibold"
          :class="step >= 3 ? 'bg-accent text-accent-ink' : 'bg-panel-2 text-muted'"
        >3</span>
        <div class="surface space-y-3 rounded-md p-5">
          <h2 class="font-semibold">
            Read the owner-scope disclosure
          </h2>
          <p class="help">
            Available once the case is approved. The read itself is committed to the ledger
            and appended to the case's access log.
          </p>
          <NuxtLink
            v-if="request?.status === 'APPROVED'"
            :to="`/vehicle/${vid}?as=POLICE&case=${encodeURIComponent(request.id)}`"
            class="btn-primary"
          >
            Open disclosure under case {{ request.id }}
          </NuxtLink>
        </div>
      </li>

      <li class="grid gap-4 sm:grid-cols-[2.5rem_1fr]">
        <span
          class="grid size-8 place-items-center rounded-full text-sm font-semibold"
          :class="step >= 4 ? 'bg-accent text-accent-ink' : 'bg-panel-2 text-muted'"
        >4</span>
        <div class="surface space-y-3 rounded-md p-5">
          <h2 class="font-semibold">
            Reveal the owner's identity
          </h2>
          <p class="help">
            The owner record is held off-ledger, encrypted under a key split between the DMT and the Police.
            Revealing checks the record against the anchor on the ledger, combines both custody shares,
            and writes an identity-reveal entry to the case before anything is shown.
          </p>
          <button
            v-if="request?.status === 'APPROVED' && !identity"
            type="button"
            class="btn-primary"
            :disabled="revealing"
            @click="reveal"
          >
            {{ revealing ? 'Opening the sealed record' : 'Reveal owner identity' }}
          </button>
          <div
            v-if="identity"
            class="rounded-md border border-alert bg-alert-bg p-4 text-sm"
            data-testid="revealed-identity"
          >
            <p class="font-semibold text-alert">
              Synthetic record. No real person is described.
            </p>
            <dl class="mt-3 grid gap-x-6 gap-y-2 sm:grid-cols-[8rem_1fr]">
              <dt class="text-muted">
                Name
              </dt>
              <dd>{{ identity.owner.name }}</dd>
              <dt class="text-muted">
                NIC number
              </dt>
              <dd class="font-mono">
                {{ identity.owner.nic }}
              </dd>
              <dt class="text-muted">
                Address
              </dt>
              <dd>{{ identity.owner.address }}</dd>
              <dt class="text-muted">
                Ledger anchor
              </dt>
              <dd class="font-mono text-xs break-all">
                {{ identity.anchor }}
              </dd>
            </dl>
          </div>
        </div>
      </li>
    </ol>

    <p
      v-if="problem"
      class="rounded-md border border-alert bg-alert-bg p-4 text-sm text-alert"
      role="alert"
      data-outcome="problem"
    >
      {{ problem }}
    </p>

    <section
      v-if="request"
      class="surface rounded-md p-5 text-sm"
      data-testid="case-state"
      aria-live="polite"
    >
      <div class="flex flex-wrap items-baseline justify-between gap-2">
        <h2 class="font-semibold">
          Case {{ request.id }}
        </h2>
        <span :class="statusChip">{{ statusName }}</span>
      </div>
      <dl class="mt-3 grid gap-x-6 gap-y-2 sm:grid-cols-2">
        <div>
          <dt class="text-muted">
            Grounding alert
          </dt>
          <dd>{{ request.ruleId }}</dd>
        </div>
        <div>
          <dt class="text-muted">
            Legal basis
          </dt>
          <dd>{{ groundName[request.groundCode as RevealGround] ?? request.groundCode }}, {{ request.groundRef }}</dd>
        </div>
        <div>
          <dt class="text-muted">
            Requested by
          </dt>
          <dd>{{ request.requestedBy }}</dd>
        </div>
        <div>
          <dt class="text-muted">
            Approvals
          </dt>
          <dd>DMT {{ request.dmtApproved ? 'yes' : 'no' }}, Police {{ request.policeApproved ? 'yes' : 'no' }}</dd>
        </div>
        <div v-if="request.fleetSize">
          <dt class="text-muted">
            Owner's vehicles in scope
          </dt>
          <dd>{{ request.fleetSize }}, fixed by the DMT at approval</dd>
        </div>
      </dl>
      <p
        v-if="request.reason"
        class="mt-2 text-muted"
      >
        {{ request.reason }}
      </p>
      <div
        v-if="request.accessLog?.length"
        class="mt-3 border-t border-line pt-3"
      >
        <p class="font-medium">
          Access log
        </p>
        <ul class="mt-1 break-all font-mono text-xs text-muted">
          <li
            v-for="(entry, i) in request.accessLog"
            :key="i"
          >
            {{ entry }}
          </li>
        </ul>
      </div>
    </section>
  </article>
</template>
