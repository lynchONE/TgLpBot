// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/access/Ownable.sol";
import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";

import "./interfaces/IPermit2.sol";
import "./interfaces/v4/Actions.sol";
import "./interfaces/v4/Currency.sol";
import "./interfaces/v4/IPositionManager.sol";
import "./interfaces/v4/IStateView.sol";
import "./interfaces/v4/PoolKey.sol";
import "./libraries/FullMath.sol";
import "./libraries/LiquidityAmounts.sol";
import "./libraries/TickMath.sol";

interface IUniswapV3PoolLike {
    function fee() external view returns (uint24);
    function token0() external view returns (address);
    function token1() external view returns (address);
}

interface IV3PositionManagerLike {
    struct IncreaseLiquidityParams {
        uint256 tokenId;
        uint256 amount0Desired;
        uint256 amount1Desired;
        uint256 amount0Min;
        uint256 amount1Min;
        uint256 deadline;
    }

    function increaseLiquidity(IncreaseLiquidityParams calldata params) external payable returns (
        uint128 liquidity,
        uint256 amount0,
        uint256 amount1
    );

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

    function ownerOf(uint256 tokenId) external view returns (address);
    function getApproved(uint256 tokenId) external view returns (address);
    function isApprovedForAll(address owner, address operator) external view returns (bool);
}

interface IERC721Like {
    function ownerOf(uint256 tokenId) external view returns (address);
    function getApproved(uint256 tokenId) external view returns (address);
    function isApprovedForAll(address owner, address operator) external view returns (bool);
}

interface IWrappedNativeAtomic is IERC20 {
    function deposit() external payable;
    function withdraw(uint256 amount) external;
}

