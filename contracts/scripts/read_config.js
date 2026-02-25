const { ethers, network } = require("hardhat");
const {
  getNetworkPrefixes,
  usesGlobalFallback,
  readEnvForNetwork,
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
    const router = await zap.okxSwapRouter();
    const okxApprove = await zap.okxTokenApprove();
    const v3pm = await zap.v3PositionManager();
    const v4pm = await zap.v4PositionManager();

    console.log("On-chain configuration:");
    console.log(`- okxSwapRouter: ${router}`);
    console.log(`- okxTokenApprove: ${okxApprove}`);
    console.log(`- v3PositionManager: ${v3pm}`);
    console.log(`- v4PositionManager: ${v4pm}`);

    const envRouter = readEnvForNetwork(networkName, "OKX_SWAP_ROUTER");
    if (envRouter && envRouter.toLowerCase() !== router.toLowerCase()) {
      console.log("WARNING: environment router does not match contract router.");
      console.log("- env router:", envRouter);
      console.log("- contract router:", router);
    } else if (!envRouter) {
      const globalHint = usesGlobalFallback(networkName) ? " (or OKX_SWAP_ROUTER)" : "";
      console.log(`Environment router missing: ${preferredPrefix}_OKX_SWAP_ROUTER${globalHint}.`);
    } else {
      console.log("Environment router matches contract.");
    }
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
