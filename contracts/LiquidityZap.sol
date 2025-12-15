// SPDX-License-Identifier: MIT
pragma solidity ^0.8.0;

interface IERC20 {
    function totalSupply() external view returns (uint256);
    function balanceOf(address account) external view returns (uint256);
    function transfer(address recipient, uint256 amount) external returns (bool);
    function allowance(address owner, address spender) external view returns (uint256);
    function approve(address spender, uint256 amount) external returns (bool);
    function transferFrom(address sender, address recipient, uint256 amount) external returns (bool);
}

interface IPancakeRouter02 {
    function factory() external pure returns (address);
    function WETH() external pure returns (address);
    
    function addLiquidity(
        address tokenA,
        address tokenB,
        uint amountADesired,
        uint amountBDesired,
        uint amountAMin,
        uint amountBMin,
        address to,
        uint deadline
    ) external returns (uint amountA, uint amountB, uint liquidity);
    
    function addLiquidityETH(
        address token,
        uint amountTokenDesired,
        uint amountTokenMin,
        uint amountETHMin,
        address to,
        uint deadline
    ) external payable returns (uint amountToken, uint amountETH, uint liquidity);
    
    function removeLiquidity(
        address tokenA,
        address tokenB,
        uint liquidity,
        uint amountAMin,
        uint amountBMin,
        address to,
        uint deadline
    ) external returns (uint amountA, uint amountB);
    
    function removeLiquidityETH(
        address token,
        uint liquidity,
        uint amountTokenMin,
        uint amountETHMin,
        address to,
        uint deadline
    ) external returns (uint amountToken, uint amountETH);
    
    function swapExactTokensForTokens(
        uint amountIn,
        uint amountOutMin,
        address[] calldata path,
        address to,
        uint deadline
    ) external returns (uint[] memory amounts);
    
    function swapExactETHForTokens(
        uint amountOutMin,
        address[] calldata path,
        address to,
        uint deadline
    ) external payable returns (uint[] memory amounts);
    
    function swapExactTokensForETH(
        uint amountIn,
        uint amountOutMin,
        address[] calldata path,
        address to,
        uint deadline
    ) external returns (uint[] memory amounts);
    
    function getAmountsOut(uint amountIn, address[] calldata path) external view returns (uint[] memory amounts);
}

interface IPancakePair {
    function token0() external view returns (address);
    function token1() external view returns (address);
    function getReserves() external view returns (uint112 reserve0, uint112 reserve1, uint32 blockTimestampLast);
}

/**
 * @title LiquidityZap
 * @dev Contract for adding/removing liquidity with a single token
 */
