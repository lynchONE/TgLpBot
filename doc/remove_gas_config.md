# 移除 Gas 配置项

## 进展日期
2026-01-06

## 修改内容

根据用户需求，移除了代码中的 `MAX_GAS_PRICE` 和 `GAS_LIMIT` 硬编码配置，改为让区块链节点自动估算 gas。

## 修改文件

### 配置层
- **config.go**: 移除了 `MaxGasPrice` 和 `GasLimit` 字段定义及相关环境变量解析

### 区块链客户端
- **client.go**: 简化了 `GetGasPrice()` 和 `GetGasPriceWithMultiplier()` 函数，移除 `MaxGasPrice` 上限限制

### 流动性服务
- **liquidity.go**: `approveToken` 函数改为设置 `auth.GasLimit = 0`，让节点自动估算
- **liquidity_exit.go**: 
  - 修改 `buildAuth()` 函数，移除 `gasLimit` 参数，内部固定设置 `GasLimit = 0`
  - 更新所有调用 `buildAuth()` 的代码
  - OKX swap（`swapDeltaToUSDTWithHash` / `swapExactInViaOKXWithHash`）：改为使用节点 `EstimateGas` 估算 gasLimit，并对 OKX 返回值取 max 后乘以安全系数（避免 out of gas；可用 `OKX_SWAP_GAS_LIMIT_*` 调整）
- **liquidity_enter.go**: 更新所有调用 `buildAuth()` 的代码
- **okx_swap.go**: OKX swap 改为使用节点 `EstimateGas` + 安全系数设置 gasLimit（raw tx 必须显式给 gasLimit）

## 改动效果

1. **自动 Gas 估算**: 所有交易的 gas limit 现在由节点自动估算，无需手动配置
2. **无 Gas 价格上限**: 移除了 `MAX_GAS_PRICE` 限制，系统会使用链上实际建议的 gas 价格
3. **简化配置**: 减少了两个环境变量配置项
4. **OKX swap 防 out-of-gas**: raw tx 采用 `EstimateGas` 并增加安全缓冲（可配置）

## 编译验证
✅ `go build ./...` 编译成功
