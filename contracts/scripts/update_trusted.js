const { ethers } = require("hardhat");

async function main() {
    console.log("🔧 Updating ZapSimple trusted addresses...");

    const zapAddress = process.env.ZAP_V3_ADDRESS || process.env.ZAP_V4_ADDRESS;
    if (!zapAddress) {
        console.error("❌ ZAP_V3_ADDRESS or ZAP_V4_ADDRESS not found in environment variables.");
        process.exit(1);
    }
    console.log(`   Contract: ${zapAddress}`);

    // Read new config from env
    const okxRouter = process.env.OKX_SWAP_ROUTER;
    const okxApprove = process.env.OKX_TOKEN_APPROVE_ADDRESS;

    // Position managers
    const pancakeV3pm = process.env.PANCAKE_V3_NPM_ADDRESS;
    const uniswapV3pm = process.env.UNISWAP_V3_NPM_ADDRESS;
    let v3pm = process.env.V3_POSITION_MANAGER_ADDRESS ||
        pancakeV3pm ||
        uniswapV3pm;

    const v4pm = process.env.UNISWAP_V4_POSITION_MANAGER_ADDRESS || process.env.V4_POSITION_MANAGER_ADDRESS;

    if (!okxRouter || !okxApprove || !v3pm) {
        console.error("❌ Missing required environment variables:");
        if (!okxRouter) console.error("   - OKX_SWAP_ROUTER");
        if (!okxApprove) console.error("   - OKX_TOKEN_APPROVE_ADDRESS");
        if (!v3pm) console.error("   - V3_POSITION_MANAGER_ADDRESS (or PANCAKE_V3_NPM_ADDRESS / UNISWAP_V3_NPM_ADDRESS)");
        console.log("Please update your .env file specific to contracts/ directory.");
        process.exit(1);
    }

    const ZapSimple = await ethers.getContractFactory("ZapSimple");
    const zap = ZapSimple.attach(zapAddress);

    // Check current values first
    const currentRouter = await zap.okxSwapRouter();
    const currentApprove = await zap.okxTokenApprove();
    const currentV3PM = await zap.v3PositionManager();

    console.log(`\n📋 Current On-Chain State:`);
    console.log(`   Router:  ${currentRouter}`);
    console.log(`   Approve: ${currentApprove}`);
    console.log(`   V3 PM:   ${currentV3PM}`);

    console.log(`\n🚀 Updating to New State:`);
    console.log(`   Router:  ${okxRouter}`);
    console.log(`   Approve: ${okxApprove}`);
    console.log(`   V3 PM:   ${v3pm}`);
    console.log(`   V4 PM:   ${v4pm || ethers.ZeroAddress}`);

    const tx = await zap.setTrustedAddresses(
        okxRouter,
        okxApprove,
        v3pm,
        v4pm || ethers.ZeroAddress
    );
    console.log(`\n⏳ Transaction sent: ${tx.hash}`);
    await tx.wait();
    console.log("✅ Configuration updated successfully!");

    // If both PancakeV3 + UniswapV3 NPMs are provided, allowlist the "other" one too.
    const extras = [pancakeV3pm, uniswapV3pm]
        .map((a) => (a || "").trim())
        .filter((a) => a && ethers.isAddress(a) && a.toLowerCase() !== v3pm.toLowerCase());
    const uniqueExtras = [...new Map(extras.map((a) => [a.toLowerCase(), a])).values()];

    if (uniqueExtras.length > 0) {
        console.log("\n🔧 Setting additional Trusted V3 PositionManagers...");
        const tx2 = await zap.setTrustedV3PositionManagers(uniqueExtras, true);
        console.log(`   tx: ${tx2.hash}`);
        await tx2.wait();
        console.log("✅ Additional V3 PositionManagers set:", uniqueExtras.join(", "));

        // Verify
        for (const pm of uniqueExtras) {
            const isTrusted = await zap.trustedV3PositionManagers(pm);
            console.log(`   ${pm}: ${isTrusted ? "✅ Trusted" : "❌ Not Trusted"}`);
        }
    } else {
        console.log("\n⚠️  No additional V3 PositionManagers to set (only one PM configured)");
    }
}

main()
    .then(() => process.exit(0))
    .catch((error) => {
        console.error(error);
        process.exit(1);
    });
