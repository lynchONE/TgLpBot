## ADDED Requirements
### Requirement: MiniApp 大型模块必须按职责拆分
MiniApp 中新增或重构后的核心功能模块 SHALL 将通用纯函数、业务状态管理、API 调用封装和展示组件分离到按职责命名的文件中，避免继续把多个业务域耦合在单个页面组件文件里。

#### Scenario: 拆分 App 级业务逻辑
- **WHEN** 开发者修改 MiniApp 顶层应用壳、热门池或开仓功能
- **THEN** 与 UI 无关的解析、筛选、权限和数学计算逻辑 SHALL 位于 `lib` 或 `features` 下的独立模块中
- **AND** `App.jsx` SHALL 主要保留应用壳、路由分发和跨模块协调逻辑

#### Scenario: 拆分 Smart Money 页面
- **WHEN** 开发者修改 Smart Money 的池子、钱包、设置、自动跟单或通知子页
- **THEN** 对应页面 SHALL 优先放在 `features/smartMoney` 或等价子目录中
- **AND** 不应继续把新子页追加到单个超大 `SmartMoneyPage.jsx` 文件中

### Requirement: MiniApp 重构必须保持行为兼容
MiniApp 模块边界重构 SHALL 保持现有用户可见行为、后端 API 调用契约、本地存储 key 和权限判断口径不变。

#### Scenario: 重构后构建验证
- **WHEN** 完成一批 MiniApp 文件拆分
- **THEN** `npm run build` MUST 成功
- **AND** 开发者 MUST 检查 diff，确认 import、调用签名和 API payload 没有遗漏更新
