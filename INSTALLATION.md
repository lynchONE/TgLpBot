# 安装和部署指南

本指南将帮助您从零开始部署 TgLpBot。

## 目录

1. [系统要求](#系统要求)
2. [准备工作](#准备工作)
3. [安装步骤](#安装步骤)
4. [配置说明](#配置说明)
5. [部署合约](#部署合约)
6. [启动机器人](#启动机器人)
7. [验证安装](#验证安装)
8. [故障排除](#故障排除)

## 系统要求

### 硬件要求
- CPU: 1 核心以上
- 内存: 2GB RAM 以上
- 磁盘: 20GB 可用空间
- 网络: 稳定的互联网连接

### 软件要求
- 操作系统: Linux (Ubuntu 20.04+ 推荐) / macOS / Windows
- Go: 1.21 或更高版本
- MySQL: 8.0 或更高版本
- Redis: 6.0 或更高版本
- Node.js: 16+ (用于部署合约)
- Git: 最新版本

## 准备工作

### 1. 创建 Telegram Bot

1. 在 Telegram 中找到 [@BotFather](https://t.me/botfather)
2. 发送 `/newbot` 创建新机器人
3. 按提示设置机器人名称和用户名
4. 保存 Bot Token（格式：`123456789:ABCdefGHIjklMNOpqrsTUVwxyz`）

### 2. 获取 BSC RPC 节点

选择以下任一方式：

**选项 A: 使用公共 RPC（免费）**
- 主网: `https://bsc-dataseed1.binance.org/`
- 测试网: `https://data-seed-prebsc-1-s1.binance.org:8545/`

**选项 B: 使用私有 RPC（推荐）**
- [QuickNode](https://www.quicknode.com/)
- [Ankr](https://www.ankr.com/)
- [Infura](https://infura.io/)

### 3. 获取 OKX API（可选）

1. 注册 [OKX](https://www.okx.com/) 账户
2. 创建 API Key
3. 保存 API Key, Secret Key, Passphrase

### 4. 准备部署钱包

1. 创建一个新的 BSC 钱包（用于部署合约）
2. 获取私钥
3. 充值少量 BNB（约 0.1 BNB 用于部署）

## 安装步骤

### 步骤 1: 安装依赖

#### Ubuntu/Debian

```bash
# 更新系统
sudo apt update && sudo apt upgrade -y

# 安装 Go
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc

# 验证安装
go version

# 安装 MySQL
sudo apt install mysql-server -y
sudo systemctl start mysql
sudo systemctl enable mysql

# 安全配置 MySQL
sudo mysql_secure_installation

# 安装 Redis
sudo apt install redis-server -y
sudo systemctl start redis
sudo systemctl enable redis

# 安装 Node.js
curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash -
sudo apt install nodejs -y

# 安装 Git
sudo apt install git -y
```

#### macOS

```bash
# 使用 Homebrew 安装
brew install go mysql redis node git

# 启动服务
brew services start mysql
brew services start redis
```

### 步骤 2: 克隆项目

```bash
# 克隆仓库
git clone https://github.com/yourusername/TgLpBot.git
cd TgLpBot

# 或者如果您已经在项目目录中
# 确保所有文件都已创建
```

### 步骤 3: 运行设置脚本

```bash
# 给脚本执行权限
chmod +x scripts/setup.sh
chmod +x scripts/deploy.sh

# 运行设置脚本
./scripts/setup.sh
```

这个脚本会：
- 检查 Go 版本
- 安装 Go 依赖
- 创建 `.env` 文件
- 生成加密密钥
- 创建数据库（如果选择）

## 配置说明

### 编辑 .env 文件

```bash
vim .env  # 或使用您喜欢的编辑器
```

### 必填配置项

```env
# Telegram Bot Token（必填）
TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrsTUVwxyz

# MySQL 配置（必填）
MYSQL_HOST=localhost
MYSQL_PORT=3306
MYSQL_USER=root
MYSQL_PASSWORD=your_secure_password
MYSQL_DATABASE=tglpbot

# Redis 配置（必填）
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# BSC 网络配置（必填）
BSC_RPC_URL=https://bsc-dataseed1.binance.org/
BSC_CHAIN_ID=56

# 加密密钥（已自动生成，不要修改）
ENCRYPTION_KEY=your_generated_key_here
```

### 可选配置项

```env
# OKX DEX API（可选，用于最优路径）
OKX_DEX_API_URL=https://www.okx.com/api/v5/dex/aggregator
OKX_API_KEY=your_okx_api_key
OKX_SECRET_KEY=your_okx_secret_key
OKX_PASSPHRASE=your_okx_passphrase

# Gas 配置（可选）
MAX_GAS_PRICE=5000000000
GAS_LIMIT=500000
```

### 创建数据库

```bash
# 登录 MySQL
mysql -u root -p

# 创建数据库
CREATE DATABASE tglpbot CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;

# 退出
exit;
```

## 部署合约

### 步骤 1: 安装合约依赖

```bash
cd contracts
npm install
```

### 步骤 2: 配置部署钱包

在 `.env` 文件中添加：

```env
DEPLOYER_PRIVATE_KEY=your_deployer_wallet_private_key
BSCSCAN_API_KEY=your_bscscan_api_key  # 可选，用于验证合约
```

### 步骤 3: 部署到测试网（推荐先测试）

```bash
npm run deploy:testnet
```

### 步骤 4: 部署到主网

```bash
npm run deploy:mainnet
```

### 步骤 5: 更新配置

部署成功后，复制合约地址并更新 `.env`：

```env
ZAP_CONTRACT_ADDRESS=0x...  # 您的合约地址
```

## 启动机器人

### 开发模式

```bash
# 返回项目根目录
cd ..

# 运行机器人
make run
```

### 生产模式

#### 方式 1: 使用部署脚本（推荐）

```bash
./scripts/deploy.sh
```

这会：
- 构建二进制文件
- 创建 systemd 服务
- 启动机器人
- 设置自动重启

#### 方式 2: 手动部署

```bash
# 构建
make build

# 运行
./build/tglpbot
```

#### 方式 3: 使用 Docker

```bash
# 构建并启动
docker-compose up -d

# 查看日志
docker-compose logs -f bot
```

## 验证安装

### 1. 检查服务状态

```bash
# 检查机器人服务
sudo systemctl status tglpbot

# 检查 MySQL
sudo systemctl status mysql

# 检查 Redis
sudo systemctl status redis
```

### 2. 测试机器人

1. 在 Telegram 中找到您的机器人
2. 发送 `/start`
3. 应该收到欢迎消息

### 3. 测试钱包功能

```
/wallet → Create Wallet
```

应该成功创建钱包并显示地址

### 4. 检查日志

```bash
# Systemd 日志
sudo journalctl -u tglpbot -f

# Docker 日志
docker-compose logs -f bot

# 文件日志
tail -f /var/log/tglpbot/output.log
```

## 故障排除

### 问题 1: 机器人无响应

**检查项：**
```bash
# 检查进程
ps aux | grep tglpbot

# 检查日志
sudo journalctl -u tglpbot -n 50

# 检查网络
curl https://api.telegram.org/bot<YOUR_TOKEN>/getMe
```

**解决方案：**
- 验证 Bot Token 正确
- 检查网络连接
- 重启机器人服务

### 问题 2: 数据库连接失败

**检查项：**
```bash
# 测试 MySQL 连接
mysql -u root -p -e "SHOW DATABASES;"

# 检查 MySQL 状态
sudo systemctl status mysql
```

**解决方案：**
- 验证数据库凭据
- 确保 MySQL 正在运行
- 检查防火墙设置

### 问题 3: Redis 连接失败

**检查项：**
```bash
# 测试 Redis
redis-cli ping

# 检查 Redis 状态
sudo systemctl status redis
```

**解决方案：**
- 确保 Redis 正在运行
- 检查 Redis 配置
- 验证端口未被占用

### 问题 4: 合约部署失败

**检查项：**
- 部署钱包有足够的 BNB
- RPC 节点正常工作
- 网络连接稳定

**解决方案：**
```bash
# 检查钱包余额
# 使用 BSCScan 查看

# 尝试不同的 RPC 节点
# 更新 hardhat.config.js 中的 RPC URL
```

### 问题 5: 交易失败

**常见原因：**
- Gas 不足
- 滑点过低
- 代币未授权
- 合约地址错误

**解决方案：**
- 增加 Gas Limit
- 提高滑点容忍度
- 检查授权状态
- 验证合约地址

## 维护建议

### 日常维护

```bash
# 每日检查日志
sudo journalctl -u tglpbot --since today

# 每周备份数据库
mysqldump -u root -p tglpbot > backup_$(date +%Y%m%d).sql

# 监控磁盘空间
df -h

# 监控内存使用
free -h
```

### 更新机器人

```bash
# 拉取最新代码
git pull

# 重新构建
make build

# 重启服务
sudo systemctl restart tglpbot
```

### 安全建议

1. **定期更新系统**
   ```bash
   sudo apt update && sudo apt upgrade -y
   ```

2. **配置防火墙**
   ```bash
   sudo ufw allow 22/tcp
   sudo ufw enable
   ```

3. **使用强密码**
   - MySQL root 密码
   - 加密密钥
   - 服务器 SSH 密钥

4. **定期备份**
   - 数据库
   - `.env` 文件
   - 钱包私钥

## 下一步

安装完成后，您可以：

1. 阅读 [QUICKSTART.md](QUICKSTART.md) 了解基本使用
2. 查看 [README.md](README.md) 了解详细功能
3. 参考 [ARCHITECTURE.md](ARCHITECTURE.md) 了解系统架构
4. 加入社区获取支持

## 获取帮助

如果遇到问题：

1. 查看本文档的故障排除部分
2. 搜索 GitHub Issues
3. 创建新的 Issue
4. 联系 Telegram 支持: @yoursupport

祝您使用愉快！🚀

