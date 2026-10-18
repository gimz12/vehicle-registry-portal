<script setup lang="ts">
import type { Agency, DeskAction, DeskInput, DeskResult } from '~~/shared/disclosure'
import { AGENCIES, DESK_ACTIONS } from '~~/shared/disclosure'

/** One desk, six agencies; each submit goes to the ledger under that agency's own certificate. */
const agencyName: Record<Agency, string> = {
  CUSTOMS: 'Sri Lanka Customs', ASSEMBLER: 'Licensed local assembler', DMT: 'Department of Motor Traffic', VET: 'Vehicle emission test centre',
  POLICE: 'Sri Lanka Police', INSURER: 'Insurer',
}
const actionName: Record<DeskAction, string> = {
  import: 'Record an import clearance',
  attest: 'Record a local assembler attestation',
  register: 'Register a vehicle',
  transfer: 'Transfer ownership',
  retire: 'Retire from the register',
  encumber: 'Record finance (absolute owner)',
  release: 'Record a finance release',
  mileage: 'Record an odometer reading',
  emission: 'Record an emission certificate',
  report: 'Record a theft report',
  theft: 'Flag stolen',
  clear: 'Clear a theft report',
  accident: 'File an accident report',
  claim: 'File a claim',
  writeoff: 'Write off',
}
const actionNote: Partial<Record<DeskAction, string>> = {
  import: 'Enter the identifiers as printed. The registry computes their keyed indices; the chassis number itself never reaches the ledger. A first registration is refused unless an import with the same engine index was committed first (F5).',
  attest: 'For vehicles assembled in Sri Lanka, which clear Customs as parts, never per chassis. The assembler records its build attestation under its own registry identity, never the DMT that registers the vehicle; registration then uses provenance "Local assembler attestation".',
  register: 'Refused unless Customs recorded the import (provenance Customs clearance) or a licensed assembler recorded its attestation (provenance Local assembler attestation) for this engine first (F5). The handle is computed from the chassis number, never generated; owner details are sealed off-ledger. A chassis or engine already registered is refused (F4).',
  transfer: 'Refused while the vehicle is flagged stolen, written off, retired, or under finance (F2 and the finance gate). The new owner\'s details replace the previous owner\'s sealed record; the ledger gets a new pseudonym and anchor.',
  encumber: 'A finance company recorded as absolute owner on the certificate of registration. Transfers are refused until its deletion letter is recorded. The buyer sees "under finance"; the lender is never named.',
  release: 'The finance company\'s deletion letter. Both events stay in the history.',
  retire: 'The record stays on the ledger so a later registration reusing its identity is caught as a resurrection.',
  mileage: 'A reading below the previous one is accepted and flagged, never rejected: rejecting would destroy the evidence.',
  emission: 'Recorded for completeness. No fraud rule reads emission certificates.',
  report: 'Step 1 of 2. The report is written to the ledger under its report number. It flags nothing and blocks no sale: a buyer sees only that a report is being verified.',
  theft: 'Step 2 of 2. Give the report number and the reporter\'s NIC: the DMT\'s database confirms, yes or no, that the reporter is the registered owner of this vehicle and writes that confirmation to the ledger; only then does the ledger accept the flag. The NIC is not stored. With an approved theft case instead, give the case id as the report number and leave the NIC empty. While flagged, any ownership transfer is refused.',
  claim: 'Cite the police report id to earn the independently anchored evidence grade. Name the assessor only when one attested the damage.',
  writeoff: 'Transfers are refused until the write-off is resolved.',
}

