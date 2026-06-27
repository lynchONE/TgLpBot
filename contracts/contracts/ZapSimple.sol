// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

// V4 interfaces and libraries
import "./interfaces/v4/Currency.sol";
import "./interfaces/v4/PoolKey.sol";
import "./interfaces/v4/Actions.sol";
import "./interfaces/v4/IPositionManager.sol";
import "./interfaces/v4/IStateView.sol";
import "./interfaces/IPermit2.sol";
import "./libraries/TickMath.sol";
import "./libraries/LiquidityAmounts.sol";
import "./libraries/FullMath.sol";

/**
 * @title ZapSimple
 * @notice 简化版 Zap 合约，支持 PancakeSwap V3、Uniswap V3/V4
 * @dev
 *   核心流程:
 *   1. Go 代码调用 calculateOptimalSwap() 获取需要 swap 的数量
 *   2. Go 代码用该数量调用 OKX API 获取 swap calldata
 *   3. Go 代码调用 zapIn() 原子执行 swap + add LP
 */

interface IUniswapV3Pool {
    function fee() external view returns (uint24);
    function token0() external view returns (address);
    function token1() external view returns (address);
}

interface INonfungiblePositionManager {
    struct MintParams {
        address token0;
        address token1;
        uint24 fee;
        int24 tickLower;
        int24 tickUpper;
        uint256 amount0Desired;
        uint256 amount1Desired;
        uint256 amount0Min;
        uint256 amount1Min;
        address recipient;
        uint256 deadline;
    }

    struct DecreaseLiquidityParams {
        uint256 tokenId;
        uint128 liquidity;
        uint256 amount0Min;
        uint256 amount1Min;
        uint256 deadline;
    }

    struct CollectParams {
        uint256 tokenId;
        address recipient;
        uint128 amount0Max;
        uint128 amount1Max;
    }

