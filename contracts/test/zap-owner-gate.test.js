const assert = require("assert");
const { ethers } = require("hardhat");

describe("Zap owner gate and aggregator targets", function () {
  async function deploySwapFixture(contractName = "ZapSimpleHarness") {
    const [owner, other] = await ethers.getSigners();
    const MockERC20 = await ethers.getContractFactory("MockERC20");
    const tokenIn = await MockERC20.deploy("Token In", "TIN", 18);
    const tokenOut = await MockERC20.deploy("Token Out", "TOUT", 18);
    await tokenIn.waitForDeployment();
    await tokenOut.waitForDeployment();

    const MockSwapTarget = await ethers.getContractFactory("MockSwapTarget");
    const swapTarget = await MockSwapTarget.deploy();
    await swapTarget.waitForDeployment();

    const Harness = await ethers.getContractFactory(contractName);
    const zap = await Harness.deploy();
    await zap.waitForDeployment();

    return { owner, other, tokenIn, tokenOut, swapTarget, zap };
  }

  async function expectRevert(promise, expectedText) {
    try {
      await promise;
    } catch (error) {
      assert(
        String(error.message || error).includes(expectedText),
        `expected revert containing ${expectedText}, got ${error.message || error}`
      );
      return;
    }
    assert.fail(`expected revert containing ${expectedText}`);
  }

  it("ZapSimple rejects non-owner zapInV3 before other validation", async function () {
    const { other, zap } = await deploySwapFixture();
    const params = {
      pool: ethers.ZeroAddress,
      positionManager: ethers.ZeroAddress,
      token0: ethers.ZeroAddress,
      token1: "0x0000000000000000000000000000000000000001",
      tickLower: -60,
      tickUpper: 60,
      recipient: other.address,
      amount0In: 0,
      amount1In: 0,
      swap: {
        target: ethers.ZeroAddress,
        approveTarget: ethers.ZeroAddress,
        tokenIn: ethers.ZeroAddress,
        tokenOut: ethers.ZeroAddress,
        amountIn: 0,
        minAmountOut: 0,
        callData: "0x",
      },
    };

    await expectRevert(zap.connect(other).zapInV3(params), "OwnableUnauthorizedAccount");
  });

  it("ZapSimple executes untrusted swap and approve targets for owner", async function () {
    const { owner, tokenIn, tokenOut, swapTarget, zap } = await deploySwapFixture();
    const amountIn = ethers.parseEther("1");
    const amountOut = ethers.parseEther("2");

    await tokenIn.mint(await zap.getAddress(), amountIn);
    await tokenOut.mint(await swapTarget.getAddress(), amountOut);

    const callData = swapTarget.interface.encodeFunctionData("swap", [
      await tokenIn.getAddress(),
      await tokenOut.getAddress(),
      amountIn,
      amountOut,
    ]);

    await zap.connect(owner).executeSwapForTest({
      target: await swapTarget.getAddress(),
      approveTarget: await swapTarget.getAddress(),
      tokenIn: await tokenIn.getAddress(),
      tokenOut: await tokenOut.getAddress(),
      amountIn,
      minAmountOut: amountOut,
      callData,
    });

    assert.equal((await tokenOut.balanceOf(await zap.getAddress())).toString(), amountOut.toString());
    assert.equal((await tokenIn.allowance(await zap.getAddress(), await swapTarget.getAddress())).toString(), "0");
  });

  it("AtomicIncreaseZap rejects non-owner zapIncreaseV3 before other validation", async function () {
    const { other, zap } = await deploySwapFixture("AtomicIncreaseZapHarness");
    const params = {
      pool: ethers.ZeroAddress,
      positionManager: ethers.ZeroAddress,
      tokenId: 1,
      funding: { token: ethers.ZeroAddress, amount: 0 },
      entrySwap: {
        target: ethers.ZeroAddress,
        approveTarget: ethers.ZeroAddress,
        tokenIn: ethers.ZeroAddress,
        tokenOut: ethers.ZeroAddress,
        amountIn: 0,
        minAmountOut: 0,
        callData: "0x",
      },
      rebalanceSwap: {
        target: ethers.ZeroAddress,
        approveTarget: ethers.ZeroAddress,
        tokenIn: ethers.ZeroAddress,
        tokenOut: ethers.ZeroAddress,
        amountIn: 0,
        minAmountOut: 0,
        callData: "0x",
      },
    };

    await expectRevert(zap.connect(other).zapIncreaseV3(params), "OwnableUnauthorizedAccount");
  });

  it("AtomicIncreaseZap executes untrusted swap and approve targets for owner", async function () {
    const { owner, tokenIn, tokenOut, swapTarget, zap } = await deploySwapFixture("AtomicIncreaseZapHarness");
    const amountIn = ethers.parseEther("1");
    const amountOut = ethers.parseEther("2");

    await tokenIn.mint(await zap.getAddress(), amountIn);
    await tokenOut.mint(await swapTarget.getAddress(), amountOut);

    const callData = swapTarget.interface.encodeFunctionData("swap", [
      await tokenIn.getAddress(),
      await tokenOut.getAddress(),
      amountIn,
      amountOut,
    ]);

    await zap.connect(owner).executeSwapForTest({
      target: await swapTarget.getAddress(),
      approveTarget: await swapTarget.getAddress(),
      tokenIn: await tokenIn.getAddress(),
      tokenOut: await tokenOut.getAddress(),
      amountIn,
      minAmountOut: amountOut,
      callData,
    });

    assert.equal((await tokenOut.balanceOf(await zap.getAddress())).toString(), amountOut.toString());
    assert.equal((await tokenIn.allowance(await zap.getAddress(), await swapTarget.getAddress())).toString(), "0");
  });
});
