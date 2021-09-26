import { hashString, Log } from "./utils";

export const logs: Set<Log> = new Set([
  createDefaultLog("pilot", "https://ct.googleapis.com/pilot/ct/v1/"),
]);

function createDefaultLog(name: string, url: string) {
  return { name, url, id: hashString(name) };
}
