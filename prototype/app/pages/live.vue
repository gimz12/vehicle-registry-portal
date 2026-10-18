<script setup lang="ts">
import type { LiveEvent, NodeId } from '~~/shared/live'
import { PEERS, describeEvent, steps } from '~~/shared/live'

/** The laboratory as a 3D scene. A packet moves only when an event says it did. */
interface SceneNode { id: NodeId | string, label: string, group: 'agency' | 'portal' | 'peer' | 'orderer' | 'custody' | 'block', fx: number, fy: number, fz: number, invalid?: boolean }
interface SceneLink { source: string, target: string, color: string, particle: string, speed: number, width: number }

/** The custodians' own line, drawn thicker and in the custody colour: it is the one path that bypasses the portal. */
const DIRECT = new Set(['CUSTODY_DMT>CUSTODY_POLICE', 'CUSTODY_POLICE>CUSTODY_DMT'])

const fixed: SceneNode[] = [
  // DMT and Police sit next to each other: their custody services hand key material over directly.
  { id: 'CUSTOMS', label: 'Customs', group: 'agency', fx: -320, fy: 170, fz: 0 },
  { id: 'ASSEMBLER', label: 'Assembler', group: 'agency', fx: -320, fy: 115, fz: 0 },
  { id: 'VET', label: 'Emission centre', group: 'agency', fx: -320, fy: 60, fz: 0 },
  { id: 'INSURER', label: 'Insurers', group: 'agency', fx: -320, fy: 5, fz: 0 },
  { id: 'DMT', label: 'DMT', group: 'agency', fx: -320, fy: -50, fz: 0 },
  { id: 'POLICE', label: 'Police', group: 'agency', fx: -320, fy: -105, fz: 0 },
  { id: 'BUYER', label: 'Buyer', group: 'agency', fx: -320, fy: -160, fz: 0 },
  { id: 'PORTAL', label: 'Portal gateway', group: 'portal', fx: -130, fy: 0, fz: 0 },
  { id: 'PEER3', label: 'peer0.org3 (Customs)', group: 'peer', fx: 60, fy: 170, fz: 0 },
  { id: 'PEER1', label: 'peer0.org1 (DMT)', group: 'peer', fx: 60, fy: 85, fz: 0 },
  { id: 'PEER4', label: 'peer0.org4 (VET)', group: 'peer', fx: 60, fy: 0, fz: 0 },
  { id: 'PEER5', label: 'peer0.org5 (insurers)', group: 'peer', fx: 60, fy: -85, fz: 0 },
  { id: 'PEER2', label: 'peer0.org2 (Police)', group: 'peer', fx: 60, fy: -170, fz: 0 },
  { id: 'ORDERER', label: 'Ordering service (3 Raft orderers)', group: 'orderer', fx: 250, fy: 0, fz: 0 },
  // Each organisation's own custody service, drawn beside that organisation only.
  { id: 'CUSTODY_CUSTOMS', label: 'Customs records', group: 'custody', fx: -450, fy: 170, fz: 0 },
  { id: 'CUSTODY_VET', label: 'VET records', group: 'custody', fx: -450, fy: 60, fz: 0 },
  { id: 'CUSTODY_INSURER', label: 'Insurer records', group: 'custody', fx: -450, fy: 5, fz: 0 },
  { id: 'CUSTODY_DMT', label: 'DMT custody', group: 'custody', fx: -450, fy: -50, fz: 0 },
  { id: 'CUSTODY_POLICE', label: 'Police custody', group: 'custody', fx: -450, fy: -105, fz: 0 },
]
const pairs: [string, string][] = [
  ...(['CUSTOMS', 'ASSEMBLER', 'DMT', 'VET', 'INSURER', 'POLICE', 'BUYER'] as const).flatMap(a => [[a, 'PORTAL'], ['PORTAL', a]] as [string, string][]),
  ...PEERS.flatMap(p => [['PORTAL', p], [p, 'PORTAL'], [p, 'ORDERER'], ['ORDERER', p]] as [string, string][]),
  // Endorsement requests between peers (the gateway peer asks the others).
  ...PEERS.flatMap(a => PEERS.filter(b => b !== a).map(b => [a, b] as [string, string])),
  // Each database is connected to its own organisation only.
  ...([['CUSTOMS', 'CUSTODY_CUSTOMS'], ['DMT', 'CUSTODY_DMT'], ['VET', 'CUSTODY_VET'], ['INSURER', 'CUSTODY_INSURER'], ['POLICE', 'CUSTODY_POLICE']] as const)
    .flatMap(([a, c]) => [[a, c], [c, a]] as [string, string][]),
  // The DMT and Police custodians read the ledger themselves, each at its own organisation's peer.
  ['CUSTODY_DMT', 'PEER1'], ['PEER1', 'CUSTODY_DMT'], ['CUSTODY_POLICE', 'PEER2'], ['PEER2', 'CUSTODY_POLICE'],
  // The two custodians hand key material to each other directly, never through the portal.
  ['CUSTODY_DMT', 'CUSTODY_POLICE'], ['CUSTODY_POLICE', 'CUSTODY_DMT'],
  // Other desks ask the DMT's database for plates and owner checks; the buyer for the chassis match.
  ...(['POLICE', 'VET', 'INSURER', 'BUYER'] as const).flatMap(a => [[a, 'CUSTODY_DMT'], ['CUSTODY_DMT', a]] as [string, string][]),
]

