import { solidityKeccak256 } from "ethers/lib/utils";

export function b64ToHex(b64String: string, addPrefix: boolean = true) {
  const prefix = addPrefix ? "0x" : "";
  return prefix + Buffer.from(b64String, "base64").toString("hex");
}

export function hashString(str: string) {
  return solidityKeccak256(["string"], [str]);
}

export type UpdateData = [string, string[], number, number, string];

export type Log = {
  name: string;
  id: string;
  url: string;
};
