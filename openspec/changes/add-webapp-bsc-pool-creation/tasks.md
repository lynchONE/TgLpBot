## 1. 后端交易能力
- [x] 1.1 新增 BSC 建池 `preview` / `execute` API，并与 `open_position` / `StrategyTask` 解耦
- [x] 1.2 补齐 `Uniswap V3` / `PancakeSwap V3` PositionManager 的 `createAndInitializePoolIfNecessary`、`mint` 绑定与参数封装
- [x] 1.3 补齐 `Uniswap V4` 的初始化、多调用与首仓动作封装，仅支持 `zero hooks + static fee`
- [x] 1.4 复用现有钱包串行执行器、ERC20 授权能力与交易回执处理，统一返回建池结果对象

## 2. WebApp 模块
- [x] 2.1 新增独立的「创建池子」模块、表单状态与预览摘要卡片
- [x] 2.2 在建池模块中接入「热门池子」「聪明钱」基础数据来源选择器
- [x] 2.3 实现最小化表单、折叠高级选项、执行中状态与成功结果卡

## 3. 验证
- [ ] 3.1 为后端补充参数推导、池子已存在校验、V4 限制校验的测试
- [ ] 3.2 完成 WebApp 构建验证，并在 BSC 环境手工验证 `Uniswap V3`、`Uniswap V4`、`PancakeSwap V3` 的预填与提交流程
