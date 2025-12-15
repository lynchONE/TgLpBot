# ZapV3V4Improved Contract

This folder contains the on-chain zap helper contract used by TgLpBot.

## Contract

- `contracts/ZapV3V4Improved.sol`: unified V3/V4 zap (mint/increase + V4 rebalance entry).

## Build

```bash
cd contracts
npm install
npm run compile
```

## Deployment

Create `contracts/.env`:

```env
DEPLOYER_PRIVATE_KEY=...
BSCSCAN_API_KEY=...

# Required: WETH address (use WBNB on BSC)
WETH_ADDRESS=0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c

# Optional
VERIFY=1
```

Deploy:

```bash
npm run deploy:mainnet
# or
npm run deploy:testnet
```

The deploy script prints suggested bot `.env` keys (`ZAP_V3_ADDRESS`, `ZAP_V4_ADDRESS`). You can set both to the same `ZapV3V4Improved` address.

## Notes

- TgLpBot builds OKX `/swap` calldata with `userWalletAddress=<ZapV3V4Improved>` and passes it as `swapCalls` to `ZapV3V4Improved`, so swap + mint happen atomically and dust is refunded by the contract.
- Exiting positions is done via the V3 NFT Position Manager and the V4 PositionManager (the zap contract does not provide an “exit to USDT” helper).
