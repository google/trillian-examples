import { Feeder } from "./Feeder";
import { logs } from "./logs";
import RootWitnessClient from "./RootWitnessClient";
import { getEnv } from "./utils";

// Main entrypoint into the application.
async function main() {
  const { contractAddress, nodeUrl, privateKeyHex } = getEnv();

  const rootWitnessClient = await RootWitnessClient.create(
    contractAddress,
    nodeUrl,
    privateKeyHex
  );
  const feeder = new Feeder(rootWitnessClient, logs);
  await feeder.feedOnce();
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