const tone = (e: LiveEvent): string => {
  if (e.kind === 'phase')
    return e.phase === 'refused' ? 'alert' : e.phase === 'committed' ? 'pass' : e.transient && e.phase === 'proposed' ? 'transient' : 'write'
  if (e.kind === 'block')
    return e.invalid ? 'alert' : 'pass'
  if (e.kind === 'offledger')
    return e.what === 'refused' ? 'alert' : 'custody'
  if (e.kind === 'read')
    return 'read'
  return 'muted'
}

const container = ref<HTMLDivElement | null>(null)
const log = ref<{ t: number, text: string, tone: string }[]>([])
const heights = ref<Record<string, number>>({})
// Each organisation's custody service, from /api/health every 5 s.
interface CustodyHealth { mode: 'services' | 'files', up: number, services: Record<string, { up: boolean, db?: boolean, ledger?: boolean }> }
const custody = ref<CustodyHealth | null>(null)
let healthTimer: ReturnType<typeof setInterval> | undefined
const custodyLine = (h: { up: boolean, ledger?: boolean }) =>
  h.up ? `database${h.ledger === undefined ? '' : ' and its own peer'} reachable` : 'down'
const pollHealth = async () => {
  try {
    custody.value = (await $fetch<{ custody: CustodyHealth }>('/api/health')).custody
  }
  catch {
    custody.value = null
  }
}
const connection = ref('connecting…')
const noWebGL = ref(false)
const blocks = ref<{ number: number, invalid: boolean }[]>([])
const time = (t: number) => new Date(t).toLocaleTimeString('en-GB')

// Events play one step at a time so a 30 ms endorsement is watchable; the log keeps real timings.
const SPEEDS = { slow: 3000, normal: 1500, fast: 600 } as const
const speed = ref<keyof typeof SPEEDS>('slow')
const caption = ref<{ text: string, at: number, index: number, total: number, tone: string } | null>(null)
const queued = ref(0)
// Events older than the page are history: they fill the log but are not played.
const openedAt = Date.now()
let skipQueue: () => void = () => {}

let es: EventSource | undefined
let destroy: (() => void) | undefined

