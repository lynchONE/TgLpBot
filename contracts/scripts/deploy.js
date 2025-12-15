const hre = require("hardhat");

async function main() {
  console.log("Deploying LiquidityZap contract...");

  // PancakeSwap Router V2 address
  const PANCAKE_ROUTER_V2 = "0x10ED43C718714eb63d5aA57B78B54704E256024E"; // Mainnet
  // For testnet use: 0xD99D1c33F9fC3444f8101754aBC46c52416550D1

  const LiquidityZap = await hre.ethers.getContractFactory("LiquidityZap");
  const liquidityZap = await LiquidityZap.deploy(PANCAKE_ROUTER_V2);

  await liquidityZap.waitForDeployment();

  const address = await liquidityZap.getAddress();
  console.log("LiquidityZap deployed to:", address);

  console.log("\n========================================");
  console.log("Deployment Summary");
  console.log("========================================");
  console.log("Contract Address:", address);
  console.log("Router Address:", PANCAKE_ROUTER_V2);
  console.log("Network:", hre.network.name);
  console.log("========================================\n");

  console.log("Add this to your .env file:");
  console.log(`ZAP_CONTRACT_ADDRESS=${address}`);

  // Wait for block confirmations before verifying
  if (hre.network.name !== "hardhat" && hre.network.name !== "localhost") {
    console.log("\nWaiting for block confirmations...");
    await liquidityZap.deploymentTransaction().wait(5);

    console.log("\nVerifying contract on BSCScan...");
    try {
      await hre.run("verify:verify", {
        address: address,
        constructorArguments: [PANCAKE_ROUTER_V2],
      });
      console.log("Contract verified successfully!");
    } catch (error) {
      console.log("Verification failed:", error.message);
    }
  }
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });

