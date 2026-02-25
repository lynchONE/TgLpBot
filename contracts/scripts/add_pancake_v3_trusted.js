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

  if (networkName === "base" || networkName === "baseSepolia") {
    console.error("This script is BSC-only. Base networks should use Uniswap/Aerodrome managers.");
    process.exit(1);
  }

  console.log(`Adding trusted Pancake V3 PM on network: ${networkName}`);
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

  const pancakeV3NPM =
    readEnvForNetwork(networkName, "PANCAKE_V3_NPM_ADDRESS") ||
    "0x46A15B0b27311cedF172AB29E4f4766fbE7F4364";

  console.log("Contract:", zapAddress);
  console.log("Pancake V3 NPM:", pancakeV3NPM);

  const ZapSimple = await ethers.getContractFactory("ZapSimple");
  const zap = ZapSimple.attach(zapAddress);

  const isTrusted = await zap.trustedV3PositionManagers(pancakeV3NPM);
  if (isTrusted) {
    console.log("Already trusted. No update needed.");
    return;
  }

  const tx = await zap.setTrustedV3PositionManagers([pancakeV3NPM], true);
  console.log("setTrustedV3PositionManagers tx:", tx.hash);
  await tx.wait();

  const verified = await zap.trustedV3PositionManagers(pancakeV3NPM);
  console.log("Verification:", verified ? "SUCCESS" : "FAILED");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
