import { hashString, Log } from "./utils";

export const logs: Log[] = [
  createSimpleLog("pilot", "https://ct.googleapis.com/pilot/ct/v1/"),
];

// Create a log with its ID generated via hashing its name.
function createSimpleLog(name: string, url: string) {
  return { name, url, id: hashString(name) };
}
