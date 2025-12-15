// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @notice Minimal Uniswap v4 PositionManager interface used by ZapV3V4Improved.
/// @dev Intentionally minimal to avoid bringing in v4-periphery remapping-only imports (e.g. "permit2/...") in Hardhat.
interface IPositionManager {
    function modifyLiquidities(bytes calldata unlockData, uint256 deadline) external payable;

    function nextTokenId() external view returns (uint256);
}

