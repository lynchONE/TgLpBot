# Zap Contracts

This folder contains the on-chain zap helper contract used by TgLpBot.

## Contract

- `contracts/ZapSimple.sol`: V3/V4 zap (OKX swap + mint; V3/V4 withdraw helpers).

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

The deploy script prints suggested bot `.env` keys (`ZAP_V3_ADDRESS`, `ZAP_V4_ADDRESS`). You can set both to the same `ZapSimple` address.

### Trusted address config (recommended)

`ZapSimple` restricts external calls (OKX router / TokenApprove / PositionManagers). The deploy script will auto-call `setTrustedAddresses` if you provide:

```env
OKX_SWAP_ROUTER=0x...
OKX_TOKEN_APPROVE_ADDRESS=0x40aA958dd87FC8305b97f2BA922CDdCa374bcD7f
V3_POSITION_MANAGER_ADDRESS=0x...
UNISWAP_V4_POSITION_MANAGER_ADDRESS=0x...
```

## Notes

- TgLpBot builds OKX `/swap` calldata with `userWalletAddress=<ZapSimple>` and passes it to `ZapSimple` (`SwapParams.callData`), so swap + mint happen atomically and dust is refunded by the contract.
- Exiting positions is done via the V3 NFT Position Manager and the V4 PositionManager (the zap contract does not provide an “exit to USDT” helper).
