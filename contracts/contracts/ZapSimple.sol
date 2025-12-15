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
    function slot0() external view returns (
        uint160 sqrtPriceX96,
        int24 tick,
        uint16 observationIndex,
        uint16 observationCardinality,
        uint16 observationCardinalityNext,
        uint8 feeProtocol,
        bool unlocked
    );
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
    function ownerOf(uint256 tokenId) external view returns (address);
}
contract ZapSimple is ReentrancyGuard, Ownable {
    using SafeERC20 for IERC20;

    /*//////////////////////////////////////////////////////////////
                               CONSTANTS
    //////////////////////////////////////////////////////////////*/

    uint256 private constant Q96 = 2**96;

    // Maximum dust threshold: 20% of input value (default)
    uint256 public constant MAX_DUST_BPS = 2000; // 20%

    /*//////////////////////////////////////////////////////////////
                                EVENTS
    //////////////////////////////////////////////////////////////*/

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
        uint256 slippageBps;      // 滑点保护 (100 = 1%)
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
        uint160 sqrtPriceX96;     // 当前价格 (从 Go 代码传入，避免链上重复调用)
        uint256 maxDustBps;       // 最大 dust 容忍度 (100 = 1%, 0 = 使用默认 1%)
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
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(pool).slot0();
        
        // 计算 tick 范围的 sqrt 价格
        uint160 sqrtPriceLower = _getSqrtPriceAtTick(tickLower);
        uint160 sqrtPriceUpper = _getSqrtPriceAtTick(tickUpper);

        // 计算理想比例
        (uint256 ideal0, uint256 ideal1) = _getIdealAmounts(
            sqrtPriceX96,
            sqrtPriceLower,
            sqrtPriceUpper,
            amount0In + amount1In // 简化：使用总输入
        );

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
        nonReentrant
        returns (ZapResult memory result)
    {
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.amount0In > 0 || params.amount1In > 0, "Zero amount");
        require(params.slippageBps <= 10000, "Slippage too high");

        // 1. 拉取代币
        if (params.amount0In > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0In);
        }
        if (params.amount1In > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1In);
        }

        // 2. 执行 OKX swap（如果需要）
        if (params.swap.amountIn > 0 && params.swap.callData.length > 0) {
            _executeSwap(params.swap);
        }

        // 3. 获取 swap 后的余额
        uint256 bal0 = IERC20(params.token0).balanceOf(address(this));
        uint256 bal1 = IERC20(params.token1).balanceOf(address(this));

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
            slippageBps: params.slippageBps,
            recipient: params.recipient
        }));

        // 5. 返还剩余代币
        _refundDust(params.token0, msg.sender);
        _refundDust(params.token1, msg.sender);

        emit ZapInV3(msg.sender, params.pool, result.tokenId, result.amount0Used, result.amount1Used, result.liquidity);
    }

    /**
     * @notice V3 撤仓
     */
    function zapOutV3(
        address positionManager,
        uint256 tokenId,
        address recipient,
        uint256 slippageBps
    ) external nonReentrant returns (uint256 amount0, uint256 amount1) {
        INonfungiblePositionManager npm = INonfungiblePositionManager(positionManager);
        
        // 获取仓位信息
        (
            ,
            ,
            address token0,
            address token1,
            ,
            ,
            ,
            uint128 liquidity,
            ,
            ,
            ,
        ) = npm.positions(tokenId);

        require(liquidity > 0, "No liquidity");

        // 转移 NFT 到合约
        npm.safeTransferFrom(msg.sender, address(this), tokenId);

        // 减少流动性
        uint256 minAmount0 = 0;
        uint256 minAmount1 = 0;
        if (slippageBps < 10000) {
            // 这里简化处理，实际应该基于当前价格计算
            minAmount0 = 0;
            minAmount1 = 0;
        }

        npm.decreaseLiquidity(INonfungiblePositionManager.DecreaseLiquidityParams({
            tokenId: tokenId,
            liquidity: liquidity,
            amount0Min: minAmount0,
            amount1Min: minAmount1,
            deadline: block.timestamp
        }));

        // 收集代币
        (amount0, amount1) = npm.collect(INonfungiblePositionManager.CollectParams({
            tokenId: tokenId,
            recipient: recipient,
            amount0Max: type(uint128).max,
            amount1Max: type(uint128).max
        }));

        // 销毁 NFT
        npm.burn(tokenId);

        emit ZapOutV3(msg.sender, tokenId, amount0, amount1);
    }

    /*//////////////////////////////////////////////////////////////
                          INTERNAL FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    /**
     * @notice 执行 OKX swap
     */
    function _executeSwap(SwapParams calldata swap) internal {
        require(swap.amountIn > 0, "Zero swap amount");

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
        uint256 slippageBps;
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

        // 计算最小接受数量
        uint256 amount0Min = p.amount0 * (10000 - p.slippageBps) / 10000;
        uint256 amount1Min = p.amount1 * (10000 - p.slippageBps) / 10000;

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
    function _refundDust(address token, address to) internal {
        uint256 balance = IERC20(token).balanceOf(address(this));
        if (balance > 0) {
            IERC20(token).safeTransfer(to, balance);
        }
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

    // V4 Action types (from Uniswap V4 PositionManager)
    uint256 constant MINT_POSITION = 1;
    uint256 constant INCREASE_LIQUIDITY = 2;
    uint256 constant DECREASE_LIQUIDITY = 3;
    uint256 constant BURN_POSITION = 4;
    uint256 constant TAKE_PAIR = 5;
    uint256 constant SETTLE_PAIR = 6;
    uint256 constant SETTLE = 11;
    uint256 constant TAKE = 12;
    uint256 constant CLOSE_CURRENCY = 9;
    uint256 constant SWEEP = 15;

    // Permit2 address (same on all chains)
    address constant PERMIT2 = 0x000000000022D473030F116dDEE9F6B43aC78BA3;

    /**
     * @notice V4 开仓（原子执行 swap + add LP）
     */
    function zapInV4(ZapInV4Params calldata params)
        external
        nonReentrant
        returns (ZapResult memory result)
    {
        PoolKeySimple memory poolKey = params.poolKey;
        require(poolKey.currency0 < poolKey.currency1, "Tokens not sorted");
        require(params.amount0In > 0 || params.amount1In > 0, "Zero amount");
        require(params.slippageBps <= 10000, "Slippage too high");
        require(params.maxDustBps <= 10000, "MaxDust too high");

        // 记录输入金额用于 dust 计算
        uint256 inputAmount0 = params.amount0In;
        uint256 inputAmount1 = params.amount1In;

        // 1. 拉取代币
        if (params.amount0In > 0) {
            IERC20(poolKey.currency0).safeTransferFrom(msg.sender, address(this), params.amount0In);
        }
        if (params.amount1In > 0) {
            IERC20(poolKey.currency1).safeTransferFrom(msg.sender, address(this), params.amount1In);
        }

        // 2. 执行 OKX swap（如果需要）
        if (params.swap.amountIn > 0 && params.swap.callData.length > 0) {
            _executeSwap(params.swap);
        }

        // 3. 获取 swap 后的余额
        uint256 bal0 = IERC20(poolKey.currency0).balanceOf(address(this));
        uint256 bal1 = IERC20(poolKey.currency1).balanceOf(address(this));

        require(bal0 > 0 || bal1 > 0, "No tokens after swap");
        require(params.positionManager != address(0), "Invalid PM address");

        // 4. 使用传入的 sqrtPriceX96（如果提供了，避免链上调用 StateView）
        uint160 sqrtPriceX96 = params.sqrtPriceX96;
        if (sqrtPriceX96 == 0) {
            // 如果没有传入，则使用 tick 范围中点估算
            sqrtPriceX96 = TickMath.getSqrtRatioAtTick((params.tickLower + params.tickUpper) / 2);
        }

        // 5. Mint V4 position
        result = _mintV4Position(
            params.positionManager,
            poolKey,
            params.tickLower,
            params.tickUpper,
            bal0,
            bal1,
            params.slippageBps,
            params.recipient,
            sqrtPriceX96
        );

        // 6. 获取 dust 并设置结果
        result.dust0 = IERC20(poolKey.currency0).balanceOf(address(this));
        result.dust1 = IERC20(poolKey.currency1).balanceOf(address(this));

        // 7. Dust 验证
        uint256 effectiveMaxDustBps = params.maxDustBps > 0 ? params.maxDustBps : MAX_DUST_BPS;
        uint256 inputValue = _calculateValueInToken1(inputAmount0, inputAmount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(result.dust0, result.dust1, sqrtPriceX96);
        
        if (inputValue > 0) {
            require(
                dustValue * 10000 <= inputValue * effectiveMaxDustBps,
                "Dust exceeds limit"
            );
        }

        // 8. 退还 dust
        _refundDust(poolKey.currency0, msg.sender);
        _refundDust(poolKey.currency1, msg.sender);

        // 构建 poolId 用于事件
        PoolKey memory v4PoolKeyForEvent = PoolKey({
            currency0: Currency.wrap(poolKey.currency0),
            currency1: Currency.wrap(poolKey.currency1),
            fee: poolKey.fee,
            tickSpacing: poolKey.tickSpacing,
            hooks: poolKey.hooks
        });
        emit ZapInV4(msg.sender, keccak256(abi.encode(v4PoolKeyForEvent)), result.tokenId, result.amount0Used, result.amount1Used, result.liquidity);
    }

    /**
     * @notice V4 撤仓
     */
    function zapOutV4(
        address positionManager,
        uint256 tokenId,
        PoolKey calldata poolKey,
        address recipient
    ) external nonReentrant returns (uint256 amount0, uint256 amount1) {
        // V4 撤仓逻辑
        // 需要通过 modifyLiquidities 执行 DECREASE_LIQUIDITY + BURN_POSITION
        
        // 获取 NFT
        IERC721(positionManager).transferFrom(msg.sender, address(this), tokenId);

        // 获取当前流动性 - V4 需要通过 PositionManager 查询
        // 暂时简化处理，使用 CLOSE_CURRENCY 关闭全部
        
        bytes memory actions = new bytes(4);
        actions[0] = bytes1(uint8(DECREASE_LIQUIDITY));
        actions[1] = bytes1(uint8(TAKE));
        actions[2] = bytes1(uint8(TAKE));
        actions[3] = bytes1(uint8(BURN_POSITION));

        bytes[] memory params = new bytes[](4);
        
        // DECREASE_LIQUIDITY params: (tokenId, liquidity, amount0Min, amount1Min, hookData)
        // 使用 type(uint128).max 减少全部流动性
        params[0] = abi.encode(tokenId, type(uint128).max, uint256(0), uint256(0), bytes(""));
        
        // TAKE params: (currency, recipient, amount) - 0 = take all
        params[1] = abi.encode(poolKey.currency0, recipient, uint256(0));
        params[2] = abi.encode(poolKey.currency1, recipient, uint256(0));
        
        // BURN_POSITION params: (tokenId)
        params[3] = abi.encode(tokenId);

        bytes memory unlockData = abi.encode(actions, params);

        // Execute
        IPositionManager(positionManager).modifyLiquidities(unlockData, block.timestamp);

        // Get amounts (simplified - actual amounts from events)
        amount0 = 0;
        amount1 = 0;

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
        PoolKeySimple memory poolKey,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0,
        uint256 amount1,
        uint256 /* slippageBps */,
        address recipient,
        uint160 sqrtPriceX96
    ) internal returns (ZapResult memory result) {
        address token0 = poolKey.currency0;
        address token1 = poolKey.currency1;

        // Setup Permit2 allowances for V4 PositionManager
        uint48 expiration = uint48(block.timestamp + 3600);
        
        if (amount0 > 0) {
            IERC20(token0).forceApprove(PERMIT2, amount0);
            IPermit2(PERMIT2).approve(token0, positionManager, uint160(amount0), expiration);
        }
        if (amount1 > 0) {
            IERC20(token1).forceApprove(PERMIT2, amount1);
            IPermit2(PERMIT2).approve(token1, positionManager, uint160(amount1), expiration);
        }

        // Calculate liquidity using actual current price
        uint128 liquidity;
        {
            uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
            uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);
            liquidity = LiquidityAmounts.getLiquidityForAmounts(
                sqrtPriceX96, sqrtRatioAX96, sqrtRatioBX96, amount0, amount1
            );
        }

        // Build V4 PoolKey with Currency type
        PoolKey memory v4PoolKey = PoolKey({
            currency0: Currency.wrap(token0),
            currency1: Currency.wrap(token1),
            fee: poolKey.fee,
            tickSpacing: poolKey.tickSpacing,
            hooks: poolKey.hooks
        });

        // Build actions: MINT_POSITION (0x02) + SETTLE_PAIR (0x0d)
        bytes memory actions = new bytes(2);
        actions[0] = bytes1(Actions.MINT_POSITION);
        actions[1] = bytes1(Actions.SETTLE_PAIR);

        // MINT_POSITION params
        bytes memory mintParams = abi.encode(
            v4PoolKey, tickLower, tickUpper, uint256(liquidity),
            uint128(amount0), uint128(amount1), recipient, bytes("")
        );

        // SETTLE_PAIR params
        bytes memory settlePairParams = abi.encode(v4PoolKey.currency0, v4PoolKey.currency1);

        // Combine params
        bytes[] memory params = new bytes[](2);
        params[0] = mintParams;
        params[1] = settlePairParams;

        bytes memory unlockData = abi.encode(actions, params);

        // Execute mint via modifyLiquidities (deadline = block.timestamp + 300)
        IPositionManager(positionManager).modifyLiquidities(unlockData, block.timestamp + 300);

        // Get tokenId
        uint256 tokenId = IPositionManager(positionManager).nextTokenId() - 1;

        // Reset Permit2 approvals
        IERC20(token0).forceApprove(PERMIT2, 0);
        IERC20(token1).forceApprove(PERMIT2, 0);

        result = ZapResult({
            tokenId: tokenId,
            liquidity: liquidity,
            amount0Used: amount0,
            amount1Used: amount1,
            dust0: 0,
            dust1: 0
        });
    }

    /**
     * @notice 将 token0 和 token1 的价值转换为以 token1 为单位的统一价值
     * @dev 用于 dust 验证
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

    /*//////////////////////////////////////////////////////////////
                           ADMIN FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    /**
     * @notice 紧急提取
     */
    function emergencyWithdraw(address token, uint256 amount) external onlyOwner {
        if (token == address(0)) {
            payable(owner()).transfer(amount);
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
}
