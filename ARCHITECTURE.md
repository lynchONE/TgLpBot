# TgLpBot Architecture

This document provides a comprehensive overview of the TgLpBot architecture, design decisions, and implementation details.

## System Overview

TgLpBot is a Telegram bot that enables users to manage liquidity pools on Binance Smart Chain (BSC) using a simple chat interface. The system integrates multiple components:

- **Telegram Bot**: User interface and interaction
- **Blockchain Layer**: BSC network interaction
- **Database**: Persistent storage (MySQL)
- **Cache**: Session and temporary data (Redis)
- **Smart Contracts**: On-chain liquidity operations
- **External APIs**: OKX DEX aggregator for optimal routing

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         Telegram                             │
│                      (User Interface)                        │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│                      TgLpBot Server                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │   Bot Layer  │  │   Services   │  │   Database   │      │
│  │              │  │              │  │              │      │
│  │ • Handlers   │─▶│ • User       │─▶│ • MySQL      │      │
│  │ • Commands   │  │ • Wallet     │  │ • Redis      │      │
│  │ • Callbacks  │  │ • Liquidity  │  │              │      │
│  └──────────────┘  └──────┬───────┘  └──────────────┘      │
│                            │                                 │
│                            ▼                                 │
│                   ┌──────────────┐                          │
│                   │  Blockchain  │                          │
│                   │    Client    │                          │
│                   └──────┬───────┘                          │
└──────────────────────────┼──────────────────────────────────┘
                           │
        ┌──────────────────┼──────────────────┐
        │                  │                  │
        ▼                  ▼                  ▼
┌──────────────┐  ┌──────────────┐  ┌──────────────┐
│     BSC      │  │  Zap Contract│  │   OKX DEX    │
│   Network    │  │              │  │     API      │
│              │  │ • zapIn()    │  │              │
│ • Tokens     │  │ • zapOut()   │  │ • Quote      │
│ • Pools      │  │              │  │ • Swap       │
└──────────────┘  └──────────────┘  └──────────────┘
```

## Component Details

### 1. Bot Layer (`bot/`)

**Responsibilities:**
- Handle Telegram updates (messages, callbacks)
- Parse user commands
- Manage conversation flow
- Format and send responses

**Key Files:**
- `bot.go`: Bot initialization and main loop
- `handlers.go`: Command handlers (/start, /wallet, etc.)
- `input_handlers.go`: User input processing

**Design Patterns:**
- Command Pattern: Each command has a dedicated handler
- State Machine: User sessions track conversation state
- Callback Pattern: Inline keyboard interactions

### 2. Services Layer (`services/`)

**Responsibilities:**
- Business logic implementation
- Data validation
- External API integration
- Transaction orchestration

**Key Services:**

#### UserService
- User registration and management
- Profile updates
- Activity tracking

#### WalletService
- Wallet creation and import
- Private key encryption/decryption
- Address management
- Default wallet selection

#### LiquidityService
- Add liquidity operations
- Remove liquidity operations
- Token approvals
- Zap contract interaction
- Transaction recording

#### OKXDexService
- Quote fetching
- Swap route optimization
- API authentication

### 3. Blockchain Layer (`blockchain/`)

**Responsibilities:**
- BSC network connection
- Smart contract interaction
- Transaction signing and sending
- Balance queries

**Key Components:**

#### Client
- RPC connection management
- Gas price estimation
- Nonce management
- Transaction broadcasting

#### ERC20
- Token balance queries
- Approval transactions
- Transfer operations

#### Zap Contract
- Single-token liquidity addition
- Single-token liquidity removal
- Slippage protection

### 4. Database Layer (`database/`)

**MySQL Schema:**

```sql
users
├── id (PK)
├── telegram_id (UNIQUE)
├── username
├── first_name
├── last_name
├── language_code
├── is_active
└── timestamps

wallets
├── id (PK)
├── user_id (FK)
├── address (UNIQUE)
├── encrypted_private_key
├── name
├── is_default
└── timestamps

lp_configs
├── id (PK)
├── user_id (FK)
├── pool_address
├── token0_address
├── token1_address
├── min/max amounts
├── slippage_tolerance
├── auto_add/remove flags
└── timestamps

transactions
├── id (PK)
├── user_id (FK)
├── tx_hash (UNIQUE)
├── type (swap/add_lp/remove_lp)
├── status (pending/confirmed/failed)
├── from/to addresses
├── token addresses
├── amounts
├── gas info
└── timestamps
```

**Redis Data Structures:**

```
session:{telegram_id}:state          → User conversation state
session:{telegram_id}:pool_address   → Temporary pool address
session:{telegram_id}:usdt_amount    → Temporary amount
cache:token:{address}:symbol         → Token symbol cache
cache:token:{address}:decimals       → Token decimals cache
```

### 5. Smart Contracts (`contracts/`)

#### LiquidityZap Contract

**Purpose:** Enable single-token liquidity operations

**Key Functions:**

```solidity
zapIn(
    address tokenIn,
    uint256 amountIn,
    address pair,
    uint256 minLiquidity,
    uint256 deadline
) → uint256 liquidity

