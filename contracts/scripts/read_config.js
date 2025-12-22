const { ethers } = require("hardhat");

async function main() {
    console.log("🔍 Checking ZapSimple contract configuration...");

    // Read address from env or args, default to current ZAP_V3_ADDRESS if available
    const zapAddress = process.env.ZAP_V3_ADDRESS || process.env.ZAP_V4_ADDRESS;
    if (!zapAddress) {
        console.error("❌ ZAP_V3_ADDRESS or ZAP_V4_ADDRESS not found in environment variables.");
        process.exit(1);
    }

    console.log(`Contract Address: ${zapAddress}`);

    const ZapSimple = await ethers.getContractFactory("ZapSimple");
    const zap = ZapSimple.attach(zapAddress);

    try {
        const router = await zap.okxSwapRouter();
        console.log(`\n📋 On-Chain Configuration:`);
        console.log(`   - okxSwapRouter:     ${router}`);

        const okxApprove = await zap.okxTokenApprove();
        console.log(`   - okxTokenApprove:   ${okxApprove}`);

        const v3pm = await zap.v3PositionManager();
        console.log(`   - v3PositionManager: ${v3pm}`);

        const v4pm = await zap.v4PositionManager();
        console.log(`   - v4PositionManager: ${v4pm}`);

        console.log("\n✅ Done.");

        // Check against env
        const envRouter = process.env.OKX_SWAP_ROUTER;
        if (envRouter && envRouter.toLowerCase() !== router.toLowerCase()) {
            console.log(`\n⚠️  MISMATCH WARNING:`);
            console.log(`   Environment OKX_SWAP_ROUTER: ${envRouter}`);
            console.log(`   Contract okxSwapRouter:      ${router}`);
            console.log(`   (This causes 'Untrusted swap target' error)`);
        } else if (!envRouter) {
            console.log(`\nℹ️  Environment OKX_SWAP_ROUTER is not set.`);
        } else {
            console.log(`\n✅ Environment OKX_SWAP_ROUTER matches contract.`);
        }

    } catch (e) {
        console.error("Error reading contract:", e);
    }
}

main()
    .then(() => process.exit(0))
    .catch((error) => {
        console.error(error);
        process.exit(1);
    });
