## ADDED Requirements

### Requirement: 首页顶部 Alpha 信息条
webapp 首页 SHALL 在顶部栏居中区域展示 Alpha 信息条，用于汇总今日空投与稳定度看板摘要。

#### Scenario: 展示今日空投
- **GIVEN** Alpha 主接口返回 `airdrops`
- **WHEN** 用户打开 webapp 首页
- **THEN** 顶部信息条 MUST 展示今日空投项目的 `token`、`name`、`amount`、`points`
- **AND** `date` 与 `time` MUST 拼接为同一个时间字段展示

#### Scenario: 展示稳定度摘要
- **GIVEN** 稳定度接口返回 `items`
- **WHEN** 用户打开 webapp 首页
- **THEN** 顶部信息条 MUST 展示稳定度摘要
- **AND** 空间有限时 MUST 优先展示异常或不稳定项目，而不是完整列表

#### Scenario: 外部接口异常
- **GIVEN** 任一 Alpha 外部接口请求失败或返回结构不可用
- **WHEN** 用户打开 webapp 首页
- **THEN** 顶部信息条 MUST 显示明确的加载失败或暂无数据状态
- **AND** 首页其它模块 MUST 继续正常渲染

#### Scenario: 顶部空间受限
- **GIVEN** 用户在窄屏或移动端打开 webapp
- **WHEN** 顶部空间不足以展示完整 Alpha 文案
- **THEN** 信息条 MUST 使用紧凑布局、截断或换行
- **AND** 登录入口和工作台主布局 MUST 不被遮挡