contract LiquidityZap {
    address public owner;
    IPancakeRouter02 public router;
    
    event ZapIn(address indexed user, address indexed pair, address tokenIn, uint256 amountIn, uint256 liquidity);
    event ZapOut(address indexed user, address indexed pair, address tokenOut, uint256 liquidity, uint256 amountOut);
    
    modifier onlyOwner() {
        require(msg.sender == owner, "Not owner");
        _;
    }
    
    constructor(address _router) {
        owner = msg.sender;
        router = IPancakeRouter02(_router);
    }
    
    /**
     * @dev Zap in with a single token to add liquidity
     * @param tokenIn The input token address
     * @param amountIn The amount of input token
     * @param pair The LP pair address
     * @param minLiquidity Minimum LP tokens to receive
     * @param deadline Transaction deadline
     */
    function zapIn(
        address tokenIn,
        uint256 amountIn,
        address pair,
        uint256 minLiquidity,
        uint256 deadline
    ) external returns (uint256 liquidity) {
        require(amountIn > 0, "Amount must be greater than 0");
        require(deadline >= block.timestamp, "Deadline expired");
        
        // Transfer tokens from user
        IERC20(tokenIn).transferFrom(msg.sender, address(this), amountIn);
        
        // Get pair tokens
        IPancakePair lpPair = IPancakePair(pair);
        address token0 = lpPair.token0();
        address token1 = lpPair.token1();
        
        require(tokenIn == token0 || tokenIn == token1, "Invalid input token");
        
        // Calculate swap amount (approximately half)
        uint256 swapAmount = amountIn / 2;
        uint256 remainingAmount = amountIn - swapAmount;
        
        address tokenOut = tokenIn == token0 ? token1 : token0;
        
        // Approve router
        IERC20(tokenIn).approve(address(router), amountIn);
        
        // Swap half of input token for the other token
        address[] memory path = new address[](2);
        path[0] = tokenIn;
        path[1] = tokenOut;
        
        uint[] memory amounts = router.swapExactTokensForTokens(
            swapAmount,
            0, // Accept any amount
            path,
            address(this),
            deadline
        );
        
        uint256 otherTokenAmount = amounts[1];
        
        // Approve tokens for adding liquidity
        IERC20(tokenIn).approve(address(router), remainingAmount);
        IERC20(tokenOut).approve(address(router), otherTokenAmount);
        
        // Add liquidity
        (uint amountA, uint amountB, uint liquidityReceived) = router.addLiquidity(
            tokenIn,
            tokenOut,
            remainingAmount,
            otherTokenAmount,
            0, // Accept any amount
            0, // Accept any amount
            msg.sender,
            deadline
        );
        
        require(liquidityReceived >= minLiquidity, "Insufficient liquidity received");
        
        // Return any leftover tokens
        uint256 leftoverTokenIn = IERC20(tokenIn).balanceOf(address(this));
        uint256 leftoverTokenOut = IERC20(tokenOut).balanceOf(address(this));
        
        if (leftoverTokenIn > 0) {
            IERC20(tokenIn).transfer(msg.sender, leftoverTokenIn);
        }
        if (leftoverTokenOut > 0) {
            IERC20(tokenOut).transfer(msg.sender, leftoverTokenOut);
        }
        
        emit ZapIn(msg.sender, pair, tokenIn, amountIn, liquidityReceived);
        
        return liquidityReceived;
    }
    
    /**
     * @dev Zap out to receive a single token from LP
     * @param pair The LP pair address
     * @param liquidity The amount of LP tokens to remove
     * @param tokenOut The desired output token
     * @param minAmountOut Minimum amount of output token to receive
     * @param deadline Transaction deadline
     */
    function zapOut(
        address pair,
        uint256 liquidity,
        address tokenOut,
        uint256 minAmountOut,
        uint256 deadline
    ) external returns (uint256 amountOut) {
        require(liquidity > 0, "Liquidity must be greater than 0");
        require(deadline >= block.timestamp, "Deadline expired");
        
        // Transfer LP tokens from user
        IERC20(pair).transferFrom(msg.sender, address(this), liquidity);
        
        // Get pair tokens
        IPancakePair lpPair = IPancakePair(pair);
        address token0 = lpPair.token0();
        address token1 = lpPair.token1();
        
        require(tokenOut == token0 || tokenOut == token1, "Invalid output token");
        
        address otherToken = tokenOut == token0 ? token1 : token0;
        
        // Approve LP token
        IERC20(pair).approve(address(router), liquidity);
        
        // Remove liquidity
        (uint amount0, uint amount1) = router.removeLiquidity(
            token0,
            token1,
            liquidity,
            0, // Accept any amount
            0, // Accept any amount
            address(this),
            deadline
        );
        
        uint256 tokenOutAmount = tokenOut == token0 ? amount0 : amount1;
        uint256 otherTokenAmount = tokenOut == token0 ? amount1 : amount0;
        
        // Swap other token for desired output token
        if (otherTokenAmount > 0) {
            IERC20(otherToken).approve(address(router), otherTokenAmount);
            
            address[] memory path = new address[](2);
            path[0] = otherToken;
            path[1] = tokenOut;
            
            uint[] memory amounts = router.swapExactTokensForTokens(
                otherTokenAmount,
                0, // Accept any amount
                path,
                address(this),
                deadline
            );
            
            tokenOutAmount += amounts[1];
        }
        
        require(tokenOutAmount >= minAmountOut, "Insufficient output amount");
        
        // Transfer output token to user
        IERC20(tokenOut).transfer(msg.sender, tokenOutAmount);
        
        emit ZapOut(msg.sender, pair, tokenOut, liquidity, tokenOutAmount);
        
        return tokenOutAmount;
    }
    
    /**
     * @dev Rescue tokens sent to this contract by mistake
     */
    function rescueTokens(address token, uint256 amount) external onlyOwner {
        IERC20(token).transfer(owner, amount);
    }
    
    /**
     * @dev Rescue BNB sent to this contract
     */
    function rescueBNB() external onlyOwner {
        payable(owner).transfer(address(this).balance);
    }
    
    receive() external payable {}
}

