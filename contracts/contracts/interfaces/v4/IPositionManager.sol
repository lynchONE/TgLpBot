// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @notice Minimal Uniswap v4 PositionManager interface used by ZapV3V4Improved.
/// @dev Intentionally minimal to avoid bringing in v4-periphery remapping-only imports (e.g. "permit2/...") in Hardhat.
interface IPositionManager {
    function modifyLiquidities(bytes calldata unlockData, uint256 deadline) external payable;

    function nextTokenId() external view returns (uint256);

    function positions(uint256 tokenId) external view returns (
        uint96 nonce,
        address operator,
        address token0,
        address token1,
        uint24 fee,
        int24 tickLower,
        int24 tickUpper,
        uint128 liquidity,
        uint256 feeGrowthInside0LastX128,
        uint256 feeGrowthInside1LastX128,
        uint128 tokensOwed0,
        uint128 tokensOwed1
    );
}
