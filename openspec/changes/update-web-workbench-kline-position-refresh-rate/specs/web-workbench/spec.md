## ADDED Requirements

### Requirement: Web workbench K 线按仓位状态区分刷新速率
Web workbench SHALL allow separate automatic refresh intervals for the K 线 panel based on whether the currently displayed K 线 token matches the user's existing position tokens.

#### Scenario: 当前 K 线代币有对应仓位
- **GIVEN** 用户已登录并存在实时仓位数据
- **AND** 当前 K 线展示代币地址命中任一仓位的 token0 或 token1 地址
- **WHEN** K 线面板执行自动轮询
- **THEN** Web workbench MUST 使用“有对应仓位”的 K 线刷新速率

#### Scenario: 当前 K 线代币没有对应仓位
- **GIVEN** 用户已登录并打开 K 线面板
- **AND** 当前 K 线展示代币地址没有命中任一仓位的 token0 或 token1 地址
- **WHEN** K 线面板执行自动轮询
- **THEN** Web workbench MUST 使用“无对应仓位”的 K 线刷新速率

#### Scenario: 用户配置两档 K 线刷新速率
- **WHEN** 用户打开 Web Workbench 设置面板
- **THEN** 设置面板 MUST 展示 K 线“有对应仓位”和“无对应仓位”两个刷新速率配置项
- **AND** 用户修改后 MUST 持久化到当前浏览器
- **AND** 旧版单一 K 线刷新配置 MUST 被迁移为“有对应仓位”刷新速率
