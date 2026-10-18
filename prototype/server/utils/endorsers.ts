import type { Transaction } from '@hyperledger/fabric-gateway'
import { common, gateway, msp, peer } from '@hyperledger/fabric-protos'

/** The MSPs whose peers signed an endorsed transaction, read from its envelope. */
export function endorsersOf(transaction: Transaction): string[] {
  try {
    const prepared = gateway.PreparedTransaction.deserializeBinary(transaction.getBytes())
    const payload = common.Payload.deserializeBinary(prepared.getEnvelope()!.getPayload_asU8())
    const tx = peer.Transaction.deserializeBinary(payload.getData_asU8())
    const action = peer.ChaincodeActionPayload.deserializeBinary(tx.getActionsList()[0]!.getPayload_asU8())
    return action.getAction()!.getEndorsementsList()
      .map(e => msp.SerializedIdentity.deserializeBinary(e.getEndorser_asU8()).getMspid())
  }
  catch {
    return []
  }
}
