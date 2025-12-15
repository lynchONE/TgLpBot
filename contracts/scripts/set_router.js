const { ethers } = require("hardhat");

async function main() {
    // Zap 合约地址
    const zapAddress = "0x83293050C7641FDD213227D5f5378BE5d827eB0D";

    // Uniswap V3 SwapRouter02 地址
    const uniswapV3SwapRouter = "0xB971eF87ede563556b2ED4b1C0b0019111Dd85d2";

    // PancakeSwap V3 SwapRouter 地址
    const pancakeV3SwapRouter = "0x1b81D678ffb9C0263b24A97847620C99d213eB14";

    console.log("获取 Zap 合约...");
    const Zap = await ethers.getContractFactory("ZapV3V4Improved");
    const zap = Zap.attach(zapAddress);

    console.log("设置 Uniswap V3 SwapRouter02 为 isRouter02=true...");
    let tx = await zap.setRouterV2(uniswapV3SwapRouter, true);
    await tx.wait();
    console.log("✅ Uniswap V3 SwapRouter02 已设置, tx:", tx.hash);

    console.log("设置 PancakeSwap V3 SwapRouter 为 isRouter02=false (标准接口)...");
    tx = await zap.setRouterV2(pancakeV3SwapRouter, false);
    await tx.wait();
    console.log("✅ PancakeSwap V3 SwapRouter 已设置, tx:", tx.hash);

    console.log("\n✅ 所有 SwapRouter 设置完成!");
}

main()
    .then(() => process.exit(0))
    .catch((error) => {
        console.error(error);
        process.exit(1);
    });
