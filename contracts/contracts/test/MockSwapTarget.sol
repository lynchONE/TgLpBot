// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./MockERC20.sol";

contract MockSwapTarget {
    function swap(address tokenIn, address tokenOut, uint256 amountIn, uint256 amountOut) external {
        MockERC20(tokenIn).transferFrom(msg.sender, address(this), amountIn);
        MockERC20(tokenOut).transfer(msg.sender, amountOut);
    }
}