// The agency's own internal fields: kept in its own service after the commit, never on the ledger.
interface InternalField { key: string, label: string, placeholder: string }
const vetFields: InternalField[] = [
  { key: 'technician', label: 'Technician', placeholder: 'name (synthetic)' },
  { key: 'certificateNo', label: 'Certificate number', placeholder: 'e.g. VET/CERT/1001' },
]
const INTERNAL_FIELDS: Partial<Record<DeskAction, InternalField[]>> = {
  import: [
    { key: 'declarant', label: 'Declarant (importer)', placeholder: 'company name (synthetic)' },
    { key: 'cusdecNo', label: 'CusDec number', placeholder: 'e.g. CUSDEC/2026/1001' },
    { key: 'valuationLkr', label: 'Declared value (LKR)', placeholder: 'e.g. 6850000' },
  ],
  mileage: vetFields,
  emission: vetFields,
  claim: [
    { key: 'policyNo', label: 'Policy number', placeholder: 'e.g. POL/A/1001' },
    { key: 'claimFile', label: 'Claim file', placeholder: 'e.g. CLM/A/1001' },
    { key: 'assessorContact', label: 'Assessor contact', placeholder: 'name or phone (synthetic)' },
    { key: 'reserveLkr', label: 'Reserve (LKR)', placeholder: 'e.g. 420000' },
  ],
}
const internalFields = computed(() => INTERNAL_FIELDS[action.value] ?? [])
const internal = reactive<Record<string, string>>({})
function internalInput(): Record<string, string> | undefined {
  const filled = internalFields.value.filter(f => internal[f.key]?.trim())
  return filled.length ? Object.fromEntries(filled.map(f => [f.key, internal[f.key]!.trim()])) : undefined
}

const agency = ref<Agency>('DMT')
const action = ref<DeskAction>('register')
watch(agency, a => action.value = DESK_ACTIONS[a][0])

// Each agency's own document reference, as it would appear on its paperwork.
const recordPrefix: Record<DeskAction, string> = {
  import: 'cusdec', attest: 'assembler-cert', register: 'registration', transfer: 'transfer-deed', retire: 'retire',
  encumber: 'finance', release: 'finance-release',
  mileage: 'vet-reading', emission: 'vet-emission', report: 'police-theft-report', theft: 'police-theft', clear: 'police-clear',
  accident: 'police-report', claim: 'claim', writeoff: 'writeoff',
}
const PREFILLED = /^[a-z-]+-(\d{7})$/
watch(action, (next) => {
  // A new action means a new form: drop the previous outcome and re-prefix a prefilled id.
  refusal.value = ''
  result.value = null
  const m = form.recordId.match(PREFILLED)
  if (m)
    form.recordId = `${recordPrefix[next]}-${m[1]}`
})

const form = reactive({
  vid: '', recordId: '', chassisNumber: '', engineNumber: '', make: '', provenance: 'CUSTOMS' as 'CUSTOMS' | 'ASSEMBLER',
  ownerName: '', ownerNic: '', ownerAddress: '',
  day: 0, odometer: 0, vetLaneId: 'lane-1', evidenceAnchor: '', insurerId: 'insurer-a', assessorId: '', flag: true,
  reference: '',
  lookupChassis: '',
  lookupPlate: '',
  // What the vehicle is (Customs at import, the DMT at registration), its plate and colour.
  model: '', year: 2016, engineCc: 1496, fuel: 'PETROL', colour: '', plate: '',
  // Theft: the Police report number and the reporter's NIC.
  reportRef: '', reporterNic: '',
})
function prefill() {
  const n = Math.floor(Math.random() * 9_000_000 + 1_000_000)
  form.chassisNumber = `NHP10-${n}`
  form.engineNumber = `1NZ-${n.toString().slice(0, 6)}`
  form.make = 'Toyota'
  form.recordId = `${recordPrefix[action.value]}-${n}`
  form.ownerName = 'Sunil Fernando (synthetic)'
  form.ownerNic = '198811223344'
  form.ownerAddress = '7 Lake Drive, Nugegoda'
  form.model = 'Axio'
  form.year = 2016
  form.engineCc = 1496
  form.fuel = 'PETROL'
  form.colour = 'White'
  form.plate = `CAB-${n.toString().slice(0, 4)}`
  form.reportRef = `PR/2026/${n.toString().slice(0, 5)}`
}