onMounted(async () => {
  const probe = document.createElement('canvas')
  if (!probe.getContext('webgl2') && !probe.getContext('webgl')) {
    noWebGL.value = true
    return
  }
  const [{ default: ForceGraph3D }, { default: SpriteText }, THREE] = await Promise.all([
    import('3d-force-graph'), import('three-spritetext'), import('three'),
  ])
  const css = getComputedStyle(document.documentElement)
  const colour = (name: string) => css.getPropertyValue(name).trim()
  const palette: Record<string, string> = {
    write: colour('--accent'), transient: '#6b4fbb', pass: colour('--pass'), alert: colour('--alert'),
    read: colour('--muted'), custody: '#4f8a6c', muted: colour('--line-strong'),
  }
  const fill: Record<SceneNode['group'], string> = {
    agency: colour('--ink'), portal: colour('--accent'), peer: '#1b4965', orderer: '#e07a5f', custody: '#4f8a6c', block: '#1b4965',
  }

  const nodes: SceneNode[] = fixed.map(n => ({ ...n }))
  const links: SceneLink[] = pairs.map(([source, target]) => ({
    source, target, particle: palette.muted!, speed: 0.01,
    // A lighter teal than the custody packets, so a packet stands out on it.
    ...(DIRECT.has(`${source}>${target}`) ? { color: '#8fc4a8', width: 3 } : { color: colour('--line'), width: 0.9 }),
  }))
  const linkOf = new Map(links.map(l => [`${l.source}>${l.target}`, l]))

  const box = container.value!
  const graph = new ForceGraph3D(box)
    .width(box.clientWidth)
    .height(560)
    .backgroundColor(colour('--paper'))
    .showNavInfo(false)
    .cooldownTicks(0)
    .enableNodeDrag(false)
    .nodeThreeObject((node: object) => {
      const n = node as SceneNode
      const group = new THREE.Group()
      const material = new THREE.MeshLambertMaterial({ color: n.invalid ? palette.alert : fill[n.group] })
      if (n.group === 'orderer') {
        // Three Raft orderers drawn as one ordering service: the portal cannot tell which one received a transaction.
        for (const x of [-16, 0, 16]) {
          const o = new THREE.Mesh(new THREE.CylinderGeometry(7, 7, 26, 20), material)
          o.position.set(x, 0, x === 0 ? 8 : -4)
          group.add(o)
        }
      }
      else {
        const geometry = n.group === 'peer' || n.group === 'block'
          ? new THREE.BoxGeometry(n.group === 'block' ? 34 : 22, n.group === 'block' ? 14 : 22, n.group === 'block' ? 20 : 22)
          : new THREE.SphereGeometry(n.group === 'agency' ? 8 : 12, 24, 24)
        group.add(new THREE.Mesh(geometry, material))
      }
      if (n.group !== 'block') {
        const text = new SpriteText(n.label, 7, colour('--ink'))
        // Custody labels sit to the left of the sphere, so they never cover the short DMT–Police custody line.
        if (n.group === 'custody')
          text.position.set(-(text.scale.x / 2) - 16, 0, 0)
        else
          text.position.set(0, n.group === 'agency' ? 14 : 24, 0)
        text.fontFace = 'IBM Plex Sans, Helvetica Neue, Arial, sans-serif'
        group.add(text)
      }
      return group
    })
    .linkColor((l: object) => (l as SceneLink).color)
    .linkWidth((l: object) => (l as SceneLink).width)
    .linkOpacity(0.75)
    .linkDirectionalParticleWidth(10)
    .linkDirectionalParticleSpeed((l: object) => (l as SceneLink).speed)
    .linkDirectionalParticleColor((l: object) => (l as SceneLink).particle)
    .graphData({ nodes, links })
  // The library fits its own camera on the first frames; set ours after that.
  const frame = () => {
    const fov = (graph.camera() as { fov?: number }).fov ?? 40
    const span = Math.max(400, 880 / (box.clientWidth / 560))
    const z = span / (2 * Math.tan((fov * Math.PI) / 360))
    graph.cameraPosition({ x: -105, y: 0, z }, { x: -105, y: 0, z: 0 }, 0)
  }
  frame()
  setTimeout(frame, 600)
  graph.scene().add(new THREE.AmbientLight(0xffffff, 1.4))
  // Size once, then only on a real window resize: the library's own sizing overflowed the panel.
  const fit = () => {
    graph.width(box.clientWidth).height(560)
    frame()
  }
  window.addEventListener('resize', fit)
  if (import.meta.dev)
    (window as unknown as { __liveGraph?: unknown }).__liveGraph = graph

  // A packet takes the whole step to cross its link; narration-only steps take half a step.
  const wait = (ms: number) => new Promise<void>(r => setTimeout(r, ms))
  interface Job {
    text: string
    from?: string
    to?: string
    also?: [string, string][]
    shade: string
    at: number
    index: number
    total: number
    tone: string
    then?: () => void
  }
  const queue: Job[] = []
  let playing = false
  skipQueue = () => {
    queue.length = 0
    queued.value = 0
  }
  const play = async () => {
    if (playing)
      return
    playing = true
    while (queue.length) {
      const job = queue.shift()!
      queued.value = queue.length
      const ms = SPEEDS[speed.value]
      caption.value = { text: job.text, at: job.at, index: job.index, total: job.total, tone: job.tone }
      const pairsNow = [...(job.from && job.to ? [[job.from, job.to]] : []), ...(job.also ?? [])]
      const moving = pairsNow.map(([a, b]) => linkOf.get(`${a}>${b}`)).filter((l): l is SceneLink => !!l)
      for (const link of moving) {
        link.particle = job.shade
        link.speed = 1 / (ms / 16.7) // fraction of the link per frame at about 60 fps
        graph.emitParticle(link as never)
      }
      job.then?.()
      await wait(moving.length ? ms : ms / 2)
    }
    playing = false
    caption.value = null
  }

  // Blocks are plain meshes in the scene, not graph nodes: re-setting the graph
  // data would rebuild every link and drop the particles in flight.
  const chain = new THREE.Group()
  graph.scene().add(chain)
  const stackBlocks = () => {
    chain.clear()
    blocks.value.slice(-10).forEach((b, i) => {
      const mesh = new THREE.Mesh(
        new THREE.BoxGeometry(34, 14, 20),
        new THREE.MeshLambertMaterial({ color: b.invalid ? palette.alert : fill.block }),
      )
      mesh.position.set(250, 40 + i * 20, 0)
      const text = new SpriteText(String(b.number), 5, colour('--ink'))
      text.position.set(30, 0, 0)
      mesh.add(text)
      chain.add(mesh)
    })
  }

  const handle = (e: LiveEvent) => {
    if (e.kind === 'metrics') {
      heights.value = e.heights
      return
    }
    if (e.kind === 'status') {
      connection.value = e.detail
    }
    // The 'connected' status is sent once and can fall out of the replay buffer;
    // a block or chaincode event is proof enough that the ledger feed is live.
    if ((e.kind === 'block' || e.kind === 'chaincode') && connection.value.startsWith('connected to the portal'))
      connection.value = 'ledger feed connected: block and chaincode events are arriving'
    const shade = palette[tone(e)] ?? palette.muted!
    const list = e.t < openedAt - 1000 ? [] : steps(e)
    if (e.kind === 'block' && !list.length)
      blocks.value = [...blocks.value, { number: e.number, invalid: e.invalid > 0 }].slice(-10)
    list.forEach((step, i) => queue.push({
      text: step.text, from: step.from, to: step.to, also: step.also, shade,
      at: e.t, index: i + 1, total: list.length, tone: tone(e),
      // The block appears when its step plays, not when the event arrives.
      then: e.kind === 'block' && i === 0
        ? () => {
            blocks.value = [...blocks.value, { number: e.number, invalid: e.invalid > 0 }].slice(-10)
            stackBlocks()
          }
        : undefined,
    }))
    queued.value = queue.length
    if (list.length)
      void play()
    else if (e.kind === 'block')
      stackBlocks()
    const text = describeEvent(e)
    if (text)
      log.value = [{ t: e.t, text, tone: tone(e) }, ...log.value].slice(0, 40)
  }

  es = new EventSource('/api/live')
  es.onopen = () => void (connection.value = 'connected to the portal; waiting for the ledger feed')
  es.onerror = () => void (connection.value = 'stream interrupted; reconnecting…')
  es.onmessage = m => handle(JSON.parse(m.data) as LiveEvent)
  destroy = () => {
    es?.close()
    window.removeEventListener('resize', fit)
    graph._destructor()
  }
})

