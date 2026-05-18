## ADDED Requirements

### Requirement: 自动跟单表单支持选择执行钱包
MiniApp 和 WebApp SHALL 在自动跟单配置表单中展示用户可用钱包，并允许用户选择本次自动跟单用于真实开仓与撤仓的执行钱包。

#### Scenario: 用户新增自动跟单配置
- **WHEN** 用户打开自动跟单配置表单
- **THEN** 表单 MUST 展示执行钱包选择控件
- **AND** 保存配置时 MUST 把选中的执行钱包 ID 发送给后端

#### Scenario: 配置列表展示执行钱包
- **WHEN** 用户查看已保存的自动跟单配置
- **THEN** 配置卡片 MUST 显示该配置的执行钱包名称或地址
- **AND** 最近任务 MUST 能展示任务使用的执行钱包

#### Scenario: 钱包列表加载失败
- **WHEN** 前端无法加载用户钱包列表
- **THEN** 用户 MUST NOT 能在缺少执行钱包信息的情况下新增或修改自动跟单配置
- **AND** 页面 MUST 显示加载失败原因