// Officers know a chassis number, not a handle: resolve it with the registry key.
const lookupBusy = ref(false)
const lookupNote = ref('')
async function lookupHandle(by: 'chassis' | 'plate' = 'chassis') {
  lookupBusy.value = true
  lookupNote.value = ''
  try {
    // A plate is looked up in the DMT's own database; it never reaches the ledger.
    const value = by === 'plate' ? form.lookupPlate : form.lookupChassis
    const r = await $api<{ vid: string }>('/handle', { method: 'POST', body: { [by]: value, agency: agency.value } })
    form.vid = r.vid
    lookupNote.value = by === 'plate'
      ? `Vehicle carrying plate ${value.trim().toUpperCase()} found by the DMT's database`
      : `Handle computed for ${value.trim().toUpperCase()}`
  }
  catch (error) {
    const e = error as { data?: { statusMessage?: string }, message?: string }
    lookupNote.value = e.data?.statusMessage || e.message || 'Could not compute the handle'
  }
  finally {
    lookupBusy.value = false
  }
}

const show = computed(() => ({
  vid: !['import', 'attest', 'register'].includes(action.value),
  recordId: !['retire', 'writeoff', 'register'].includes(action.value),
  identity: ['import', 'attest', 'register'].includes(action.value),
  owner: ['register', 'transfer'].includes(action.value),
  day: ['mileage', 'emission', 'accident', 'claim'].includes(action.value),
  odometer: action.value === 'mileage',
  lane: ['mileage', 'emission'].includes(action.value),
  claim: action.value === 'claim',
  reference: ['encumber', 'release'].includes(action.value),
  flag: ['retire', 'writeoff'].includes(action.value),
  description: ['import', 'attest', 'register'].includes(action.value),
  plate: action.value === 'register',
  reportRef: ['report', 'theft'].includes(action.value),
  reporterNic: action.value === 'theft',
}))

const recordHelp = computed(() => {
  switch (action.value) {
    case 'import': return 'The Customs declaration (CusDec) number for this import. The DMT does not cite it; the match is by engine index.'
    case 'attest': return 'The assembler\'s build certificate number.'
    case 'accident': return 'The police report number. Insurers cite it on their claims.'
    case 'claim': return 'The insurer\'s claim number.'
    case 'mileage':
    case 'emission': return 'The VET certificate number.'
    case 'report': return 'This ledger entry\'s own id. The report number goes below.'
    case 'theft': return 'This flag\'s own id. The report number it rests on goes below.'
    case 'clear': return 'The police clearance reference.'
    default: return 'The agency\'s own reference for this document.'
  }
})
const flagLabel = computed(() => action.value === 'retire'
  ? 'Retired (untick to return to the active register)'
  : 'Written off (untick to resolve)')

const busy = ref(false)
const result = ref<DeskResult | null>(null)
const refusal = ref('')
// Only a 409 is the chaincode's own refusal; a 400 never left the portal.
const refusalLabel = ref('')
const history = ref<{ ok: boolean, text: string, agency: Agency, action: DeskAction, vid?: string }[]>([])

