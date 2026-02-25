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
BASESCAN_API_KEY=...
BASE_RPC_URL=...
BASE_SEPOLIA_RPC_URL=...

# Optional
VERIFY=1

# Optional: force deploy gasPrice
# GAS_PRICE_GWEI=1
```

Deploy:

```bash
npm run deploy:mainnet
# or
npm run deploy:testnet
```

ZapSimple deploy:

```bash
npm run deploy-zap:mainnet
# or
npm run deploy-zap:testnet
# or
npm run deploy-zap:base
# or
npm run deploy-zap:baseSepolia
```

The deploy script prints suggested bot `.env` keys (`ZAP_V3_ADDRESS`, `ZAP_V4_ADDRESS`). You can set both to the same `ZapSimple` address.

### Trusted address config (recommended)

`ZapSimple` restricts external calls (OKX router / TokenApprove / PositionManagers).  
The deploy script auto-calls `setTrustedAddresses` from env values.

You can use global keys (legacy path, mainly for BSC):

```env
OKX_SWAP_ROUTER=0x...
OKX_TOKEN_APPROVE_ADDRESS=0x40aA958dd87FC8305b97f2BA922CDdCa374bcD7f
V3_POSITION_MANAGER_ADDRESS=0x...
UNISWAP_V4_POSITION_MANAGER_ADDRESS=0x...
```

Or network-scoped keys (recommended when one `.env` serves multiple chains, and required for Base isolation):

```env
# For --network base:
BASE_OKX_SWAP_ROUTER=0x...
BASE_OKX_TOKEN_APPROVE_ADDRESS=0x...
BASE_V3_POSITION_MANAGER_ADDRESS=0x...
BASE_UNISWAP_V3_NPM_ADDRESS=0x...
BASE_AERODROME_V3_NPM_ADDRESS=0x...
BASE_UNISWAP_V4_POSITION_MANAGER_ADDRESS=0x...

# For --network baseSepolia:
BASE_SEPOLIA_OKX_SWAP_ROUTER=0x...
BASE_SEPOLIA_OKX_TOKEN_APPROVE_ADDRESS=0x...
BASE_SEPOLIA_V3_POSITION_MANAGER_ADDRESS=0x...
BASE_SEPOLIA_UNISWAP_V3_NPM_ADDRESS=0x...
BASE_SEPOLIA_AERODROME_V3_NPM_ADDRESS=0x...
BASE_SEPOLIA_UNISWAP_V4_POSITION_MANAGER_ADDRESS=0x...
```

Network behavior:
- `bsc` / `bscTestnet`: V3 managers = Pancake + Uniswap.
- `base` / `baseSepolia`: V3 managers = Uniswap + Aerodrome (no Pancake).
- Base networks use chain-scoped env keys (`BASE_*` / `BASE_SEPOLIA_*`) to avoid coupling with BSC config.
- BSC networks can use either chain-scoped (`BSC_*` / `BSC_TESTNET_*`) or legacy global keys.

## Notes

- TgLpBot builds OKX `/swap` calldata with `userWalletAddress=<ZapSimple>` and passes it to `ZapSimple` (`SwapParams.callData`), so swap + mint happen atomically and dust is refunded by the contract.
- Exiting positions is done via the V3 NFT Position Manager and the V4 PositionManager (the zap contract does not provide an "exit to USDT" helper).
- If you hit `execution reverted: 0x3f68539a` on `ZapInV4`, that selector is `Permit2AllowanceIsFixedAtInfinity()`. This often shows up during `eth_estimateGas` (so no tx appears on BscScan). Deploy the latest `ZapSimple` and update the bot `ZAP_V4_ADDRESS` to the new contract address.
- If deployment fails with `insufficient funds for gas * price + value`, top up the deployer BNB. You can also set `GAS_PRICE_GWEI` to override the gas price used by `scripts/deploy_zap_simple.js`.
