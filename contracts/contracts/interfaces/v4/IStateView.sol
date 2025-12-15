// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "./PoolKey.sol";

interface IStateView {
    function getSlot0(PoolId poolId) external view returns (
        uint160 sqrtPriceX96,
        int24 tick,
        uint24 protocolFee,
        uint24 lpFee
    );
    
    function getLiquidity(PoolId poolId) external view returns (uint128);
    
    function getPosition(
        PoolId poolId,
        address owner,
        int24 tickLower,
        int24 tickUpper,
        bytes32 salt
    ) external view returns (uint128 liquidity, uint256 feeGrowthInside0LastX128, uint256 feeGrowthInside1LastX128);
}