async function submit() {
  busy.value = true
  refusal.value = ''
  result.value = null
  const input: DeskInput = {
    agency: agency.value, action: action.value,
    vid: show.value.vid ? form.vid.trim().toLowerCase() : undefined,
    recordId: show.value.recordId ? form.recordId.trim() : undefined,
    chassisNumber: show.value.identity ? form.chassisNumber.trim() : undefined,
    engineNumber: show.value.identity ? form.engineNumber.trim() : undefined,
    make: show.value.identity ? form.make.trim() : undefined,
    provenance: show.value.identity ? form.provenance : undefined,
    owner: show.value.owner
      ? { name: form.ownerName.trim(), nic: form.ownerNic.trim(), address: form.ownerAddress.trim() }
      : undefined,
    day: show.value.day ? Number(form.day) : undefined,
    odometer: show.value.odometer ? Number(form.odometer) : undefined,
    vetLaneId: show.value.lane ? form.vetLaneId : undefined,
    evidenceAnchor: show.value.claim ? form.evidenceAnchor.trim() || undefined : undefined,
    insurerId: show.value.claim ? form.insurerId.trim() : undefined,
    assessorId: show.value.claim ? form.assessorId.trim() || undefined : undefined,
    reference: show.value.reference ? form.reference.trim() : undefined,
    flag: show.value.flag ? form.flag : undefined,
    internal: internalInput(),
    ...(show.value.description && form.model.trim()
      ? { model: form.model.trim(), year: Number(form.year), engineCc: Number(form.engineCc), fuel: form.fuel }
      : {}),
    ...(show.value.plate ? { plate: form.plate.trim() || undefined, colour: form.colour.trim() || undefined } : {}),
    reportRef: show.value.reportRef ? form.reportRef.trim() : undefined,
    reporterNic: show.value.reporterNic ? form.reporterNic.trim() || undefined : undefined,
  }
  try {
    const r = await $api<DeskResult>('/desk', { method: 'POST', body: input })
    result.value = r
    history.value.unshift({ ok: true, text: r.message, agency: r.agency, action: r.action, vid: r.vid })
  }
  catch (error) {
    const e = error as {
      data?: { statusMessage?: string }
      statusCode?: number
      statusMessage?: string
      message?: string
    }
    refusal.value = e.data?.statusMessage || e.statusMessage || e.message || 'The write failed'
    // A 502 after the commit means the ledger write stands and only the off-ledger
    // store failed: resubmitting would write the record twice.
    const committed = refusal.value.startsWith('The ledger committed')
    refusalLabel.value = committed
      ? 'Committed on the ledger; off-ledger storage failed. Do not resubmit.'
      : refusal.value.startsWith('DMT custody service') && e.statusCode === 409
        ? 'Refused by the DMT.'
        : ({
            409: 'Refused by the ledger.',
            403: 'Refused by the DMT.',
            429: 'Too many attempts.',
            501: 'Needs the custody services.',
            400: 'Not sent: correct the form.',
          } as Record<number, string>)[e.statusCode ?? 0] ?? 'Ledger unavailable.'
    history.value.unshift({
      ok: committed, text: refusal.value, agency: agency.value, action: action.value, vid: input.vid,
    })
  }
  finally {
    busy.value = false
  }
}
</script>

