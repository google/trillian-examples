import RootWitnessClient from "./RootWitnessClient";
import { hashString, Log, UpdateData } from "./utils";
import { BigNumber, ethers } from "ethers";
import CTLogClient from "./CTLogClient";

// Feeder encapsulates functionality related to feeding
// Ethereum-based RootWitness logs updates.
export class CTFeeder {
  constructor(
    private readonly rootWitnessClient: RootWitnessClient,
    private readonly logs: Log[]
  ) {}

  // Entrypoint into feeder;
  // used to run an iteration of feeding every log, once.
  async feedOnce() {
    console.log(`Processing for ${this.logs.length} logs: \n`, ...this.logs);
    const pendingUpdateDatas = [...this.logs].map((log) =>
      this.getUpdateDataForLog(log)
    );
    const updateDatas = (await Promise.all(pendingUpdateDatas)).filter(
      (updateData) => updateData != undefined
    ) as UpdateData[];
    const receipt = await this.rootWitnessClient.batchUpdate(updateDatas);
    const gasUsed = receipt.gasUsed.toNumber();
    const gasPrice = receipt.effectiveGasPrice.toNumber();
    const weiTxnCost = gasUsed * gasPrice;
    const ethTxnCost = ethers.utils.formatUnits(weiTxnCost, "ether");
    console.log(`Done processing for all IDs!`, {
      hash: receipt.transactionHash,
      gasUsed,
      gasPrice,
      weiTxnCost,
      ethTxnCost,
    });
  }

  // Helper to process a single log.
  // Returns either an UpdateData for a log that should be updated,
  // or `undefined` for logs that shouldn't be.
  async getUpdateDataForLog(log: Log): Promise<UpdateData | undefined> {
    console.log("Processing log\n", log);
    const { id, url } = log;
    const logClient = new CTLogClient(url);
    const [
      { size: witnessedSize, timestamp: witnessedTimestamp },
      { hash, signature, treeSize: currentSize, timestamp },
    ] = await Promise.all([
      this.rootWitnessClient.getTreeDataById(id),
      logClient.getSth(),
    ]);
    console.log(`Found current state of log ${id}`, {
      witnessedSize: witnessedSize.toNumber(),
      witnessedTimestamp: witnessedTimestamp.toNumber(),
      hash,
      signature,
      currentSize,
      timestamp,
      id,
    });
    // If the new timestamp isn't past the witness timestamp...
    if (witnessedTimestamp.gte(BigNumber.from(timestamp))) {
      // Don't update this log
      return undefined;
    }
    // If currently witnessed size is empty, the proof is only the new root.
    // Otherwise, get the consistency proof from the witnessed size to current via the log.
    const proof = witnessedSize.isZero()
      ? [hash]
      : await logClient.getConsistencyProof(
          witnessedSize.toNumber(),
          currentSize
        );
    return [id, proof, currentSize, timestamp, hashString(signature)];
  }
}