onMounted(() => {
  void pollHealth()
  healthTimer = setInterval(pollHealth, 5000)
})
onBeforeUnmount(() => {
  destroy?.()
  clearInterval(healthTimer)
})

// Triggers: real calls through the same routes the desk and the Police page use.
const DEMO = {
  vid: '1339a27a6ffb3006e62bc93d78fbbb9f4286ad6e108bb35e2282d5704fb3ea4e', // demo car R2 (see documents/DEMO_GUIDE.md)
  caseId: 'REHEARSAL/2026/R2', // approved case on R2
}
const trigger = reactive({ vid: DEMO.vid, caseId: DEMO.caseId, busy: '', note: '' })
const owner = { name: 'Live Demo Owner (synthetic)', nic: '199099988877', address: '1, Demo Lane, Colombo' }

async function run(name: string, steps: () => Promise<string>) {
  trigger.busy = name
  trigger.note = ''
  try {
    trigger.note = await steps()
  }
  catch (error) {
    const e = error as { data?: { statusMessage?: string }, message?: string }
    trigger.note = e.data?.statusMessage || e.message || 'failed'
  }
  finally {
    trigger.busy = ''
  }
}
const desk = (body: Record<string, unknown>) => $api<{ message: string }>('/desk', { method: 'POST', body })
const stamp = () => `${Date.now().toString(36).toUpperCase()}`

