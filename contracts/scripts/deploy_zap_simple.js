const { ethers } = require("hardhat");

async function main() {
    console.log("开始部署 ZapSimple 合约...");

    const ZapSimple = await ethers.getContractFactory("ZapSimple");
    const zapSimple = await ZapSimple.deploy();
    await zapSimple.waitForDeployment();

    const address = await zapSimple.getAddress();
    console.log("✅ ZapSimple 部署成功!");
    console.log("   合约地址:", address);

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
