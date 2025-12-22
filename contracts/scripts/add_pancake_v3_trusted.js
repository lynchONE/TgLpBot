const { ethers } = require("hardhat");

async function main() {
    console.log("🔧 Adding PancakeSwap V3 Position Manager to trusted list...");

    const zapAddress = process.env.ZAP_V3_ADDRESS || process.env.ZAP_V4_ADDRESS;
    if (!zapAddress) {
        console.error("❌ ZAP_V3_ADDRESS or ZAP_V4_ADDRESS not found in environment variables.");
        process.exit(1);
    }
    console.log(`   Contract: ${zapAddress}`);

    const pancakeV3NPM = process.env.PANCAKE_V3_NPM_ADDRESS || "0x46A15B0b27311cedF172AB29E4f4766fbE7F4364";
    console.log(`   PancakeSwap V3 NPM: ${pancakeV3NPM}`);

    const ZapSimple = await ethers.getContractFactory("ZapSimple");
    const zap = ZapSimple.attach(zapAddress);

    // Check current whitelist status
    const isTrusted = await zap.trustedV3PositionManagers(pancakeV3NPM);
    console.log(`\n📋 Current Status: ${isTrusted ? "✅ Already trusted" : "❌ Not trusted"}`);

    if (isTrusted) {
        console.log("✅ PancakeSwap V3 NPM is already in the trusted list, no action needed.");
        return;
    }

    console.log(`\n🚀 Adding PancakeSwap V3 NPM to trusted list...`);

    // Call setTrustedV3PositionManagers([pancakeV3NPM], true)
    const tx = await zap.setTrustedV3PositionManagers([pancakeV3NPM], true);
    console.log(`⏳ Transaction sent: ${tx.hash}`);
    await tx.wait();
    console.log("✅ PancakeSwap V3 NPM successfully added to trusted list!");

    // Verify
    const isNowTrusted = await zap.trustedV3PositionManagers(pancakeV3NPM);
    console.log(`\n✅ Verification: ${isNowTrusted ? "SUCCESS - Trusted" : "FAILED - Not trusted"}`);
}

main()
    .then(() => process.exit(0))
    .catch((error) => {
        console.error(error);
        process.exit(1);
    });
