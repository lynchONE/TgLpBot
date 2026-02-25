const { ethers, run, network } = require("hardhat");
const {
  getNetworkPrefixes,
  resolveTrustedConfigForNetwork,
  getExplorerApiKeyForNetwork,
  isTruthyEnv,
  getNativeSymbolForNetwork,
} = require("./utils/network_env");

async function main() {
  const networkName = network.name;
  const prefixes = getNetworkPrefixes(networkName);
  const nativeSymbol = getNativeSymbolForNetwork(networkName);
  const preferredPrefix = prefixes[0];

  console.log(`Deploying ZapSimple on network: ${networkName}`);
  console.log(`Environment prefixes (priority): ${prefixes.join(", ")}`);

  const [deployer] = await ethers.getSigners();
  const deployerAddress = await deployer.getAddress();
  const provider = deployer.provider;
  if (!provider) {
    throw new Error("No provider available (Hardhat runtime not initialized?)");
  }

  const balance = await provider.getBalance(deployerAddress);
  console.log("Deployer:", deployerAddress);
  console.log("Balance:", ethers.formatEther(balance), nativeSymbol);

  const ZapSimple = await ethers.getContractFactory("ZapSimple");

  // Optional pre-check: estimate deployment cost and fail fast if balance is insufficient.
  try {
    const deployTx = await ZapSimple.getDeployTransaction();
    const feeData = await provider.getFeeData();
    const gasPrice = process.env.GAS_PRICE_GWEI
      ? ethers.parseUnits(process.env.GAS_PRICE_GWEI, "gwei")
      : feeData.gasPrice || feeData.maxFeePerGas;

    if (gasPrice) {
      const gasEstimate = await provider.estimateGas({
        ...deployTx,
        from: deployerAddress,
      });

      const estimatedCost = gasEstimate * gasPrice;
      const bufferCost = estimatedCost + estimatedCost / 10n; // +10% buffer

      console.log("Estimated deploy gas:", gasEstimate.toString(), "gas");
      console.log("Estimated gas price:", ethers.formatUnits(gasPrice, "gwei"), "gwei");
      console.log(`Estimated deploy cost: ~${ethers.formatEther(estimatedCost)} ${nativeSymbol}`);

      if (balance < bufferCost) {
        const short = bufferCost - balance;
        console.error(
          `Insufficient deployer balance: need about ${ethers.formatEther(bufferCost)} ${nativeSymbol} (with 10% buffer), short ${ethers.formatEther(short)} ${nativeSymbol}.`
        );
        process.exit(1);
      }
    } else {
      console.log("Could not determine gas price; skipping balance pre-check.");
    }
  } catch (error) {
    console.log("Pre-check failed (will attempt deploy anyway):", error?.message || error);
  }

  const deployOverrides = {};
  if (process.env.GAS_PRICE_GWEI) {
    deployOverrides.gasPrice = ethers.parseUnits(process.env.GAS_PRICE_GWEI, "gwei");
    console.log("Using GAS_PRICE_GWEI override:", process.env.GAS_PRICE_GWEI, "gwei");
  }

  const zapSimple = await ZapSimple.deploy(deployOverrides);
  await zapSimple.waitForDeployment();
  const address = await zapSimple.getAddress();

  console.log("ZapSimple deployed successfully.");
  console.log("Contract address:", address);

  // Optional: configure trusted addresses (recommended)
  const trusted = resolveTrustedConfigForNetwork(networkName);
  const okxRouter = trusted.okxRouter;
  const okxApprove = trusted.okxApprove;
  const v3pm = trusted.v3Primary;
  const v4pm = trusted.v4pm;

  if (okxRouter && okxApprove && v3pm) {
    console.log("Setting trusted addresses...");
    const tx = await zapSimple.setTrustedAddresses(okxRouter, okxApprove, v3pm, v4pm || ethers.ZeroAddress);
    console.log("setTrustedAddresses tx:", tx.hash);
    await tx.wait();

    console.log("Trusted addresses set.");
    console.log("OKX Router:", okxRouter);
    console.log("OKX TokenApprove:", okxApprove);
    console.log("V3 PositionManager:", v3pm);
    console.log("V4 PositionManager:", v4pm || ethers.ZeroAddress);

    // If multiple V3 position managers are configured for this chain family, allowlist additional ones.
    const uniqueExtras = trusted.v3Extras
      .filter((item) => ethers.isAddress(item))
      .filter((item) => item.toLowerCase() !== v3pm.toLowerCase());

    if (uniqueExtras.length > 0) {
      console.log("Setting extra trusted V3 position managers...");
      const tx2 = await zapSimple.setTrustedV3PositionManagers(uniqueExtras, true);
      console.log("setTrustedV3PositionManagers tx:", tx2.hash);
      await tx2.wait();
      console.log("Extra trusted V3 position managers:", uniqueExtras.join(", "));
    }
  } else {
    console.log("Skipped setTrustedAddresses because required env is missing.");
    console.log(`Required keys for ${networkName}:`);
    for (const hint of trusted.missingHints) {
      console.log(`- ${hint}`);
    }
    if (trusted.family === "base") {
      console.log(
        `- ${preferredPrefix} uses Uniswap/Aerodrome V3 managers only (no Pancake for Base networks)`
      );
    }
  }

  console.log("Update bot env with:");
  console.log(`ZAP_V3_ADDRESS=${address}`);
  console.log(`ZAP_V4_ADDRESS=${address}`);
  console.log(`${preferredPrefix}_ZAP_V3_ADDRESS=${address}`);
  console.log(`${preferredPrefix}_ZAP_V4_ADDRESS=${address}`);

  // Verify if explicitly enabled, or if explorer API key is configured.
  const explorerApiKey = getExplorerApiKeyForNetwork(networkName);
  const shouldVerify = isTruthyEnv("VERIFY") || Boolean(explorerApiKey);
  if (shouldVerify) {
    if (!explorerApiKey) {
      console.log("VERIFY is enabled but explorer API key is missing; skipping verification.");
    } else {
      console.log("Waiting for block confirmations before verification...");
      await new Promise((resolve) => setTimeout(resolve, 30000));
      try {
        await run("verify:verify", {
          address,
          constructorArguments: [],
        });
        console.log(`Contract verified on explorer for ${networkName}.`);
      } catch (error) {
        console.log("Verification failed:", error?.message || error);
      }
    }
  }
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
