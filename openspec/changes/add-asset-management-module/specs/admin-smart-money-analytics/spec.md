## ADDED Requirements

### Requirement: 双端管理员聪明钱资产分析视图
系统 SHALL 在 MiniApp 与 webapp 的资产管理模块中，为管理员提供“聪明钱资产”视图，用于查看钱包资产变化、LP 统计与排行榜。

该视图 MUST：
- 仅对管理员开放
- 展示聪明钱钱包总览与钱包详情
- 支持 `近1天`、`近7天`、`近30天` 维度切换
- 提供每日排行榜

#### Scenario: 管理员进入 MiniApp 聪明钱资产页
- **WHEN** 管理员打开 MiniApp 中“资产管理 > 聪明钱资产”页签
- **THEN** 系统展示总览卡片、钱包列表与默认排行榜

#### Scenario: 管理员进入 webapp 聪明钱资产模块
- **WHEN** 管理员打开 webapp 的资产管理模块并切换到“聪明钱资产”
- **THEN** 系统展示与 MiniApp 口径一致的总览、钱包详情与排行榜

#### Scenario: 非管理员访问聪明钱资产分析接口
- **WHEN** 非管理员请求聪明钱资产分析接口
- **THEN** 系统返回权限不足响应

### Requirement: 聪明钱钱包资产变化曲线
系统 SHALL 为每个聪明钱钱包提供资产变化曲线，并明确该曲线基于“可识别总资产”口径。

可识别总资产 MUST 至少包含：
- 原生币余额折算 USD
- 稳定币余额折算 USD
- 近 30 天参与 LP 的 token 余额折算 USD
- 当前 open LP 的估算持仓价值

#### Scenario: 查看聪明钱钱包详情
- **WHEN** 管理员打开某个聪明钱钱包详情
- **THEN** 系统展示该钱包的资产曲线、资产拆分与当前活跃池子信息

#### Scenario: 钱包存在无法识别的长尾资产
- **WHEN** 钱包中存在未纳入“可识别总资产”范围的资产
- **THEN** 系统保留曲线展示，并通过说明文案标记该曲线口径为“可识别总资产”

### Requirement: 聪明钱 LP 历史统计不包含当天
系统 SHALL 为每个聪明钱钱包提供 `近1天`、`近7天`、`近30天` 的 LP 历史统计，且历史统计 MUST 不包含当天数据。

统计项 MUST 至少包括：
- 估算已实现收益
- add 次数
- remove 次数
- 活跃池子数
- 未匹配事件数

#### Scenario: 管理员切换统计窗口
- **WHEN** 管理员从 `近7天` 切换到 `近30天`
- **THEN** 系统基于完整自然日重算该钱包的历史统计，不包含当天数据

#### Scenario: 钱包当天发生新的 add 或 remove
- **WHEN** 聪明钱钱包在当天新增 LP 活动
- **THEN** 这些活动只出现在“今日数据”区域，不并入历史窗口统计

### Requirement: 聪明钱收益排行
系统 SHALL 提供按自然日生成的聪明钱收益排行，默认展示“昨日已实现收益额榜”。

系统 SHOULD 同时支持：
- 收益额榜
- 收益率榜
- 参与次数榜

#### Scenario: 管理员查看昨日收益额榜
- **WHEN** 管理员打开默认排行榜
- **THEN** 系统按上一自然日的估算已实现收益额从高到低展示钱包排行

#### Scenario: 排行中存在未匹配收益事件
- **WHEN** 某些钱包的 remove 事件无法配对或缺少 USD 快照
- **THEN** 这些未匹配事件不计入收益额，并在排行榜或详情中显示未匹配提示

### Requirement: 聪明钱资产分析接口
系统 MUST 暴露管理员专用的聪明钱资产分析接口，并要求 Telegram WebApp `initData` 与管理员权限。

接口至少包括：
- `POST /api/admin/assets/smart_money_overview`
- `POST /api/admin/assets/smart_money_wallet`
- `POST /api/admin/assets/smart_money_leaderboard`

#### Scenario: 管理员请求钱包详情接口
- **WHEN** 管理员携带有效 `initData` 请求某个聪明钱钱包详情
- **THEN** 系统返回钱包曲线、历史统计、今日数据和警告信息

#### Scenario: 管理员请求排行榜接口
- **WHEN** 管理员请求昨日排行榜或指定窗口排行榜
- **THEN** 系统返回排行榜数据以及对应的统计口径说明