zapOut(
    address pair,
    uint256 liquidity,
    address tokenOut,
    uint256 minAmountOut,
    uint256 deadline
) → uint256 amountOut
```

**Flow:**

**Zap In:**
1. Receive single token from user
2. Swap half for the other pool token
3. Add liquidity with both tokens
4. Return LP tokens to user

**Zap Out:**
1. Receive LP tokens from user
2. Remove liquidity to both tokens
3. Swap one token for the other
4. Return single token to user

## Data Flow

### Add Liquidity Flow

```
User: /addlp
  ↓
Bot: Request pool address
  ↓
User: 0x123...
  ↓
Bot: Request USDT amount
  ↓
User: 100
  ↓
Bot: Request slippage
  ↓
User: 0.5
  ↓
Service: Get user wallet
  ↓
Service: Approve USDT to Zap
  ↓
Service: Call zapIn()
  ↓
Blockchain: Execute transaction
  ↓
Service: Record transaction
  ↓
Bot: Send confirmation with tx hash
```

### Remove Liquidity Flow

```
User: /removelp
  ↓
Bot: Request pool address
  ↓
User: 0x123...
  ↓
Bot: Request LP amount
  ↓
User: 0.5
  ↓
Bot: Request slippage
  ↓
User: 0.5
  ↓
Service: Get user wallet
  ↓
Service: Approve LP to Zap
  ↓
Service: Call zapOut()
  ↓
Blockchain: Execute transaction
  ↓
Service: Record transaction
  ↓
Bot: Send confirmation with tx hash
```

## Security Architecture

### Private Key Security

1. **Encryption at Rest**
   - AES-256-GCM encryption
   - Unique nonce per encryption
   - Key stored in environment variable

2. **Access Control**
   - Keys only decrypted when needed
   - Never logged or displayed
   - Deleted from memory after use

3. **User Messages**
   - Private key messages deleted immediately
   - No message history retention

### Transaction Security

1. **Slippage Protection**
   - User-defined slippage tolerance
   - Minimum output amount checks
   - Deadline enforcement

2. **Gas Management**
   - Maximum gas price limits
   - Gas estimation before execution
   - Sufficient balance checks

3. **Approval Management**
   - Exact amount approvals
   - Approval checks before transactions
   - No unlimited approvals

## Performance Considerations

### Caching Strategy

1. **Token Metadata**
   - Symbol, decimals cached in Redis
   - TTL: 24 hours
   - Reduces RPC calls

2. **User Sessions**
   - Conversation state in Redis
   - TTL: 30 minutes
   - Fast state retrieval

3. **Database Queries**
   - Indexed columns (telegram_id, address, tx_hash)
   - Connection pooling
   - Prepared statements

### Scalability

1. **Horizontal Scaling**
   - Stateless bot instances
   - Shared Redis for sessions
   - Load balancer ready

2. **Database Optimization**
   - Indexed foreign keys
   - Partitioning by date (transactions)
   - Regular cleanup of old data

3. **Rate Limiting**
   - Per-user command rate limits
   - API call throttling
   - Queue for blockchain transactions

## Error Handling

### Levels of Error Handling

1. **User-Facing Errors**
   - Friendly error messages
   - Actionable suggestions
   - No technical details

2. **Logged Errors**
   - Full stack traces
   - Context information
   - Timestamp and user ID

3. **Critical Errors**
   - Database connection failures
   - Blockchain connection issues
   - Automatic restart attempts

### Recovery Strategies

1. **Transaction Failures**
   - Record failed transactions
   - Allow retry with adjusted parameters
   - Refund gas if possible

2. **Network Issues**
   - Automatic reconnection
   - Exponential backoff
   - Fallback RPC endpoints

3. **Data Inconsistencies**
   - Transaction verification
   - Balance reconciliation
   - Manual intervention alerts

## Monitoring and Logging

### Metrics to Track

1. **System Metrics**
   - CPU and memory usage
   - Database connections
   - Redis memory usage

2. **Business Metrics**
   - Active users
   - Transactions per day
   - Success/failure rates
   - Average transaction value

3. **Performance Metrics**
   - Response time
   - Transaction confirmation time
   - API latency

### Logging Strategy

1. **Structured Logging**
   - JSON format
   - Log levels (DEBUG, INFO, WARN, ERROR)
   - Contextual information

2. **Log Rotation**
   - Daily rotation
   - Compression of old logs
   - Retention policy (30 days)

3. **Alerting**
   - Critical errors → Immediate notification
   - High error rate → Warning
   - System resource limits → Alert

## Future Enhancements

### Planned Features

1. **Multi-Chain Support**
   - Ethereum, Polygon, Arbitrum
   - Chain-specific configurations
   - Cross-chain bridges

2. **Advanced Trading**
   - Limit orders
   - Stop-loss/take-profit
   - DCA strategies

3. **Portfolio Management**
   - Total value tracking
   - P&L calculations
   - Performance analytics

4. **Social Features**
   - Share strategies
   - Copy trading
   - Leaderboards

### Technical Improvements

1. **GraphQL API**
   - Flexible data queries
   - Real-time subscriptions
   - Better client integration

2. **WebSocket Support**
   - Real-time price updates
   - Transaction notifications
   - Live portfolio tracking

3. **Machine Learning**
   - Optimal slippage prediction
   - Gas price forecasting
   - Risk assessment

## Conclusion

TgLpBot is designed with security, scalability, and user experience in mind. The modular architecture allows for easy maintenance and feature additions while maintaining code quality and performance.

For questions or contributions, please refer to the main README.md or open an issue on GitHub.

