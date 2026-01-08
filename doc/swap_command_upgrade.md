# Swap 零钱兑换功能升级 - 2026-01-08

## 变更概述
将原来的 `/swap_to_usdt` 指令改为 `/swap`，描述改为"零钱兑换"，并实现先扫描展示再确认兑换的交互流程。

## 主要修改

### 指令变更
| 原指令 | 新指令 | 描述 |
|--------|--------|------|
| `/swap_to_usdt` | `/swap` | 零钱兑换 |

### 新交互流程
1. 用户点击**零钱兑换**或输入 `/swap`
2. 系统扫描钱包，获取价值大于 0.1 USDT 的代币
3. 展示代币列表：符号、数量、USDT 价值
4. 显示总价值和滑点设置
5. 用户确认后执行兑换

### 价值筛选
- 只显示价值 **≥ 0.1 USDT** 的代币
- 排除 BNB/WBNB 和稳定币（USDT/USDC/BUSD）
- 通过 OKX DEX 报价获取实时价值

## 修改文件
- `bot.go`: 添加 `/swap` 命令映射
- `handlers.go`: 更新帮助文本和钱包管理按钮
- `wallet_swap_to_usdt.go`: 重写 `promptWalletSwapToUSDT` 函数
- `liquidity_wallet_swap.go`: 添加 `ScanWalletTokensForSwap` 和 `getTokenValueInUSDT` 方法

## 验证状态
✅ 编译通过
