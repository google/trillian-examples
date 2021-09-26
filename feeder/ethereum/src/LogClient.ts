import got from "got";
import { b64ToHex } from "./utils";

type GetSthResult = {
  hash: string;
  signature: string;
  timestamp: number;
  treeSize: number;
};

export default class LogClient {
  constructor() {}

  static async getSth(logUrl: string): Promise<GetSthResult> {
    const {
      tree_size: treeSize,
      timestamp,
      sha256_root_hash: hash,
      tree_head_signature: signature,
    } = await got.get(logUrl + "get-sth").json();
    return {
      signature,
      treeSize,
      timestamp,
      hash: b64ToHex(hash),
    };
  }

  static async getConsistencyProof(
    logUrl: string,
    oldSize: number,
    newSize: number
  ): Promise<string[]> {
    const { consistency: proof } = await got
      .get(logUrl + "get-sth-consistency", {
        searchParams: { first: oldSize, second: newSize },
      })
      .json();
    return proof.map((el: string) => b64ToHex(el));
  }
}
