// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "@openzeppelin/contracts/token/ERC20/IERC20.sol";
import "@openzeppelin/contracts/token/ERC20/utils/SafeERC20.sol";
import "@openzeppelin/contracts/utils/ReentrancyGuard.sol";
import "@openzeppelin/contracts/access/Ownable.sol";

import "./interfaces/IUniswapV3Pool.sol";
import "./interfaces/INonfungiblePositionManager.sol";
import "./interfaces/ISwapRouter.sol";
import "./interfaces/ISwapRouter02.sol";
import "./interfaces/v4/IPositionManager.sol";
import "./interfaces/v4/IStateView.sol";
import "./interfaces/v4/PoolKey.sol";
import "./interfaces/v4/Currency.sol";
import "./interfaces/v4/Actions.sol";
import "./interfaces/IPermit2.sol";
import "./libraries/TickMath.sol";
import "./libraries/FullMath.sol";
import "./libraries/LiquidityAmounts.sol";

/**
 * @title ZapV3V4Improved
 * @notice 自定义路由 + 流动性添加合约
 * @dev
 *   - 使用 OKX API 询价获取最优路径（链下）
 *   - 合约直接调用底层 DEX 执行 swap
 *   - 一笔交易完成 swap + add liquidity
 *   - 强制 dust < 1%
 *   - 支持跨 DEX swap（如通过 Uniswap 0.3% 池 swap，在 PancakeSwap 1% 池添加流动性）
 *   - 精确配平算法，减少 dust
 */
