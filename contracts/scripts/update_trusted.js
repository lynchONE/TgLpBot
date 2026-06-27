const { ethers, network } = require("hardhat");
const {
  getNetworkPrefixes,
  usesGlobalFallback,
  readZapAddressForNetwork,
  resolveTrustedConfigForNetwork,
  getWrappedNativeForNetwork,
} = require("./utils/network_env");

async function main() {
  const networkName = network.name;
  const prefixes = getNetworkPrefixes(networkName);
  const preferredPrefix = prefixes[0];

  console.log(`Updating ZapSimple trusted addresses on network: ${networkName}`);
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
  console.log("Contract:", zapAddress);

  const trusted = resolveTrustedConfigForNetwork(networkName);
  const okxRouter = trusted.okxRouter;
  const okxApprove = trusted.okxApprove;
  const v3pm = trusted.v3Primary;
  const v4pm = trusted.v4pm;
  const wrappedNative = getWrappedNativeForNetwork(networkName);

  if (!okxRouter || !okxApprove || !v3pm) {
    console.error("Missing required env keys:");
    for (const hint of trusted.missingHints) {
      console.error(`- ${hint}`);
    }
    if (trusted.family === "base") {
      console.error(
        `- ${preferredPrefix} uses Uniswap/Aerodrome V3 managers only (no Pancake for Base networks)`
      );
    }
    process.exit(1);
  }

  const ZapSimple = await ethers.getContractFactory("ZapSimple");
  const zap = ZapSimple.attach(zapAddress);

  const currentRouter = await zap.okxSwapRouter();
  const currentApprove = await zap.okxTokenApprove();
  const currentV3PM = await zap.v3PositionManager();
  const currentV4PM = await zap.v4PositionManager();
  let currentWrappedNative = ethers.ZeroAddress;
  try {
    currentWrappedNative = await zap.wrappedNative();
  } catch (error) {
    console.log("Current contract does not expose wrappedNative(); redeploy ZapSimple before using native V4 pools.");
  }

  console.log("Current on-chain state:");
  console.log("- OKX Router:", currentRouter);
  console.log("- OKX TokenApprove:", currentApprove);
  console.log("- V3 PositionManager:", currentV3PM);
  console.log("- V4 PositionManager:", currentV4PM);
  console.log("- Wrapped Native:", currentWrappedNative);

  console.log("Applying new trusted config:");
  console.log("- OKX Router:", okxRouter);
  console.log("- OKX TokenApprove:", okxApprove);
  console.log("- Binance Swap Targets:", trusted.binanceSwapTargets.length ? trusted.binanceSwapTargets.join(", ") : "(none)");
  console.log("- Binance Approve Targets:", trusted.binanceApproveTargets.length ? trusted.binanceApproveTargets.join(", ") : "(none)");
  console.log("- V3 PositionManager:", v3pm);
  console.log("- V4 PositionManager:", v4pm || ethers.ZeroAddress);
  console.log("- Wrapped Native:", wrappedNative || ethers.ZeroAddress);

  const tx = await zap.setTrustedAddresses(okxRouter, okxApprove, v3pm, v4pm || ethers.ZeroAddress);
  console.log("setTrustedAddresses tx:", tx.hash);
  await tx.wait();
  console.log("Trusted addresses updated successfully.");

  const swapTargets = [okxRouter, ...trusted.binanceSwapTargets].filter((item) => ethers.isAddress(item));
  if (swapTargets.length > 0) {
    const txSwapTargets = await zap.setTrustedSwapTargets([...new Set(swapTargets.map((item) => ethers.getAddress(item)))], true);
    console.log("setTrustedSwapTargets tx:", txSwapTargets.hash);
    await txSwapTargets.wait();
    console.log("Trusted swap targets updated.");
  }

  const approveTargets = [okxApprove, ...trusted.binanceApproveTargets].filter((item) => ethers.isAddress(item));
  if (approveTargets.length > 0) {
    const txApproveTargets = await zap.setTrustedApproveTargets([...new Set(approveTargets.map((item) => ethers.getAddress(item)))], true);
    console.log("setTrustedApproveTargets tx:", txApproveTargets.hash);
    await txApproveTargets.wait();
    console.log("Trusted approve targets updated.");
  }

  if (wrappedNative) {
    try {
      const txWrapped = await zap.setWrappedNative(wrappedNative);
      console.log("setWrappedNative tx:", txWrapped.hash);
      await txWrapped.wait();
      console.log("Wrapped native updated successfully.");
    } catch (error) {
      console.log("setWrappedNative failed; redeploy ZapSimple with the latest contract before using native V4 pools.");
      throw error;
    }
  } else {
    console.log("Skipped setWrappedNative because wrapped native env is missing.");
  }

  // If multiple V3 position managers are configured for this chain family, allowlist additional ones.
  const uniqueExtras = trusted.v3Extras
    .filter((item) => ethers.isAddress(item))
    .filter((item) => item.toLowerCase() !== v3pm.toLowerCase());

  if (uniqueExtras.length > 0) {
    const tx2 = await zap.setTrustedV3PositionManagers(uniqueExtras, true);
    console.log("setTrustedV3PositionManagers tx:", tx2.hash);
    await tx2.wait();
    console.log("Extra trusted V3 position managers:", uniqueExtras.join(", "));
  } else {
    console.log("No extra V3 position managers to update.");
  }
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
