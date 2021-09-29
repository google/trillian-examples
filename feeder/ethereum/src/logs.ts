import { hashString, Log } from "./utils";

// "Config in code" constant var of logs that should be fed.
export const logs: Log[] = [
  createSimpleLog("pilot", "https://ct.googleapis.com/pilot/ct/v1/"),
];

// Create a log with its ID generated via hashing its name.
function createSimpleLog(name: string, url: string) {
  return { name, url, id: hashString(name) };
}
