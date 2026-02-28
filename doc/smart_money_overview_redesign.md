# 聪明钱概览页面重新设计

**日期**: 2026-02-28

## 修改内容

### 1. SmartMoneyCard.jsx — 概览Tab重新设计

**核心问题**：区间数值位数多时被 `truncate` 截断显示不全；页面一次性加载20个池子的钱包明细导致卡顿。

**改动点**：

- **新增 `compactPrice` 格式函数**：极小数值使用下标零表示法（如 `0.0₃6959` 表示 `0.0006959`），在有限空间内传达完整价格信息
- **新增 `PoolOverviewCard` 子组件**：每个池子独立为卡片组件
  - 钱包行采用两行布局：第一行=钱包地址+金额+占比，第二行=区间独占整行不再截断
  - 钱包行去除 `NumberFlowValue` 动画组件改用纯文本 `<span>`，减少渲染开销
  - 默认展示前3个钱包，超过3个时显示"展开全部"按钮
  - 未预加载的池子显示"点击加载钱包明细"按需加载按钮
  - 加载失败时显示重试按钮
- **预加载优化**：`preloadPools` 从20个减少到3个，其余池子按需加载
- **池子头部增加总金额展示**：右上角显示钱包总投入金额

### 2. App.jsx — 修复构建错误

上次会话的 `multi_replace` 操作导致 App.jsx 大范围缩进错位和 JSX 结构损坏。

**处理**：用 `git checkout HEAD` 恢复原始文件后，干净地重新应用了4项开仓页面修改：
- quickRangeOptions 改为 ±1/3/5/10/20/30
- 移除 `setOpenPositionAllowSwap(false)`
- `allowEntrySwap` 硬编码为 `true`，移除 entry-swap 错误处理
- 移除"允许兑换" UI 区块

## 构建验证

✅ `npx vite build --mode development` — 70 modules transformed, built in 1.79s