const triggers = [
  {
    name: 'Register a vehicle',
    watch: 'Two commits (Customs, then the DMT), each endorsed by three of the five peers and landing on all five; then teal packets: the DMT desk sends the record to its own custody service, which checks the anchor at its own peer, hands the Police half straight to the Police custody service (which checks the anchor at its own peer), and stores the sealed record. The portal is not in that hand-over. Only the hash is on the block.',
    go: () => run('Register a vehicle', async () => {
      const id = stamp()
      await desk({
        agency: 'CUSTOMS', action: 'import', recordId: `LIVE/CUSDEC/${id}`,
        chassisNumber: `LIVE-CH-${id}`, engineNumber: `LIVE-EN-${id}`, make: 'Toyota',
      })
      const r = await desk({
        agency: 'DMT', action: 'register', chassisNumber: `LIVE-CH-${id}`, engineNumber: `LIVE-EN-${id}`,
        make: 'Toyota', provenance: 'CUSTOMS', owner,
      })
      return r.message
    }),
  },
  {
    name: 'Buyer reads a vehicle',
    watch: 'A grey packet to one peer and back. No block is cut: a read is not a transaction.',
    go: () => run('Buyer reads a vehicle', async () => {
      await $api(`/vehicle/${trigger.vid}?as=BUYER`)
      return 'Disclosure returned at profile P2'
    }),
  },
  {
    name: 'Refused write (no Customs record)',
    watch: 'The proposal reaches the DMT’s peer and comes back red with the chaincode’s own words. Nothing goes to the orderers; no block.',
    go: () => run('Refused write', async () => {
      const id = stamp()
      await desk({
        agency: 'DMT', action: 'register', chassisNumber: `LIVE-NOIMPORT-${id}`, engineNumber: `LIVE-NOENG-${id}`,
        make: 'Toyota', owner,
      })
      return 'Unexpectedly committed'
    }),
  },
  {
    name: 'DMT owner-scope check (F6b)',
    watch: 'A purple packet: the owner’s vehicle list travels as transient data. It reaches the peers but is never written to the block; the block holds only the verdict and a hash of the list.',
    go: () => run('DMT owner-scope check', async () => {
      const r = await $api<{ status: string, strength: string }>('/escalation', {
        method: 'POST', body: { action: 'predicate', as: 'DMT', vid: trigger.vid, ruleId: 'F6b' },
      })
      return `Verdict: ${r.status} at ${r.strength} strength`
    }),
  },
  {
    name: 'Police read under the approved case',
    watch: 'Also purple and then a commit: the owner-scope read is submitted as a transaction so the OWNER_SCOPE audit entry is written to the case; a block is cut for a read.',
    go: () => run('Police read under case', async () => {
      const query = `as=POLICE&case=${encodeURIComponent(trigger.caseId)}`
      const r = await $api<{ profile: string }>(`/vehicle/${trigger.vid}?${query}`)
      return `Disclosure returned at profile ${r.profile}`
    }),
  },
  {
    name: 'Reveal the owner under the approved case',
    watch: 'Teal: the Police desk asks its own custody service, which checks the case at its own peer and asks the DMT custody service directly for its part; the DMT service checks the case at its own peer too. The Police service decrypts and writes the reveal to the case on the ledger before the identity leaves it.',
    go: () => run('Reveal under case', async () => {
      const r = await $api<{ owner: { name: string } }>('/reveal', { method: 'POST', body: { caseId: trigger.caseId, vid: trigger.vid } })
      return `Identity revealed (synthetic): ${r.owner.name}`
    }),
  },
]