contract ZapV3V4Improved is ReentrancyGuard, Ownable {
    using SafeERC20 for IERC20;

    /*//////////////////////////////////////////////////////////////
                               CONSTANTS
    //////////////////////////////////////////////////////////////*/

    uint256 private constant Q96 = 2**96;

    // Maximum dust threshold: 1% of input value
    uint256 public constant MAX_DUST_BPS = 100; // 1%

    // Permit2 address (same on all chains)
    address public constant PERMIT2 = 0x000000000022D473030F116dDEE9F6B43aC78BA3;

    /*//////////////////////////////////////////////////////////////
                            STATE VARIABLES
    //////////////////////////////////////////////////////////////*/

    bool public whitelistEnabled = false;
    mapping(address => bool) public whitelist;

    // 批准的 DEX Router（PancakeSwap, Uniswap 等）
    mapping(address => bool) public approvedRouters;

    // 标记使用 SwapRouter02 接口的路由器（无 deadline 参数）
    mapping(address => bool) public isRouter02;

    uint256 public maxSlippageBps = 1000; // 10%
    uint256 public dustThreshold = 1000;

    address public immutable WETH;

    /*//////////////////////////////////////////////////////////////
                                EVENTS
    //////////////////////////////////////////////////////////////*/

    event ZapExecuted(
        address indexed user,
        uint256 indexed tokenId,
        uint256 amount0In,
        uint256 amount1In,
        uint128 liquidity,
        uint256 dust0,
        uint256 dust1
    );

    event SwapExecuted(
        address indexed router,
        address indexed tokenIn,
        address indexed tokenOut,
        uint256 amountIn,
        uint256 amountOut
    );

    event SwapCallExecuted(
        address indexed target,
        address indexed tokenIn,
        address indexed tokenOut,
        uint256 amountIn,
        uint256 amountOut
    );

    event RouterUpdated(address indexed router, bool approved);
    event Router02Updated(address indexed router, bool isV2);

    event ZapV4Executed(
        address indexed user,
        uint256 indexed tokenId,
        bytes32 indexed poolId,
        uint256 amount0In,
        uint256 amount1In,
        uint128 liquidity,
        uint256 dust0,
        uint256 dust1
    );

    /*//////////////////////////////////////////////////////////////
                               STRUCTS
    //////////////////////////////////////////////////////////////*/

    /// @notice 单跳 swap 路径
    struct SwapStep {
        address router;      // DEX router 地址 (PancakeSwap, Uniswap 等)
        address tokenIn;     // 输入代币
        address tokenOut;    // 输出代币
        uint24 fee;          // Pool fee tier (500, 2500, 3000, 10000)
        uint256 amountIn;    // 输入数量
        uint256 minAmountOut; // 最小输出（滑点保护）
    }

    /// @notice 多跳 swap 路径（用于复杂路由）
    struct MultiHopSwap {
        address router;      // DEX router 地址
        bytes path;          // 编码的路径 (tokenA, fee, tokenB, fee, tokenC...)
        uint256 amountIn;    // 输入数量
        uint256 minAmountOut; // 最小输出
    }

    /// @notice 任意 calldata swap（支持 OKX 聚合器等复杂路由）
    /// @dev 允许传入 OKX API 返回的 calldata，实现多路径、分拆交易
    struct SwapCall {
        address target;       // 目标合约 (OKX Router, PancakeSwap Router 等)
        address approveTarget; // Approve 目标 (OKX 的 approve 地址与 router 不同)，如果为 0 则使用 target
        bytes callData;       // 编码的调用数据 (从 OKX API 获取)
        address tokenIn;      // 输入代币 (用于 approve, 0xEeee... 表示原生代币)
        address tokenOut;     // 输出代币 (用于验证, 0xEeee... 表示原生代币)
        uint256 amountIn;     // 输入数量 (用于 approve 或 value)
        uint256 minAmountOut; // 最小输出 (滑点保护)
        uint256 value;        // 发送的原生代币数量 (ETH/BNB)，0 表示不发送
    }

    struct V3Config {
        address pool;
        address positionManager;
    }

    struct ZapParams {
        address token0;
        address token1;
        uint256 amount0;        // 输入的 token0 数量
        uint256 amount1;        // 输入的 token1 数量
        int24 tickLower;
        int24 tickUpper;
        uint256 tokenId;        // 0 = 新建 position
        uint256 slippageBps;
        address recipient;
        V3Config v3Config;
        // Swap 路径（从 OKX API 获取最优路径后构建）
        SwapStep[] swapSteps;   // 单跳 swap 列表
    }

    /// @notice Zap 参数（支持任意 calldata swap）
    /// @dev 用于 OKX 聚合器等复杂多路径交易
    struct ZapParamsWithCalls {
        address token0;
        address token1;
        uint256 amount0;        // 输入的 token0 数量
        uint256 amount1;        // 输入的 token1 数量
        int24 tickLower;
        int24 tickUpper;
        uint256 tokenId;        // 0 = 新建 position
        uint256 slippageBps;
        address recipient;
        V3Config v3Config;
        // 任意 calldata swap（支持 OKX 等聚合器）
        SwapCall[] swapCalls;   // 可以是多个 swap 调用
    }

    /// @notice Zap 参数（支持 OKX calldata + 链上二次配平）
    /// @dev 用于 OKX 聚合器 + 二次配平，解决 OKX 报价与执行时价格偏差导致的滑点问题
    ///      工作流程:
    ///      1. 执行 OKX 聚合器 swap (swapCalls)
    ///      2. 获取实际余额
    ///      3. 链上计算理想比例
    ///      4. 如果余额比例偏差过大，执行二次配平 swap
    ///      5. Mint position
    struct ZapParamsWithCallsAndRebalance {
        address token0;
        address token1;
        uint256 amount0;        // 输入的 token0 数量
        uint256 amount1;        // 输入的 token1 数量
        int24 tickLower;
        int24 tickUpper;
        uint256 tokenId;        // 0 = 新建 position
        uint256 slippageBps;
        address recipient;
        V3Config v3Config;
        // 任意 calldata swap（支持 OKX 等聚合器）
        SwapCall[] swapCalls;   // OKX 聚合器 swap 调用
        // 二次配平配置（使用 V3 router 进行微调）
        RebalanceConfig rebalance;
        uint256 maxDustBps;     // 最大 dust 容忍度 (e.g., 100 = 1%, 200 = 2%, 0 = use default 1%)
    }

    struct ZapResult {
        uint256 tokenId;
        uint128 liquidity;
        uint256 amount0Used;
        uint256 amount1Used;
        uint256 dust0;
        uint256 dust1;
    }

    /// @notice 二次配平 swap 配置
    /// @dev 用于链上二次配平，在主 swap 后根据实际余额进行微调
    struct RebalanceConfig {
        address router;           // 用于二次配平的 router
        uint24 fee;               // 用于二次配平的 pool fee
        bool enabled;             // 是否启用二次配平
        uint256 maxRebalanceBps;  // 最大二次配平比例 (e.g., 500 = 5%)
    }

    /// @notice Zap 参数（支持链上二次配平）
    /// @dev
    ///   工作流程:
    ///   1. 执行主 swap (swapSteps 或 swapCalls)
    ///   2. 获取实际余额
    ///   3. 链上计算理想比例
    ///   4. 如果 dust 太大，执行二次配平 swap
    ///   5. Mint position
    struct ZapParamsWithRebalance {
        address token0;
        address token1;
        uint256 amount0;
        uint256 amount1;
        int24 tickLower;
        int24 tickUpper;
        uint256 tokenId;
        uint256 slippageBps;
        address recipient;
        V3Config v3Config;
        SwapStep[] swapSteps;       // 主 swap 步骤
        RebalanceConfig rebalance;  // 二次配平配置
        uint256 maxDustBps;         // 最大 dust 容忍度 (e.g., 100 = 1%, 200 = 2%, 0 = use default 1%)
    }

    /*//////////////////////////////////////////////////////////////
                            V4 STRUCTS
    //////////////////////////////////////////////////////////////*/

    /// @notice V4 Pool 配置
    struct V4Config {
        address stateView;          // StateView contract for reading pool state
        address positionManager;    // V4 PositionManager address
        PoolKey poolKey;            // V4 PoolKey (currency0, currency1, fee, tickSpacing, hooks)
    }

    /// @notice V4 Zap 参数（支持链上二次配平）
    struct ZapV4Params {
        address token0;             // Must be sorted (token0 < token1)
        address token1;
        uint256 amount0;            // 输入的 token0 数量
        uint256 amount1;            // 输入的 token1 数量
        int24 tickLower;
        int24 tickUpper;
        uint256 slippageBps;
        address recipient;
        V4Config v4Config;
        SwapStep[] swapSteps;       // 主 swap 步骤（使用 V3 router 进行 swap）
        RebalanceConfig rebalance;  // 二次配平配置
        uint160 sqrtPriceX96;       // 当前价格（从 StateView 获取，避免链上再次调用）
        uint256 maxDustBps;         // 最大 dust 容忍度 (e.g., 100 = 1%, 200 = 2%, 0 = use default 1%)
    }

    /// @notice V4 Zap 结果
    struct ZapV4Result {
        uint256 tokenId;
        uint128 liquidity;
        uint256 amount0Used;
        uint256 amount1Used;
        uint256 dust0;
        uint256 dust1;
        bytes32 poolId;
    }

    /// @notice V4 Zap 参数（支持任意 calldata swap，如 OKX 聚合器）
    /// @dev 允许使用 OKX API 返回的 calldata 进行 swap，而不是通过 V3 router
    struct ZapV4ParamsWithCalls {
        address token0;             // Must be sorted (token0 < token1)
        address token1;
        uint256 amount0;            // 输入的 token0 数量
        uint256 amount1;            // 输入的 token1 数量
        int24 tickLower;
        int24 tickUpper;
        uint256 slippageBps;
        address recipient;
        V4Config v4Config;
        SwapCall[] swapCalls;       // 任意 calldata swap（支持 OKX 等聚合器）
        RebalanceConfig rebalance;  // 二次配平配置
        uint160 sqrtPriceX96;       // 当前价格（从 StateView 获取，避免链上再次调用）
        uint256 maxDustBps;         // 最大 dust 容忍度 (e.g., 100 = 1%, 200 = 2%, 0 = use default 1%)
    }

    /// @notice V4 Zap 参数（使用 V4 池内 swap）
    /// @dev 当 OKX 无法提供路由时，直接使用目标 V4 池的流动性进行 swap
    ///      通过 V4 PositionManager 的 SWAP_EXACT_IN_SINGLE action 执行
    struct ZapV4ParamsWithPoolSwap {
        address token0;             // Must be sorted (token0 < token1)
        address token1;
        uint256 amount0;            // 输入的 token0 数量
        uint256 amount1;            // 输入的 token1 数量
        int24 tickLower;
        int24 tickUpper;
        uint256 slippageBps;
        address recipient;
        V4Config v4Config;
        // V4 池内 swap 参数
        bool zeroForOne;            // swap 方向: true = token0 -> token1
        uint256 amountToSwap;       // swap 数量
        uint160 sqrtPriceLimitX96;  // 价格限制 (滑点保护, 0 = 无限制)
        // 其他参数
        uint160 sqrtPriceX96;       // 当前价格（从 StateView 获取）
        uint256 maxDustBps;         // 最大 dust 容忍度
    }

    /*//////////////////////////////////////////////////////////////
                            CONSTRUCTOR
    //////////////////////////////////////////////////////////////*/

    constructor(address _weth) Ownable(msg.sender) {
        WETH = _weth;
    }

    /*//////////////////////////////////////////////////////////////
                          ADMIN FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    function setWhitelistEnabled(bool _enabled) external onlyOwner {
        whitelistEnabled = _enabled;
    }

    function setWhitelist(address _user, bool _status) external onlyOwner {
        whitelist[_user] = _status;
    }

    function setMaxSlippage(uint256 _maxSlippageBps) external onlyOwner {
        require(_maxSlippageBps <= 10000, "Invalid slippage");
        maxSlippageBps = _maxSlippageBps;
    }

    function setDustThreshold(uint256 _dustThreshold) external onlyOwner {
        dustThreshold = _dustThreshold;
    }

    /// @notice 批准 DEX router（如 PancakeSwap V3 Router）
    function setApprovedRouter(address _router, bool _approved) external onlyOwner {
        approvedRouters[_router] = _approved;
        emit RouterUpdated(_router, _approved);
    }

    /// @notice 设置 router 是否使用 SwapRouter02 接口（Uniswap V3 on BSC 等）
    function setRouterV2(address _router, bool _isV2) external onlyOwner {
        isRouter02[_router] = _isV2;
        emit Router02Updated(_router, _isV2);
    }

    /// @notice 批量批准 routers
    function setApprovedRouters(address[] calldata _routers, bool _approved) external onlyOwner {
        for (uint256 i = 0; i < _routers.length; i++) {
            approvedRouters[_routers[i]] = _approved;
            emit RouterUpdated(_routers[i], _approved);
        }
    }

    /*//////////////////////////////////////////////////////////////
                         CORE ZAP FUNCTION
    //////////////////////////////////////////////////////////////*/

    /**
     * @notice 执行 Swap + Add Liquidity V3
     * @dev
     *   1. 接收用户代币
     *   2. 按照 swapSteps 执行 swap（直接调用底层 DEX）
     *   3. Mint V3 position
     *   4. 检查 dust < 1%，否则 revert
     *   5. 退还 dust
     *
     * @param params Zap 参数，包含 swap 路径
     * @return result 执行结果
     */
    function zap(ZapParams calldata params)
        external
        nonReentrant
        returns (ZapResult memory result)
    {
        // 白名单检查
        if (whitelistEnabled) {
            require(whitelist[msg.sender], "Not whitelisted");
        }

        // 验证
        require(params.slippageBps <= maxSlippageBps, "Slippage too high");
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.amount0 > 0 || params.amount1 > 0, "Zero amount");

        // 记录输入金额用于 dust 计算
        uint256 inputAmount0 = params.amount0;
        uint256 inputAmount1 = params.amount1;

        // 转入代币
        if (params.amount0 > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0);
        }
        if (params.amount1 > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1);
        }

        // 执行 swap 步骤
        for (uint256 i = 0; i < params.swapSteps.length; i++) {
            _executeSwapStep(params.swapSteps[i]);
        }

        // 获取 swap 后的余额
        uint256 bal0 = IERC20(params.token0).balanceOf(address(this));
        uint256 bal1 = IERC20(params.token1).balanceOf(address(this));

        // 获取 pool 信息
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(params.v3Config.pool).slot0();
        uint24 poolFee = IUniswapV3Pool(params.v3Config.pool).fee();

        // Mint position
        result = _mintV3Position(
            params.v3Config.positionManager,
            params.token0,
            params.token1,
            params.tickLower,
            params.tickUpper,
            bal0,
            bal1,
            params.slippageBps,
            params.recipient,
            poolFee
        );

        // 获取 dust
        uint256 dust0 = IERC20(params.token0).balanceOf(address(this));
        uint256 dust1 = IERC20(params.token1).balanceOf(address(this));

        // 计算 dust 价值占比
        uint256 inputValue = _calculateValueInToken1(inputAmount0, inputAmount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 强制 dust < 默认 1%（zap 函数使用固定值）
        if (inputValue > 0) {
            require(
                dustValue * 10000 <= inputValue * MAX_DUST_BPS,
                "Dust exceeds limit"
            );
        }

        // 退还 dust
        _refundDust(params.token0, msg.sender);
        _refundDust(params.token1, msg.sender);

        result.dust0 = dust0;
        result.dust1 = dust1;

        emit ZapExecuted(
            msg.sender,
            result.tokenId,
            inputAmount0,
            inputAmount1,
            result.liquidity,
            dust0,
            dust1
        );
    }

    /**
     * @notice 执行 Swap + 链上二次配平 + Add Liquidity V3
     * @dev
     *   最精确的配平方案：
     *   1. 执行主 swap (通过外部路由，如 OKX 推荐的低费率池)
     *   2. 获取实际余额
     *   3. 链上重新计算理想比例（使用最新价格）
     *   4. 如果 dust 预计 > 阈值，执行二次配平 swap
     *   5. Mint position
     *   6. 验证最终 dust < 1%
     *
     * 这解决了以下问题：
     *   - 链下计算到链上执行之间的价格变动
     *   - 外部路由报价与实际执行的差异
     *   - 价格影响导致的偏差
     *
     * @param params Zap 参数，包含主 swap 和二次配平配置
     * @return result 执行结果
     */
    function zapWithRebalance(ZapParamsWithRebalance calldata params)
        external
        nonReentrant
        returns (ZapResult memory result)
    {
        // 白名单检查
        if (whitelistEnabled) {
            require(whitelist[msg.sender], "Not whitelisted");
        }

        // 验证
        require(params.slippageBps <= maxSlippageBps, "Slippage too high");
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.amount0 > 0 || params.amount1 > 0, "Zero amount");

        // 记录输入金额用于 dust 计算
        uint256 inputAmount0 = params.amount0;
        uint256 inputAmount1 = params.amount1;

        // 转入代币
        if (params.amount0 > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0);
        }
        if (params.amount1 > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1);
        }

        // 执行主 swap 步骤
        for (uint256 i = 0; i < params.swapSteps.length; i++) {
            _executeSwapStep(params.swapSteps[i]);
        }

        // 获取主 swap 后的余额
        uint256 bal0 = IERC20(params.token0).balanceOf(address(this));
        uint256 bal1 = IERC20(params.token1).balanceOf(address(this));

        // 获取最新 pool 信息
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(params.v3Config.pool).slot0();
        uint24 poolFee = IUniswapV3Pool(params.v3Config.pool).fee();

        // 如果启用二次配平，执行链上配平
        if (params.rebalance.enabled && params.rebalance.router != address(0)) {
            (bal0, bal1) = _executeRebalanceSwap(
                params.v3Config.pool,
                params.rebalance.router,
                params.token0,
                params.token1,
                bal0,
                bal1,
                params.tickLower,
                params.tickUpper,
                params.rebalance.fee,
                params.rebalance.maxRebalanceBps,
                sqrtPriceX96
            );
        }

        // Mint position
        result = _mintV3Position(
            params.v3Config.positionManager,
            params.token0,
            params.token1,
            params.tickLower,
            params.tickUpper,
            bal0,
            bal1,
            params.slippageBps,
            params.recipient,
            poolFee
        );

        // 获取 dust
        uint256 dust0 = IERC20(params.token0).balanceOf(address(this));
        uint256 dust1 = IERC20(params.token1).balanceOf(address(this));

        // 计算 dust 价值占比
        uint256 inputValue = _calculateValueInToken1(inputAmount0, inputAmount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 使用传入的 maxDustBps，如果为 0 则使用默认值 1%
        uint256 effectiveMaxDustBps = params.maxDustBps > 0 ? params.maxDustBps : MAX_DUST_BPS;
        if (inputValue > 0) {
            require(
                dustValue * 10000 <= inputValue * effectiveMaxDustBps,
                "Dust exceeds limit"
            );
        }

        // 退还 dust
        _refundDust(params.token0, msg.sender);
        _refundDust(params.token1, msg.sender);

        result.dust0 = dust0;
        result.dust1 = dust1;

        emit ZapExecuted(
            msg.sender,
            result.tokenId,
            inputAmount0,
            inputAmount1,
            result.liquidity,
            dust0,
            dust1
        );
    }

    /**
     * @notice 执行 Swap + Add Liquidity V3（支持任意 calldata）
     * @dev
     *   - 支持 OKX 聚合器等复杂多路径交易
     *   - 传入从 OKX API 获取的 calldata
     *   - 适用于大额交易需要分拆路由的场景
     *
     * 工作流程:
     *   1. 接收用户代币
     *   2. 按照 swapCalls 执行任意 swap（调用 OKX Router 等）
     *   3. Mint V3 position
     *   4. 检查 dust < 1%，否则 revert
     *   5. 退还 dust
     *
     * @param params Zap 参数，包含 swap calldata
     * @return result 执行结果
     */
    function zapWithCalls(ZapParamsWithCalls calldata params)
        external
        nonReentrant
        returns (ZapResult memory result)
    {
        // 白名单检查
        if (whitelistEnabled) {
            require(whitelist[msg.sender], "Not whitelisted");
        }

        // 验证
        require(params.slippageBps <= maxSlippageBps, "Slippage too high");
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.amount0 > 0 || params.amount1 > 0, "Zero amount");

        // 记录输入金额用于 dust 计算
        uint256 inputAmount0 = params.amount0;
        uint256 inputAmount1 = params.amount1;

        // 转入代币
        if (params.amount0 > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0);
        }
        if (params.amount1 > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1);
        }

        // 执行任意 calldata swap
        for (uint256 i = 0; i < params.swapCalls.length; i++) {
            _executeSwapCall(params.swapCalls[i]);
        }

        // 获取 swap 后的余额
        uint256 bal0 = IERC20(params.token0).balanceOf(address(this));
        uint256 bal1 = IERC20(params.token1).balanceOf(address(this));

        // 获取 pool 信息
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(params.v3Config.pool).slot0();
        uint24 poolFee = IUniswapV3Pool(params.v3Config.pool).fee();

        // Mint position
        result = _mintV3Position(
            params.v3Config.positionManager,
            params.token0,
            params.token1,
            params.tickLower,
            params.tickUpper,
            bal0,
            bal1,
            params.slippageBps,
            params.recipient,
            poolFee
        );

        // 获取 dust
        uint256 dust0 = IERC20(params.token0).balanceOf(address(this));
        uint256 dust1 = IERC20(params.token1).balanceOf(address(this));

        // 计算 dust 价值占比
        uint256 inputValue = _calculateValueInToken1(inputAmount0, inputAmount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 强制 dust < 默认 1%（zapWithCalls 使用固定值）
        if (inputValue > 0) {
            require(
                dustValue * 10000 <= inputValue * MAX_DUST_BPS,
                "Dust exceeds limit"
            );
        }

        // 退还 dust
        _refundDust(params.token0, msg.sender);
        _refundDust(params.token1, msg.sender);

        result.dust0 = dust0;
        result.dust1 = dust1;

        emit ZapExecuted(
            msg.sender,
            result.tokenId,
            inputAmount0,
            inputAmount1,
            result.liquidity,
            dust0,
            dust1
        );
    }

    /**
     * @notice 执行 Swap + Add Liquidity V3（支持 OKX calldata + 链上二次配平）
     * @dev
     *   - 支持 OKX 聚合器等复杂多路径交易
     *   - 在 OKX swap 后执行链上二次配平，解决价格偏差导致的滑点问题
     *   - 适用于 OKX 报价与执行时价格有偏差的场景
     *
     * 工作流程:
     *   1. 接收用户代币
     *   2. 按照 swapCalls 执行 OKX swap（调用 OKX Router）
     *   3. 获取 swap 后的实际余额
     *   4. 如果启用二次配平，计算理想比例并执行微调 swap
     *   5. Mint V3 position
     *   6. 检查 dust 是否超限，否则 revert
     *   7. 退还 dust
     *
     * @param params Zap 参数，包含 OKX swap calldata 和二次配平配置
     * @return result 执行结果
     */
    function zapWithCallsAndRebalance(ZapParamsWithCallsAndRebalance calldata params)
        external
        nonReentrant
        returns (ZapResult memory result)
    {
        // 白名单检查
        if (whitelistEnabled) {
            require(whitelist[msg.sender], "Not whitelisted");
        }

        // 验证
        require(params.slippageBps <= maxSlippageBps, "Slippage too high");
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.amount0 > 0 || params.amount1 > 0, "Zero amount");

        // 记录输入金额用于 dust 计算
        uint256 inputAmount0 = params.amount0;
        uint256 inputAmount1 = params.amount1;

        // 转入代币
        if (params.amount0 > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0);
        }
        if (params.amount1 > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1);
        }

        // 执行 OKX 聚合器 swap
        for (uint256 i = 0; i < params.swapCalls.length; i++) {
            _executeSwapCall(params.swapCalls[i]);
        }

        // 获取 swap 后的余额
        uint256 bal0 = IERC20(params.token0).balanceOf(address(this));
        uint256 bal1 = IERC20(params.token1).balanceOf(address(this));

        // 获取最新的 pool 价格
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(params.v3Config.pool).slot0();
        uint24 poolFee = IUniswapV3Pool(params.v3Config.pool).fee();

        // 如果启用二次配平，执行链上微调 swap
        if (params.rebalance.enabled && params.rebalance.router != address(0)) {
            (bal0, bal1) = _executeRebalanceSwap(
                params.v3Config.pool,
                params.rebalance.router,
                params.token0,
                params.token1,
                bal0,
                bal1,
                params.tickLower,
                params.tickUpper,
                params.rebalance.fee,
                params.rebalance.maxRebalanceBps,
                sqrtPriceX96
            );
        }

        // Mint position
        result = _mintV3Position(
            params.v3Config.positionManager,
            params.token0,
            params.token1,
            params.tickLower,
            params.tickUpper,
            bal0,
            bal1,
            params.slippageBps,
            params.recipient,
            poolFee
        );

        // 获取 dust
        uint256 dust0 = IERC20(params.token0).balanceOf(address(this));
        uint256 dust1 = IERC20(params.token1).balanceOf(address(this));

        // 计算 dust 价值占比
        uint256 inputValue = _calculateValueInToken1(inputAmount0, inputAmount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 使用传入的 maxDustBps，如果为 0 则使用默认值 1%
        uint256 effectiveMaxDustBps = params.maxDustBps > 0 ? params.maxDustBps : MAX_DUST_BPS;
        if (inputValue > 0) {
            require(
                dustValue * 10000 <= inputValue * effectiveMaxDustBps,
                "Dust exceeds limit"
            );
        }

        // 退还 dust
        _refundDust(params.token0, msg.sender);
        _refundDust(params.token1, msg.sender);

        result.dust0 = dust0;
        result.dust1 = dust1;

        emit ZapExecuted(
            msg.sender,
            result.tokenId,
            inputAmount0,
            inputAmount1,
            result.liquidity,
            dust0,
            dust1
        );
    }

    /*//////////////////////////////////////////////////////////////
                          SWAP EXECUTION
    //////////////////////////////////////////////////////////////*/

    /**
     * @notice 执行链上二次配平 swap
     * @dev
     *   - 根据当前余额和最新价格，计算需要配平的数量
     *   - 只有当预计 dust > 某个阈值时才执行配平
     *   - 使用指定的 router 和费率进行 swap
     */
    function _executeRebalanceSwap(
        address pool,
        address router,
        address token0,
        address token1,
        uint256 amount0,
        uint256 amount1,
        int24 tickLower,
        int24 tickUpper,
        uint24 swapFee,
        uint256 maxRebalanceBps,
        uint160 sqrtPriceX96
    ) internal returns (uint256 newAmount0, uint256 newAmount1) {
        // 计算理想比例
        uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
        uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);

        // 检查价格是否在范围内
        if (sqrtPriceX96 <= sqrtRatioAX96 || sqrtPriceX96 >= sqrtRatioBX96) {
            return (amount0, amount1);
        }

        // 计算添加流动性能使用多少
        uint128 liquidity = LiquidityAmounts.getLiquidityForAmounts(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            amount0,
            amount1
        );

        (uint256 amount0Needed, uint256 amount1Needed) = LiquidityAmounts.getAmountsForLiquidity(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            liquidity
        );

        // 计算预计 dust
        uint256 dust0 = amount0 > amount0Needed ? amount0 - amount0Needed : 0;
        uint256 dust1 = amount1 > amount1Needed ? amount1 - amount1Needed : 0;

        // 计算 dust 价值
        uint256 totalValue = _calculateValueInToken1(amount0, amount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 如果 dust 太小，不需要配平
        // 阈值: dust 价值 < 总价值的 0.1%
        if (totalValue == 0 || dustValue * 1000 < totalValue) {
            return (amount0, amount1);
        }

        // 确定配平方向和数量
        // 转换 dust 到统一单位比较
        uint256 dust0ValueInToken1 = FullMath.mulDiv(
            FullMath.mulDiv(dust0, uint256(sqrtPriceX96), Q96),
            uint256(sqrtPriceX96),
            Q96
        );

        bool zeroForOne;
        uint256 rebalanceAmount;

        if (dust0ValueInToken1 > dust1) {
            // token0 多余，swap token0 -> token1
            zeroForOne = true;
            // 计算需要 swap 的数量（保守估计）
            // rebalanceAmount ≈ dust0 / 2 （因为 swap 会减少 dust0，增加 dust1）
            rebalanceAmount = dust0 / 2;
        } else {
            // token1 多余，swap token1 -> token0
            zeroForOne = false;
            rebalanceAmount = dust1 / 2;
        }

        // 限制配平数量不超过最大比例
        uint256 maxRebalance = zeroForOne
            ? (amount0 * maxRebalanceBps) / 10000
            : (amount1 * maxRebalanceBps) / 10000;

        if (rebalanceAmount > maxRebalance) {
            rebalanceAmount = maxRebalance;
        }

        // 如果配平数量太小，跳过
        if (rebalanceAmount == 0) {
            return (amount0, amount1);
        }

        // 执行配平 swap (移除白名单限制，调用者自行负责 router 安全性)
        address tokenIn = zeroForOne ? token0 : token1;
        address tokenOut = zeroForOne ? token1 : token0;

        IERC20(tokenIn).forceApprove(router, rebalanceAmount);

        uint256 amountOut;
        if (isRouter02[router]) {
            ISwapRouter02.ExactInputSingleParams memory swapParams = ISwapRouter02.ExactInputSingleParams({
                tokenIn: tokenIn,
                tokenOut: tokenOut,
                fee: swapFee,
                recipient: address(this),
                amountIn: rebalanceAmount,
                amountOutMinimum: 0, // 二次配平不设置最小输出，因为数量小
                sqrtPriceLimitX96: 0
            });
            amountOut = ISwapRouter02(router).exactInputSingle(swapParams);
        } else {
            ISwapRouter.ExactInputSingleParams memory swapParams = ISwapRouter.ExactInputSingleParams({
                tokenIn: tokenIn,
                tokenOut: tokenOut,
                fee: swapFee,
                recipient: address(this),
                deadline: block.timestamp,
                amountIn: rebalanceAmount,
                amountOutMinimum: 0,
                sqrtPriceLimitX96: 0
            });
            amountOut = ISwapRouter(router).exactInputSingle(swapParams);
        }

        IERC20(tokenIn).forceApprove(router, 0);

        // 计算新余额
        if (zeroForOne) {
            newAmount0 = amount0 - rebalanceAmount;
            newAmount1 = amount1 + amountOut;
        } else {
            newAmount0 = amount0 + amountOut;
            newAmount1 = amount1 - rebalanceAmount;
        }

        emit SwapExecuted(router, tokenIn, tokenOut, rebalanceAmount, amountOut);
    }

    /**
     * @notice 执行单跳 swap
     * @dev 直接调用底层 DEX router，支持 SwapRouter 和 SwapRouter02 接口
     */
    function _executeSwapStep(SwapStep calldata step) internal {
        require(step.amountIn > 0, "Zero swap amount");
        // 移除白名单限制，调用者自行负责 router 安全性

        // Approve router
        IERC20(step.tokenIn).forceApprove(step.router, step.amountIn);

        // 记录输出代币余额
        uint256 balanceBefore = IERC20(step.tokenOut).balanceOf(address(this));

        uint256 amountOut;

        // 检查是否是 SwapRouter02 接口（Uniswap V3 on BSC）
        if (isRouter02[step.router]) {
            // SwapRouter02 接口（无 deadline 参数）
            ISwapRouter02.ExactInputSingleParams memory swapParams = ISwapRouter02.ExactInputSingleParams({
                tokenIn: step.tokenIn,
                tokenOut: step.tokenOut,
                fee: step.fee,
                recipient: address(this),
                amountIn: step.amountIn,
                amountOutMinimum: step.minAmountOut,
                sqrtPriceLimitX96: 0
            });
            amountOut = ISwapRouter02(step.router).exactInputSingle(swapParams);
        } else {
            // 原始 SwapRouter 接口（有 deadline 参数）
            ISwapRouter.ExactInputSingleParams memory swapParams = ISwapRouter.ExactInputSingleParams({
                tokenIn: step.tokenIn,
                tokenOut: step.tokenOut,
                fee: step.fee,
                recipient: address(this),
                deadline: block.timestamp,
                amountIn: step.amountIn,
                amountOutMinimum: step.minAmountOut,
                sqrtPriceLimitX96: 0
            });
            amountOut = ISwapRouter(step.router).exactInputSingle(swapParams);
        }

        // 验证输出
        uint256 balanceAfter = IERC20(step.tokenOut).balanceOf(address(this));
        require(balanceAfter >= balanceBefore + step.minAmountOut, "Insufficient output");

        // 重置 approval
        IERC20(step.tokenIn).forceApprove(step.router, 0);

        emit SwapExecuted(step.router, step.tokenIn, step.tokenOut, step.amountIn, amountOut);
    }

    // Native token address used by OKX and other aggregators
    address private constant NATIVE_TOKEN = 0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE;

    /**
     * @notice 执行任意 calldata swap
     * @dev
     *   - 支持 OKX 聚合器等复杂路由
     *   - 传入从 OKX API 获取的 calldata，直接调用
     *   - 支持原生代币 (ETH/BNB) swap
     *   - 安全: 检查输出代币余额变化
     */
    function _executeSwapCall(SwapCall calldata call) internal {
        require(call.amountIn > 0, "Zero swap amount");
        // 移除白名单限制，调用者自行负责 target 安全性

        bool isNativeIn = call.tokenIn == NATIVE_TOKEN || call.tokenIn == address(0);
        bool isNativeOut = call.tokenOut == NATIVE_TOKEN || call.tokenOut == address(0);

        // 确定 approve 目标：如果 approveTarget 为 0，则使用 target
        address spender = call.approveTarget != address(0) ? call.approveTarget : call.target;

        // 如果不是原生代币输入，需要 approve
        if (!isNativeIn) {
            IERC20(call.tokenIn).forceApprove(spender, call.amountIn);
        }

        // 记录输出代币余额
        uint256 balanceBefore;
        if (isNativeOut) {
            balanceBefore = address(this).balance;
        } else {
            balanceBefore = IERC20(call.tokenOut).balanceOf(address(this));
        }

        // 执行任意 calldata（调用 OKX Router 等）
        // 支持传递 value 用于原生代币 swap
        (bool success, ) = call.target.call{value: call.value}(call.callData);
        require(success, "Swap call failed");

        // 验证输出
        uint256 balanceAfter;
        if (isNativeOut) {
            balanceAfter = address(this).balance;
        } else {
            balanceAfter = IERC20(call.tokenOut).balanceOf(address(this));
        }
        uint256 amountOut = balanceAfter - balanceBefore;
        require(amountOut >= call.minAmountOut, "Insufficient output");

        // 重置 approval（仅对 ERC20）
        if (!isNativeIn) {
            IERC20(call.tokenIn).forceApprove(spender, 0);
        }

        emit SwapCallExecuted(call.target, call.tokenIn, call.tokenOut, call.amountIn, amountOut);
    }

    /**
     * @notice 执行多跳 swap（用于复杂路由）
     * @dev path 格式: tokenA, fee, tokenB, fee, tokenC...
     */
    function executeMultiHopSwap(MultiHopSwap calldata swap) external returns (uint256 amountOut) {
        require(swap.amountIn > 0, "Zero swap amount");
        // 移除白名单限制，调用者自行负责 router 安全性

        // 从 path 中提取第一个 token (前 20 bytes)
        require(swap.path.length >= 20, "Invalid path");
        address tokenIn;
        bytes memory pathData = swap.path;
        assembly {
            tokenIn := mload(add(pathData, 20))
        }

        IERC20(tokenIn).forceApprove(swap.router, swap.amountIn);

        ISwapRouter.ExactInputParams memory params = ISwapRouter.ExactInputParams({
            path: swap.path,
            recipient: address(this),
            deadline: block.timestamp,
            amountIn: swap.amountIn,
            amountOutMinimum: swap.minAmountOut
        });

        amountOut = ISwapRouter(swap.router).exactInput(params);

        IERC20(tokenIn).forceApprove(swap.router, 0);
    }

    /*//////////////////////////////////////////////////////////////
                          POSITION MINTING
    //////////////////////////////////////////////////////////////*/

    function _mintV3Position(
        address positionManager,
        address token0,
        address token1,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0,
        uint256 amount1,
        uint256 slippageBps,
        address recipient,
        uint24 poolFee
    ) internal returns (ZapResult memory result) {
        if (amount0 > 0) {
            IERC20(token0).forceApprove(positionManager, amount0);
        }
        if (amount1 > 0) {
            IERC20(token1).forceApprove(positionManager, amount1);
        }

        // Calculate min amounts with adaptive slippage
        // When one amount is very small relative to the other, use more relaxed min
        // This handles edge cases when price is near tick boundary
        uint256 amount0Min;
        uint256 amount1Min;

        // Check for narrow tick range - these are much more sensitive to price movements
        // Narrow range = tick range is <= 2 * tickSpacing (e.g., 20 ticks for 0.05% pool)
        int24 tickRange = tickUpper - tickLower;
        int24 tickSpacing = _getTickSpacing(poolFee);
        bool isNarrowRange = tickRange <= tickSpacing * 3; // <= 3 tick spacings

        if (amount0 == 0) {
            amount0Min = 0;
            // For narrow ranges, use even more relaxed slippage
            uint256 effectiveSlippage = isNarrowRange ? _max(slippageBps, 3000) : slippageBps;
            amount1Min = FullMath.mulDiv(amount1, 10000 - effectiveSlippage, 10000);
        } else if (amount1 == 0) {
            uint256 effectiveSlippage = isNarrowRange ? _max(slippageBps, 3000) : slippageBps;
            amount0Min = FullMath.mulDiv(amount0, 10000 - effectiveSlippage, 10000);
            amount1Min = 0;
        } else {
            // Check if amounts are highly imbalanced (ratio > 100:1 in value terms)
            // In such cases, the smaller amount might become 0 after price movement
            // Use more relaxed slippage for the smaller amount
            uint256 effectiveSlippage0 = slippageBps;
            uint256 effectiveSlippage1 = slippageBps;

            // For narrow ranges, use at least 30% slippage tolerance
            if (isNarrowRange) {
                effectiveSlippage0 = _max(effectiveSlippage0, 3000);
                effectiveSlippage1 = _max(effectiveSlippage1, 3000);
            }

            // If one amount is < 1% of the other in raw terms, use 50% slippage for it
            if (amount0 < amount1 / 100) {
                effectiveSlippage0 = 5000; // 50%
            }
            if (amount1 < amount0 / 100) {
                effectiveSlippage1 = 5000; // 50%
            }

            amount0Min = FullMath.mulDiv(amount0, 10000 - effectiveSlippage0, 10000);
            amount1Min = FullMath.mulDiv(amount1, 10000 - effectiveSlippage1, 10000);
        }

        INonfungiblePositionManager.MintParams memory mintParams = INonfungiblePositionManager.MintParams({
            token0: token0,
            token1: token1,
            fee: poolFee,
            tickLower: tickLower,
            tickUpper: tickUpper,
            amount0Desired: amount0,
            amount1Desired: amount1,
            amount0Min: amount0Min,
            amount1Min: amount1Min,
            recipient: recipient,
            deadline: block.timestamp
        });

        (uint256 tokenId, uint128 liquidity, uint256 amount0Used, uint256 amount1Used) =
            INonfungiblePositionManager(positionManager).mint(mintParams);

        result = ZapResult({
            tokenId: tokenId,
            liquidity: liquidity,
            amount0Used: amount0Used,
            amount1Used: amount1Used,
            dust0: 0,
            dust1: 0
        });

        // 重置 approvals
        IERC20(token0).forceApprove(positionManager, 0);
        IERC20(token1).forceApprove(positionManager, 0);
    }

    /*//////////////////////////////////////////////////////////////
                          HELPER FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    /**
     * @notice Get tick spacing for a fee tier
     * @dev Standard Uniswap V3 tick spacings
     */
    function _getTickSpacing(uint24 fee) internal pure returns (int24) {
        if (fee == 100) return 1;
        if (fee == 500) return 10;
        if (fee == 2500) return 50;
        if (fee == 3000) return 60;
        if (fee == 10000) return 200;
        return 10; // Default
    }

    /**
     * @notice Return the maximum of two uint256 values
     */
    function _max(uint256 a, uint256 b) internal pure returns (uint256) {
        return a >= b ? a : b;
    }

    function _calculateValueInToken1(
        uint256 amount0,
        uint256 amount1,
        uint160 sqrtPriceX96
    ) internal pure returns (uint256 value) {
        uint256 amount0InToken1 = FullMath.mulDiv(
            FullMath.mulDiv(amount0, uint256(sqrtPriceX96), Q96),
            uint256(sqrtPriceX96),
            Q96
        );
        value = amount0InToken1 + amount1;
    }

    function _refundDust(address token, address recipient) internal {
        uint256 balance = IERC20(token).balanceOf(address(this));
        if (balance > dustThreshold) {
            IERC20(token).safeTransfer(recipient, balance);
        }
    }

    function emergencyWithdraw(address token, uint256 amount) external onlyOwner {
        if (token == address(0)) {
            payable(owner()).transfer(amount);
        } else {
            IERC20(token).safeTransfer(owner(), amount);
        }
    }

    /*//////////////////////////////////////////////////////////////
                          VIEW FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    /**
     * @notice 计算最优 swap 数量（供 Bot 链下使用）
     * @dev Bot 可以用这个结果去 OKX API 询价
     */
    function calculateOptimalSwap(
        address pool,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0In,
        uint256 amount1In
    ) external view returns (bool zeroForOne, uint256 amountToSwap) {
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(pool).slot0();
        uint24 poolFee = IUniswapV3Pool(pool).fee();

        uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
        uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);

        if (sqrtPriceX96 <= sqrtRatioAX96 || sqrtPriceX96 >= sqrtRatioBX96) {
            return (false, 0);
        }

        uint128 virtualLiquidity = 1e18;
        (uint256 amount0Ideal, uint256 amount1Ideal) = LiquidityAmounts.getAmountsForLiquidity(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            virtualLiquidity
        );

        if (amount0Ideal == 0 || amount1Ideal == 0) {
            return (false, 0);
        }

        uint256 value0 = FullMath.mulDiv(amount0In, amount1Ideal, 1);
        uint256 value1 = FullMath.mulDiv(amount1In, amount0Ideal, 1);

        uint256 feeMultiplier = 1e6 - uint256(poolFee);

        if (value0 > value1) {
            zeroForOne = true;
            uint256 numerator = value0 - value1;

            uint256 amount0IdealInToken1 = FullMath.mulDiv(
                FullMath.mulDiv(amount0Ideal, uint256(sqrtPriceX96), Q96),
                uint256(sqrtPriceX96),
                Q96
            );

            uint256 adjustedAmount0InToken1 = FullMath.mulDiv(amount0IdealInToken1, feeMultiplier, 1e6);
            uint256 denominator = amount1Ideal + adjustedAmount0InToken1;

            if (denominator == 0) return (false, 0);

            amountToSwap = numerator / denominator;
            if (amountToSwap > amount0In) amountToSwap = amount0In;

        } else if (value1 > value0) {
            zeroForOne = false;
            uint256 numerator = value1 - value0;

            uint256 amount1IdealInToken0 = FullMath.mulDiv(
                FullMath.mulDiv(amount1Ideal, Q96, uint256(sqrtPriceX96)),
                Q96,
                uint256(sqrtPriceX96)
            );

            uint256 adjustedAmount1InToken0 = FullMath.mulDiv(amount1IdealInToken0, feeMultiplier, 1e6);
            uint256 denominator = amount0Ideal + adjustedAmount1InToken0;

            if (denominator == 0) return (false, 0);

            amountToSwap = numerator / denominator;
            if (amountToSwap > amount1In) amountToSwap = amount1In;
        }
    }

    /**
     * @notice 获取理想代币比例
     */
    function getIdealRatio(
        address pool,
        int24 tickLower,
        int24 tickUpper
    ) external view returns (uint256 amount0Ideal, uint256 amount1Ideal) {
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(pool).slot0();
        uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
        uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);

        uint128 virtualLiquidity = 1e18;
        (amount0Ideal, amount1Ideal) = LiquidityAmounts.getAmountsForLiquidity(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            virtualLiquidity
        );
    }

    /**
     * @notice 计算最优 swap 数量（支持外部 swap 费率）
     * @dev
     *   - 用于跨 DEX swap 场景（如通过 Uniswap 0.3% swap，在 PancakeSwap 1% 池添加流动性）
     *   - swapFee 是实际执行 swap 的池子费率，不是目标流动性池的费率
     *   - Bot 应该先调用此函数，然后用结果去 OKX 询价
     *
     * @param pool 目标流动性池地址
     * @param tickLower 仓位下界 tick
     * @param tickUpper 仓位上界 tick
     * @param amount0In 输入的 token0 数量
     * @param amount1In 输入的 token1 数量
     * @param swapFee Swap 池的费率 (e.g., 500 = 0.05%, 3000 = 0.3%, 10000 = 1%)
     * @return zeroForOne 是否从 token0 换成 token1
     * @return amountToSwap 需要 swap 的数量
     */
    function calculateOptimalSwapWithFee(
        address pool,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0In,
        uint256 amount1In,
        uint24 swapFee
    ) external view returns (bool zeroForOne, uint256 amountToSwap) {
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(pool).slot0();

        uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
        uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);

        if (sqrtPriceX96 <= sqrtRatioAX96 || sqrtPriceX96 >= sqrtRatioBX96) {
            return (false, 0);
        }

        uint128 virtualLiquidity = 1e18;
        (uint256 amount0Ideal, uint256 amount1Ideal) = LiquidityAmounts.getAmountsForLiquidity(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            virtualLiquidity
        );

        if (amount0Ideal == 0 || amount1Ideal == 0) {
            return (false, 0);
        }

        uint256 value0 = FullMath.mulDiv(amount0In, amount1Ideal, 1);
        uint256 value1 = FullMath.mulDiv(amount1In, amount0Ideal, 1);

        // 使用传入的 swapFee 而不是目标池的 fee
        uint256 feeMultiplier = 1e6 - uint256(swapFee);

        if (value0 > value1) {
            zeroForOne = true;
            uint256 numerator = value0 - value1;

            uint256 amount0IdealInToken1 = FullMath.mulDiv(
                FullMath.mulDiv(amount0Ideal, uint256(sqrtPriceX96), Q96),
                uint256(sqrtPriceX96),
                Q96
            );

            uint256 adjustedAmount0InToken1 = FullMath.mulDiv(amount0IdealInToken1, feeMultiplier, 1e6);
            uint256 denominator = amount1Ideal + adjustedAmount0InToken1;

            if (denominator == 0) return (false, 0);

            amountToSwap = numerator / denominator;
            if (amountToSwap > amount0In) amountToSwap = amount0In;

        } else if (value1 > value0) {
            zeroForOne = false;
            uint256 numerator = value1 - value0;

            uint256 amount1IdealInToken0 = FullMath.mulDiv(
                FullMath.mulDiv(amount1Ideal, Q96, uint256(sqrtPriceX96)),
                Q96,
                uint256(sqrtPriceX96)
            );

            uint256 adjustedAmount1InToken0 = FullMath.mulDiv(amount1IdealInToken0, feeMultiplier, 1e6);
            uint256 denominator = amount0Ideal + adjustedAmount1InToken0;

            if (denominator == 0) return (false, 0);

            amountToSwap = numerator / denominator;
            if (amountToSwap > amount1In) amountToSwap = amount1In;
        }
    }

    /**
     * @notice 根据预期 swap 输出计算精确的 swap 数量
     * @dev
     *   - Bot 首先调用 calculateOptimalSwap 获得初始估算
     *   - 然后用 OKX API 获取真实报价（含价格影响）
     *   - 最后用此函数基于真实报价反推最优 swap 数量
     *   - 这样可以显著减少 dust
     *
     * @param pool 目标流动性池地址
     * @param tickLower 仓位下界 tick
     * @param tickUpper 仓位上界 tick
     * @param amount0In 输入的 token0 数量
     * @param amount1In 输入的 token1 数量
     * @param zeroForOne swap 方向
     * @param expectedSwapOutput OKX 报价的预期输出数量
     * @param swapInputAmount 预计 swap 的输入数量
     * @return adjustedSwapAmount 调整后的最优 swap 数量
     */
    function calculatePreciseSwapAmount(
        address pool,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0In,
        uint256 amount1In,
        bool zeroForOne,
        uint256 expectedSwapOutput,
        uint256 swapInputAmount
    ) external view returns (uint256 adjustedSwapAmount) {
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(pool).slot0();
        uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
        uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);

        if (sqrtPriceX96 <= sqrtRatioAX96 || sqrtPriceX96 >= sqrtRatioBX96) {
            return swapInputAmount;
        }

        // 计算 swap 后的代币数量
        uint256 newAmount0;
        uint256 newAmount1;

        if (zeroForOne) {
            // token0 -> token1
            newAmount0 = amount0In - swapInputAmount;
            newAmount1 = amount1In + expectedSwapOutput;
        } else {
            // token1 -> token0
            newAmount0 = amount0In + expectedSwapOutput;
            newAmount1 = amount1In - swapInputAmount;
        }

        // 计算 swap 后能添加多少流动性
        uint128 liquidity = LiquidityAmounts.getLiquidityForAmounts(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            newAmount0,
            newAmount1
        );

        // 计算这个流动性需要多少代币
        (uint256 amount0Needed, uint256 amount1Needed) = LiquidityAmounts.getAmountsForLiquidity(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            liquidity
        );

        // 计算 dust
        uint256 dust0 = newAmount0 > amount0Needed ? newAmount0 - amount0Needed : 0;
        uint256 dust1 = newAmount1 > amount1Needed ? newAmount1 - amount1Needed : 0;

        // 如果 dust 太大，尝试调整 swap 数量
        // 计算有效交换率
        if (swapInputAmount == 0 || expectedSwapOutput == 0) {
            return swapInputAmount;
        }

        // effectiveRate = expectedSwapOutput / swapInputAmount
        // 如果 zeroForOne (token0 -> token1):
        //   - dust0 多 -> 多 swap 一些 token0
        //   - dust1 多 -> 少 swap 一些 token0
        // 如果 !zeroForOne (token1 -> token0):
        //   - dust0 多 -> 少 swap 一些 token1
        //   - dust1 多 -> 多 swap 一些 token1

        if (zeroForOne) {
            // 转换 dust 到相同单位进行比较
            uint256 dust0ValueInToken1 = FullMath.mulDiv(
                FullMath.mulDiv(dust0, uint256(sqrtPriceX96), Q96),
                uint256(sqrtPriceX96),
                Q96
            );

            if (dust0ValueInToken1 > dust1) {
                // token0 多余，需要多 swap 一点
                uint256 excessValue = dust0ValueInToken1 - dust1;
                // 需要额外 swap 的 token0 数量 = excessValue / (1 + rate)
                // 其中 rate = expectedSwapOutput / swapInputAmount
                // 简化: adjustment = excessValue * swapInputAmount / (swapInputAmount + expectedSwapOutput)
                uint256 adjustment = FullMath.mulDiv(excessValue, swapInputAmount, swapInputAmount + expectedSwapOutput);
                // 转换回 token0 单位
                uint256 adjustmentInToken0 = FullMath.mulDiv(
                    FullMath.mulDiv(adjustment, Q96, uint256(sqrtPriceX96)),
                    Q96,
                    uint256(sqrtPriceX96)
                );
                adjustedSwapAmount = swapInputAmount + adjustmentInToken0 / 2; // 保守调整
            } else {
                // token1 多余，需要少 swap 一点
                uint256 excessValue = dust1 - dust0ValueInToken1;
                uint256 adjustment = FullMath.mulDiv(excessValue, swapInputAmount, swapInputAmount + expectedSwapOutput);
                uint256 adjustmentInToken0 = FullMath.mulDiv(
                    FullMath.mulDiv(adjustment, Q96, uint256(sqrtPriceX96)),
                    Q96,
                    uint256(sqrtPriceX96)
                );
                if (swapInputAmount > adjustmentInToken0 / 2) {
                    adjustedSwapAmount = swapInputAmount - adjustmentInToken0 / 2;
                } else {
                    adjustedSwapAmount = swapInputAmount;
                }
            }
        } else {
            // token1 -> token0
            uint256 dust1ValueInToken0 = FullMath.mulDiv(
                FullMath.mulDiv(dust1, Q96, uint256(sqrtPriceX96)),
                Q96,
                uint256(sqrtPriceX96)
            );

            if (dust1ValueInToken0 > dust0) {
                // token1 多余，需要多 swap 一点
                uint256 excessValue = dust1ValueInToken0 - dust0;
                uint256 adjustment = FullMath.mulDiv(excessValue, swapInputAmount, swapInputAmount + expectedSwapOutput);
                uint256 adjustmentInToken1 = FullMath.mulDiv(
                    FullMath.mulDiv(adjustment, uint256(sqrtPriceX96), Q96),
                    uint256(sqrtPriceX96),
                    Q96
                );
                adjustedSwapAmount = swapInputAmount + adjustmentInToken1 / 2;
            } else {
                // token0 多余，需要少 swap 一点
                uint256 excessValue = dust0 - dust1ValueInToken0;
                uint256 adjustment = FullMath.mulDiv(excessValue, swapInputAmount, swapInputAmount + expectedSwapOutput);
                uint256 adjustmentInToken1 = FullMath.mulDiv(
                    FullMath.mulDiv(adjustment, uint256(sqrtPriceX96), Q96),
                    uint256(sqrtPriceX96),
                    Q96
                );
                if (swapInputAmount > adjustmentInToken1 / 2) {
                    adjustedSwapAmount = swapInputAmount - adjustmentInToken1 / 2;
                } else {
                    adjustedSwapAmount = swapInputAmount;
                }
            }
        }

        // 确保不超过输入数量
        uint256 maxInput = zeroForOne ? amount0In : amount1In;
        if (adjustedSwapAmount > maxInput) {
            adjustedSwapAmount = maxInput;
        }
    }

    /**
     * @notice 模拟 swap 后的 dust 数量
     * @dev 供 Bot 链下使用，用于评估不同 swap 数量的效果
     */
    function simulateDust(
        address pool,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0In,
        uint256 amount1In,
        bool zeroForOne,
        uint256 swapInputAmount,
        uint256 expectedSwapOutput
    ) external view returns (uint256 dust0, uint256 dust1, uint256 dustValueInToken1) {
        (uint160 sqrtPriceX96, , , , , , ) = IUniswapV3Pool(pool).slot0();
        uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
        uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);

        // 计算 swap 后的代币数量
        uint256 newAmount0;
        uint256 newAmount1;

        if (zeroForOne) {
            newAmount0 = amount0In - swapInputAmount;
            newAmount1 = amount1In + expectedSwapOutput;
        } else {
            newAmount0 = amount0In + expectedSwapOutput;
            newAmount1 = amount1In - swapInputAmount;
        }

        // 计算 swap 后能添加多少流动性
        uint128 liquidity = LiquidityAmounts.getLiquidityForAmounts(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            newAmount0,
            newAmount1
        );

        // 计算这个流动性需要多少代币
        (uint256 amount0Needed, uint256 amount1Needed) = LiquidityAmounts.getAmountsForLiquidity(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            liquidity
        );

        // 计算 dust
        dust0 = newAmount0 > amount0Needed ? newAmount0 - amount0Needed : 0;
        dust1 = newAmount1 > amount1Needed ? newAmount1 - amount1Needed : 0;

        // 计算 dust 总价值（以 token1 计价）
        dustValueInToken1 = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);
    }

    /*//////////////////////////////////////////////////////////////
                          V4 ZAP FUNCTIONS
    //////////////////////////////////////////////////////////////*/

    /**
     * @notice 执行 Swap + 链上二次配平 + Add Liquidity V4
     * @dev
     *   V4 版本的原子化流动性添加：
     *   1. 接收用户代币
     *   2. 执行主 swap (通过 V3 router)
     *   3. 获取实际余额
     *   4. 从 V4 StateView 获取最新价格
     *   5. 链上计算理想比例，执行二次配平 swap
     *   6. 调用 V4 PositionManager.modifyLiquidities 添加流动性
     *   7. 验证最终 dust < 1%
     *   8. 退还 dust
     *
     * @param params V4 Zap 参数
     * @return result 执行结果
     */
    function zapV4WithRebalance(ZapV4Params calldata params)
        external
        nonReentrant
        returns (ZapV4Result memory result)
    {
        // 白名单检查
        if (whitelistEnabled) {
            require(whitelist[msg.sender], "Not whitelisted");
        }

        // 验证
        require(params.slippageBps <= maxSlippageBps, "Slippage too high");
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.amount0 > 0 || params.amount1 > 0, "Zero amount");

        // 记录输入金额用于 dust 计算
        uint256 inputAmount0 = params.amount0;
        uint256 inputAmount1 = params.amount1;

        // 转入代币
        if (params.amount0 > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0);
        }
        if (params.amount1 > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1);
        }

        // 执行主 swap 步骤（使用 V3 router）
        for (uint256 i = 0; i < params.swapSteps.length; i++) {
            _executeSwapStep(params.swapSteps[i]);
        }

        // 获取主 swap 后的余额
        uint256 bal0 = IERC20(params.token0).balanceOf(address(this));
        uint256 bal1 = IERC20(params.token1).balanceOf(address(this));

        // 从 V4 StateView 获取最新价格
        PoolId poolId = PoolIdLibrary.toId(params.v4Config.poolKey);
        (uint160 sqrtPriceX96, , , ) = IStateView(params.v4Config.stateView).getSlot0(poolId);

        // 如果启用二次配平，执行链上配平
        if (params.rebalance.enabled && params.rebalance.router != address(0)) {
            (bal0, bal1) = _executeRebalanceSwapV4(
                params.v4Config.stateView,
                poolId,
                params.rebalance.router,
                params.token0,
                params.token1,
                bal0,
                bal1,
                params.tickLower,
                params.tickUpper,
                params.rebalance.fee,
                params.rebalance.maxRebalanceBps,
                sqrtPriceX96
            );
        }

        // Mint V4 position
        result = _mintV4Position(
            params.v4Config.positionManager,
            params.v4Config.poolKey,
            params.tickLower,
            params.tickUpper,
            bal0,
            bal1,
            params.slippageBps,
            params.recipient,
            sqrtPriceX96  // Pass the actual price
        );
        result.poolId = PoolId.unwrap(poolId);

        // 获取 dust
        uint256 dust0 = IERC20(params.token0).balanceOf(address(this));
        uint256 dust1 = IERC20(params.token1).balanceOf(address(this));

        // 计算 dust 价值占比
        uint256 inputValue = _calculateValueInToken1(inputAmount0, inputAmount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 使用传入的 maxDustBps，如果为 0 则使用默认值 1%
        uint256 effectiveMaxDustBpsV4 = params.maxDustBps > 0 ? params.maxDustBps : MAX_DUST_BPS;
        if (inputValue > 0) {
            require(
                dustValue * 10000 <= inputValue * effectiveMaxDustBpsV4,
                "Dust exceeds limit"
            );
        }

        // 退还 dust
        _refundDust(params.token0, msg.sender);
        _refundDust(params.token1, msg.sender);

        result.dust0 = dust0;
        result.dust1 = dust1;

        emit ZapV4Executed(
            msg.sender,
            result.tokenId,
            result.poolId,
            inputAmount0,
            inputAmount1,
            result.liquidity,
            dust0,
            dust1
        );
    }

    /**
     * @notice V4 Zap 使用任意 calldata swap（支持 OKX 聚合器等）
     * @dev
     *   工作流程:
     *   1. 转入用户代币
     *   2. 执行任意 calldata swap（OKX API 返回的 calldata）
     *   3. 获取 swap 后余额
     *   4. 从 V4 StateView 获取最新价格
     *   5. 如果启用，执行链上二次配平
     *   6. Mint V4 position
     *   7. 检查 dust < maxDustBps，否则 revert
     *   8. 退还 dust
     *
     * @param params V4 Zap 参数（包含 OKX calldata）
     * @return result 执行结果
     */
    function zapV4WithCalls(ZapV4ParamsWithCalls calldata params)
        external
        nonReentrant
        returns (ZapV4Result memory result)
    {
        // 白名单检查
        if (whitelistEnabled) {
            require(whitelist[msg.sender], "Not whitelisted");
        }

        // 验证
        require(params.slippageBps <= maxSlippageBps, "Slippage too high");
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.amount0 > 0 || params.amount1 > 0, "Zero amount");

        // 记录输入金额用于 dust 计算
        uint256 inputAmount0 = params.amount0;
        uint256 inputAmount1 = params.amount1;

        // 转入代币
        if (params.amount0 > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0);
        }
        if (params.amount1 > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1);
        }

        // 执行任意 calldata swap（OKX 等聚合器）
        for (uint256 i = 0; i < params.swapCalls.length; i++) {
            _executeSwapCall(params.swapCalls[i]);
        }

        // 获取 swap 后的余额
        uint256 bal0 = IERC20(params.token0).balanceOf(address(this));
        uint256 bal1 = IERC20(params.token1).balanceOf(address(this));

        // 从 V4 StateView 获取最新价格
        PoolId poolId = PoolIdLibrary.toId(params.v4Config.poolKey);
        (uint160 sqrtPriceX96, , , ) = IStateView(params.v4Config.stateView).getSlot0(poolId);

        // 如果启用二次配平，执行链上配平
        if (params.rebalance.enabled && params.rebalance.router != address(0)) {
            (bal0, bal1) = _executeRebalanceSwapV4(
                params.v4Config.stateView,
                poolId,
                params.rebalance.router,
                params.token0,
                params.token1,
                bal0,
                bal1,
                params.tickLower,
                params.tickUpper,
                params.rebalance.fee,
                params.rebalance.maxRebalanceBps,
                sqrtPriceX96
            );
        }

        // Mint V4 position
        result = _mintV4Position(
            params.v4Config.positionManager,
            params.v4Config.poolKey,
            params.tickLower,
            params.tickUpper,
            bal0,
            bal1,
            params.slippageBps,
            params.recipient,
            sqrtPriceX96
        );
        result.poolId = PoolId.unwrap(poolId);

        // 获取 dust
        uint256 dust0 = IERC20(params.token0).balanceOf(address(this));
        uint256 dust1 = IERC20(params.token1).balanceOf(address(this));

        // 计算 dust 价值占比
        uint256 inputValue = _calculateValueInToken1(inputAmount0, inputAmount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 使用传入的 maxDustBps，如果为 0 则使用默认值 1%
        uint256 effectiveMaxDustBpsV4Calls = params.maxDustBps > 0 ? params.maxDustBps : MAX_DUST_BPS;
        if (inputValue > 0) {
            require(
                dustValue * 10000 <= inputValue * effectiveMaxDustBpsV4Calls,
                "Dust exceeds limit"
            );
        }

        // 退还 dust
        _refundDust(params.token0, msg.sender);
        _refundDust(params.token1, msg.sender);

        result.dust0 = dust0;
        result.dust1 = dust1;

        emit ZapV4Executed(
            msg.sender,
            result.tokenId,
            result.poolId,
            inputAmount0,
            inputAmount1,
            result.liquidity,
            dust0,
            dust1
        );
    }

    /**
     * @notice V4 Zap: 使用目标池自身流动性进行 swap + 添加流动性
     * @dev 当外部聚合器（如 OKX）无法提供路由时使用此函数
     *      通过 V4 PositionManager 的 SWAP_EXACT_IN_SINGLE action 直接在池内 swap
     *      然后通过 MINT_POSITION 添加流动性
     *
     * @param params V4 Zap 参数（包含池内 swap 参数）
     * @return result 执行结果
     */
    function zapV4WithPoolSwap(ZapV4ParamsWithPoolSwap calldata params)
        external
        nonReentrant
        returns (ZapV4Result memory result)
    {
        // 白名单检查
        if (whitelistEnabled) {
            require(whitelist[msg.sender], "Not whitelisted");
        }

        // 验证
        require(params.slippageBps <= maxSlippageBps, "Slippage too high");
        require(params.token0 < params.token1, "Tokens not sorted");
        require(params.amount0 > 0 || params.amount1 > 0, "Zero amount");

        // 记录输入金额用于 dust 计算
        uint256 inputAmount0 = params.amount0;
        uint256 inputAmount1 = params.amount1;

        // 转入代币
        if (params.amount0 > 0) {
            IERC20(params.token0).safeTransferFrom(msg.sender, address(this), params.amount0);
        }
        if (params.amount1 > 0) {
            IERC20(params.token1).safeTransferFrom(msg.sender, address(this), params.amount1);
        }

        // 获取当前余额
        uint256 bal0 = params.amount0;
        uint256 bal1 = params.amount1;

        // 如果需要 swap，通过 V4 PositionManager 执行池内 swap
        if (params.amountToSwap > 0) {
            (bal0, bal1) = _executeV4PoolSwap(
                params.v4Config.positionManager,
                params.v4Config.poolKey,
                params.zeroForOne,
                params.amountToSwap,
                params.sqrtPriceLimitX96,
                bal0,
                bal1
            );
        }

        // 从 V4 StateView 获取最新价格
        PoolId poolId = PoolIdLibrary.toId(params.v4Config.poolKey);
        (uint160 sqrtPriceX96, , , ) = IStateView(params.v4Config.stateView).getSlot0(poolId);

        // Mint V4 position
        result = _mintV4Position(
            params.v4Config.positionManager,
            params.v4Config.poolKey,
            params.tickLower,
            params.tickUpper,
            bal0,
            bal1,
            params.slippageBps,
            params.recipient,
            sqrtPriceX96
        );
        result.poolId = PoolId.unwrap(poolId);

        // 获取 dust
        uint256 dust0 = IERC20(params.token0).balanceOf(address(this));
        uint256 dust1 = IERC20(params.token1).balanceOf(address(this));

        // 计算 dust 价值占比
        uint256 inputValue = _calculateValueInToken1(inputAmount0, inputAmount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 使用传入的 maxDustBps，如果为 0 则使用默认值 1%
        uint256 effectiveMaxDustBps = params.maxDustBps > 0 ? params.maxDustBps : MAX_DUST_BPS;
        if (inputValue > 0) {
            require(
                dustValue * 10000 <= inputValue * effectiveMaxDustBps,
                "Dust exceeds limit"
            );
        }

        // 退还 dust
        _refundDust(params.token0, msg.sender);
        _refundDust(params.token1, msg.sender);

        result.dust0 = dust0;
        result.dust1 = dust1;

        emit ZapV4Executed(
            msg.sender,
            result.tokenId,
            result.poolId,
            inputAmount0,
            inputAmount1,
            result.liquidity,
            dust0,
            dust1
        );
    }

    /**
     * @notice 执行 V4 池内 swap
     * @dev 通过 V4 PositionManager 的 SWAP_EXACT_IN_SINGLE action 执行
     *      注意：V4 swap 通过 modifyLiquidities 的 action 模式执行
     */
    function _executeV4PoolSwap(
        address positionManager,
        PoolKey memory poolKey,
        bool zeroForOne,
        uint256 amountIn,
        uint160 sqrtPriceLimitX96,
        uint256 currentBal0,
        uint256 currentBal1
    ) internal returns (uint256 newBal0, uint256 newBal1) {
        address token0 = Currency.unwrap(poolKey.currency0);
        address token1 = Currency.unwrap(poolKey.currency1);
        address tokenIn = zeroForOne ? token0 : token1;

        // Setup Permit2 allowances for V4 PositionManager
        IERC20(tokenIn).forceApprove(PERMIT2, amountIn);
        IPermit2(PERMIT2).approve(tokenIn, positionManager, uint160(amountIn), uint48(block.timestamp + 3600));

        // Build swap action data
        // Actions: SWAP_EXACT_IN_SINGLE (0x06) + SETTLE (0x0b) + TAKE (0x0e)
        bytes memory actions = new bytes(3);
        actions[0] = bytes1(Actions.SWAP_EXACT_IN_SINGLE);
        actions[1] = bytes1(Actions.SETTLE);
        actions[2] = bytes1(Actions.TAKE);

        // SWAP_EXACT_IN_SINGLE params:
        // ExactInputSingleParams { PoolKey poolKey, bool zeroForOne, uint128 amountIn, uint128 amountOutMinimum, bytes hookData }
        bytes memory swapParams = abi.encode(
            poolKey,
            zeroForOne,
            uint128(amountIn),
            uint128(0), // amountOutMinimum - protected by sqrtPriceLimitX96
            sqrtPriceLimitX96,
            bytes("")  // hookData
        );

        // SETTLE params: (Currency currency, uint256 amount, bool payerIsUser)
        // We settle the input token that was swapped
        bytes memory settleParams = abi.encode(
            zeroForOne ? poolKey.currency0 : poolKey.currency1,
            amountIn,
            false  // payerIsUser = false, contract pays
        );

        // TAKE params: (Currency currency, address recipient, uint256 amount)
        // We take the output token from the swap
        bytes memory takeParams = abi.encode(
            zeroForOne ? poolKey.currency1 : poolKey.currency0,
            address(this),
            uint256(0)  // 0 = take all available
        );

        // Combine actions and params
        bytes[] memory params = new bytes[](3);
        params[0] = swapParams;
        params[1] = settleParams;
        params[2] = takeParams;

        bytes memory unlockData = abi.encode(actions, params);

        // Execute swap via modifyLiquidities
        IPositionManager(positionManager).modifyLiquidities(unlockData, block.timestamp + 300);

        // Reset approval
        IERC20(tokenIn).forceApprove(PERMIT2, 0);

        // Get new balances after swap
        newBal0 = IERC20(token0).balanceOf(address(this));
        newBal1 = IERC20(token1).balanceOf(address(this));

        // Emit swap event
        address tokenOut = zeroForOne ? token1 : token0;
        uint256 amountOut = zeroForOne ? (newBal1 - (currentBal1 - (zeroForOne ? 0 : amountIn))) : (newBal0 - (currentBal0 - (zeroForOne ? amountIn : 0)));
        emit SwapExecuted(positionManager, tokenIn, tokenOut, amountIn, amountOut);
    }

    /**
     * @notice V4 二次配平 swap
     * @dev 使用 V4 StateView 获取价格，但通过 V3 router 执行 swap
     */
    function _executeRebalanceSwapV4(
        address stateView,
        PoolId poolId,
        address router,
        address token0,
        address token1,
        uint256 amount0,
        uint256 amount1,
        int24 tickLower,
        int24 tickUpper,
        uint24 swapFee,
        uint256 maxRebalanceBps,
        uint160 sqrtPriceX96
    ) internal returns (uint256 newAmount0, uint256 newAmount1) {
        // 计算理想比例
        uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
        uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);

        // 检查价格是否在范围内
        if (sqrtPriceX96 <= sqrtRatioAX96 || sqrtPriceX96 >= sqrtRatioBX96) {
            return (amount0, amount1);
        }

        // 计算添加流动性能使用多少
        uint128 liquidity = LiquidityAmounts.getLiquidityForAmounts(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            amount0,
            amount1
        );

        (uint256 amount0Needed, uint256 amount1Needed) = LiquidityAmounts.getAmountsForLiquidity(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            liquidity
        );

        // 计算预计 dust
        uint256 dust0 = amount0 > amount0Needed ? amount0 - amount0Needed : 0;
        uint256 dust1 = amount1 > amount1Needed ? amount1 - amount1Needed : 0;

        // 计算 dust 价值
        uint256 totalValue = _calculateValueInToken1(amount0, amount1, sqrtPriceX96);
        uint256 dustValue = _calculateValueInToken1(dust0, dust1, sqrtPriceX96);

        // 如果 dust 太小，不需要配平
        if (totalValue == 0 || dustValue * 1000 < totalValue) {
            return (amount0, amount1);
        }

        // 确定配平方向和数量
        uint256 dust0ValueInToken1 = FullMath.mulDiv(
            FullMath.mulDiv(dust0, uint256(sqrtPriceX96), Q96),
            uint256(sqrtPriceX96),
            Q96
        );

        bool zeroForOne;
        uint256 rebalanceAmount;

        if (dust0ValueInToken1 > dust1) {
            zeroForOne = true;
            rebalanceAmount = dust0 / 2;
        } else {
            zeroForOne = false;
            rebalanceAmount = dust1 / 2;
        }

        // 限制配平数量不超过最大比例
        uint256 maxRebalance = zeroForOne
            ? (amount0 * maxRebalanceBps) / 10000
            : (amount1 * maxRebalanceBps) / 10000;

        if (rebalanceAmount > maxRebalance) {
            rebalanceAmount = maxRebalance;
        }

        if (rebalanceAmount == 0) {
            return (amount0, amount1);
        }

        // 执行配平 swap（通过 V3 router）
        // 移除白名单限制，调用者自行负责 router 安全性

        address tokenIn = zeroForOne ? token0 : token1;
        address tokenOut = zeroForOne ? token1 : token0;

        IERC20(tokenIn).forceApprove(router, rebalanceAmount);

        uint256 amountOut;
        if (isRouter02[router]) {
            ISwapRouter02.ExactInputSingleParams memory swapParams = ISwapRouter02.ExactInputSingleParams({
                tokenIn: tokenIn,
                tokenOut: tokenOut,
                fee: swapFee,
                recipient: address(this),
                amountIn: rebalanceAmount,
                amountOutMinimum: 0,
                sqrtPriceLimitX96: 0
            });
            amountOut = ISwapRouter02(router).exactInputSingle(swapParams);
        } else {
            ISwapRouter.ExactInputSingleParams memory swapParams = ISwapRouter.ExactInputSingleParams({
                tokenIn: tokenIn,
                tokenOut: tokenOut,
                fee: swapFee,
                recipient: address(this),
                deadline: block.timestamp,
                amountIn: rebalanceAmount,
                amountOutMinimum: 0,
                sqrtPriceLimitX96: 0
            });
            amountOut = ISwapRouter(router).exactInputSingle(swapParams);
        }

        IERC20(tokenIn).forceApprove(router, 0);

        // 计算新余额
        if (zeroForOne) {
            newAmount0 = amount0 - rebalanceAmount;
            newAmount1 = amount1 + amountOut;
        } else {
            newAmount0 = amount0 + amountOut;
            newAmount1 = amount1 - rebalanceAmount;
        }

        emit SwapExecuted(router, tokenIn, tokenOut, rebalanceAmount, amountOut);
    }

    /**
     * @notice Mint V4 position using modifyLiquidities
     * @dev
     *   V4 使用 action 模式：
     *   - MINT_POSITION: 创建新仓位
     *   - SETTLE_PAIR: 结算代币债务（从合约转入 PoolManager）
     *
     *   V4 PositionManager 使用 Permit2 进行授权，所以需要：
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
    ) internal returns (ZapV4Result memory result) {
        // Get token addresses
        address token0 = Currency.unwrap(poolKey.currency0);
        address token1 = Currency.unwrap(poolKey.currency1);

        // Setup Permit2 allowances for V4 PositionManager
        // Step 1: Approve tokens to Permit2
        if (amount0 > 0) {
            IERC20(token0).forceApprove(PERMIT2, amount0);
            // Step 2: Permit2 approve to PositionManager (expires in 1 hour)
            IPermit2(PERMIT2).approve(token0, positionManager, uint160(amount0), uint48(block.timestamp + 3600));
        }
        if (amount1 > 0) {
            IERC20(token1).forceApprove(PERMIT2, amount1);
            IPermit2(PERMIT2).approve(token1, positionManager, uint160(amount1), uint48(block.timestamp + 3600));
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

        // Build and execute modifyLiquidities call
        {
            bytes memory unlockData = _encodeV4MintData(
                poolKey, tickLower, tickUpper, liquidity, amount0, amount1, recipient
            );
            IPositionManager(positionManager).modifyLiquidities(unlockData, block.timestamp + 300);
        }

        // Get tokenId and reset approvals
        uint256 tokenId = IPositionManager(positionManager).nextTokenId() - 1;

        // Reset Permit2 approvals
        IERC20(token0).forceApprove(PERMIT2, 0);
        IERC20(token1).forceApprove(PERMIT2, 0);

        result = ZapV4Result({
            tokenId: tokenId,
            liquidity: liquidity,
            amount0Used: amount0,
            amount1Used: amount1,
            dust0: 0,
            dust1: 0,
            poolId: bytes32(0)
        });
    }

    /**
     * @notice Encode V4 mint data for modifyLiquidities
     */
    function _encodeV4MintData(
        PoolKey memory poolKey,
        int24 tickLower,
        int24 tickUpper,
        uint128 liquidity,
        uint256 amount0,
        uint256 amount1,
        address recipient
    ) internal pure returns (bytes memory) {
        // Actions: MINT_POSITION (0x02) + SETTLE_PAIR (0x0d)
        bytes memory actions = new bytes(2);
        actions[0] = bytes1(Actions.MINT_POSITION);
        actions[1] = bytes1(Actions.SETTLE_PAIR);

        // MINT_POSITION params
        bytes memory mintParams = abi.encode(
            poolKey, tickLower, tickUpper, uint256(liquidity),
            uint128(amount0), uint128(amount1), recipient, bytes("")
        );

        // SETTLE_PAIR params
        bytes memory settlePairParams = abi.encode(poolKey.currency0, poolKey.currency1);

        // Combine
        bytes[] memory params = new bytes[](2);
        params[0] = mintParams;
        params[1] = settlePairParams;

        return abi.encode(actions, params);
    }

    /**
     * @notice 计算 V4 池的最优 swap 数量
     * @dev 供 Bot 链下使用
     */
    function calculateOptimalSwapV4(
        address stateView,
        PoolKey calldata poolKey,
        int24 tickLower,
        int24 tickUpper,
        uint256 amount0In,
        uint256 amount1In
    ) external view returns (bool zeroForOne, uint256 amountToSwap) {
        PoolId poolId = PoolIdLibrary.toId(poolKey);
        (uint160 sqrtPriceX96, , , ) = IStateView(stateView).getSlot0(poolId);

        uint160 sqrtRatioAX96 = TickMath.getSqrtRatioAtTick(tickLower);
        uint160 sqrtRatioBX96 = TickMath.getSqrtRatioAtTick(tickUpper);

        if (sqrtPriceX96 <= sqrtRatioAX96 || sqrtPriceX96 >= sqrtRatioBX96) {
            return (false, 0);
        }

        uint128 virtualLiquidity = 1e18;
        (uint256 amount0Ideal, uint256 amount1Ideal) = LiquidityAmounts.getAmountsForLiquidity(
            sqrtPriceX96,
            sqrtRatioAX96,
            sqrtRatioBX96,
            virtualLiquidity
        );

        if (amount0Ideal == 0 || amount1Ideal == 0) {
            return (false, 0);
        }

        uint256 value0 = FullMath.mulDiv(amount0In, amount1Ideal, 1);
        uint256 value1 = FullMath.mulDiv(amount1In, amount0Ideal, 1);

        // Use pool fee from poolKey
        uint256 feeMultiplier = 1e6 - uint256(poolKey.fee);

        if (value0 > value1) {
            zeroForOne = true;
            uint256 numerator = value0 - value1;

            uint256 amount0IdealInToken1 = FullMath.mulDiv(
                FullMath.mulDiv(amount0Ideal, uint256(sqrtPriceX96), Q96),
                uint256(sqrtPriceX96),
                Q96
            );

            uint256 adjustedAmount0InToken1 = FullMath.mulDiv(amount0IdealInToken1, feeMultiplier, 1e6);
            uint256 denominator = amount1Ideal + adjustedAmount0InToken1;

            if (denominator == 0) return (false, 0);

            amountToSwap = numerator / denominator;
            if (amountToSwap > amount0In) amountToSwap = amount0In;

        } else if (value1 > value0) {
            zeroForOne = false;
            uint256 numerator = value1 - value0;

            uint256 amount1IdealInToken0 = FullMath.mulDiv(
                FullMath.mulDiv(amount1Ideal, Q96, uint256(sqrtPriceX96)),
                Q96,
                uint256(sqrtPriceX96)
            );

            uint256 adjustedAmount1InToken0 = FullMath.mulDiv(amount1IdealInToken0, feeMultiplier, 1e6);
            uint256 denominator = amount0Ideal + adjustedAmount1InToken0;

            if (denominator == 0) return (false, 0);

            amountToSwap = numerator / denominator;
            if (amountToSwap > amount1In) amountToSwap = amount1In;
        }
    }

    receive() external payable {}
}
