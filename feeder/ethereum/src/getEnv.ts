import "dotenv/config";

export function getEnv() {
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
