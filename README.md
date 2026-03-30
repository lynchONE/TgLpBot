# TgLpBot - Telegram Liquidity Pool Bot

A Telegram bot for managing concentrated-liquidity positions on Binance Smart Chain (BSC) (V3 pool address / V4 PoolId). Deposit and withdraw using USDT with swaps routed via the OKX DEX aggregator.

## Features

- 💼 **Wallet Management**: Create or import wallets with encrypted private key storage
- 📈 **Create Positions**: Open V3/V4 positions from USDT (tick-range based)
- 📉 **Exit Positions**: Close positions back to USDT
- 🔄 **Optimal Routing**: Uses OKX DEX aggregator for swaps
- ⚡ **ZapV3V4Improved**: On-chain mint/rebalance entry for V3/V4
- 🧠 **Strategy Tasks**: Monitor range and auto-reopen (optional)
- 📊 **Transaction Tracking**: Monitor all your transactions
- 🔒 **Secure**: AES-256 encryption for private keys

## Architecture

```
TgLpBot/
├── blockchain/          # Blockchain interaction layer
│   ├── client.go       # BSC client initialization
│   ├── erc20.go        # ERC20 token interactions
│   ├── okx.go          # OKX tx types
│   ├── v3_pool.go      # V3 pool reads
│   ├── v3_position_manager.go # V3 position manager calls
│   ├── v4_pool.go      # V4 pool reads
│   ├── v4_position_manager.go # V4 position manager calls
│   └── zap_v3v4_improved.go   # ZapV3V4Improved bindings
├── bot/                # Telegram bot handlers
│   ├── bot.go          # Bot initialization
│   ├── handlers.go     # Command handlers
│   └── input_handlers.go # User input handlers
├── config/             # Configuration management
│   └── config.go
├── contracts/          # Smart contracts
│   └── contracts/ZapV3V4Improved.sol # Unified V3/V4 zap
├── database/           # Database layer
│   ├── mysql.go        # MySQL connection
│   └── redis.go        # Redis connection
├── models/             # Data models
│   ├── user.go
│   ├── wallet.go
│   ├── strategy.go
│   └── transaction.go
├── services/           # Business logic
│   ├── user.go
│   ├── wallet.go
│   ├── liquidity_enter.go
│   ├── liquidity_exit.go
│   ├── okx_dex.go
│   └── okx_swap.go
├── .env.example        # Environment variables template
├── go.mod
└── main.go

```

## Prerequisites

