const { ethers, network } = require("hardhat");
const {
  getNetworkPrefixes,
  usesGlobalFallback,
  readZapAddressForNetwork,
} = require("./utils/network_env");

async function main() {
  const networkName = network.name;
  const prefixes = getNetworkPrefixes(networkName);
  const preferredPrefix = prefixes[0];

  console.log(`Checking ZapSimple config on network: ${networkName}`);
  console.log(`Environment prefixes (priority): ${prefixes.join(", ")}`);

  const zapAddress = readZapAddressForNetwork(networkName);
  if (!zapAddress) {
    const globalHint = usesGlobalFallback(networkName)
      ? ", or global ZAP_V3_ADDRESS / ZAP_V4_ADDRESS"
      : "";
    console.error(
      `Missing zap address. Set ${preferredPrefix}_ZAP_V3_ADDRESS / ${preferredPrefix}_ZAP_V4_ADDRESS${globalHint}.`
    );
    process.exit(1);
  }

  console.log("Contract address:", zapAddress);

  const ZapSimple = await ethers.getContractFactory("ZapSimple");
  const zap = ZapSimple.attach(zapAddress);

  try {
    const v3pm = await zap.v3PositionManager();
    const v4pm = await zap.v4PositionManager();
    let wrappedNative = ethers.ZeroAddress;
    try {
      wrappedNative = await zap.wrappedNative();
    } catch (error) {
      console.log("Current contract does not expose wrappedNative().");
    }

    console.log("On-chain configuration:");
    console.log(`- v3PositionManager: ${v3pm}`);
    console.log(`- v4PositionManager: ${v4pm}`);
    console.log(`- wrappedNative: ${wrappedNative}`);
  } catch (error) {
    console.error("Error reading contract:", error);
  }
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
