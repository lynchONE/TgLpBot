// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/// @dev Minimal Uniswap V3 pool interface used by ZapV3V4Improved.
interface IUniswapV3Pool {
    function slot0()
        external
        view
        returns (
            uint160 sqrtPriceX96,
            int24 tick,
            uint16 observationIndex,
            uint16 observationCardinality,
            uint16 observationCardinalityNext,
            uint8 feeProtocol,
            bool unlocked
        );

    function fee() external view returns (uint24);
}

