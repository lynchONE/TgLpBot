// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @dev Minimal SwapRouter02 interface variant (no deadline in ExactInputSingle) used by ZapV3V4Improved.
interface ISwapRouter02 {
    struct ExactInputSingleParams {
        address tokenIn;
        address tokenOut;
        uint24 fee;
        address recipient;
        uint256 amountIn;
        uint256 amountOutMinimum;
        uint160 sqrtPriceLimitX96;
    }

    function exactInputSingle(ExactInputSingleParams calldata params) external payable returns (uint256 amountOut);
}

