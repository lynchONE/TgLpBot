const { ethers } = require("hardhat");

async function main() {
    console.log("开始部署 ZapSimple 合约...");

    const ZapSimple = await ethers.getContractFactory("ZapSimple");
    const zapSimple = await ZapSimple.deploy();
    await zapSimple.waitForDeployment();

    const address = await zapSimple.getAddress();
    console.log("✅ ZapSimple 部署成功!");
    console.log("   合约地址:", address);

    // Optional: configure trusted addresses (recommended)
    const okxRouter = process.env.OKX_SWAP_ROUTER;
    const okxApprove = process.env.OKX_TOKEN_APPROVE_ADDRESS;
    const pancakeV3pm = process.env.PANCAKE_V3_NPM_ADDRESS;
    const uniswapV3pm = process.env.UNISWAP_V3_NPM_ADDRESS;
    const v3pm =
        process.env.V3_POSITION_MANAGER_ADDRESS ||
        pancakeV3pm ||
        uniswapV3pm;
    const v4pm = process.env.UNISWAP_V4_POSITION_MANAGER_ADDRESS || process.env.V4_POSITION_MANAGER_ADDRESS;

    if (okxRouter && okxApprove && v3pm) {
        console.log("\n🔧 正在设置合约 TrustedAddresses...");
        const tx = await zapSimple.setTrustedAddresses(okxRouter, okxApprove, v3pm, v4pm || ethers.ZeroAddress);
        console.log("   tx:", tx.hash);
        await tx.wait();
        console.log("✅ TrustedAddresses 已设置");
        console.log("   OKX Router:", okxRouter);
        console.log("   OKX TokenApprove:", okxApprove);
        console.log("   V3 PositionManager:", v3pm);
        console.log("   V4 PositionManager:", v4pm || ethers.ZeroAddress);

        // If both PancakeV3 + UniswapV3 NPMs are provided, allowlist the "other" one too.
        const extras = [pancakeV3pm, uniswapV3pm]
            .map((a) => (a || "").trim())
            .filter((a) => a && ethers.isAddress(a) && a.toLowerCase() !== v3pm.toLowerCase());
        const uniqueExtras = [...new Map(extras.map((a) => [a.toLowerCase(), a])).values()];

        if (uniqueExtras.length > 0) {
            console.log("\n🔧 正在设置额外 Trusted V3 PositionManagers...");
            const tx2 = await zapSimple.setTrustedV3PositionManagers(uniqueExtras, true);
            console.log("   tx:", tx2.hash);
            await tx2.wait();
            console.log("✅ 额外 V3 PositionManagers 已设置:", uniqueExtras.join(", "));
        }
    } else {
        console.log("\n⚠️  未设置 TrustedAddresses（缺少 env：OKX_SWAP_ROUTER / OKX_TOKEN_APPROVE_ADDRESS / V3_POSITION_MANAGER_ADDRESS 或 PANCAKE_V3_NPM_ADDRESS 或 UNISWAP_V3_NPM_ADDRESS）");
    }

    console.log("\n📝 请更新 .env 文件:");
    console.log(`   ZAP_V3_ADDRESS=${address}`);
    console.log(`   ZAP_V4_ADDRESS=${address}`);

    // Verify on BSCScan (if API key is set)
    if (process.env.BSCSCAN_API_KEY) {
        console.log("\n等待区块确认后验证合约...");
        await new Promise(resolve => setTimeout(resolve, 30000));

        try {
            await run("verify:verify", {
                address: address,
                constructorArguments: [],
            });
            console.log("✅ 合约已在 BSCScan 上验证");
        } catch (error) {
            console.log("验证失败:", error.message);
        }
    }
}

main()
    .then(() => process.exit(0))
    .catch((error) => {
        console.error(error);
        process.exit(1);
    });
