# 钱包与兑换 UI/UX 体验升级

## 完成日期
2026-04-07

## 目标
1. 钱包管理模块支持增删改查。
2. 将系统中的“管理”模块名称统一更改为“我的”。
3. 将一键兑换页面重构为类似 Uniswap 的专业单代币兑换界面，支持在任意代币间使用集成的 OKX 功能进行兑换，并废弃原有的“一键兑换（批量扫描余额兑换）”模式。支持对 Miniapp 和 WebApp 进行同步改造。

## 已完成功能

### 1. 导航栏和标题优化 ✅
- WebApp 和 Miniapp 的底部导航将 `管理` 更名为 `我的`，符合现代 Web3 钱包应用的普遍规范。
- 修改了 `App.jsx`, `utils.js` 中的文案映射及标题。

### 2. 钱包管理 (CRUD) 功能 ✅
- **新增后端接口**: 增加了 `wallet_crud` 相关的 HTTP Endpoint，支持通过网页端安全地进行钱包操作。
  - 支持 `import`: 通过私钥和名称导入本地加密钱包。
  - 支持 `create`: 新建随机钱包。
  - 支持 `rename`: 重命名现有钱包名称。
  - 支持 `set_default`: 设置默认使用的钱包。
  - 支持 `delete`: 删除钱包记录。
- **Miniapp & WebApp UI 重构**: 重新设计 `WalletManagePage.jsx` 和 `WalletManagePanel.jsx`。
  - 从纯展示模式改成了交互式表单。
  - 支持点击按钮展开对应表单（导入、重命名、新建）。
  - 提供视觉统一的操作按钮面板。

### 3. Uniswap 风格单币兑换功能重构 ✅
- **新增后端接口**: 增加了 `wallet_swap_single` 的 HTTP Endpoint。
  - 完全复用后端的 OKX Swap 接口能力（支持任意代币对）。
  - 自动读取代币精度，允许前端直接传递 float 格式的人类可读数量（例如：`1.5` BNB）。
  - 支持先获取包含预计滑点和数量的 `quote` 报价，以及最终执行的 `swap` 操作。
- **Miniapp & WebApp UI 重构**: 重新设计 `SwapPage.jsx` 和 `SwapPanel.jsx`。
  - 移除了旧版的自动全盘扫描垃圾代币界面。
  - 实现了类似 Uniswap 和聚合器 UI 的专业兑换面板。
  - **组件布局**：顶层钱包下拉及自定义配置（滑点）；主区域分为 `支付(From)` 和 `接收(To)` 行，内置代币合约地址输入框和代币数量输入区；带有反方向切换箭头。
  - 在用户改变代币/数量时，会自动防抖请求最新汇率进行询价展示。

## 涉及修改的模块
- **Backend API**: `wallet_crud_api.go` (新建), `wallet_swap_single_api.go` (新建), `liquidity_wallet_swap.go`, `compat_routes.go`
- **Miniapp**: `api.js`, `SwapPage.jsx`, `WalletManagePage.jsx`, `App.jsx`, `AssetManagementPage.jsx`
- **WebApp**: `api.js`, `SwapPanel.jsx`, `WalletManagePanel.jsx`, `App.jsx`, `AssetManagementPanel.jsx`, `utils.js`