contract AtomicIncreaseZap is ReentrancyGuard, Ownable {
    using SafeERC20 for IERC20;

    uint256 private constant BPS_DENOMINATOR = 10_000;
    address private constant PERMIT2 = 0x000000000022D473030F116dDEE9F6B43aC78BA3;
    bytes4 private constant PERMIT2_ALLOWANCE_IS_FIXED_AT_INFINITY = 0x3f68539a;

    address public okxSwapRouter;
    address public okxTokenApprove;
    mapping(address => bool) public trustedSwapTargets;
    mapping(address => bool) public trustedApproveTargets;
    address public v3PositionManager;
    mapping(address => bool) public trustedV3PositionManagers;
    address public v4PositionManager;
    address public wrappedNative;

    event TrustedAddressesUpdated(
        address okxSwapRouter,
        address okxTokenApprove,
        address v3PositionManager,
        address v4PositionManager
    );

    event TrustedV3PositionManagerUpdated(address indexed positionManager, bool trusted);
    event TrustedSwapTargetUpdated(address indexed target, bool trusted);
    event TrustedApproveTargetUpdated(address indexed target, bool trusted);
    event WrappedNativeUpdated(address indexed wrappedNative);

    event ZapIncreaseV3(
        address indexed user,
        address indexed pool,
        uint256 indexed tokenId,
        uint256 amount0,
        uint256 amount1,
        uint128 liquidity
    );

    event ZapIncreaseV4(
        address indexed user,
        bytes32 indexed poolId,
        uint256 indexed tokenId,
        uint256 amount0,
        uint256 amount1,
        uint128 liquidity
    );

    event SwapExecuted(
        address indexed target,
        address tokenIn,
        address tokenOut,
        uint256 amountIn,
        uint256 amountOut
    );

    struct SwapParams {
        address target;
        address approveTarget;
        address tokenIn;
        address tokenOut;
        uint256 amountIn;
        uint256 minAmountOut;
        bytes callData;
    }

    struct FundingParams {
        address token;
        uint256 amount;
    }

    struct PoolKeySimple {
        address currency0;
        address currency1;
        uint24 fee;
        int24 tickSpacing;
        address hooks;
    }

    struct ZapIncreaseV3Params {
        address pool;
        address positionManager;
        uint256 tokenId;
        FundingParams funding;
        SwapParams entrySwap;
        SwapParams rebalanceSwap;
    }

    struct ZapIncreaseV4Params {
        PoolKeySimple poolKey;
        address stateView;
        address positionManager;
        uint256 tokenId;
        int24 tickLower;
        int24 tickUpper;
        uint256 slippageBps;
        FundingParams funding;
        SwapParams entrySwap;
        SwapParams rebalanceSwap;
        uint160 sqrtPriceX96;
    }

    struct ZapResult {
        uint256 tokenId;
        uint128 liquidity;
        uint256 amount0Used;
        uint256 amount1Used;
        uint256 dust0;
        uint256 dust1;
    }

    constructor() Ownable(msg.sender) {}

    function setTrustedAddresses(
        address _okxSwapRouter,
        address _okxTokenApprove,
        address _v3PositionManager,
        address _v4PositionManager
    ) external onlyOwner {
        okxSwapRouter = _okxSwapRouter;
        okxTokenApprove = _okxTokenApprove;
        v3PositionManager = _v3PositionManager;
        v4PositionManager = _v4PositionManager;
        if (_okxSwapRouter != address(0)) {
            trustedSwapTargets[_okxSwapRouter] = true;
            emit TrustedSwapTargetUpdated(_okxSwapRouter, true);
        }
        if (_okxTokenApprove != address(0)) {
            trustedApproveTargets[_okxTokenApprove] = true;
            emit TrustedApproveTargetUpdated(_okxTokenApprove, true);
        }
        emit TrustedAddressesUpdated(_okxSwapRouter, _okxTokenApprove, _v3PositionManager, _v4PositionManager);
    }

    function setTrustedSwapTargets(address[] calldata targets, bool trusted) external onlyOwner {
        for (uint256 i = 0; i < targets.length; i++) {
            address target = targets[i];
            require(target != address(0), "bad swap target");
            trustedSwapTargets[target] = trusted;
            emit TrustedSwapTargetUpdated(target, trusted);
        }
    }

    function setTrustedApproveTargets(address[] calldata targets, bool trusted) external onlyOwner {
        for (uint256 i = 0; i < targets.length; i++) {
            address target = targets[i];
            require(target != address(0), "bad approve target");
            trustedApproveTargets[target] = trusted;
            emit TrustedApproveTargetUpdated(target, trusted);
        }
    }

    function setTrustedV3PositionManagers(address[] calldata positionManagers, bool trusted) external onlyOwner {
        for (uint256 i = 0; i < positionManagers.length; i++) {
            address pm = positionManagers[i];
            require(pm != address(0), "bad pm");
            trustedV3PositionManagers[pm] = trusted;
            emit TrustedV3PositionManagerUpdated(pm, trusted);
        }
    }

    function setWrappedNative(address _wrappedNative) external onlyOwner {
        require(_wrappedNative != address(0), "bad wrapped");
        wrappedNative = _wrappedNative;
        emit WrappedNativeUpdated(_wrappedNative);
    }

    function zapIncreaseV3(ZapIncreaseV3Params calldata params)
        external
        onlyOwner
        nonReentrant
        returns (ZapResult memory result)
    {
        require(params.pool != address(0), "bad pool");
        require(params.positionManager != address(0), "bad pm");
        require(params.funding.token != address(0), "bad funding");
        require(params.funding.amount > 0, "zero amount");
        require(v3PositionManager != address(0), "v3 pm unset");
        require(
            params.positionManager == v3PositionManager || trustedV3PositionManagers[params.positionManager],
            "untrusted pm"
        );

        IV3PositionManagerLike npm = IV3PositionManagerLike(params.positionManager);
        address owner = npm.ownerOf(params.tokenId);
        require(owner == msg.sender, "not owner");
        require(
            npm.getApproved(params.tokenId) == address(this) || npm.isApprovedForAll(owner, address(this)),
            "nft not approved"
        );

        (
            uint96 _nonce,
            address _operator,
            address token0,
            address token1,
            uint24 fee,
            int24 tickLower,
            int24 tickUpper,
            uint128 currentLiquidity,
            uint256 _feeGrowthInside0LastX128,
            uint256 _feeGrowthInside1LastX128,
            uint128 _tokensOwed0,
            uint128 _tokensOwed1
        ) = npm.positions(params.tokenId);
        _nonce;
        _operator;
        _feeGrowthInside0LastX128;
        _feeGrowthInside1LastX128;
        _tokensOwed0;
        _tokensOwed1;

        require(currentLiquidity > 0, "no liquidity");
        require(token0 < token1, "bad tokens");
        require(IUniswapV3PoolLike(params.pool).token0() == token0, "pool token0");
        require(IUniswapV3PoolLike(params.pool).token1() == token1, "pool token1");
        require(IUniswapV3PoolLike(params.pool).fee() == fee, "pool fee");
        require(tickLower < tickUpper, "bad ticks");

        uint256 fundingBalBefore = IERC20(params.funding.token).balanceOf(address(this));
        uint256 token0BalBefore = IERC20(token0).balanceOf(address(this));
        uint256 token1BalBefore = IERC20(token1).balanceOf(address(this));

        IERC20(params.funding.token).safeTransferFrom(msg.sender, address(this), params.funding.amount);
        uint256 fundingAvailable = IERC20(params.funding.token).balanceOf(address(this)) - fundingBalBefore;

        if (params.entrySwap.amountIn > 0 && params.entrySwap.callData.length > 0) {
            _validateTrustedSwap(params.entrySwap);
            require(params.entrySwap.tokenIn == params.funding.token, "entry tokenIn");
            require(params.entrySwap.tokenOut == token0 || params.entrySwap.tokenOut == token1, "entry tokenOut");
            require(params.entrySwap.amountIn <= fundingAvailable, "entry amount");
            _executeSwap(params.entrySwap);
        }

        uint256 rebalanceAvail0 = IERC20(token0).balanceOf(address(this)) - token0BalBefore;
        uint256 rebalanceAvail1 = IERC20(token1).balanceOf(address(this)) - token1BalBefore;
        if (params.rebalanceSwap.amountIn > 0 && params.rebalanceSwap.callData.length > 0) {
            _validateTrustedSwap(params.rebalanceSwap);
            require(
                (params.rebalanceSwap.tokenIn == token0 && params.rebalanceSwap.tokenOut == token1)
                    || (params.rebalanceSwap.tokenIn == token1 && params.rebalanceSwap.tokenOut == token0),
                "rebalance pair"
            );
            uint256 maxIn = params.rebalanceSwap.tokenIn == token0 ? rebalanceAvail0 : rebalanceAvail1;
            require(params.rebalanceSwap.amountIn <= maxIn, "rebalance amount");
            _executeSwap(params.rebalanceSwap);
        }

        uint256 amount0 = IERC20(token0).balanceOf(address(this)) - token0BalBefore;
        uint256 amount1 = IERC20(token1).balanceOf(address(this)) - token1BalBefore;
        require(amount0 > 0 || amount1 > 0, "no tokens");

        result = _increaseV3Position(params.positionManager, params.tokenId, token0, token1, amount0, amount1);
        result.dust0 = IERC20(token0).balanceOf(address(this)) - token0BalBefore;
        result.dust1 = IERC20(token1).balanceOf(address(this)) - token1BalBefore;

        _refundDelta(params.funding.token, msg.sender, fundingBalBefore);
        if (token0 != params.funding.token) {
            _refundDelta(token0, msg.sender, token0BalBefore);
        }
        if (token1 != params.funding.token && token1 != token0) {
            _refundDelta(token1, msg.sender, token1BalBefore);
        }

        emit ZapIncreaseV3(msg.sender, params.pool, params.tokenId, result.amount0Used, result.amount1Used, result.liquidity);
    }

    function zapIncreaseV4(ZapIncreaseV4Params calldata params)
        external
        onlyOwner
        nonReentrant
        returns (ZapResult memory result)
    {
        PoolKeySimple memory poolKey = params.poolKey;
        require(poolKey.currency0 < poolKey.currency1, "bad tokens");
        require(params.positionManager != address(0), "bad pm");
        require(v4PositionManager != address(0), "v4 pm unset");
        require(params.positionManager == v4PositionManager, "untrusted pm");
        require(params.stateView != address(0), "bad stateView");
        require(params.tickLower < params.tickUpper, "bad ticks");
        require(params.slippageBps <= BPS_DENOMINATOR, "slippage");
        require(params.funding.token != address(0), "bad funding");
        require(params.funding.amount > 0, "zero amount");

        IERC721Like nft = IERC721Like(params.positionManager);
        address owner = nft.ownerOf(params.tokenId);
        require(owner == msg.sender, "not owner");
        require(
            nft.getApproved(params.tokenId) == address(this) || nft.isApprovedForAll(owner, address(this)),
            "nft not approved"
        );

        uint256 fundingBalBefore = IERC20(params.funding.token).balanceOf(address(this));
        uint256 token0FundingBefore = _fundingBalanceForCurrency(poolKey.currency0);
        uint256 token1FundingBefore = _fundingBalanceForCurrency(poolKey.currency1);
        uint256 token0BalBefore = _balanceForCurrency(poolKey.currency0);
        uint256 token1BalBefore = _balanceForCurrency(poolKey.currency1);

        IERC20(params.funding.token).safeTransferFrom(msg.sender, address(this), params.funding.amount);
        uint256 fundingAvailable = IERC20(params.funding.token).balanceOf(address(this)) - fundingBalBefore;

        if (params.entrySwap.amountIn > 0 && params.entrySwap.callData.length > 0) {
            _validateTrustedSwap(params.entrySwap);
            require(params.entrySwap.tokenIn == params.funding.token, "entry tokenIn");
            require(
                _matchesPoolCurrency(params.entrySwap.tokenOut, poolKey.currency0)
                    || _matchesPoolCurrency(params.entrySwap.tokenOut, poolKey.currency1),
                "entry tokenOut"
            );
            require(params.entrySwap.amountIn <= fundingAvailable, "entry amount");
            _executeSwap(params.entrySwap);
        }

        uint256 rebalanceAvail0 = _fundingBalanceForCurrency(poolKey.currency0) - token0FundingBefore;
        uint256 rebalanceAvail1 = _fundingBalanceForCurrency(poolKey.currency1) - token1FundingBefore;
        if (params.rebalanceSwap.amountIn > 0 && params.rebalanceSwap.callData.length > 0) {
            _validateTrustedSwap(params.rebalanceSwap);
            require(
                (_matchesPoolCurrency(params.rebalanceSwap.tokenIn, poolKey.currency0)
                    && _matchesPoolCurrency(params.rebalanceSwap.tokenOut, poolKey.currency1))
                    || (_matchesPoolCurrency(params.rebalanceSwap.tokenIn, poolKey.currency1)
                        && _matchesPoolCurrency(params.rebalanceSwap.tokenOut, poolKey.currency0)),
                "rebalance pair"
            );
            uint256 maxIn = _matchesPoolCurrency(params.rebalanceSwap.tokenIn, poolKey.currency0)
                ? rebalanceAvail0
                : rebalanceAvail1;
            require(params.rebalanceSwap.amountIn <= maxIn, "rebalance amount");
            _executeSwap(params.rebalanceSwap);
        }

        uint256 amount0 = _fundingBalanceForCurrency(poolKey.currency0) - token0FundingBefore;
        uint256 amount1 = _fundingBalanceForCurrency(poolKey.currency1) - token1FundingBefore;
        require(amount0 > 0 || amount1 > 0, "no tokens");

        _unwrapForNativeCurrency(poolKey.currency0, amount0);
        _unwrapForNativeCurrency(poolKey.currency1, amount1);

        PoolKey memory v4PoolKey = PoolKey({
            currency0: Currency.wrap(poolKey.currency0),
            currency1: Currency.wrap(poolKey.currency1),
            fee: poolKey.fee,
            tickSpacing: poolKey.tickSpacing,
            hooks: poolKey.hooks
        });
        PoolId poolId = PoolIdLibrary.toId(v4PoolKey);
        (uint160 sqrtPriceX96, , , ) = IStateView(params.stateView).getSlot0(poolId);
        require(sqrtPriceX96 > 0, "bad sqrtPrice");

        if (params.sqrtPriceX96 > 0 && params.slippageBps > 0) {
            uint256 diff = uint256(params.sqrtPriceX96) > uint256(sqrtPriceX96)
                ? uint256(params.sqrtPriceX96) - uint256(sqrtPriceX96)
                : uint256(sqrtPriceX96) - uint256(params.sqrtPriceX96);
            require(FullMath.mulDiv(diff, BPS_DENOMINATOR, uint256(sqrtPriceX96)) <= params.slippageBps, "price moved");
        }

        result = _increaseV4Position(
            params.positionManager,
            params.tokenId,
            v4PoolKey,
            params.tickLower,
            params.tickUpper,
            amount0,
            amount1,
            sqrtPriceX96
        );
        result.dust0 = _balanceForCurrency(poolKey.currency0) - token0BalBefore;
        result.dust1 = _balanceForCurrency(poolKey.currency1) - token1BalBefore;
        result.amount0Used = amount0 > result.dust0 ? amount0 - result.dust0 : 0;
        result.amount1Used = amount1 > result.dust1 ? amount1 - result.dust1 : 0;

        _refundDelta(params.funding.token, msg.sender, fundingBalBefore);
        if (poolKey.currency0 != params.funding.token) {
            _refundDelta(poolKey.currency0, msg.sender, token0BalBefore);
        }
        if (poolKey.currency1 != params.funding.token && poolKey.currency1 != poolKey.currency0) {
            _refundDelta(poolKey.currency1, msg.sender, token1BalBefore);
        }

        emit ZapIncreaseV4(msg.sender, keccak256(abi.encode(v4PoolKey)), params.tokenId, result.amount0Used, result.amount1Used, result.liquidity);
    }

    function _increaseV3Position(
        address positionManager,
        uint256 tokenId,
        address token0,
        address token1,
        uint256 amount0,
        uint256 amount1
    ) internal returns (ZapResult memory result) {
        if (amount0 > 0) {
            IERC20(token0).forceApprove(positionManager, amount0);
        }
        if (amount1 > 0) {
            IERC20(token1).forceApprove(positionManager, amount1);
        }

        (uint128 liquidity, uint256 amount0Used, uint256 amount1Used) = IV3PositionManagerLike(positionManager).increaseLiquidity(
            IV3PositionManagerLike.IncreaseLiquidityParams({
                tokenId: tokenId,
                amount0Desired: amount0,
                amount1Desired: amount1,
                amount0Min: 0,
                amount1Min: 0,
                deadline: block.timestamp
            })
        );

        if (amount0 > 0) {
            IERC20(token0).forceApprove(positionManager, 0);
        }
        if (amount1 > 0) {
            IERC20(token1).forceApprove(positionManager, 0);
        }

        result.tokenId = tokenId;
        result.liquidity = liquidity;
        result.amount0Used = amount0Used;
        result.amount1Used = amount1Used;
        result.dust0 = amount0 - amount0Used;
        result.dust1 = amount1 - amount1Used;
    }

    function _increaseV4Position(
        address positionManager,
        uint256 tokenId,
        PoolKey memory poolKey,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0,
        uint256 amount1,
        uint160 sqrtPriceX96
    ) internal returns (ZapResult memory result) {
        address token0 = Currency.unwrap(poolKey.currency0);
        address token1 = Currency.unwrap(poolKey.currency1);

        require(amount0 <= type(uint128).max, "amount0 size");
        require(amount1 <= type(uint128).max, "amount1 size");

        if (amount0 > 0 && token0 != address(0)) {
            _forceApprovePermit2Infinity(token0);
            _permit2ApproveInfinity(token0, positionManager);
        }
        if (amount1 > 0 && token1 != address(0)) {
            _forceApprovePermit2Infinity(token1);
            _permit2ApproveInfinity(token1, positionManager);
        }

        uint128 liquidity = _estimateV4Liquidity(sqrtPriceX96, tickLower, tickUpper, amount0, amount1);
        require(liquidity > 0, "zero liquidity");

        uint256 nativeValue = 0;
        if (token0 == address(0) && amount0 > 0) {
            nativeValue += amount0;
        }
        if (token1 == address(0) && amount1 > 0) {
            nativeValue += amount1;
        }

        bytes memory unlockData = _buildV4IncreaseUnlockData(poolKey, tokenId, amount0, amount1);
        IPositionManager(positionManager).modifyLiquidities{value: nativeValue}(unlockData, block.timestamp + 300);

        result.tokenId = tokenId;
        result.liquidity = liquidity;
    }

    function _estimateV4Liquidity(
        uint160 sqrtPriceX96,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0,
        uint256 amount1
    ) internal pure returns (uint128 liquidity) {
        liquidity = uint128(
            LiquidityAmounts.getLiquidityForAmounts(
                sqrtPriceX96,
                TickMath.getSqrtRatioAtTick(tickLower),
                TickMath.getSqrtRatioAtTick(tickUpper),
                amount0,
                amount1
            )
        );
    }

    function _buildV4IncreaseUnlockData(PoolKey memory poolKey, uint256 tokenId, uint256 amount0, uint256 amount1)
        internal
        pure
        returns (bytes memory)
    {
        uint256 actionCount = 3;
        if (amount0 > 0) actionCount++;
        if (amount1 > 0) actionCount++;

        bytes memory actions = new bytes(actionCount);
        bytes[] memory params = new bytes[](actionCount);
        uint256 index = 0;

        if (amount0 > 0) {
            actions[index] = bytes1(Actions.SETTLE);
            params[index] = abi.encode(poolKey.currency0, amount0, true);
            index++;
        }
        if (amount1 > 0) {
            actions[index] = bytes1(Actions.SETTLE);
            params[index] = abi.encode(poolKey.currency1, amount1, true);
            index++;
        }

        actions[index] = bytes1(Actions.INCREASE_LIQUIDITY_FROM_DELTAS);
        params[index] = abi.encode(tokenId, uint128(amount0), uint128(amount1), bytes(""));
        index++;

        actions[index] = bytes1(Actions.CLOSE_CURRENCY);
        params[index] = abi.encode(poolKey.currency0);
        index++;

        actions[index] = bytes1(Actions.CLOSE_CURRENCY);
        params[index] = abi.encode(poolKey.currency1);

        return abi.encode(actions, params);
    }

    function _requireWrappedNative() internal view returns (address) {
        address token = wrappedNative;
        require(token != address(0), "wrapped unset");
        return token;
    }

    function _fundingTokenForCurrency(address currency) internal view returns (address) {
        return currency == address(0) ? _requireWrappedNative() : currency;
    }

    function _fundingBalanceForCurrency(address currency) internal view returns (uint256) {
        return IERC20(_fundingTokenForCurrency(currency)).balanceOf(address(this));
    }

    function _balanceForCurrency(address currency) internal view returns (uint256) {
        return currency == address(0) ? address(this).balance : IERC20(currency).balanceOf(address(this));
    }

    function _unwrapForNativeCurrency(address currency, uint256 amount) internal {
        if (currency == address(0) && amount > 0) {
            IWrappedNativeAtomic(_requireWrappedNative()).withdraw(amount);
        }
    }

    function _matchesPoolCurrency(address token, address currency) internal view returns (bool) {
        if (currency == address(0)) {
            return token == _requireWrappedNative();
        }
        return token == currency;
    }

    function _validateTrustedSwap(SwapParams calldata swap) internal view {
        require(swap.target != address(0), "swap target");
        require(swap.tokenIn != address(0) && swap.tokenOut != address(0), "swap token");
        require(swap.tokenIn != swap.tokenOut, "swap same");
        require(swap.amountIn > 0, "swap amount");
    }

    function _executeSwap(SwapParams calldata swap) internal {
        require(swap.amountIn > 0, "swap amount");
        require(swap.target != address(0), "swap target");

        address spender = swap.approveTarget != address(0) ? swap.approveTarget : swap.target;
        IERC20(swap.tokenIn).forceApprove(spender, swap.amountIn);

        uint256 balBefore = IERC20(swap.tokenOut).balanceOf(address(this));
        (bool success, bytes memory returnData) = swap.target.call(swap.callData);
        if (!success) {
            if (returnData.length > 0) {
                assembly {
                    revert(add(returnData, 32), mload(returnData))
                }
            } else {
                revert("swap failed");
            }
        }

        uint256 balAfter = IERC20(swap.tokenOut).balanceOf(address(this));
        uint256 amountOut = balAfter - balBefore;
        require(amountOut >= swap.minAmountOut, "swap output");

        IERC20(swap.tokenIn).forceApprove(spender, 0);
        emit SwapExecuted(swap.target, swap.tokenIn, swap.tokenOut, swap.amountIn, amountOut);
    }

    function _refundDelta(address token, address to, uint256 balanceBefore) internal {
        uint256 balanceAfter = _balanceForCurrency(token);
        uint256 delta = balanceAfter - balanceBefore;
        if (delta > 0) {
            if (token == address(0)) {
                (bool ok, ) = payable(to).call{value: delta}("");
                require(ok, "native refund");
            } else {
                IERC20(token).safeTransfer(to, delta);
            }
        }
    }

    receive() external payable {}

    function _permit2ApproveInfinity(address token, address spender) internal {
        (uint160 allowedAmount, uint48 allowedExpiration, ) = IPermit2(PERMIT2).allowance(address(this), token, spender);
        if (allowedAmount == type(uint160).max && allowedExpiration == type(uint48).max) {
            return;
        }

        try IPermit2(PERMIT2).approve(token, spender, type(uint160).max, type(uint48).max) {
            return;
        } catch (bytes memory reason) {
            bytes4 selector;
            if (reason.length >= 4) {
                assembly {
                    selector := mload(add(reason, 32))
                }
            }
            if (selector == PERMIT2_ALLOWANCE_IS_FIXED_AT_INFINITY) {
                return;
            }
            assembly {
                revert(add(reason, 32), mload(reason))
            }
        }
    }

    function _forceApprovePermit2Infinity(address token) internal {
        if (IERC20(token).allowance(address(this), PERMIT2) == type(uint256).max) {
            return;
        }
        IERC20(token).forceApprove(PERMIT2, type(uint256).max);
    }
}
