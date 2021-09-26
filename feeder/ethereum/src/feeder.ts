import RootWitnessClient from "./RootWitnessClient";
import { getEnv } from "./getEnv";
import { UpdateData } from "./utils";
import { logs } from "./logs";
import { getUpdateDataForLog } from "./getUpdateDataForLog";
import { ethers } from "ethers";

// Method to complete a single loop of feeding the Ethereum witness.
export async function feedOnce() {
  const { contractAddress, nodeUrl, privateKeyHex } = getEnv();
  console.log(`Processing for ${logs.size} logs: \n`, ...logs);
  const rootWitnessClient = await RootWitnessClient.create(
    contractAddress,
    nodeUrl,
    privateKeyHex
  );
  const pendingUpdateDatas = [...logs].map((log) =>
    getUpdateDataForLog(log, rootWitnessClient)
  );
  const updateDatas = (await Promise.all(pendingUpdateDatas)).filter(
    (updateData) => updateData != undefined
  ) as UpdateData[];
  const receipt = await rootWitnessClient.batchUpdate(updateDatas);
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

if (require.main === module) {
  feedOnce()
    .then(() => process.exit(0))
    .catch((error) => {
      console.error(error);
      process.exit(1);
    });
}
