import got from "got";
import { b64ToHex } from "./utils";

// Encapsulates functionality related to interfacing with public log APIs.
// Performs operations like getting current STHs or calculating consistency proofs.
export default class LogClient {
  constructor(private readonly url: string) {}

  // Fetch the current STH for this log.
  async getSth(): Promise<GetSthResult> {
    const {
      tree_size: treeSize,
      timestamp,
      sha256_root_hash: hash,
      tree_head_signature: signature,
    } = await got.get(this.url + "get-sth").json();
    return {
      signature,
      treeSize,
      timestamp,
      hash: b64ToHex(hash),
    };
  }

  // Given an old and new size, fetch a consistency proof for this log.
  async getConsistencyProof(
    oldSize: number,
    newSize: number
  ): Promise<string[]> {
    const { consistency: proof } = await got
      .get(this.url + "get-sth-consistency", {
        searchParams: { first: oldSize, second: newSize },
      })
      .json();
    return proof.map((el: string) => b64ToHex(el));
  }
}

type GetSthResult = {
  hash: string;
  signature: string;
  timestamp: number;
  treeSize: number;
};