- Go 1.21 or higher
- MySQL 8.0+
- Redis 6.0+
- Telegram Bot Token (from [@BotFather](https://t.me/botfather))
- BSC RPC endpoint
- OKX API credentials (optional, for DEX aggregator)

## Installation

### 1. Clone the repository

```bash
git clone <repository-url>
cd TgLpBot
```

### 2. Install dependencies

```bash
go mod download
```

### 3. Set up MySQL database

```sql
CREATE DATABASE tglpbot CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

### 4. Set up Redis

Make sure Redis is running on your system:

```bash
redis-server
```

### 5. Configure environment variables

Copy `.env.example` to `.env` and fill in your values:

```bash
cp .env.example .env
```

Edit `.env` with your configuration:

```env
# Telegram Bot Token
TELEGRAM_BOT_TOKEN=your_bot_token_here

# Bark (optional; iOS push notifications)
# Configure in Bot -> 全局配置 (per user). No .env variables required.

# Database
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASSWORD=your_password
MYSQL_DATABASE=tglpbot

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# Pools sync
POOLS_SYNC_ENABLED=1
POOLS_SYNC_POOLM_BASE_URL=https://mapi.poolm.xyz
POOLS_SYNC_CHAIN=bsc
POOLS_SYNC_DEXES=pcsv3,univ3,univ4
POOLS_SYNC_INTERVAL_SECONDS=60
POOLS_SYNC_FETCH_DELAY_MILLIS=250
POOLS_RETENTION_HOURS=24

# BSC Network
BSC_RPC_URL=https://bsc-dataseed1.binance.org/
BSC_CHAIN_ID=56

# Liquidity exit RPC sync (optional; prevents missing tokens on swap when public RPC lags)
EXIT_TOKEN_SYNC_TIMEOUT_SECONDS=30
EXIT_TOKEN_SYNC_POLL_MILLIS=500

# Encryption (generate a random 32-byte hex string)
ENCRYPTION_KEY=your_32_byte_hex_key_here

# OKX DEX API (optional)
OKX_API_KEY=your_okx_api_key
OKX_SECRET_KEY=your_okx_secret_key
OKX_PASSPHRASE=your_okx_passphrase
OKX_SWAP_ROUTER=0x...  # OKX DEX Router 地址
OKX_TOKEN_APPROVE_ADDRESS=0x...  # OKX DEX TokenApprove 合约地址（BSC: 0x40aA958dd87FC8305b97f2BA922CDdCa374bcD7f）

# V3 Position Managers (必需 - Required for V3 liquidity operations)
PANCAKE_V3_NPM_ADDRESS=0x46A15B0b27311cedF172AB29E4f4766fbE7F4364  # PancakeSwap V3 NonfungiblePositionManager on BSC
UNISWAP_V3_NPM_ADDRESS=0x7b8A01B39D58278b5DE7e48c8449c9f4F5170613   # Uniswap V3 NonfungiblePositionManager on BSC

# V4 Position Manager (必需 - Required for V4 liquidity operations)
UNISWAP_V4_POSITION_MANAGER_ADDRESS=0x...  # Uniswap V4 PositionManager address (根据实际部署填写)

# Zap Contract (必需 - Required for liquidity operations)
ZAP_V3_ADDRESS=0x...  # ZapSimple.sol 合约地址 (see contracts/README.md for deployment)
ZAP_V4_ADDRESS=0x...  # 可选，V4 使用相同的 ZapSimple 合约
```

### 6. Deploy Zap Contract

Deploy the `contracts/contracts/ZapV3V4Improved.sol` contract to BSC:

1. Compile and deploy (constructor arg: WETH; use WBNB on BSC mainnet `0xbb4CdB9CBd36B01bD1cBaEBF2De08d9173bc095c`)
2. Update `ZAP_V3_ADDRESS` (and `ZAP_V4_ADDRESS`) in `.env` with the deployed address

This repo includes a Hardhat deploy script in `contracts/` (see `contracts/README.md`).

### 7. Generate encryption key

Generate a secure 32-byte encryption key:

```bash
openssl rand -hex 32
```

Add this to your `.env` file as `ENCRYPTION_KEY`.

## Running the Bot

### Development

```bash
go run main.go
```

### Production

Build the binary:

```bash
go build -o tglpbot main.go
```

Run the binary:

```bash
./tglpbot
```

### Using systemd (Linux)

Create a systemd service file `/etc/systemd/system/tglpbot.service`:

```ini
[Unit]
Description=Telegram LP Bot
After=network.target mysql.service redis.service

[Service]
Type=simple
User=your_user
WorkingDirectory=/path/to/TgLpBot
ExecStart=/path/to/TgLpBot/tglpbot
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Enable and start the service:

```bash
sudo systemctl enable tglpbot
sudo systemctl start tglpbot
sudo systemctl status tglpbot
```

## Usage

### Bot Commands

- `/start` - Start the bot and see welcome message
- `/help` - Show help and available commands
- `/wallet` - Manage wallets (create, import, view)
- `/balance` - Check wallet balances
- `/newposition` - Create a new position
- `/positions` - View and manage positions
- `/config` - Global config (slippage/stop-loss/rebalance/extra notifications)
- `/transactions` - View transaction history

### Workflow

1. **Create/Import Wallet**
   ```
   /wallet → Create Wallet or Import Wallet
   ```

2. **Create a Position (V3/V4)**
   ```
   /newposition (or send a pool address / PoolId) → Enter tick range → Enter amount → Confirm
   ```

3. **Manage Positions**
   ```
   /positions
   ```

## Security Considerations

⚠️ **Important Security Notes:**

1. **Private Keys**: All private keys are encrypted using AES-256-GCM before storage
2. **Environment Variables**: Never commit `.env` file to version control
3. **Database Access**: Use strong passwords and restrict database access
4. **API Keys**: Keep your OKX API keys secure
5. **Bot Token**: Never share your Telegram bot token
6. **Encryption Key**: Store the encryption key securely and never lose it

## Smart Contract Security

The Zap contract includes:
- Reentrancy protection
- Owner-only rescue functions
- Slippage protection
- Deadline checks

**Audit Recommendation**: Get the contract audited before using with significant funds.

## Troubleshooting

### Bot not responding
- Check if the bot process is running
- Verify Telegram bot token is correct
- Check network connectivity

### Database connection errors
- Verify MySQL is running
- Check database credentials in `.env`
- Ensure database exists

### Transaction failures
- Check wallet has sufficient BNB for gas
- Verify token approvals
- Check slippage settings
- Ensure Zap contract is deployed

### ❌ 开仓失败：V3 position manager address not configured

**问题描述**：在尝试为 PancakeSwap V3 池子开仓时出现此错误，而 Uniswap V3/V4 正常。

**原因分析**：
系统根据池子的 `Exchange` 字段自动选择对应的 Position Manager 地址：
- PancakeSwap V3 → 使用 `PANCAKE_V3_NPM_ADDRESS`
- Uniswap V3 → 使用 `UNISWAP_V3_NPM_ADDRESS`
- Uniswap V4 → 使用 `UNISWAP_V4_POSITION_MANAGER_ADDRESS`

如果对应的环境变量未配置或为空，开仓时会报此错误。

**解决方案**：
在 `.env` 文件中添加以下配置（BSC 主网地址）：

```env
# PancakeSwap V3 NonfungiblePositionManager
PANCAKE_V3_NPM_ADDRESS=0x46A15B0b27311cedF172AB29E4f4766fbE7F4364

# Uniswap V3 NonfungiblePositionManager (BSC)
UNISWAP_V3_NPM_ADDRESS=0x7b8A01B39D58278b5DE7e48c8449c9f4F5170613

# Uniswap V4 PositionManager (根据实际部署)
UNISWAP_V4_POSITION_MANAGER_ADDRESS=0x你的V4地址
```

配置后重启 Bot 即可。

**验证配置**：
启动 Bot 后，查看日志输出：
```
📝 配置信息:
   - Pancake V3 NPM: 0x46A15B0b27311cedF172AB29E4f4766fbE7F4364
   - Uniswap V3 NPM: 0x7b8A01B39D58278b5DE7e48c8449c9f4F5170613
```

如果显示为空，说明配置未生效。

**进一步诊断**：

如果配置文件中有值但仍报错，可能的原因：

1. **配置文件路径问题**：确保 `.env` 文件在运行目录下（通常是 `backend/` 目录）
2. **配置未重新加载**：修改 `.env` 后需要重启 Bot
3. **Exchange 字段值异常**：数据库中保存的 `Exchange` 字段可能不包含 "pancake" 或 "uniswap"

**调试步骤**：

重启 Bot 后，尝试创建一个 PancakeSwap V3 池子的任务，查看日志输出：

```
[Liquidity] V3 enter: task.Exchange=PancakeSwap V3 (lowercased: pancakeswap v3)
[Liquidity] V3 enter: config.PancakeV3PositionManagerAddress=0x46A15B0b27311cedF172AB29E4f4766fbE7F4364
[Liquidity] V3 enter: 选择 PancakeSwap V3 NPM: 0x46A15B0b27311cedF172AB29E4f4766fbE7F4364
```

如果看到类似以下输出，说明匹配失败：
```
[Liquidity] V3 enter: task.Exchange=V3 Pool (lowercased: v3 pool)
[Liquidity] V3 enter: ⚠️ 无法匹配到合适的 Position Manager (exchange=v3 pool)
```

这说明池子查询时**未能正确识别交易所**。需要检查：
- 池子地址是否正确
- 是否真的是 PancakeSwap V3 或 Uniswap V3 的池子
- 工厂合约地址是否正确配置（见 [`pool.go:184-193`](file:///e:/goProject/TgLpBot/backend/services/pool.go#L184-L193)）

**临时解决方案**：

如果池子查询无法识别交易所，可以手动在数据库中修改任务的 `exchange` 字段为 `"PancakeSwap V3"` 或 `"Uniswap V3"`。

### ❌ PancakeSwap V3 开仓交易 Revert

**问题描述**：PancakeSwap V3 池子识别正常，但开仓交易 revert，错误信息为 `execution reverted` 或 `Untrusted PM`。

**原因分析**：
ZapSimple 合约使用白名单机制限制可信的 Position Manager。如果合约初始化时只添加了 Uniswap V3 的 NPM，而没有将 PancakeSwap V3 的 NPM 加入白名单（`trustedV3PositionManagers`），则会在第 359 行校验时 revert：
```solidity
require(
    params.positionManager == v3PositionManager || trustedV3PositionManagers[params.positionManager],
    "Untrusted PM"
);
```

**解决方案**：

1. **检查合约配置**
   ```bash
   cd contracts
   npx hardhat console --network bsc
   > const zap = await ethers.getContractAt("ZapSimple", "YOUR_ZAP_ADDRESS")
   > await zap.trustedV3PositionManagers("0x46A15B0b27311cedF172AB29E4f4766fbE7F4364")
   # 如果返回 false，说明未加入白名单
   ```

2. **添加 PancakeSwap V3 到白名单**
   ```bash
   cd contracts
   # 确保 .env 中有 PANCAKE_V3_NPM_ADDRESS 和 ZAP_V3_ADDRESS
   npx hardhat run scripts/add_pancake_v3_trusted.js --network bsc
   ```

3. **验证配置**
   开仓后查看日志，应该看到选择了正确的 Position Manager：
   ```
   [Liquidity] V3 enter: 选择 PancakeSwap V3 NPM: 0x46A15B0b27311cedF172AB29E4f4766fbE7F4364
   ```

**一次性配置多个 Position Manager**：

编辑 `contracts/.env`，确保包含：
```env
PANCAKE_V3_NPM_ADDRESS=0x46A15B0b27311cedF172AB29E4f4766fbE7F4364
UNISWAP_V3_NPM_ADDRESS=0x7b8A01B39D58278b5DE7e48c8449c9f4F5170613
```

然后运行白名单脚本将所有需要的 Position Manager 添加到合约中。

### OKX Swap 调用失败 (Swap call failed)
**常见原因**：
1. **未配置 OKX_TOKEN_APPROVE_ADDRESS**：OKX DEX 使用单独的 TokenApprove 合约接收代币 approve，而非直接 approve 给 Router。
   - BSC 主网地址: `0x40aA958dd87FC8305b97f2BA922CDdCa374bcD7f`
   - 请在 `.env` 中添加: `OKX_TOKEN_APPROVE_ADDRESS=0x40aA958dd87FC8305b97f2BA922CDdCa374bcD7f`
2. **OKX calldata 过期**：OKX 返回的 swap 数据有 deadline（通常几分钟），如果交易确认慢，可能过期。
3. **滑点设置太小**：如果价格变动超过设置的滑点，swap 会失败。
4. **Zap 合约代币余额不足**：确保用户已经 approve 足够的代币给 Zap 合约。

### Redis connection errors
- Verify Redis is running
- Check Redis connection settings

## Development

### Project Structure

- `blockchain/` - Blockchain interaction and contract bindings
- `bot/` - Telegram bot logic and handlers
- `config/` - Configuration management
- `database/` - Database connections (MySQL, Redis)
- `models/` - Data models and schemas
- `services/` - Business logic layer
- `contracts/` - Solidity smart contracts

### Adding New Features

1. Add models in `models/` if needed
2. Implement business logic in `services/`
3. Add bot handlers in `bot/`
4. Update database migrations if needed

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

MIT License - see LICENSE file for details

## Disclaimer

This software is provided "as is", without warranty of any kind. Use at your own risk. Always test with small amounts first. The developers are not responsible for any losses incurred while using this bot.

## Support

For issues and questions:
- Open an issue on GitHub
- Contact: @yoursupport on Telegram

## Roadmap

- [ ] Multi-chain support (Ethereum, Polygon, etc.)
- [ ] Advanced trading strategies
- [ ] Price alerts and notifications
- [ ] Portfolio tracking
- [ ] Automated rebalancing
- [ ] Gas optimization
- [ ] Web dashboard

## V4 Update (2025-12-12)

### 1. Enhanced V4 Pool Query
- Refactored to use `PoolManager.poolKeys` for direct on-chain token retrieval.
- This checks if the `PoolManager` contract stores the keys for the given PoolId.
- Includes error handling for empty responses to prevent crashes.

### 2. Automated Strategy Monitoring
- **New Feature**: Added background Strategy Service (`StrategyService`).
- **Workflow**:
  1. User confirms position creation.
  2. System creates a strategy task.
  3. Service monitors pool tick every 30 seconds.
  4. **Stop Loss/Take Profit**: If price goes out of tick range:
     - Automatically executes Zap Out (Liquidity -> USDT).
     - Updates task status to `Waiting`.
  5. **Auto Reopen**:
     - Waits for configured delay (default 5 mins).
     - Automatically executes Zap In (USDT -> Liquidity).
     - Updates task status to `Running`.

### 3. Zap Contract Integration
- V3 entry uses `ZapV3V4Improved.zap(...)`.
- V4 entry uses `ZapV3V4Improved.zapV4WithRebalance(...)`.
- Exits use the V3 NFT Position Manager and the V4 PositionManager directly.

### Usage
1. Send a V3 pool address or V4 PoolId to bot.
2. Enter Tick Range (e.g., `-887220,887220`) or a percentage range (e.g., `5 100`).
3. Enter Amount.
4. Click "Confirm".
5. Use `/positions` to check status.


## 2025-12-14 Zap 合约重构更新

### 1. 合约升级
- 启用 ZapSimple.sol，替代旧的 ZapV3V4Improved.sol。
- 合约地址已更新至 .env (ZAP_V3_ADDRESS / ZAP_V4_ADDRESS)。

### 2. 开仓流程 (Enter V3)
- **原子化操作**：不再分步 approve/swap/add，而是通过 Zap 合约一次性完成。
- **OKX 路由**：Go 代码先调用 OKX API 获取最优路径 calldata，然后透传给 Zap 合约执行。
- **流程**：calculateOptimalSwap -> OKX API -> ZapSimple.zapInV3。

### 3. 撤仓流程 (Exit V3)
- **原子化操作**：使用 ZapSimple.zapOutV3 进行一键撤出流动性并收集手续费。
- **后续处理**：Go 代码负责将撤回的 token0/token1 兑换回 USDT。
- **NFT 处理**：撤仓时 NFT 会被 Approve 给 Zap 合约。

### 4. 其他
- V4 功能暂时标记为待实现。
- 删除所有旧合约相关代码。

## 2025-12-15 TokenID 保存和验证修复

### 问题描述
用户在停止任务时遇到 "Invalid token ID" 错误。经过分析发现，问题出现在三个环节：
1. **解析环节**：从交易 receipt 解析 tokenId 时可能返回 0
2. **保存环节**：没有验证 tokenId 是否为 "0" 就保存到数据库
3. **使用环节**：停止任务时没有检查 tokenId 是否为 "0"

### 修复内容

#### 1. 增强 TokenID 解析验证
- **文件**: `services/liquidity_enter.go`
- **修改**: 在 `parseZapInV3FromReceipt` 函数中添加验证，确保解析到的 tokenId 不为 0
- **效果**: 防止无效的 tokenId 被返回

#### 2. 增强 TokenID 保存验证
- **文件**: `bot/position_callbacks.go`
- **修改**: 在保存 tokenId 到数据库前，验证其不为空且不为 "0"
- **效果**: 防止无效的 tokenId 被保存到数据库

#### 3. 增强停止任务验证
- **文件**: `bot/task_callbacks.go`
- **修改**: 在检查是否可以退出时，验证 tokenId 不为空且不为 "0"
- **效果**: 对于无效 tokenId 的任务，显示友好的错误提示而不是合约 revert 错误

#### 4. 增强退出流动性验证
- **文件**: `services/liquidity_exit.go`
- **修改**: 在 V3 和 V4 退出逻辑中，添加 tokenId 有效性检查
- **效果**: 防止传入无效的 tokenId 导致合约调用失败

#### 5. 任务面板显示头寸 ID
- **文件**: `bot/task_views.go`
- **修改**: 在任务详情卡片中显示头寸 ID（V3TokenID 或 V4TokenID）
- **效果**: 用户可以直接在任务面板查看头寸 ID，方便验证和追踪

### 使用说明
- 创建任务成功后，任务面板会显示 🎫 头寸 ID
- 如果头寸 ID 为空或为 0，停止任务时会显示友好的错误提示
- 建议在创建任务后检查任务详情，确认头寸 ID 已正确保存

### 技术细节
**TokenID 解析问题**：Solidity 事件中，`indexed uint256` 类型的参数会被存储为 keccak256 hash，而不是原始值。因此无法直接从 `ZapInV3` 事件的 topics 中解析 tokenId。

**解决方案**：从 `NonfungiblePositionManager` 合约的 `IncreaseLiquidity` 事件中解析 tokenId。该事件在 mint 新头寸时触发，tokenId 作为第一个 indexed 参数，可以正确解析。

**价格范围计算**：
- Uniswap V3 中，tick 表示 token1/token0 的价格（价格 = 1.0001 ^ tick）
- 如果 token0 是 USDT，价格范围直接显示为 token1 以 USDT 计价
- 如果 token1 是 USDT，需要取倒数（1/价格）来显示 token0 以 USDT 计价
- 这样确保价格范围始终显示为"另一个币以 USDT 计价"的范围

### 交易链接显示
停止任务时，机器人会自动收集所有相关交易哈希并显示 BSCScan 链接：
- **撤出流动性** - ZapOut 交易
- **兑换 Token0→USDT** - 将 token0 兑换成 USDT
- **兑换 Token1→USDT** - 将 token1 兑换成 USDT

每个交易都带有描述说明，可以直接点击查看详情。


## 2025-12-16 授权码系统更新

### 功能说明
为 Bot 添加完整的授权码管理系统，实现访问控制和用户管理。

### 配置要求
在 `.env` 文件中添加管理员钱包地址：
```env
ADMIN_WALLET_ADDRESS=0x你的管理员钱包地址
```

### 管理员功能
只有绑定了 `ADMIN_WALLET_ADDRESS` 钱包的 Telegram 用户才能访问管理员菜单。

- `/admin` - 进入管理员控制面板
- **授权码管理** - 生成和查看授权码
- **用户管理** - 查看用户权限、编辑额度、停用/恢复用户
- **发布公告** - 向所有用户群发公告（支持普通和置顶两种类型）

### 用户管理功能
- **分页显示** - 每页8个用户，支持翻页
- **搜索用户** - 按用户名（@username）或 Telegram ID 搜索
- **统计信息** - 显示总用户数和活跃用户数
- **编辑权限** - 可修改用户的钱包上限和任务上限
- **停用/恢复** - 可暂停或恢复用户的访问权限

### 授权码生成
管理员可以选择预设方案或自定义参数：
- 有效期（天数，0 表示永久）
- 可使用人数
- 钱包数量上限
- 任务数量上限

### 授权码编辑
生成后的授权码支持二次编辑：
- 点击授权码列表中的授权码进入详情页
- 可修改使用人数、钱包上限、任务上限
- 可停用/启用授权码

### 用户授权流程
1. 新用户发送 `/start` 查看欢迎消息和授权状态
2. 未授权用户尝试创建钱包时会提示输入授权码
3. 用户输入有效授权码后即可正常使用 Bot
4. 授权用户受钱包数量和任务数量限制

### 权限检查
- 创建钱包时检查授权状态和钱包数量限制
- 创建任务时检查授权状态和活跃任务数量限制
- 管理员用户不受限制

## 2025-12-19 池子查询机制优化

### 改动背景
之前的实现依赖 The Graph Token API 进行池子信息查询，但该 API 对 BSC 链上的 PancakeSwap V3 池子支持不完整，导致大量池子查询失败。

### 解决方案
**完全移除外部 API 依赖，改为纯链上查询**。直接从区块链智能合约读取池子信息，确保所有符合标准的 V3/V4 池子都能正常查询。

### 技术实现

#### V3 池子查询流程
1. 从池子合约调用 `token0()` 和 `token1()` 获取代币地址
2. 调用 `fee()` 获取手续费率（单位：pips）
3. 通过 ERC20 合约的 `symbol()` 获取代币符号
4. 尝试从工厂合约（PancakeSwap V3 / Uniswap V3）反查确定交易所类型
5. 根据手续费率计算 tick spacing

#### V4 池子查询流程
1. 从 V4 PositionManager 合约调用 `poolKeys(bytes25(poolId))` 获取完整 PoolKey
2. PoolKey 包含：currency0, currency1, fee, tickSpacing, hooks
3. 通过 ERC20 合约获取代币符号

#### 新增函数
- [`blockchain.GetV3PoolFee()`](file:///e:/goProject/TgLpBot/backend/blockchain/v3_pool.go#L215-L255) - 读取 V3 池子手续费
- [`services.getPoolInfoFromChain()`](file:///e:/goProject/TgLpBot/backend/services/pool.go#L111-L180) - 链上查询 V3 池子完整信息
- [`services.determineExchangeFromFactory()`](file:///e:/goProject/TgLpBot/backend/services/pool.go#L182-L196) - 通过工厂合约识别交易所

#### 移除内容
- 删除 `thegraph_api.go` 及相关代码
- 移除 `PoolService.graphAPI` 和 `GeckoService.GraphAPI` 字段
- 简化查询逻辑，去除 API fallback 机制

### 优势
- ✅ **100% 覆盖**：支持所有符合标准的 V3/V4 池子
- ✅ **无需依赖**：不依赖任何外部 API，避免服务中断
- ✅ **数据准确**：直接从链上读取，数据最权威
- ✅ **代码简化**：移除复杂的 API 集成和错误处理逻辑

### 性能考虑
链上查询需要 4-5 次 RPC 调用，耗时约 200-500ms（取决于 RPC 节点速度）。对于用户交互场景（查询池子后创建任务），这个延迟是可接受的。

## 2025-12-19 V3 池子交易所识别修复

### 问题描述
之前的实现通过工厂合约反查来判断池子属于哪个交易所，但这种方式不够可靠，导致大部分池子都被识别为 "V3 Pool"，无法正确匹配对应的 Position Manager。

### 解决方案
**改为直接读取池子的 `factory()` 方法**来精确识别交易所：

#### 新增函数
- [`blockchain.GetV3PoolFactory()`](file:///e:/goProject/TgLpBot/backend/blockchain/v3_factory.go#L66-L114) - 读取池子的 factory 地址

#### 修改逻辑
- [`determineExchangeFromFactory()`](file:///e:/goProject/TgLpBot/backend/services/pool.go#L182-L210) - 根据 factory 地址精确判断交易所：
  - `0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865` → PancakeSwap V3
  - `0xdB1d10011AD0Ff90774D0C6Bb92e5C5c8b4461F7` → Uniswap V3

### 日志输出
查询池子时会显示识别结果：
```
[PoolService] Pool factory 地址: 0x0BFbcf9fa4f9C56B0F40a671Ad40E0805A091865
[PoolService] 识别为 PancakeSwap V3
[PoolService] Pool info retrieved from chain: PancakeSwap V3 WBNB/USDT (fee: 2500)
```

### 优势
- ✅ **精确识别**：直接从池子读取工厂地址，100% 准确
- ✅ **减少 RPC 调用**：只需 1 次调用而非多次尝试
- ✅ **日志清晰**：可以直观看到识别过程

## 2025-12-31 撤退卫士交易记录修复

### 问题描述
在 auto 模式下，撤退卫士（止损/再平衡）执行撤出流动性时，`TradeRecord.CloseUSDTReceived`（撤出金额）显示为 0，但实际钱包中确实收到了 USDT。这导致收益计算错误。

### 问题根因
在 `ExitTaskToUSDTWithOptions` 函数中，当 `sweepWallet=true` 时（auto 模式默认启用），流程如下：

1. 调用 `exitV3ToUSDT`（或 V4），此时 `swapDeltas=false`，**内部不执行 swap**
2. 调用 `swapWalletTokensToUSDT` 执行真正的 swap

如果 `swapWalletTokensToUSDT` 返回任何错误（包括验证时发现钱包仍有少量余额），函数会在第 125-134 行**提前返回**，跳过：
- 余额差额计算
- `CloseLatestOpenRecord` 调用
- `Transaction.AmountOut` 更新

这导致 `TradeRecord` 不会被更新，`CloseUSDTReceived` 保持为创建时的初始值 "0"。

### 修复方案
重构 `ExitTaskToUSDTWithOptions` 函数逻辑：

1. **先计算，后返回**：无论是否有错误，都先计算实际收到的 USDT 和消耗的 Gas
2. **条件更新**：只要有实际收到的金额（`actualReceived > 0`）或有交易哈希，就更新交易记录
3. **最后返回错误**：在更新记录后再返回错误，确保数据不丢失

### 涉及文件
- [`services/liquidity_exit.go`](file:///e:/goProject/TgLpBot/backend/services/liquidity_exit.go) - `ExitTaskToUSDTWithOptions` 函数

### 效果
- ✅ **即使部分失败也会记录**：撤出成功但 swap 失败时，仍能记录已收到的 USDT
- ✅ **收益计算正确**：`TradeRecord.ProfitUSDT` 和 `ProfitPct` 能正确反映实际盈亏
- ✅ **交易历史完整**：`/transactions` 命令显示正确的撤出金额

## 2025-12-31 AutoLP 止损逻辑修复

### 问题描述
用户刚开启 AutoLP 模式，就立即触发之前的亏损关闭条件，导致 AutoLP 被自动关闭。

### 问题根因
`sumAutoRealizedProfitWei` 函数在计算累计收益时，没有过滤 `LastEnabledAt` 时间。这导致系统会累加用户**所有历史** auto 任务的收益，包括上一次开启 AutoLP 时的亏损。

当用户重新开启 AutoLP 时：
1. 系统立即检查止盈/止损条件
2. 查询出历史累计亏损（如 -10.71 USDT）
3. 发现累计亏损超过设置的止损阈值（如 10 USDT）
4. 立即触发亏损关闭

### 修复方案
修改 `sumAutoRealizedProfitWei` 函数，添加 `lastEnabledAt` 参数：
- 只计算 `LastEnabledAt` 之后关闭的交易记录的收益
- 每次重新开启 AutoLP 时，收益计算会从 0 开始

### 涉及文件
- [`services/auto_lp_service.go`](file:///e:/goProject/TgLpBot/backend/services/auto_lp_service.go):
  - `applyUserStopConditions` 函数：传入 `cfg.LastEnabledAt`
  - `sumAutoRealizedProfitWei` 函数：添加时间过滤逻辑

### 效果
- ✅ **每次开启独立计算**：重新开启 AutoLP 后，从 0 开始计算收益
- ✅ **历史不影响当前**：之前的亏损不会影响新一轮的止盈/止损判断
- ✅ **止损逻辑正常**：只有本轮运行期间的亏损才会触发止损

## 2025-12-31 Allowance 检查偶发失败修复

### 问题描述
AutoLP 自动开仓时偶发出现 `allowance token1 insufficient: 0 < 30000000000000000000` 错误，第二次自动开仓没有这个问题。

### 问题根因
这是一个 **RPC 节点状态同步延迟** 问题。当 approve 交易刚被确认时，RPC 节点可能还没有同步到最新的状态，导致紧接着的 `Allowance` 查询读取到旧的状态（0）。

流程：
1. `approveToken` 发送 approve 交易并等待确认
2. 交易确认后立即查询 `Allowance`
3. 由于 RPC 节点状态延迟，查询结果可能是旧值（0）
4. 导致 `allowance insufficient` 错误

### 修复方案
在检查 allowance 失败时，添加 **2 秒延迟后重试** 机制：
- 第一次检查失败时打印日志，等待 2 秒
- 重新查询 allowance
- 如果仍然失败才报错

### 涉及文件
- [`services/liquidity_enter.go`](file:///e:/goProject/TgLpBot/backend/services/liquidity_enter.go): `enterV3FromToken` 函数中的 token0 和 token1 allowance 检查

### 效果
- ✅ **解决偶发失败**：RPC 节点延迟不再导致开仓失败
- ✅ **日志可追溯**：如果触发重试会打印日志，便于排查
- ✅ **不影响正常情况**：已授权的情况下不会有额外延迟

## 2025-12-31 开仓 Gas 费记录为 0 修复

### 问题描述
AutoLP 自动开仓的交易记录中，Gas 费显示为 `0.000000 BNB`。

### 问题根因
在 `EnterTaskFromUSDTWithOptions` 函数中，开仓交易完成后立即查询 BNB 余额，RPC 节点可能还没有同步最新状态，导致 `bnbBefore` 和 `bnbAfter` 相同，计算出的 `gasSpent` 为 0。

### 修复方案
1. 在查询 `After` 余额之前添加 **500ms 延迟**，等待 RPC 节点状态同步
2. 添加 **日志跟踪**：输出 `bnbBefore`、`bnbAfter`、`gasSpent` 的值，便于调试

### 涉及文件
- [`services/liquidity_enter.go`](file:///e:/goProject/TgLpBot/backend/services/liquidity_enter.go): `EnterTaskFromUSDTWithOptions` 函数

### 效果
- ✅ **Gas 费记录正确**：等待状态同步后读取余额，确保差额计算准确
- ✅ **日志可追溯**：可以在日志中看到余额变化详情

## 2025-12-31 撤出流动性记录修复

### 问题描述
1. **撤出金额显示为 0**：auto 模式撤退卫士撤出流动性后，交易记录中的撤出金额（`AmountOut`）显示为 0
2. **分两次兑换代币**：撤出后分别兑换"钱包余额的代币"和"流动性返还的代币"

### 问题根因
当 `sweepWallet=true`（auto 模式默认启用）时：
1. `exitV3ToUSDT` 中 `swapDeltas=false`，不执行 swap
2. 但仍然创建 `Transaction` 记录，此时 `AmountOut = usdtAfter - usdtBefore = 0`（因为还没有 swap）
3. 后续虽然顶层有更新逻辑，但因为记录已存在且 `AmountOut` 是字符串"0"，更新逻辑未能正确执行

### 修复方案
1. **`exitV3ToUSDT`**：只有当 `swapDeltas=true` 时才创建 `Transaction` 记录
2. **`ExitTaskToUSDTWithOptions`**：改进 Transaction 记录逻辑为 **Upsert** 模式：
   - 先尝试更新已有记录（`swapDeltas=true` 时 `exitV3ToUSDT` 已创建）
   - 如果记录不存在则创建新记录（`sweepWallet` 模式）

### 关于"分两次兑换"
这是设计如此。`sweepWallet` 模式会清仓钱包中**所有**非 USDT 代币，包括：
- 之前开仓残余的代币
- 本次撤出返还的代币

这样可以确保完全清仓，但日志显示可能是多次兑换。

### 涉及文件
- [`services/liquidity_exit.go`](file:///e:/goProject/TgLpBot/backend/services/liquidity_exit.go):
  - `exitV3ToUSDT`: 条件创建 Transaction 记录
  - `ExitTaskToUSDTWithOptions`: Upsert 逻辑

### 效果
- ✅ **撤出金额正确记录**：交易记录中显示正确的 USDT 撤出金额
- ✅ **避免重复记录**：不会创建 AmountOut=0 的无效记录

## 2025-12-31 同一代币重复兑换问题修复

### 问题描述
同一种代币被分两次兑换：先兑换"钱包余额的代币"，再兑换"流动性返还的代币"，浪费 Gas。

### 问题根因
`swapWalletTokensToUSDT` 函数在 swap 完成后会校验余额（第 341-356 行）。由于 swap 可能有滑点或小额残余，校验发现残余时返回错误，触发重试机制：

1. **第一次调用**：撤出 LP + swap 代币 → 校验发现小额残余 → 返回错误
2. **重试调用**：LP 已空（跳过撤出）+ **再次 swap 同一代币的残余部分**

### 修复方案
修改 `swapWalletTokensToUSDT` 的校验逻辑：
- 将"因残余返回错误"改为"仅打印警告日志"
- 只有 swap 本身失败才返回错误

### 涉及文件
- [`services/liquidity_exit.go`](file:///e:/goProject/TgLpBot/backend/services/liquidity_exit.go):
  - `ExitTaskToUSDTWithOptions`: 在 `swapWalletTokensToUSDT` 调用前添加 1 秒延迟
  - `swapWalletTokensToUSDT`: 移除因残余返回错误的逻辑

### 效果
- ✅ **一次性兑换完成**：等待 RPC 同步后，一次获取所有代币余额（残余 + 撤出返还的）
- ✅ **节省 Gas**：不会因 RPC 延迟导致分两次兑换同一代币
- ✅ **日志可追溯**：如有残余会打印警告日志


## Web Dashboard Update (2026-01-02)

### 1. UI/UX 全面升级 (Glassmorphism)
- **视觉风格**: 采用 "Glassmorphism" 毛玻璃风格，配合 Mesh Gradient 动态背景。
- **字体优化**: 引入 `Outfit` (标题) 和 `Inter` (正文) 字体，提升阅读体验。
- **深色模式**: 优化了 Dark Mode 的配色，从 slate 灰转向更深邃的 zinc 色系。

### 2. 组件重构
- **PositionCard**: 重新设计为全息卡片样式，增强了展示效果和交互动画。
- **PoolTable**: 简化了表格边框，增加了悬停高亮和自定义 Badge 样式。
- **布局**: 头部导航改为悬浮玻璃栏，优化了移动端适配。

## 2026-02 MiniApp 小程序位置卡片 UI 优化

### 问题描述
- **文本重叠与挤压**: 交易对名称过长或多标签时会导致绝对定位下的 UI 元素重叠，影响视觉。
- **表格列空间不足**: 余额明细中如果 Token 名称过长会导致数值被挤压。

### 修复方案
- **FlexBox 重构标题区域**: 使用 flex 布局替换原来的 absolute 绝对定位坐标，使得组件尺寸可以在名称长内容情况下自适应截断 (truncate)。
- **表格按比例分配空间**: 余额明细采用 `grid-cols-[1.5fr_1fr_1fr_1fr]` 的按比例布局以让 Token 名称得到更多的可视空间，并对溢出的表格内容增加截断属性，避免样式错乱。
- **智能提取标题**: 自动处理池子长标题 (如 `panv3-USDT-POWER-1.0%`) 截取为精简的 `USDT-POWER`。
- **去除冗余白条**: 移除了原有的卡片左侧装饰用状态条。
- **价格带指针重绘**: 彻底移除了原先发绿光且笨重的圆点，将其绘制成了带有下延竖线与倒三角的专业滑块 (Slider) 指针，并且改用了高级对比色的渐变蓝作为主视觉颜色。

## 2026-02 MiniApp 小程序导航栏与图标重构

### 问题描述
- **图标老旧**: 原有的 SVG 路径定义的图标样式陈旧，视觉上廉价。
- **导航栏呆板**: 底部和顶部导航栏样式简单，缺乏互动的生命力和层次感，影响 Web3 应用的沉浸式体验。

### 修复方案
- **接入 Lucide React**: 全面将原来代码中自定义的硬编码 SVG Icon 替换成了业界现代标准的 `lucide-react` 图标库，涉及首页、仓位页、分析页以及管理台的各个关键入口图标。
- **现代化浮动底栏 (Floating Pill Nav)**: 将底部固定的简单边框导航，重塑成了带有毛玻璃 (Backdrop Blur) 特效的浮动胶囊样式，增加交互触感与缩放动画 (`scale-110`)。
- **顶部排序栏重绘**: 顶部如热门池子的筛选切换标签 (Tabs)，重新设计了凹凸质感与流畅的宽胶囊内嵌过渡效果，活动状态下赋予更加细腻的高对比颜色指示。

## 2026-03 监控通知(Smart Money)模块双端重构

### 问题描述
- **模块名称过时**: 原"聪明钱监控/金狗通知"名称不够准确，需更名为更直观的"监控通知"。
- **UI 交互拥挤**: 原配置页面将各类设置杂糅在一页，缺乏合理的区块划分，视觉显得凌乱笨重。

### 修复方案
- **全局文案更名**: 将各个主入口、标题栏从 金狗通知 全面更名为 监控通知 或 监控通知中心。
- **status Separation**: 把监控细分为 "智能狗通知 (聪明的钱)" 和 "池子监控 (PoolM)" 两个并列选项卡。
- **UI 质感升级**:
  - miniapp: 重构了顶层 Layout，改用带圆角的现代过滤按钮组与 TailwindCSS animate-in 入场动画。
  - webapp: 手写平滑的 transition 与渐变背景框，实现了沉浸式的深色主题选项卡，并修复了历史遗留的组件重复挂载问题。

## 2026-03 开仓风控校验与界面UI优化

### 问题描述
- **报错宽泛不明确**: 当用户打开开仓界面时，无论是流动性不足还是价格偏离异常，经常出现弹窗宽泛地提示“开仓风控校验失败”，没有任何详情说明。
- **视觉层板正**: 报警块等 UI 设计传统死板、缺乏层级感。

### 修复方案
- **Golang Typed Nil 拆箱吞噬 Bug 修复**: 在 `backend/service/liquidity/open_position_guard.go` 中，防范检查（`CheckOpenPositionSafety`）在返回具体的 `*ZapSafetyError(nil)` 时由于 Go 语言机制变成了非 nil 的 `error` interface 抛出。这导致后端 `open_position.go` API 无法正确还原真实错误，直接输出了没有任何 `liquidity_usd` 等元数据的 Fallback "开仓风控校验失败"。此时已显式拦截并修复该错误。
- **UI 毛玻璃高级化重构**: 在 `miniapp/src/App.jsx` 及 `webapp/src/components/OpenPositionModal.jsx` 对双端的开仓流程错误和警告元素进行了重置：
  - 引入了 `lucide-react` 的 `AlertTriangle`、`Check`、`X` 结构化功能图标。
  - 添加了 `bg-gradient-to-br` 以及毛玻璃面板。
  - 修改了复选框 Checkbox 的玻璃悬浮感，使交互更精致。
  - 显示出了具体开仓允许最高金额以及流动性限制文字，改善用户理解。
