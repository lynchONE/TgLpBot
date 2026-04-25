## ADDED Requirements

### Requirement: MiniApp 聪明钱池子视图如存在池子列表必须支持一致筛选
如果 MiniApp 提供与 Web Workbench 等价的聪明钱池子列表视图，系统 SHALL 提供同等的池子级筛选能力，支持按当前聪明钱仓位金额和池子费率过滤。

#### Scenario: MiniApp 用户筛选聪明钱池子
- **WHEN** MiniApp 聪明钱池子列表展示当前聪明钱池子数据
- **THEN** 用户可以设置最低聪明钱仓位金额和最高池子费率
- **AND** 列表仅展示同时满足筛选条件的池子

#### Scenario: MiniApp 不存在等价池子列表
- **WHEN** MiniApp 当前版本没有独立的聪明钱池子视图
- **THEN** 本变更不要求新增一个全新 MiniApp 视图
- **AND** 不影响 Web Workbench 的筛选能力交付