<template>
  <article class="space-y-8">
    <header class="border-b border-line pb-5">
      <h1 class="text-2xl font-semibold sm:text-3xl">
        Agency desk
      </h1>
      <p class="mt-2 max-w-2xl text-muted">
        Each agency writes only the records its role allows, under its own certificate.
        The ledger's answer is shown as it comes back, refusals included. Use synthetic details only.
      </p>
    </header>

    <div class="grid gap-8 lg:grid-cols-[18rem_1fr]">
      <aside class="space-y-4 lg:sticky lg:top-6 lg:self-start">
        <div class="surface rounded-md p-4">
          <label
            for="agency"
            class="label"
          >Acting as</label>
          <select
            id="agency"
            v-model="agency"
            class="field"
          >
            <option
              v-for="a in AGENCIES"
              :key="a"
              :value="a"
            >
              {{ agencyName[a] }}
            </option>
          </select>
          <label
            for="action"
            class="label mt-4"
          >Action</label>
          <select
            id="action"
            v-model="action"
            class="field"
          >
            <option
              v-for="a in DESK_ACTIONS[agency]"
              :key="a"
              :value="a"
            >
              {{ actionName[a] }}
            </option>
          </select>
          <p
            v-if="actionNote[action]"
            class="help mt-3"
          >
            {{ actionNote[action] }}
          </p>
          <button
            type="button"
            class="btn-quiet mt-3 px-0"
            @click="prefill"
          >
            Fill with synthetic test values
          </button>
        </div>

        <div
          v-if="history.length"
          class="surface rounded-md p-4"
        >
          <h2 class="text-sm font-semibold">
            This session
          </h2>
          <ol class="mt-2 space-y-2 text-sm">
            <li
              v-for="(h, i) in history"
              :key="i"
              class="flex gap-2"
            >
              <span :class="h.ok ? 'chip-pass' : 'chip-alert'">{{ h.ok ? 'Committed' : 'Refused' }}</span>
              <span class="min-w-0">
                <span class="block">{{ agencyName[h.agency] }}: {{ actionName[h.action].toLowerCase() }}</span>
                <span class="block text-xs text-muted">{{ h.text }}</span>
                <NuxtLink
                  v-if="h.vid"
                  :to="`/vehicle/${h.vid}?as=DMT`"
                  class="text-xs text-accent hover:underline"
                >View this vehicle</NuxtLink>
              </span>
            </li>
          </ol>
        </div>
      </aside>

      <form
        class="surface space-y-5 rounded-md p-5"
        @submit.prevent="submit"
      >
        <h2 class="font-semibold">
          {{ actionName[action] }}
        </h2>

        <div class="grid gap-4 sm:grid-cols-2">
          <div
            v-if="show.vid"
            class="sm:col-span-2 space-y-2"
          >
            <label
              for="vid"
              class="label"
            >Vehicle handle</label>
            <input
              id="vid"
              v-model="form.vid"
              class="field font-mono text-sm"
              required
              pattern="[A-Fa-f0-9]{64}"
              placeholder="64 hexadecimal characters, as printed on the registration certificate"
              spellcheck="false"
            >
            <div class="flex flex-col gap-2 sm:flex-row">
              <input
                v-model="form.lookupChassis"
                class="field sm:max-w-xs"
                placeholder="or enter the chassis number"
                aria-label="Chassis number to resolve"
                spellcheck="false"
              >
              <button
                type="button"
                class="btn-secondary"
                :disabled="lookupBusy || !form.lookupChassis.trim()"
                @click="lookupHandle('chassis')"
              >
                Compute handle
              </button>
            </div>
            <div
              v-if="agency !== 'ASSEMBLER'"
              class="flex flex-col gap-2 sm:flex-row"
            >
              <input
                v-model="form.lookupPlate"
                class="field sm:max-w-xs"
                placeholder="or enter the number plate"
                aria-label="Number plate to look up"
                spellcheck="false"
              >
              <button
                type="button"
                class="btn-secondary"
                :disabled="lookupBusy || !form.lookupPlate.trim()"
                @click="lookupHandle('plate')"
              >
                Find by plate
              </button>
            </div>
            <p
              v-if="lookupNote"
              class="help"
            >
              {{ lookupNote }}
            </p>
          </div>
          <div v-if="show.recordId">
            <label
              for="recordId"
              class="label"
            >Record id</label>
            <input
              id="recordId"
              v-model="form.recordId"
              class="field"
              required
              :placeholder="`${recordPrefix[action]}-…`"
            >
            <p class="help mt-1">
              {{ recordHelp }}
            </p>
          </div>
          <template v-if="show.identity">
            <div>
              <label
                for="chassis"
                class="label"
              >Chassis number</label>
              <input
                id="chassis"
                v-model="form.chassisNumber"
                class="field"
                required
                placeholder="NHP10-1234567"
                spellcheck="false"
              >
            </div>
            <div>
              <label
                for="make"
                class="label"
              >Make</label>
              <input
                id="make"
                v-model="form.make"
                class="field"
                required
                placeholder="Toyota"
              >
            </div>
            <div>
              <label
                for="engine"
                class="label"
              >Engine number</label>
              <input
                id="engine"
                v-model="form.engineNumber"
                class="field"
                required
                placeholder="1NZ-A123456"
                spellcheck="false"
              >
            </div>
            <div v-if="action === 'register'">
              <label
                for="provenance"
                class="label"
              >Provenance</label>
              <select
                id="provenance"
                v-model="form.provenance"
                class="field"
              >
                <option value="CUSTOMS">
                  Customs clearance
                </option>
                <option value="ASSEMBLER">
                  Local assembler attestation
                </option>
              </select>
            </div>
          </template>
          <template v-if="show.description">
            <div class="sm:col-span-2 rounded-sm bg-accent-wash p-3 text-sm">
              What the vehicle is. A keyed fingerprint of make, model, year, engine size and fuel goes on the
              ledger; the registration must reproduce what Customs declared. The fields themselves stay in the
              DMT's database. Leave the model empty to record no description.
            </div>
            <div>
              <label
                for="model"
                class="label"
              >Model</label>
              <input
                id="model"
                v-model="form.model"
                class="field"
                placeholder="Axio"
              >
            </div>
            <div>
              <label
                for="year"
                class="label"
              >Year of manufacture</label>
              <input
                id="year"
                v-model.number="form.year"
                type="number"
                min="1950"
                max="2100"
                class="field"
              >
            </div>
            <div>
              <label
                for="engineCc"
                class="label"
              >Engine capacity (cc)</label>
              <input
                id="engineCc"
                v-model.number="form.engineCc"
                type="number"
                min="0"
                max="20000"
                class="field"
              >
            </div>
            <div>
              <label
                for="fuel"
                class="label"
              >Fuel</label>
              <select
                id="fuel"
                v-model="form.fuel"
                class="field"
              >
                <option
                  v-for="f in ['PETROL', 'DIESEL', 'HYBRID', 'ELECTRIC', 'OTHER']"
                  :key="f"
                  :value="f"
                >
                  {{ f.charAt(0) + f.slice(1).toLowerCase() }}
                </option>
              </select>
            </div>
          </template>
          <template v-if="show.plate">
            <div>
              <label
                for="plate"
                class="label"
              >Number plate</label>
              <input
                id="plate"
                v-model="form.plate"
                class="field"
                placeholder="CAB-1234"
                spellcheck="false"
              >
              <p class="help mt-1">
                Kept in the DMT's database only; agency desks can find the vehicle by it. Never on the ledger.
              </p>
            </div>
            <div>
              <label
                for="colour"
                class="label"
              >Colour</label>
              <input
                id="colour"
                v-model="form.colour"
                class="field"
                placeholder="White"
              >
            </div>
          </template>
          <template v-if="show.reportRef">
            <div>
              <label
                for="reportRef"
                class="label"
              >Police report number</label>
              <input
                id="reportRef"
                v-model="form.reportRef"
                class="field"
                required
                placeholder="PR/2026/0001"
                spellcheck="false"
              >
            </div>
          </template>
          <template v-if="show.reporterNic">
            <div>
              <label
                for="reporterNic"
                class="label"
              >Reporter's NIC</label>
              <input
                id="reporterNic"
                v-model="form.reporterNic"
                class="field"
                placeholder="the owner's NIC (synthetic)"
                spellcheck="false"
              >
              <p class="help mt-1">
                The DMT answers only yes or no; the NIC is not stored.
              </p>
            </div>
          </template>
          <template v-if="show.owner">
            <div class="sm:col-span-2 rounded-sm bg-accent-wash p-3 text-sm">
              {{ action === 'transfer' ? 'New owner.' : '' }}
              The owner's pseudonym is computed from the NIC and the vehicle handle. The details below are
              encrypted off-ledger with a key split between the DMT and the Police; only a hash of the sealed
              record is written to the ledger. Enter synthetic values.
            </div>
            <div>
              <label
                for="ownerName"
                class="label"
              >Owner name</label>
              <input
                id="ownerName"
                v-model="form.ownerName"
                class="field"
                required
              >
            </div>
            <div>
              <label
                for="ownerNic"
                class="label"
              >NIC number</label>
              <input
                id="ownerNic"
                v-model="form.ownerNic"
                class="field"
                required
              >
            </div>
            <div class="sm:col-span-2">
              <label
                for="ownerAddress"
                class="label"
              >Address</label>
              <input
                id="ownerAddress"
                v-model="form.ownerAddress"
                class="field"
                required
              >
            </div>
          </template>
          <div v-if="show.day">
            <label
              for="day"
              class="label"
            >Day (registry day number)</label>
            <input
              id="day"
              v-model.number="form.day"
              type="number"
              min="0"
              class="field"
              required
            >
          </div>
          <div v-if="show.odometer">
            <label
              for="odometer"
              class="label"
            >Odometer, km</label>
            <input
              id="odometer"
              v-model.number="form.odometer"
              type="number"
              min="0"
              class="field"
              required
            >
          </div>
          <div v-if="show.lane">
            <label
              for="lane"
              class="label"
            >Test lane</label>
            <input
              id="lane"
              v-model="form.vetLaneId"
              class="field"
            >
          </div>
          <template v-if="show.claim">
            <div>
              <label
                for="insurer"
                class="label"
              >Insurer id</label>
              <input
                id="insurer"
                v-model="form.insurerId"
                class="field"
                required
              >
            </div>
            <div>
              <label
                for="anchor"
                class="label"
              >Police report cited (optional)</label>
              <input
                id="anchor"
                v-model="form.evidenceAnchor"
                class="field"
                placeholder="record id of the accident report"
              >
            </div>
            <div>
              <label
                for="assessor"
                class="label"
              >Assessor (optional)</label>
              <input
                id="assessor"
                v-model="form.assessorId"
                class="field"
                placeholder="assessor id, only if one attested"
              >
            </div>
          </template>
          <div
            v-if="show.reference"
            class="sm:col-span-2"
          >
            <label
              for="reference"
              class="label"
            >{{ action === 'encumber' ? 'Absolute-owner endorsement reference' : 'Deletion-letter reference' }}</label>
            <input
              id="reference"
              v-model="form.reference"
              class="field"
              required
              placeholder="e.g. AO/2026/115"
            >
          </div>
          <fieldset
            v-if="internalFields.length"
            class="grid gap-3 rounded-md border border-line p-3 sm:col-span-2 sm:grid-cols-2"
          >
            <legend class="px-1 text-sm text-muted">
              Kept in {{ agencyName[agency] }}’s own records service, never sent to the ledger (optional)
            </legend>
            <div
              v-for="f in internalFields"
              :key="f.key"
            >
              <label
                :for="`internal-${f.key}`"
                class="label"
              >{{ f.label }}</label>
              <input
                :id="`internal-${f.key}`"
                v-model="internal[f.key]"
                class="field"
                :placeholder="f.placeholder"
              >
            </div>
          </fieldset>
          <div
            v-if="show.flag"
            class="sm:col-span-2"
          >
            <label class="flex items-center gap-2 text-sm">
              <input
                v-model="form.flag"
                type="checkbox"
                class="size-4"
              >
              {{ flagLabel }}
            </label>
          </div>
        </div>

        <div class="flex flex-wrap items-center gap-3">
          <button
            type="submit"
            class="btn-primary"
            :disabled="busy"
          >
            {{ busy ? 'Submitting to the ledger' : `Submit as ${agencyName[agency]}` }}
          </button>
        </div>

        <p
          v-if="refusal"
          class="rounded-md border border-alert bg-alert-bg p-4 text-sm"
          role="alert"
        >
          <span class="font-semibold text-alert">{{ refusalLabel }}</span>
          {{ refusal }}
        </p>
        <div
          v-if="result"
          class="rounded-md border border-pass bg-pass-bg p-4 text-sm"
          aria-live="polite"
        >
          <span class="font-semibold text-pass">Committed.</span>
          {{ result.message }}
          <NuxtLink
            v-if="result.vid && action !== 'import' && action !== 'attest'"
            :to="`/vehicle/${result.vid}?as=${agency === 'POLICE' ? 'POLICE' : 'DMT'}`"
            class="ml-2 text-accent hover:underline"
          >
            Open the disclosure
          </NuxtLink>
        </div>
      </form>
    </div>
  </article>
</template>