const toneClass: Record<string, string> = {
  write: 'text-accent', transient: 'text-[#6b4fbb]', pass: 'text-pass', alert: 'text-alert', read: 'text-muted', custody: 'text-[#4f8a6c]', muted: 'text-muted',
}
</script>

<template>
  <article class="space-y-6">
    <header class="border-b border-line pb-5">
      <h1 class="text-2xl font-semibold sm:text-3xl">
        Live network
      </h1>
      <p class="mt-2 max-w-3xl text-muted">
        What happens inside the registry while the desk, the disclosure pages and the Police case run. Every packet
        is an event the portal or the ledger reported: a proposal to the agency's own peer, endorsements from
        a majority of the five organisations, the ordering service cutting a block, all five peers committing it,
        and the owner record going to off-ledger custody instead of the chain. Nothing is drawn that did not happen.
      </p>
    </header>

    <div
      v-if="noWebGL"
      class="rounded-md border border-alert bg-alert-bg p-6 text-sm"
      role="alert"
    >
      This browser has no WebGL, so the 3D scene cannot be drawn. The event log still works on the agency desk and
      the Police page; open this page in a browser with WebGL to see the scene.
    </div>

    <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_320px]">
      <section
        class="surface relative self-start overflow-hidden rounded-md"
        aria-label="Network scene"
      >
        <div
          ref="container"
          class="h-[560px] w-full"
        />
        <!-- Below the scene, not over it, so the custody row stays visible while its packets play. -->
        <div
          class="min-h-[92px] border-t border-line p-4"
          aria-live="polite"
        >
          <div
            v-if="caption"
            class="rounded-md border border-line bg-panel/90 px-4 py-3 text-sm shadow-sm"
          >
            <p class="flex items-center justify-between gap-3 text-xs text-muted">
              <span>Step {{ caption.index }} of {{ caption.total }} · happened at {{ time(caption.at) }}</span>
              <span v-if="queued">
                {{ queued }} more step{{ queued === 1 ? '' : 's' }} queued ·
                <button
                  type="button"
                  class="underline"
                  @click="skipQueue()"
                >
                  skip
                </button>
              </span>
            </p>
            <p
              class="mt-1 font-medium"
              :class="toneClass[caption.tone]"
            >
              {{ caption.text }}
            </p>
          </div>
          <p
            v-else
            class="inline-block rounded-md border border-line bg-panel/90 px-3 py-2 text-xs text-muted"
          >
            Idle. Steps play one at a time at the chosen speed; the time shown is when each really happened.
          </p>
        </div>
        <label
          class="absolute top-3 right-3 flex items-center gap-2 rounded-md border border-line bg-panel/90 px-3 py-1.5
            text-xs"
        >
          <span class="text-muted">Playback</span>
          <select
            v-model="speed"
            class="bg-transparent font-medium"
            aria-label="Playback speed"
          >
            <option value="slow">
              Slow (3 s a step)
            </option>
            <option value="normal">
              Normal (1.5 s)
            </option>
            <option value="fast">
              Fast (0.6 s)
            </option>
          </select>
        </label>
      </section>

      <aside class="space-y-4 text-sm">
        <div class="surface rounded-md p-4">
          <p class="font-medium">
            Connection
          </p>
          <p class="mt-1 text-muted">
            {{ connection }}
          </p>
          <dl class="mt-3 grid grid-cols-2 gap-y-1">
            <template
              v-for="(h, name) in heights"
              :key="name"
            >
              <dt class="text-muted">
                {{ name }} height
              </dt>
              <dd class="font-mono">
                {{ h }}
              </dd>
            </template>
          </dl>
          <p class="mt-3 font-medium">
            Custody
          </p>
          <p
            v-if="!custody"
            class="mt-1 text-muted"
          >
            checking…
          </p>
          <p
            v-else-if="custody.mode === 'files'"
            class="mt-1 text-alert"
          >
            Fallback mode: the portal is using the file store on this host; no custodian checks the ledger.
          </p>
          <ul
            v-else
            class="mt-1 space-y-0.5"
          >
            <li
              v-for="(h, org) in custody.services"
              :key="org"
              :class="h.up ? 'text-pass' : 'text-alert'"
            >
              {{ h.up ? '●' : '○' }} {{ org }}: {{ custodyLine(h) }}
            </li>
          </ul>
        </div>

        <div class="surface rounded-md p-4">
          <p class="font-medium">
            Trigger real traffic
          </p>
          <p class="mt-1 text-muted">
            Each button calls the same portal route an officer would. Watch the scene, then the log.
          </p>
          <div class="mt-3 grid gap-2">
            <button
              v-for="t in triggers"
              :key="t.name"
              type="button"
              class="btn-secondary text-left"
              :disabled="trigger.busy !== ''"
              :title="t.watch"
              @click="t.go"
            >
              {{ trigger.busy === t.name ? 'Running…' : t.name }}
            </button>
          </div>
          <p
            v-if="trigger.note"
            class="mt-2 text-muted"
            aria-live="polite"
          >
            {{ trigger.note }}
          </p>
          <details class="mt-3">
            <summary class="cursor-pointer text-muted">
              What each button shows
            </summary>
            <ul class="mt-2 space-y-2 text-muted">
              <li
                v-for="t in triggers"
                :key="t.name"
              >
                <span class="font-medium text-ink">{{ t.name }}.</span> {{ t.watch }}
              </li>
            </ul>
            <label
              class="label mt-3"
              for="live-vid"
            >Vehicle handle used by the read and case buttons</label>
            <input
              id="live-vid"
              v-model="trigger.vid"
              class="field font-mono text-xs"
            >
            <label
              class="label mt-2"
              for="live-case"
            >Approved case number</label>
            <input
              id="live-case"
              v-model="trigger.caseId"
              class="field font-mono text-xs"
            >
          </details>
        </div>

        <div class="surface rounded-md p-4">
          <p class="font-medium">
            How to read the scene
          </p>
          <ul class="mt-2 space-y-1 text-muted">
            <li>
              <span class="font-medium text-ink">White spheres</span>
              the agencies and the buyer; each agency is joined to its own database on its left
            </li>
            <li>
              <span class="font-medium text-accent">Maroon sphere</span>
              the portal gateway, which signs each call with that agency’s certificate
            </li>
            <li>
              <span class="font-medium text-ink">Blue cubes</span>
              one peer per organisation, beside its agency; a write needs three of the five to endorse
            </li>
            <li>
              <span class="font-medium text-ink">Orange cylinders</span>
              the ordering service: three Raft orderers that agree the order of endorsed transactions and cut blocks
            </li>
            <li>
              <span class="font-medium text-ink">Blue slabs</span>
              the newest blocks, stacked above the orderers with their numbers;
              a red slab holds a transaction the peers marked invalid
            </li>
            <li>
              <span class="font-medium text-[#4f8a6c]">Teal spheres</span>
              each organisation’s own off-ledger service and database, connected to that organisation only: DMT
              custody (sealed owner records, the DMT key half, the owner index), Police custody (the Police key
              half), and the Customs, VET and insurer records the ledger never sees. In the laboratory the agency
              desks are pages of the one portal, which calls each service with that agency’s own token
            </li>
            <li>
              <span class="font-medium text-ink">Lines</span>
              the paths data can take; a packet moves along one only when an event says it did. The short line
              between DMT custody and Police custody joins them directly: the two custodians hand key material to
              each other there. Ledger writes and reads go through the portal gateway to the agency’s own peer
            </li>
            <li>
              <span class="font-medium text-ink">Heights</span>
              the block count each node reports from its own operations endpoint; they agree when the peers are in step
            </li>
          </ul>
        </div>

        <div class="surface rounded-md p-4">
          <p class="font-medium">
            Packets
          </p>
          <ul class="mt-2 space-y-1 text-muted">
            <li><span class="font-medium text-accent">Maroon</span> a write on its way to be endorsed and ordered</li>
            <li>
              <span class="font-medium text-[#6b4fbb]">Purple</span> transient data: reaches the peers, never a block
            </li>
            <li><span class="font-medium text-pass">Green</span> endorsed, committed, or a valid block</li>
            <li><span class="font-medium text-alert">Red</span> refused, or a block with an invalid transaction</li>
            <li>
              <span class="font-medium text-[#4f8a6c]">Teal</span> off-ledger: a record or key half to or from a custody
              service, one custodian handing key material to the other, or a custodian checking the case at its own peer
            </li>
            <li><span class="font-medium">Grey</span> a read; no block is written</li>
          </ul>
        </div>

        <div
          class="surface rounded-md p-4"
          aria-live="polite"
        >
          <p class="font-medium">
            Events
          </p>
          <p
            v-if="log.length === 0"
            class="mt-2 text-muted"
          >
            Waiting. Make a desk write or open a disclosure in another tab.
          </p>
          <ol class="mt-2 max-h-[420px] space-y-1.5 overflow-y-auto">
            <li
              v-for="(entry, i) in log"
              :key="i"
              class="flex gap-2"
            >
              <span class="shrink-0 font-mono text-xs text-muted">{{ time(entry.t) }}</span>
              <span :class="toneClass[entry.tone]">{{ entry.text }}</span>
            </li>
          </ol>
        </div>
      </aside>
    </div>

    <p class="text-xs text-muted">
      Laboratory layout: five organisations, each with its own peer and certificate authority: Org1 DMT (also the
      licensed assembler), Org2 Police, Org3 Customs, Org4 emission centres, Org5 insurers; three Raft orderers.
      All run on one laboratory host. Blocks stack above the orderers, newest on top; a red block carries a
      transaction the peers marked invalid. Needs the live Fabric network.
    </p>
  </article>
</template>
