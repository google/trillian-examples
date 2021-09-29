import "dotenv/config";
import { solidityKeccak256 } from "ethers/lib/utils";

// Helper to read env vars.
export function getEnv(): Env {
  const {
    CONTRACT_ADDRESS: contractAddress = "",
    NODE_URL: nodeUrl = "",
    PRIVATE_KEY_HEX: privateKeyHex = "",
  } = process.env;

  return {
    contractAddress,
    nodeUrl,
    privateKeyHex,
  };
}

// Helper to convert base64 strings to hexidecimal, optionally adding a `0x` prefix.
export function b64ToHex(b64String: string, addPrefix: boolean = true) {
  const prefix = addPrefix ? "0x" : "";
  return prefix + Buffer.from(b64String, "base64").toString("hex");
}

// Helper to hash a string using Solidity's keccak256 hash function.
export function hashString(str: string) {
  return solidityKeccak256(["string"], [str]);
}

type Env = {
  contractAddress: string;
  nodeUrl: string;
  privateKeyHex: string;
};

export type UpdateData = [string, string[], number, number, string];

export type Log = {
  name: string;
  id: string;
  url: string;
};
