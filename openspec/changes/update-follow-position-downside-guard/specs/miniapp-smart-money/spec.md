## ADDED Requirements
### Requirement: 跟单仓位必须展示真实自动处理规则
系统 MUST 在 WebApp 与 MiniApp 的仓位卡片中，对自动跟单创建的仓位展示跟单专用策略说明，说明内容 MUST 包含目标钱包撤仓跟随、下破区间保底撤出、上破区间继续跟随目标钱包。

#### Scenario: 用户查看跟单仓位
- **WHEN** 用户在 WebApp 或 MiniApp 查看 `is_follow=true` 的仓位卡片
- **THEN** 系统 MUST 展示该仓位是自动跟单仓位
- **AND** MUST 展示“目标撤仓跟随”
- **AND** MUST 展示“下破保底撤出”
- **AND** MUST 展示“上破继续跟随”

#### Scenario: 用户查看普通仓位
- **WHEN** 用户在 WebApp 或 MiniApp 查看非跟单仓位卡片
- **THEN** 系统 MUST 继续展示普通任务区间策略按钮或标签
- **AND** MUST NOT 将普通仓位标记为跟单保底策略
