import LogClient from "./LogClient";
import RootWitnessClient from "./RootWitnessClient";
import { Log, UpdateData, hashString } from "./utils";

export async function getUpdateDataForLog(
  log: Log,
  rootWitnessClient: RootWitnessClient
): Promise<UpdateData | undefined> {
  console.log("Processing log\n", log);
  const { id, url } = log;
  const [
    { size: witnessedSize, timestamp: witnessedTimestamp },
    { hash, signature, treeSize: currentSize, timestamp },
  ] = await Promise.all([
    rootWitnessClient.getTreeDataById(id),
    LogClient.getSth(url),
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
  if (timestamp.toString() == witnessedTimestamp.toString()) {
    return undefined;
  }
  // If currently witnessed size is empty, the proof is only the new root.
  // Otherwise, get the consistency proof from the witnessed size to current via the log.
  const proof = witnessedSize.isZero()
    ? [hash]
    : await LogClient.getConsistencyProof(
        url,
        witnessedSize.toNumber(),
        currentSize
      );
  return [id, proof, currentSize, timestamp, hashString(signature)];
}
