# TgLpBot - Telegram Liquidity Pool Bot

A Telegram bot for managing liquidity pools on Binance Smart Chain (BSC). Add and remove liquidity using USDT with optimal swap routes via OKX DEX aggregator.

## Features

- 💼 **Wallet Management**: Create or import wallets with encrypted private key storage
- 💰 **Add Liquidity**: Add liquidity to any BSC pool using only USDT
- 📤 **Remove Liquidity**: Remove liquidity and receive USDT
- 🔄 **Optimal Routing**: Uses OKX DEX aggregator for best swap rates
- ⚡ **Zap Contract**: Custom smart contract for single-token liquidity operations
- 📊 **Transaction Tracking**: Monitor all your transactions
- 🔒 **Secure**: AES-256 encryption for private keys

## Architecture

```
TgLpBot/
├── blockchain/          # Blockchain interaction layer
│   ├── client.go       # BSC client initialization
│   ├── erc20.go        # ERC20 token interactions
│   └── zap.go          # Zap contract bindings
├── bot/                # Telegram bot handlers
│   ├── bot.go          # Bot initialization
│   ├── handlers.go     # Command handlers
│   └── input_handlers.go # User input handlers
├── config/             # Configuration management
│   └── config.go
├── contracts/          # Smart contracts
│   └── LiquidityZap.sol # Zap contract for single-token LP
├── database/           # Database layer
│   ├── mysql.go        # MySQL connection
│   └── redis.go        # Redis connection
├── models/             # Data models
│   ├── user.go
│   ├── wallet.go
│   ├── lp_config.go
│   └── transaction.go
├── services/           # Business logic
│   ├── user.go
│   ├── wallet.go
│   ├── liquidity.go
│   └── okx_dex.go
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

# Database
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASSWORD=your_password
MYSQL_DATABASE=tglpbot

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# BSC Network
BSC_RPC_URL=https://bsc-dataseed1.binance.org/
BSC_CHAIN_ID=56

# Encryption (generate a random 32-byte hex string)
ENCRYPTION_KEY=your_32_byte_hex_key_here

# OKX DEX API (optional)
OKX_API_KEY=your_okx_api_key
OKX_SECRET_KEY=your_okx_secret_key
OKX_PASSPHRASE=your_okx_passphrase
```

### 6. Deploy Zap Contract

Deploy the `contracts/LiquidityZap.sol` contract to BSC:

1. Compile the contract using Hardhat, Truffle, or Remix
2. Deploy with PancakeSwap Router address: `0x10ED43C718714eb63d5aA57B78B54704E256024E`
3. Update `ZAP_CONTRACT_ADDRESS` in `.env` with the deployed address

Example using Remix:
- Open [Remix IDE](https://remix.ethereum.org/)
- Create a new file and paste the contract code
- Compile with Solidity 0.8.0+
- Deploy to BSC Mainnet with router address
- Copy the deployed contract address

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
- `/addlp` - Add liquidity with USDT
- `/removelp` - Remove liquidity to USDT
- `/config` - Configure LP pool parameters
- `/transactions` - View transaction history

### Workflow

1. **Create/Import Wallet**
   ```
   /wallet → Create Wallet or Import Wallet
   ```

2. **Configure LP Pool**
   ```
   /config → Enter pool address
   ```

3. **Add Liquidity**
   ```
   /addlp → Enter pool address → Enter USDT amount → Set slippage
   ```

4. **Remove Liquidity**
   ```
   /removelp → Enter pool address → Enter LP amount → Set slippage
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

