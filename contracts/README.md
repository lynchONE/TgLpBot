# LiquidityZap Smart Contract

This directory contains the LiquidityZap smart contract for single-token liquidity operations on PancakeSwap.

## Features

- **Zap In**: Add liquidity to any PancakeSwap V2 pool using a single token
- **Zap Out**: Remove liquidity and receive a single token
- **Slippage Protection**: Minimum output amount checks
- **Deadline Protection**: Transaction deadline enforcement
- **Rescue Functions**: Owner can rescue stuck tokens

## Contract Details

- **Solidity Version**: 0.8.19
- **License**: MIT
- **Router**: PancakeSwap Router V2

## Setup

### 1. Install Dependencies

```bash
cd contracts
npm install
```

### 2. Configure Environment

Create a `.env` file in the `contracts` directory:

```env
DEPLOYER_PRIVATE_KEY=your_private_key_here
BSCSCAN_API_KEY=your_bscscan_api_key_here
```

⚠️ **Never commit your private key!**

## Deployment

### Deploy to BSC Testnet

```bash
npm run deploy:testnet
```

### Deploy to BSC Mainnet

```bash
npm run deploy:mainnet
```

The deployment script will:
1. Deploy the LiquidityZap contract
2. Wait for confirmations
3. Verify the contract on BSCScan
4. Display the contract address

### Manual Verification

If automatic verification fails:

```bash
npx hardhat verify --network bscMainnet <CONTRACT_ADDRESS> "0x10ED43C718714eb63d5aA57B78B54704E256024E"
```

## Testing

Run tests (if available):

```bash
npm test
```

## Usage

### Zap In (Add Liquidity)

```solidity
function zapIn(
    address tokenIn,      // Input token address
    uint256 amountIn,     // Amount of input token
    address pair,         // LP pair address
    uint256 minLiquidity, // Minimum LP tokens to receive
    uint256 deadline      // Transaction deadline
) external returns (uint256 liquidity)
```

**Example Flow:**
1. User approves USDT to Zap contract
2. User calls `zapIn` with USDT
3. Contract swaps half of USDT for the other token
4. Contract adds liquidity to the pool
5. LP tokens are sent to the user

### Zap Out (Remove Liquidity)

```solidity
function zapOut(
    address pair,          // LP pair address
    uint256 liquidity,     // Amount of LP tokens to remove
    address tokenOut,      // Desired output token
    uint256 minAmountOut,  // Minimum amount of output token
    uint256 deadline       // Transaction deadline
) external returns (uint256 amountOut)
```

**Example Flow:**
1. User approves LP tokens to Zap contract
2. User calls `zapOut` specifying USDT as output
3. Contract removes liquidity
4. Contract swaps the other token for USDT
5. All USDT is sent to the user

## Security Considerations

### Auditing

⚠️ **This contract has not been audited.** Use at your own risk.

For production use with significant funds:
- Get a professional audit
- Test thoroughly on testnet
- Start with small amounts

### Known Limitations

1. **Slippage**: Large trades may experience significant slippage
2. **Gas Costs**: Multiple swaps increase gas costs
3. **Price Impact**: Large amounts can impact pool prices
4. **MEV**: Transactions may be front-run

### Best Practices

1. Always set appropriate slippage tolerance
2. Use reasonable deadlines (10-20 minutes)
3. Test with small amounts first
4. Monitor gas prices
5. Check pool liquidity before large trades

## Contract Addresses

### BSC Mainnet
- LiquidityZap: `TBD` (deploy and update)
- PancakeSwap Router V2: `0x10ED43C718714eb63d5aA57B78B54704E256024E`
- PancakeSwap Factory V2: `0xcA143Ce32Fe78f1f7019d7d551a6402fC5350c73`

### BSC Testnet
- LiquidityZap: `TBD` (deploy and update)
- PancakeSwap Router V2: `0xD99D1c33F9fC3444f8101754aBC46c52416550D1`
- PancakeSwap Factory V2: `0x6725F303b657a9451d8BA641348b6761A6CC7a17`

## Gas Estimates

Approximate gas costs (may vary):
- Zap In: ~300,000 - 500,000 gas
- Zap Out: ~300,000 - 500,000 gas

## Troubleshooting

### Deployment Issues

**Error: Insufficient funds**
- Ensure deployer wallet has enough BNB for gas

**Error: Nonce too high**
- Reset your account in MetaMask or wait for pending transactions

**Error: Contract verification failed**
- Manually verify using the command above
- Check that constructor arguments match

### Transaction Issues

**Error: Insufficient liquidity received**
- Increase slippage tolerance
- Check pool liquidity
- Reduce trade amount

**Error: Deadline expired**
- Increase deadline
- Submit transaction faster

**Error: Transfer amount exceeds allowance**
- Approve tokens to Zap contract first

## Development

### Compile Contracts

```bash
npm run compile
```

### Run Tests

```bash
npm test
```

### Clean Build Artifacts

```bash
npx hardhat clean
```

## License

MIT License - see LICENSE file for details

## Support

For issues and questions:
- Open an issue on GitHub
- Contact: @yoursupport on Telegram

