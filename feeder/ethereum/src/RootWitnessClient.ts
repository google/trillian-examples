import assert from "assert";
import fs from "fs-extra";
import { ethers } from "ethers";
import { isAddress } from "ethers/lib/utils";
import { UpdateData } from "./utils";

const abiPromise = fs.readJson("artifacts/RootWitness.json");

export default class RootWitnessClient {
  static readonly MAX_GAS_LIMIT: number = 6_000_000;
  static readonly MAX_MAX_PRIORITY_FEE_PER_GAS = Number(
    ethers.utils.parseUnits("2", "gwei")
  );
  static readonly MAX_MAX_FEE_PER_GAS = Number(
    ethers.utils.parseUnits("100", "gwei")
  );
  readonly provider: ethers.providers.JsonRpcProvider;
  private readonly wallet: ethers.Wallet;

  private constructor(
    private readonly contractAddress: string,
    nodeUrl: string,
    privateKeyHex: string
  ) {
    assert(isAddress(contractAddress));
    this.provider = new ethers.providers.JsonRpcProvider(nodeUrl);
    try {
      this.wallet = new ethers.Wallet(privateKeyHex, this.provider);
    } catch (e) {
      throw new Error(
        "EthClient: failed to generate wallet from privateKeyHex input"
      );
    }
  }

  // Public method for accessing wallet address.
  async getWalletAddress() {
    return this.wallet.getAddress();
  }

  async estimateFees() {
    const { maxFeePerGas, maxPriorityFeePerGas } =
      await this.provider.getFeeData();
    return {
      maxFeePerGas: Math.min(
        maxFeePerGas!.toNumber(),
        RootWitnessClient.MAX_MAX_FEE_PER_GAS
      ),
      maxPriorityFeePerGas: Math.min(
        maxPriorityFeePerGas!.toNumber(),
        RootWitnessClient.MAX_MAX_PRIORITY_FEE_PER_GAS
      ),
    };
  }

  private async getContractObject() {
    return new ethers.Contract(
      this.contractAddress,
      await abiPromise,
      this.wallet
    );
  }

  // Read from the contract the current state of a given treeId.
  async getTreeDataById(treeId: string): Promise<GetTreeDataByIdResponse> {
    console.log("Getting treeData for treeId ", treeId);
    const rootWitness = await this.getContractObject();
    const result = await rootWitness.getTreeDataById(treeId);
    console.log(`Got treeData for treeId ${treeId}\n`, result);
    return result;
  }

  // Submit a batchUpdate to the contract, updating many trees.
  async batchUpdate(
    updateDatas: UpdateData[]
  ): Promise<ethers.ContractReceipt> {
    console.log(
      `batchUpdate for ${updateDatas.length} updateDatas\n`,
      updateDatas
    );
    const rootWitness = await this.getContractObject();
    const estimatedGasLimit = await rootWitness.estimateGas.batchUpdate(
      updateDatas
    );
    const { maxFeePerGas, maxPriorityFeePerGas } = await this.estimateFees();
    const metaTxParams = {
      maxFeePerGas,
      maxPriorityFeePerGas,
      gasLimit: Math.min(
        estimatedGasLimit.toNumber(),
        RootWitnessClient.MAX_GAS_LIMIT
      ),
    };
    console.log(
      `Sending batchUpdate for ${updateDatas.length} updateDatas:\n`,
      metaTxParams
    );
    const tx = (await rootWitness.batchUpdate(
      updateDatas,
      metaTxParams
    )) as ethers.ContractTransaction;
    console.log(`batchUpdate sent\n`, { hash: tx.hash, nonce: tx.nonce });
    return await tx.wait();
  }

  static async create(
    contractAddress: string,
    nodeUrl: string,
    privateKeyHex: string
  ) {
    const client = new RootWitnessClient(
      contractAddress,
      nodeUrl,
      privateKeyHex
    );

    // Validate params and environment are set up properly.
    assert(Array.isArray(await abiPromise));
    try {
      console.log("Connected to EthClient with config", {
        contractAddress,
        nodeUrl,
        blockNum: await client.provider.getBlockNumber(),
        chainId: (await client.provider.getNetwork()).chainId,
        walletAddress: await client.getWalletAddress(),
      });
      return client;
    } catch (e) {
      console.error("Bad EthClient nodeUrl", nodeUrl);
      throw e;
    }
  }
}

type GetTreeDataByIdResponse = {
  root: string;
  size: ethers.BigNumber;
  timestamp: ethers.BigNumber;
  signature: string;
};
