// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "../ZapSimple.sol";

contract ZapSimpleHarness is ZapSimple {
    function executeSwapForTest(SwapParams calldata swap) external onlyOwner {
        _executeSwap(swap);
    }

    function validateSwapParamsForTest(
        address token0,
        address token1,
        SwapParams calldata swap,
        uint256 token0Available,
        uint256 token1Available
    ) external view {
        _validateSwapParams(token0, token1, swap, token0Available, token1Available);
    }
}