    function mint(MintParams calldata params) external payable returns (
        uint256 tokenId,
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

    function decreaseLiquidity(DecreaseLiquidityParams calldata params) external payable returns (uint256 amount0, uint256 amount1);
    function collect(CollectParams calldata params) external payable returns (uint256 amount0, uint256 amount1);
    function burn(uint256 tokenId) external payable;
    function safeTransferFrom(address from, address to, uint256 tokenId) external;
    function getApproved(uint256 tokenId) external view returns (address);
    function ownerOf(uint256 tokenId) external view returns (address);
    function isApprovedForAll(address owner, address operator) external view returns (bool);
}

interface IWrappedNative is IERC20 {
    function deposit() external payable;
    function withdraw(uint256 amount) external;
}

contract ZapSimple is ReentrancyGuard, Ownable {
    using SafeERC20 for IERC20;

    /*//////////////////////////////////////////////////////////////
                                CONSTANTS
    //////////////////////////////////////////////////////////////*/

    uint256 private constant Q96 = 2**96;

    uint256 private constant BPS_DENOMINATOR = 10_000;

    bytes4 private constant SLOT0_SELECTOR = 0x3850c7bd;

    /// @dev Custom error selector: Permit2AllowanceIsFixedAtInfinity()
    /// Some Permit2 deployments revert on approve() once allowance is set to infinity.
    bytes4 private constant PERMIT2_ALLOWANCE_IS_FIXED_AT_INFINITY = 0x3f68539a;

    /*//////////////////////////////////////////////////////////////
                                 CONFIG
    //////////////////////////////////////////////////////////////*/

    /// @notice Trusted V3 NonfungiblePositionManager (Pancake/Uniswap V3 style)
    address public v3PositionManager;

    /// @notice Optional allowlist for additional trusted V3 NPMs (to support multiple V3 deployments)
    mapping(address => bool) public trustedV3PositionManagers;

    /// @notice Trusted V4 PositionManager
    address public v4PositionManager;

    /// @notice Wrapped native token used as the ERC20 funding token for V4 native currency pools.
    address public wrappedNative;

    /*//////////////////////////////////////////////////////////////
                                 EVENTS
    //////////////////////////////////////////////////////////////*/

    event PositionManagersUpdated(address v3PositionManager, address v4PositionManager);

    event TrustedV3PositionManagerUpdated(address indexed positionManager, bool trusted);

    event WrappedNativeUpdated(address indexed wrappedNative);

    event ZapInV3(
        address indexed user,
        address indexed pool,
        uint256 indexed tokenId,
        uint256 amount0,
        uint256 amount1,
        uint128 liquidity
    );

    event ZapInV4(
        address indexed user,
        bytes32 indexed poolId,
        uint256 indexed tokenId,
        uint256 amount0,
        uint256 amount1,
        uint128 liquidity
    );

    event ZapOutV3(
        address indexed user,
        uint256 indexed tokenId,
        uint256 amount0,
        uint256 amount1
    );

    event ZapOutV4(
        address indexed user,
        uint256 indexed tokenId,
        uint256 amount0,
        uint256 amount1
    );

    event SwapExecuted(
        address indexed target,
        address tokenIn,
        address tokenOut,
        uint256 amountIn,
        uint256 amountOut
    );

    // DEBUG event for tracking execution progress
    event DebugStep(
        string step,
        uint256 value1,
        uint256 value2
    );

    /*//////////////////////////////////////////////////////////////
                               STRUCTS
    //////////////////////////////////////////////////////////////*/

    /// @notice OKX swap calldata
    struct SwapParams {
        address target;           // OKX Router 地址
        address approveTarget;    // OKX TokenApprove 地址
        address tokenIn;
        address tokenOut;
        uint256 amountIn;
        uint256 minAmountOut;
        bytes callData;           // OKX API 返回的 calldata
    }

    /// @notice V3 Zap 参数
    struct ZapInV3Params {
        address pool;             // V3 池子地址
        address positionManager;  // NPM 地址
        address token0;
        address token1;
        int24 tickLower;
        int24 tickUpper;
        address recipient;
        uint256 amount0In;        // 用户输入的 token0 数量
        uint256 amount1In;        // 用户输入的 token1 数量
        SwapParams swap;          // OKX swap 参数
    }

    /// @notice V4 PoolKey (简化版，使用 address)
    /// @dev 内部会转换为 Currency 类型的 PoolKey
    struct PoolKeySimple {
        address currency0;
        address currency1;
        uint24 fee;
        int24 tickSpacing;
        address hooks;
    }

    /// @notice V4 Zap 参数 (基于 ZapV3V4Improved 优化)
    struct ZapInV4Params {
        PoolKeySimple poolKey;
        address stateView;        // StateView 合约地址
        address positionManager;  // V4 PositionManager 地址
        int24 tickLower;
        int24 tickUpper;
        address recipient;
        uint256 amount0In;
        uint256 amount1In;
        uint256 slippageBps;      // 滑点保护 (100 = 1%)
        SwapParams swap;          // OKX swap 参数
        uint160 sqrtPriceX96;     // 当前价格提示 (用于链上价格偏差校验)
    }

    /// @notice Zap 结果
    struct ZapResult {
        uint256 tokenId;
        uint128 liquidity;
        uint256 amount0Used;
        uint256 amount1Used;
        uint256 dust0;
        uint256 dust1;
    }

    /*//////////////////////////////////////////////////////////////
                             CONSTRUCTOR
    //////////////////////////////////////////////////////////////*/

    constructor() Ownable(msg.sender) {}

    /*//////////////////////////////////////////////////////////////
                           CORE FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    /// @notice Sets position manager addresses (onlyOwner).
    function setPositionManagers(
        address _v3PositionManager,
        address _v4PositionManager
    ) external onlyOwner {
        v3PositionManager = _v3PositionManager;
        v4PositionManager = _v4PositionManager;
        emit PositionManagersUpdated(_v3PositionManager, _v4PositionManager);
    }

    /// @notice Adds/removes trusted V3 NPMs (onlyOwner).
    /// @dev `v3PositionManager` remains the primary/default, but calls can use any allowlisted manager.
    function setTrustedV3PositionManagers(address[] calldata positionManagers, bool trusted) external onlyOwner {
        for (uint256 i = 0; i < positionManagers.length; i++) {
            address pm = positionManagers[i];
            require(pm != address(0), "Invalid PM address");
            trustedV3PositionManagers[pm] = trusted;
            emit TrustedV3PositionManagerUpdated(pm, trusted);
        }
    }

    /// @notice Sets wrapped native token (WBNB/WETH) used for V4 pools whose PoolKey contains address(0).
    function setWrappedNative(address _wrappedNative) external onlyOwner {
        require(_wrappedNative != address(0), "Invalid wrapped native");
        wrappedNative = _wrappedNative;
        emit WrappedNativeUpdated(_wrappedNative);
    }

    /**
     * @notice 计算最优 swap 数量
     * @param pool V3 池子地址
     * @param tickLower 仓位下界
     * @param tickUpper 仓位上界
     * @param amount0In 输入的 token0 数量
     * @param amount1In 输入的 token1 数量
     * @return zeroForOne 是否从 token0 swap 到 token1
     * @return amountToSwap 需要 swap 的数量
     */
    function calculateOptimalSwap(
        address pool,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0In,
        uint256 amount1In
    ) external view returns (bool zeroForOne, uint256 amountToSwap) {
        uint160 sqrtPriceX96 = _getPoolSqrtPriceX96(pool);
        require(sqrtPriceX96 > 0, "Invalid SqrtPrice");
        
        // 计算 tick 范围的 sqrt 价格
        uint160 sqrtPriceLower = _getSqrtPriceAtTick(tickLower);
        uint160 sqrtPriceUpper = _getSqrtPriceAtTick(tickUpper);

        // 价格在范围外，目标是单边资产
        if (sqrtPriceX96 <= sqrtPriceLower) {
            // 目标是 token0，优先把 token1 全部换成 token0
            return (false, amount1In);
        }
        if (sqrtPriceX96 >= sqrtPriceUpper) {
            // 目标是 token1，优先把 token0 全部换成 token1
            return (true, amount0In);
        }

        // 将总输入统一换算为 token1 价值，避免单位混淆
        uint256 totalValueInToken1 = _calculateValueInToken1(amount0In, amount1In, sqrtPriceX96);
        if (totalValueInToken1 == 0) {
            return (true, 0);
        }

        uint256 ratio0 = uint256(sqrtPriceUpper) - uint256(sqrtPriceX96);
        uint256 ratio1 = uint256(sqrtPriceX96) - uint256(sqrtPriceLower);
        uint256 ratioSum = ratio0 + ratio1;
        if (ratioSum == 0) {
            return (true, 0);
        }

        // 以 token1 价值比例分配，再转换为 token0 数量
        uint256 value0InToken1 = FullMath.mulDiv(totalValueInToken1, ratio0, ratioSum);
        uint256 ideal0 = _amount0FromValueInToken1(value0InToken1, sqrtPriceX96);
        uint256 ideal1 = totalValueInToken1 - value0InToken1;

        // 确定 swap 方向和数量
        if (amount0In > ideal0 && ideal0 > 0) {
            // 需要把多余的 token0 换成 token1
            zeroForOne = true;
            amountToSwap = amount0In - ideal0;
        } else if (amount1In > ideal1 && ideal1 > 0) {
            // 需要把多余的 token1 换成 token0
            zeroForOne = false;
            amountToSwap = amount1In - ideal1;
        } else {
            // 不需要 swap
            zeroForOne = true;
            amountToSwap = 0;
        }

        return (zeroForOne, amountToSwap);
    }

    /**
     * @notice V3 开仓（原子执行 swap + add LP）
     */
    function zapInV3(ZapInV3Params calldata params)
        external
        onlyOwner
        nonReentrant
        returns (ZapResult memory result)
    {
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.pool != address(0), "Invalid pool");
        require(params.amount0In > 0 || params.amount1In > 0, "Zero amount");
        require(params.positionManager != address(0), "Invalid PM address");
        require(v3PositionManager != address(0), "V3 PM not set");
        require(
            params.positionManager == v3PositionManager || trustedV3PositionManagers[params.positionManager],
            "Untrusted PM"
        );
        require(params.tickLower < params.tickUpper, "Invalid ticks");
        require(params.recipient != address(0), "Invalid recipient");

        // Sanity check pool tokens match params (prevents misconfiguration).
        require(IUniswapV3Pool(params.pool).token0() == params.token0, "Pool token0 mismatch");
        require(IUniswapV3Pool(params.pool).token1() == params.token1, "Pool token1 mismatch");

        // Track pre-existing balances to avoid mixing/refunding other users' funds.
        uint256 token0BalBefore = IERC20(params.token0).balanceOf(address(this));
        uint256 token1BalBefore = IERC20(params.token1).balanceOf(address(this));

        // 1. 拉取代币
        if (params.amount0In > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0In);
        }
        if (params.amount1In > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1In);
        }

        uint256 token0BalAfterPull = IERC20(params.token0).balanceOf(address(this));
        uint256 token1BalAfterPull = IERC20(params.token1).balanceOf(address(this));
        uint256 token0DeltaAfterPull = token0BalAfterPull - token0BalBefore;
        uint256 token1DeltaAfterPull = token1BalAfterPull - token1BalBefore;

        // 2. 执行 OKX swap（如果需要）
        if (params.swap.amountIn > 0 && params.swap.callData.length > 0) {
            _validateSwapParams(params.token0, params.token1, params.swap, token0DeltaAfterPull, token1DeltaAfterPull);
            _executeSwap(params.swap);
        }

        // 3. 获取 swap 后的余额
        uint256 token0BalAfterSwap = IERC20(params.token0).balanceOf(address(this));
        uint256 token1BalAfterSwap = IERC20(params.token1).balanceOf(address(this));
        uint256 bal0 = token0BalAfterSwap - token0BalBefore;
        uint256 bal1 = token1BalAfterSwap - token1BalBefore;
        require(bal0 > 0 || bal1 > 0, "No tokens after swap");

        // 4. 添加流动性
        uint24 poolFee = IUniswapV3Pool(params.pool).fee();
        result = _mintV3Position(MintV3Params({
            positionManager: params.positionManager,
            token0: params.token0,
            token1: params.token1,
            fee: poolFee,
            tickLower: params.tickLower,
            tickUpper: params.tickUpper,
            amount0: bal0,
            amount1: bal1,
            recipient: params.recipient
        }));

        // 5. Track dust for reporting/refund.
        {
            uint256 dust0 = IERC20(params.token0).balanceOf(address(this)) - token0BalBefore;
            uint256 dust1 = IERC20(params.token1).balanceOf(address(this)) - token1BalBefore;
            result.dust0 = dust0;
            result.dust1 = dust1;
        }

        // 6. 返还剩余代币
        _refundDelta(params.token0, msg.sender, token0BalBefore);
        _refundDelta(params.token1, msg.sender, token1BalBefore);

        emit ZapInV3(msg.sender, params.pool, result.tokenId, result.amount0Used, result.amount1Used, result.liquidity);
    }

    /**
     * @notice V3 撤仓
     */
    function zapOutV3(
        address positionManager,
        uint256 tokenId,
        address recipient,
        uint256 amount0Min,
        uint256 amount1Min
    ) external onlyOwner nonReentrant returns (uint256 amount0, uint256 amount1) {
        require(positionManager != address(0), "Invalid PM address");
        require(v3PositionManager != address(0), "V3 PM not set");
        require(
            positionManager == v3PositionManager || trustedV3PositionManagers[positionManager],
            "Untrusted PM"
        );
        require(recipient != address(0), "Invalid recipient");
        INonfungiblePositionManager npm = INonfungiblePositionManager(positionManager);
        
        // 获取仓位信息
        (
            ,
            ,
            ,
            ,
            ,
            ,
            ,
            uint128 liquidity,
            ,
            ,
            ,
        ) = npm.positions(tokenId);

        require(liquidity > 0, "No liquidity");

        address owner = npm.ownerOf(tokenId);
        require(owner == msg.sender, "Not owner");
        require(
            npm.getApproved(tokenId) == address(this) || npm.isApprovedForAll(owner, address(this)),
            "NFT not approved"
        );

        npm.decreaseLiquidity(INonfungiblePositionManager.DecreaseLiquidityParams({
            tokenId: tokenId,
            liquidity: liquidity,
            amount0Min: amount0Min,
            amount1Min: amount1Min,
            deadline: block.timestamp
        }));

        // 收集代币
        (amount0, amount1) = npm.collect(INonfungiblePositionManager.CollectParams({
            tokenId: tokenId,
            recipient: recipient,
            amount0Max: type(uint128).max,
            amount1Max: type(uint128).max
        }));

        emit ZapOutV3(msg.sender, tokenId, amount0, amount1);
    }

    /*//////////////////////////////////////////////////////////////
                          INTERNAL FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    function _requireWrappedNative() internal view returns (address) {
        address token = wrappedNative;
        require(token != address(0), "Wrapped native not set");
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

    function _recipientBalanceForCurrency(address currency, address account) internal view returns (uint256) {
        return currency == address(0) ? account.balance : IERC20(currency).balanceOf(account);
    }

    function _transferFundingForCurrency(address currency, address from, uint256 amount) internal {
        if (amount == 0) {
            return;
        }
        IERC20(_fundingTokenForCurrency(currency)).safeTransferFrom(from, address(this), amount);
    }

    function _unwrapForNativeCurrency(address currency, uint256 amount) internal {
        if (currency == address(0) && amount > 0) {
            IWrappedNative(_requireWrappedNative()).withdraw(amount);
        }
    }

    function _matchesPoolCurrency(address token, address currency) internal view returns (bool) {
        if (currency == address(0)) {
            return token == _requireWrappedNative();
        }
        return token == currency;
    }

    /**
     * @notice 执行 OKX swap
     */
    function _executeSwap(SwapParams calldata swap) internal {
        require(swap.amountIn > 0, "Zero swap amount");
        require(swap.target != address(0), "Invalid swap target");

        // Approve
        address spender = swap.approveTarget != address(0) ? swap.approveTarget : swap.target;
        IERC20(swap.tokenIn).forceApprove(spender, swap.amountIn);

        // 记录余额
        uint256 balBefore = IERC20(swap.tokenOut).balanceOf(address(this));

        // 执行 swap
        (bool success, bytes memory returnData) = swap.target.call(swap.callData);
        if (!success) {
            if (returnData.length > 0) {
                assembly {
                    revert(add(returnData, 32), mload(returnData))
                }
            } else {
                revert("Swap failed");
            }
        }

        // 验证输出
        uint256 balAfter = IERC20(swap.tokenOut).balanceOf(address(this));
        uint256 amountOut = balAfter - balBefore;
        require(amountOut >= swap.minAmountOut, "Insufficient output");

        // 重置 approve
        IERC20(swap.tokenIn).forceApprove(spender, 0);

        emit SwapExecuted(swap.target, swap.tokenIn, swap.tokenOut, swap.amountIn, amountOut);
    }

    /**
     * @notice Mint V3 position
     */
    struct MintV3Params {
        address positionManager;
        address token0;
        address token1;
        uint24 fee;
        int24 tickLower;
        int24 tickUpper;
        uint256 amount0;
        uint256 amount1;
        address recipient;
    }

    function _mintV3Position(MintV3Params memory p) internal returns (ZapResult memory result) {
        // Approve
        if (p.amount0 > 0) {
            IERC20(p.token0).forceApprove(p.positionManager, p.amount0);
        }
        if (p.amount1 > 0) {
            IERC20(p.token1).forceApprove(p.positionManager, p.amount1);
        }

        // Use dust checks instead of mint min-amounts (min amounts are unreliable when price is out of range).
        uint256 amount0Min = 0;
        uint256 amount1Min = 0;

        // Mint
        (
            uint256 tokenId,
            uint128 liquidity,
            uint256 amount0Used,
            uint256 amount1Used
        ) = INonfungiblePositionManager(p.positionManager).mint(
            INonfungiblePositionManager.MintParams({
                token0: p.token0,
                token1: p.token1,
                fee: p.fee,
                tickLower: p.tickLower,
                tickUpper: p.tickUpper,
                amount0Desired: p.amount0,
                amount1Desired: p.amount1,
                amount0Min: amount0Min,
                amount1Min: amount1Min,
                recipient: p.recipient,
                deadline: block.timestamp
            })
        );

        // 重置 approve
        IERC20(p.token0).forceApprove(p.positionManager, 0);
        IERC20(p.token1).forceApprove(p.positionManager, 0);

        result.tokenId = tokenId;
        result.liquidity = liquidity;
        result.amount0Used = amount0Used;
        result.amount1Used = amount1Used;
        result.dust0 = p.amount0 - amount0Used;
        result.dust1 = p.amount1 - amount1Used;
    }

    /**
     * @notice 返还剩余代币
     */
    function _refundDelta(address token, address to, uint256 balanceBefore) internal {
        uint256 balanceAfter = _balanceForCurrency(token);
        uint256 delta = balanceAfter - balanceBefore;
        if (delta > 0) {
            if (token == address(0)) {
                (bool ok, ) = payable(to).call{value: delta}("");
                require(ok, "Native refund failed");
            } else {
                IERC20(token).safeTransfer(to, delta);
            }
        }
    }

    function _validateSwapParams(
        address token0,
        address token1,
        SwapParams calldata swap,
        uint256 token0Available,
        uint256 token1Available
    ) internal view {
        require(swap.target != address(0), "Invalid swap target");
        require(_matchesPoolCurrency(swap.tokenIn, token0) || _matchesPoolCurrency(swap.tokenIn, token1), "Invalid swap tokenIn");
        require(_matchesPoolCurrency(swap.tokenOut, token0) || _matchesPoolCurrency(swap.tokenOut, token1), "Invalid swap tokenOut");
        require(swap.tokenIn != swap.tokenOut, "Swap tokens same");

        uint256 maxIn = _matchesPoolCurrency(swap.tokenIn, token0) ? token0Available : token1Available;
        require(swap.amountIn <= maxIn, "Swap amount exceeds input");
    }

    /**
     * @notice 计算 tick 对应的 sqrt 价格
     */
    function _getSqrtPriceAtTick(int24 tick) internal pure returns (uint160) {
        unchecked {
            uint256 absTick = tick < 0 ? uint256(-int256(tick)) : uint256(int256(tick));
            require(absTick <= uint256(int256(887272)), "T");

            uint256 ratio = absTick & 0x1 != 0 ? 0xfffcb933bd6fad37aa2d162d1a594001 : 0x100000000000000000000000000000000;
            if (absTick & 0x2 != 0) ratio = (ratio * 0xfff97272373d413259a46990580e213a) >> 128;
            if (absTick & 0x4 != 0) ratio = (ratio * 0xfff2e50f5f656932ef12357cf3c7fdcc) >> 128;
            if (absTick & 0x8 != 0) ratio = (ratio * 0xffe5caca7e10e4e61c3624eaa0941cd0) >> 128;
            if (absTick & 0x10 != 0) ratio = (ratio * 0xffcb9843d60f6159c9db58835c926644) >> 128;
            if (absTick & 0x20 != 0) ratio = (ratio * 0xff973b41fa98c081472e6896dfb254c0) >> 128;
            if (absTick & 0x40 != 0) ratio = (ratio * 0xff2ea16466c96a3843ec78b326b52861) >> 128;
            if (absTick & 0x80 != 0) ratio = (ratio * 0xfe5dee046a99a2a811c461f1969c3053) >> 128;
            if (absTick & 0x100 != 0) ratio = (ratio * 0xfcbe86c7900a88aedcffc83b479aa3a4) >> 128;
            if (absTick & 0x200 != 0) ratio = (ratio * 0xf987a7253ac413176f2b074cf7815e54) >> 128;
            if (absTick & 0x400 != 0) ratio = (ratio * 0xf3392b0822b70005940c7a398e4b70f3) >> 128;
            if (absTick & 0x800 != 0) ratio = (ratio * 0xe7159475a2c29b7443b29c7fa6e889d9) >> 128;
            if (absTick & 0x1000 != 0) ratio = (ratio * 0xd097f3bdfd2022b8845ad8f792aa5825) >> 128;
            if (absTick & 0x2000 != 0) ratio = (ratio * 0xa9f746462d870fdf8a65dc1f90e061e5) >> 128;
            if (absTick & 0x4000 != 0) ratio = (ratio * 0x70d869a156d2a1b890bb3df62baf32f7) >> 128;
            if (absTick & 0x8000 != 0) ratio = (ratio * 0x31be135f97d08fd981231505542fcfa6) >> 128;
            if (absTick & 0x10000 != 0) ratio = (ratio * 0x9aa508b5b7a84e1c677de54f3e99bc9) >> 128;
            if (absTick & 0x20000 != 0) ratio = (ratio * 0x5d6af8dedb81196699c329225ee604) >> 128;
            if (absTick & 0x40000 != 0) ratio = (ratio * 0x2216e584f5fa1ea926041bedfe98) >> 128;
            if (absTick & 0x80000 != 0) ratio = (ratio * 0x48a170391f7dc42444e8fa2) >> 128;

            if (tick > 0) ratio = type(uint256).max / ratio;

            return uint160((ratio >> 32) + (ratio % (1 << 32) == 0 ? 0 : 1));
        }
    }

    /**
     * @notice 计算理想的代币数量
     */
    function _getIdealAmounts(
        uint160 sqrtPriceX96,
        uint160 sqrtPriceLower,
        uint160 sqrtPriceUpper,
        uint256 totalValue
    ) internal pure returns (uint256 amount0, uint256 amount1) {
        // 简化计算：基于当前价格位置计算比例
        if (sqrtPriceX96 <= sqrtPriceLower) {
            // 价格低于范围，全部是 token0
            amount0 = totalValue;
            amount1 = 0;
        } else if (sqrtPriceX96 >= sqrtPriceUpper) {
            // 价格高于范围，全部是 token1
            amount0 = 0;
            amount1 = totalValue;
        } else {
            // 价格在范围内，按比例分配
            uint256 ratio0 = uint256(sqrtPriceUpper - sqrtPriceX96);
            uint256 ratio1 = uint256(sqrtPriceX96 - sqrtPriceLower);
            uint256 total = ratio0 + ratio1;
            
            amount0 = (totalValue * ratio0) / total;
            amount1 = (totalValue * ratio1) / total;
        }
    }

    /*//////////////////////////////////////////////////////////////
                            V4 FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    // Permit2 address (same on all chains)
    address constant PERMIT2 = 0x000000000022D473030F116dDEE9F6B43aC78BA3;

    /**
     * @notice V4 开仓（原子执行 swap + add LP）
     */
    function zapInV4(ZapInV4Params calldata params)
        external
        onlyOwner
        nonReentrant
        returns (ZapResult memory result)
    {
        PoolKeySimple memory poolKey = params.poolKey;
        require(poolKey.currency0 < poolKey.currency1, "Tokens not sorted");
        require(params.amount0In > 0 || params.amount1In > 0, "Zero amount");
        require(params.slippageBps <= BPS_DENOMINATOR, "Slippage too high");
        require(params.positionManager != address(0), "Invalid PM address");
        require(v4PositionManager != address(0), "V4 PM not set");
        require(params.positionManager == v4PositionManager, "Untrusted PM");
        require(params.tickLower < params.tickUpper, "Invalid ticks");
        require(params.recipient != address(0), "Invalid recipient");

        // Track pre-existing balances to avoid mixing/refunding other users' funds.
        uint256 token0FundingBefore = _fundingBalanceForCurrency(poolKey.currency0);
        uint256 token1FundingBefore = _fundingBalanceForCurrency(poolKey.currency1);
        uint256 token0BalBefore = _balanceForCurrency(poolKey.currency0);
        uint256 token1BalBefore = _balanceForCurrency(poolKey.currency1);

        // 1. 拉取代币 + 2. 执行 OKX swap
        {
            if (params.amount0In > 0) {
                _transferFundingForCurrency(poolKey.currency0, msg.sender, params.amount0In);
            }
            if (params.amount1In > 0) {
                _transferFundingForCurrency(poolKey.currency1, msg.sender, params.amount1In);
            }

            if (params.swap.amountIn > 0 && params.swap.callData.length > 0) {
                uint256 d0 = _fundingBalanceForCurrency(poolKey.currency0) - token0FundingBefore;
                uint256 d1 = _fundingBalanceForCurrency(poolKey.currency1) - token1FundingBefore;
                _validateSwapParams(poolKey.currency0, poolKey.currency1, params.swap, d0, d1);
                _executeSwap(params.swap);
            }
        }

        // 3. 获取 swap 后的余额
        uint256 bal0 = _fundingBalanceForCurrency(poolKey.currency0) - token0FundingBefore;
        uint256 bal1 = _fundingBalanceForCurrency(poolKey.currency1) - token1FundingBefore;
        require(bal0 > 0 || bal1 > 0, "No tokens after swap");

        // 4. 构建 V4 PoolKey + 5. 获取实时价格 + 价格校验
        PoolKey memory v4PoolKey;
        uint160 sqrtPriceX96;
        {
            v4PoolKey = PoolKey({
                currency0: Currency.wrap(poolKey.currency0),
                currency1: Currency.wrap(poolKey.currency1),
                fee: poolKey.fee,
                tickSpacing: poolKey.tickSpacing,
                hooks: poolKey.hooks
            });
            PoolId poolId = PoolIdLibrary.toId(v4PoolKey);
            (sqrtPriceX96, , , ) = IStateView(params.stateView).getSlot0(poolId);
            require(sqrtPriceX96 > 0, "Invalid SqrtPrice");

            if (params.sqrtPriceX96 > 0 && params.slippageBps > 0) {
                uint256 diff = uint256(params.sqrtPriceX96) > uint256(sqrtPriceX96)
                    ? uint256(params.sqrtPriceX96) - uint256(sqrtPriceX96)
                    : uint256(sqrtPriceX96) - uint256(params.sqrtPriceX96);
                require(FullMath.mulDiv(diff, BPS_DENOMINATOR, uint256(sqrtPriceX96)) <= params.slippageBps, "Price moved");
            }
        }

        _unwrapForNativeCurrency(poolKey.currency0, bal0);
        _unwrapForNativeCurrency(poolKey.currency1, bal1);

        // 6. Mint V4 position
        {
            (uint256 tokenId, uint128 liquidity) = _mintV4Position(
                params.positionManager,
                v4PoolKey,
                params.tickLower,
                params.tickUpper,
                bal0,
                bal1,
                params.slippageBps,
                params.recipient,
                sqrtPriceX96
            );
            result.tokenId = tokenId;
            result.liquidity = liquidity;
        }

        // 7. 获取 dust 并设置结果
        {
            uint256 dust0 = _balanceForCurrency(poolKey.currency0) - token0BalBefore;
            uint256 dust1 = _balanceForCurrency(poolKey.currency1) - token1BalBefore;
            result.dust0 = dust0;
            result.dust1 = dust1;
            result.amount0Used = bal0 > dust0 ? bal0 - dust0 : 0;
            result.amount1Used = bal1 > dust1 ? bal1 - dust1 : 0;
        }

        // 8. 退还 dust
        _refundDelta(poolKey.currency0, msg.sender, token0BalBefore);
        _refundDelta(poolKey.currency1, msg.sender, token1BalBefore);

        emit ZapInV4(msg.sender, keccak256(abi.encode(v4PoolKey)), result.tokenId, result.amount0Used, result.amount1Used, result.liquidity);
    }

    /**
     * @notice V4 撤仓
     */
    function zapOutV4(
        address positionManager,
        uint256 tokenId,
        PoolKey calldata poolKey,
        address recipient
    ) external onlyOwner nonReentrant returns (uint256 amount0, uint256 amount1) {
        require(positionManager != address(0), "Invalid PM address");
        require(v4PositionManager != address(0), "V4 PM not set");
        require(positionManager == v4PositionManager, "Untrusted PM");
        require(recipient != address(0), "Invalid recipient");
        address currency0 = Currency.unwrap(poolKey.currency0);
        address currency1 = Currency.unwrap(poolKey.currency1);
        require(currency0 < currency1, "Tokens not sorted");
        // V4 撤仓逻辑
        // 需要通过 modifyLiquidities 执行 DECREASE_LIQUIDITY + TAKE_PAIR

        IERC721 nft = IERC721(positionManager);
        address owner = nft.ownerOf(tokenId);
        require(owner == msg.sender, "Not owner");
        require(
            nft.getApproved(tokenId) == address(this) || nft.isApprovedForAll(owner, address(this)),
            "NFT not approved"
        );

        uint256 bal0Before = _recipientBalanceForCurrency(currency0, recipient);
        uint256 bal1Before = _recipientBalanceForCurrency(currency1, recipient);

        // 获取当前流动性 - 优先读取 PositionManager.positions，失败则回退到 sentinel
        uint128 liquidity = type(uint128).max;
        try IPositionManager(positionManager).positions(tokenId) returns (
            uint96,
            address,
            address,
            address,
            uint24,
            int24,
            int24,
            uint128 posLiquidity,
            uint256,
            uint256,
            uint128,
            uint128
        ) {
            require(posLiquidity > 0, "No liquidity");
            liquidity = posLiquidity;
        } catch {
            // fallback to sentinel; PositionManager may not expose positions()
        }
        
        bytes memory actions = new bytes(2);
        actions[0] = bytes1(Actions.DECREASE_LIQUIDITY);
        actions[1] = bytes1(Actions.TAKE_PAIR);

        bytes[] memory params = new bytes[](2);
        
        // DECREASE_LIQUIDITY params: (tokenId, liquidity, amount0Min, amount1Min, hookData)
        // 使用 type(uint128).max 减少全部流动性
        params[0] = abi.encode(tokenId, liquidity, uint256(0), uint256(0), bytes(""));
        
        // TAKE_PAIR params: (currency0, currency1, recipient)
        params[1] = abi.encode(poolKey.currency0, poolKey.currency1, recipient);

        bytes memory unlockData = abi.encode(actions, params);

        // Execute
        IPositionManager(positionManager).modifyLiquidities(unlockData, block.timestamp);

        uint256 bal0After = _recipientBalanceForCurrency(currency0, recipient);
        uint256 bal1After = _recipientBalanceForCurrency(currency1, recipient);
        amount0 = bal0After > bal0Before ? bal0After - bal0Before : 0;
        amount1 = bal1After > bal1Before ? bal1After - bal1Before : 0;

        emit ZapOutV4(msg.sender, tokenId, amount0, amount1);
    }

    /**
     * @notice Mint V4 position (基于 ZapV3V4Improved 正确实现)
     * @dev
     *   V4 使用 action 模式：
     *   - MINT_POSITION (0x02): 创建新仓位
     *   - SETTLE_PAIR (0x0d): 结算代币债务
     *
     *   V4 PositionManager 使用 Permit2 进行授权：
     *   1. approve token -> Permit2
     *   2. Permit2.approve -> PositionManager
     */
    function _mintV4Position(
        address positionManager,
        PoolKey memory poolKey,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0,
        uint256 amount1,
        uint256 /* slippageBps */,
        address recipient,
        uint160 sqrtPriceX96
    ) internal returns (uint256 tokenId, uint128 liquidity) {
        address token0 = Currency.unwrap(poolKey.currency0);
        address token1 = Currency.unwrap(poolKey.currency1);

        require(amount0 <= type(uint128).max, "amount0 too large");
        require(amount1 <= type(uint128).max, "amount1 too large");

        // Setup Permit2 allowances for V4 PositionManager
        if (amount0 > 0 && token0 != address(0)) {
            _forceApprovePermit2Infinity(token0);
            _permit2ApproveInfinity(token0, positionManager);
        }
        if (amount1 > 0 && token1 != address(0)) {
            _forceApprovePermit2Infinity(token1);
            _permit2ApproveInfinity(token1, positionManager);
        }

        // Calculate liquidity using actual current price
        {
            uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
            uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);
            liquidity = LiquidityAmounts.getLiquidityForAmounts(
                sqrtPriceX96, sqrtRatioAX96, sqrtRatioBX96, amount0, amount1
            );
        }

        // PoolKey is passed in directly
        // PoolKey memory v4PoolKey = poolKey;

        uint256 nativeValue = 0;
        bool sweepNative = false;
        Currency nativeCurrency;
        if (token0 == address(0) && amount0 > 0) {
            nativeValue += amount0;
            nativeCurrency = poolKey.currency0;
            sweepNative = true;
        }
        if (token1 == address(0) && amount1 > 0) {
            nativeValue += amount1;
            nativeCurrency = poolKey.currency1;
            sweepNative = true;
        }

        // Build actions: MINT_POSITION (0x02) + SETTLE_PAIR (0x0d), optionally SWEEP native refund.
        uint256 actionCount = sweepNative ? 3 : 2;
        bytes memory actions = new bytes(actionCount);
        actions[0] = bytes1(Actions.MINT_POSITION);
        actions[1] = bytes1(Actions.SETTLE_PAIR);
        if (sweepNative) {
            actions[2] = bytes1(Actions.SWEEP);
        }

        // MINT_POSITION params
        bytes memory mintParams = abi.encode(
            poolKey, tickLower, tickUpper, uint256(liquidity),
            uint128(amount0), uint128(amount1), recipient, bytes("")
        );

        // SETTLE_PAIR params
        bytes memory settlePairParams = abi.encode(poolKey.currency0, poolKey.currency1);

        // Combine params
        bytes[] memory params = new bytes[](actionCount);
        params[0] = mintParams;
        params[1] = settlePairParams;
        if (sweepNative) {
            params[2] = abi.encode(nativeCurrency, address(this));
        }

        bytes memory unlockData = abi.encode(actions, params);

        // Execute mint via modifyLiquidities (deadline = block.timestamp + 300)
        IPositionManager(positionManager).modifyLiquidities{value: nativeValue}(unlockData, block.timestamp + 300);

        // Get tokenId
        tokenId = IPositionManager(positionManager).nextTokenId() - 1;
    }

    /// @dev Approve Permit2 allowance for `spender` to infinity.
    /// V4 PositionManager implementations may require Permit2 allowance to be fixed at infinity.
    function _permit2ApproveInfinity(address token, address spender) internal {
        // If already infinite, skip external state change (and avoid revert on some Permit2 variants).
        (uint160 allowedAmount, uint48 allowedExpiration, ) = IPermit2(PERMIT2).allowance(address(this), token, spender);
        if (allowedAmount == type(uint160).max && allowedExpiration == type(uint48).max) {
            return;
        }

        try IPermit2(PERMIT2).approve(token, spender, type(uint160).max, type(uint48).max) {
            // ok
        } catch (bytes memory reason) {
            bytes4 selector;
            if (reason.length >= 4) {
                assembly {
                    selector := mload(add(reason, 32))
                }
            }
            if (selector == PERMIT2_ALLOWANCE_IS_FIXED_AT_INFINITY) {
                // Allowance already fixed at infinity; nothing to do.
                return;
            }
            assembly {
                revert(add(reason, 32), mload(reason))
            }
        }
    }

    /// @dev Ensure ERC20 allowance from this contract -> Permit2 is infinite.
    /// Some tokens (notably tokens that integrate with Permit2) revert if approving Permit2 to any value other than `type(uint256).max`.
    function _forceApprovePermit2Infinity(address token) internal {
        uint256 cur = IERC20(token).allowance(address(this), PERMIT2);
        if (cur == type(uint256).max) {
            return;
        }
        IERC20(token).forceApprove(PERMIT2, type(uint256).max);
    }

    /**
     * @notice Read sqrtPriceX96 from a V3-style pool without decoding fork-specific tail fields.
     * @dev Pancake V3 slot0() packs later fields differently from Uniswap V3, so only the first word is trusted here.
     */
    function _getPoolSqrtPriceX96(address pool) internal view returns (uint160 sqrtPriceX96) {
        (bool ok, bytes memory data) = pool.staticcall(abi.encodeWithSelector(SLOT0_SELECTOR));
        require(ok, "slot0 call failed");
        require(data.length >= 32, "slot0 return too short");
        sqrtPriceX96 = abi.decode(data, (uint160));
    }

    /**
     * @notice 将 token0 和 token1 的价值转换为以 token1 为单位的统一价值
     * @dev Used for V3 optimal-swap calculation.
     */
    function _calculateValueInToken1(
        uint256 amount0,
        uint256 amount1,
        uint160 sqrtPriceX96
    ) internal pure returns (uint256 value) {
        // token0 价值(以 token1 计) = amount0 * (sqrtPriceX96 / Q96)^2
        // 使用 FullMath.mulDiv 避免 sqrtPriceX96^2 造成溢出。
        uint256 amount0InToken1 = FullMath.mulDiv(
            FullMath.mulDiv(amount0, uint256(sqrtPriceX96), Q96),
            uint256(sqrtPriceX96),
            Q96
        );
        value = amount0InToken1 + amount1;
    }

    function _amount0FromValueInToken1(uint256 valueInToken1, uint160 sqrtPriceX96) internal pure returns (uint256 amount0) {
        if (valueInToken1 == 0) {
            return 0;
        }
        amount0 = FullMath.mulDiv(valueInToken1, Q96, uint256(sqrtPriceX96));
        amount0 = FullMath.mulDiv(amount0, Q96, uint256(sqrtPriceX96));
    }

    /*//////////////////////////////////////////////////////////////
                           ADMIN FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    /**
     * @notice 紧急提取
     */
    function emergencyWithdraw(address token, uint256 amount) external onlyOwner {
        if (token == address(0)) {
            (bool ok, ) = payable(owner()).call{value: amount}("");
            require(ok, "ETH transfer failed");
        } else {
            IERC20(token).safeTransfer(owner(), amount);
        }
    }

    receive() external payable {}

    /**
     * @notice 接收 ERC721
     */
    function onERC721Received(
        address,
        address,
        uint256,
        bytes calldata
    ) external pure returns (bytes4) {
        return this.onERC721Received.selector;
    }
}

// ERC721 interface
interface IERC721 {
    function transferFrom(address from, address to, uint256 tokenId) external;
    function ownerOf(uint256 tokenId) external view returns (address);
    function getApproved(uint256 tokenId) external view returns (address);
    function isApprovedForAll(address owner, address operator) external view returns (bool);
}
