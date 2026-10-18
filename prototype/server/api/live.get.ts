import type { ChaincodeEvent } from '@hyperledger/fabric-gateway'
import type { peer } from '@hyperledger/fabric-protos'
import type { LiveEvent } from '~~/shared/live'
import type { FabricRuntimeConfig } from '~~/server/utils/fabric'
import { identityFor, openGateway } from '~~/server/utils/fabric'
import { publish, recentEvents, subscribe } from '~~/server/utils/liveBus'

// Server-sent events for /live: the portal's bus plus committed blocks and chaincode
// events. The ledger feed is opened once per process and shared by every stream.
let feedStarted = false

const VALIDATION: Record<number, string> = {
  0: 'VALID', 10: 'ENDORSEMENT_POLICY_FAILURE', 11: 'MVCC_READ_CONFLICT', 12: 'PHANTOM_READ_CONFLICT', 255: 'INVALID_OTHER_REASON',
}

async function startLedgerFeed(config: FabricRuntimeConfig) {
  if (feedStarted || config.demoDir)
    return
  feedStarted = true
  const { gateway } = await openGateway(identityFor(config, 'DMT'))
  const network = gateway.getNetwork(config.channelName)
  const blocks = await network.getFilteredBlockEvents()
  const events = await network.getChaincodeEvents(config.chaincodeName)
  publish({ kind: 'status', detail: 'ledger feed connected: block and chaincode events from the next committed block' })
  void (async () => {
    for await (const block of blocks as AsyncIterable<peer.FilteredBlock>) {
      const txs = block.getFilteredTransactionsList()
      const codes = txs.map(t => VALIDATION[t.getTxValidationCode()] ?? `code ${t.getTxValidationCode()}`)
      const valid = codes.filter(c => c === 'VALID').length
      publish({ kind: 'block', number: block.getNumber(), txCount: txs.length, valid, invalid: txs.length - valid, codes: [...new Set(codes.filter(c => c !== 'VALID'))] })
    }
  })().catch((error) => {
    feedStarted = false // the next viewer reopens the feed
    publish({ kind: 'status', detail: `block feed closed: ${(error as Error).message}` })
  })
  void (async () => {
    for await (const e of events as AsyncIterable<ChaincodeEvent>)
      publish({ kind: 'chaincode', name: e.eventName, txId: e.transactionId, block: Number(e.blockNumber) })
  })().catch((error) => {
    feedStarted = false
    publish({ kind: 'status', detail: `chaincode feed closed: ${(error as Error).message}` })
  })
}

// Ledger height per node from the operations endpoints, published on change.
let lastHeights = ''
async function pollMetrics(config: FabricRuntimeConfig) {
  const heights: Record<string, number> = {}
  for (const pair of config.ops.split(',')) {
    const [name, url] = pair.split('=') as [string, string]
    try {
      const text = await $fetch<string>(`${url}/metrics`, { responseType: 'text', timeout: 1500 })
      const m = text.match(new RegExp(`^ledger_blockchain_height\\{channel="${config.channelName}"\\} (\\d+)`, 'm'))
        ?? text.match(new RegExp(`^consensus_etcdraft_committed_block_number\\{channel="${config.channelName}"\\} (\\d+)`, 'm'))
      if (m)
        heights[name] = Number(m[1])
    }
    catch { /* node down or no metrics: leave it out */ }
  }
  const key = JSON.stringify(heights)
  if (key !== lastHeights) {
    lastHeights = key
    publish({ kind: 'metrics', heights })
  }
}

export default defineEventHandler(async (event) => {
  const config = useRuntimeConfig(event).fabric
  const stream = createEventStream(event)
  const send = (e: LiveEvent) => void stream.push(JSON.stringify(e))
  for (const e of recentEvents())
    send(e)
  const unsubscribe = subscribe(send)
  if (config.demoDir) {
    publish({ kind: 'status', detail: 'demo mode: no ledger feed; the live view needs the Fabric network' })
  }
  else {
    startLedgerFeed(config).catch((error) => {
      feedStarted = false
      publish({ kind: 'status', detail: `ledger feed unavailable: ${(error as Error).message}` })
    })
    void pollMetrics(config)
  }
  const timer = config.demoDir ? undefined : setInterval(() => void pollMetrics(config), 3000)
  stream.onClosed(() => {
    unsubscribe()
    if (timer)
      clearInterval(timer)
  })
  return stream.send()
})
