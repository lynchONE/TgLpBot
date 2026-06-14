## ADDED Requirements

### Requirement: 池子雷达候选钱包流式预览
系统 SHALL 提供池子雷达候选钱包的流式预览能力。用户发起扫描后，后端 MUST 在发现符合条件的钱包时立即推送候选事件，前端 MUST 在不中断扫描的情况下立即展示或更新该钱包。

#### Scenario: 后端发现一个符合条件的钱包
- **WHEN** 用户提交 V3 池子地址或 V4 poolId、最低 USD 金额、时间范围和数量限制后发起流式扫描
- **THEN** 后端 MUST 开始通过 SSE 返回扫描事件
- **AND** 后端 MUST 在每个钱包通过 owner 归因、金额计算和阈值过滤后推送 `candidate` 事件
- **AND** 前端 MUST 在收到 `candidate` 事件后立即把该钱包插入候选列表

#### Scenario: 同一钱包后续命中更高金额或更新交易
- **WHEN** 流式扫描中同一钱包再次出现符合条件的加池事件
- **THEN** 后端 SHOULD 推送同一钱包的更新候选事件
- **AND** 前端 MUST 按钱包地址 upsert 候选行，而不是展示重复钱包
- **AND** 候选行 MUST 保留最高加池金额、最近交易哈希、交易时间、池子标识、交易对和协议字段

#### Scenario: 扫描阶段和执行细节可见
- **WHEN** 流式扫描正在执行
- **THEN** 后端 MUST 推送 `stage` 或 `warning` 事件展示关键阶段、排除数量、warning 或 partial 信息
- **AND** 前端 MUST 在扫描日志中展示这些事件
- **AND** 前端 MUST 展示运行中状态和耗时

#### Scenario: 流式扫描完成
- **WHEN** 后端完成目标时间范围扫描或达到用户请求的候选数量限制
- **THEN** 后端 MUST 推送 `summary` 事件
- **AND** 后端 MUST 推送 `done` 事件关闭流
- **AND** 前端 MUST 将扫描状态更新为完成或部分完成，并保留已经展示的候选钱包

#### Scenario: 流式扫描失败或被取消
- **WHEN** RPC、价格服务或 owner 归因在没有任何候选可返回前失败，或用户主动停止扫描
- **THEN** 后端 MUST 推送 `error` 或因请求取消停止扫描
- **AND** 前端 MUST 展示失败或已停止状态
- **AND** 系统 MUST NOT 用空列表或默认值伪装成功扫描

### Requirement: 流式预览代理必须透传事件
WebApp 和 MiniApp 的 API proxy SHALL 支持池子雷达 SSE endpoint 的流式透传。代理 MUST NOT 等待完整上游响应结束后才返回给浏览器。

#### Scenario: 通过代理使用流式扫描
- **WHEN** 前端通过同源 `/api/sm?endpoint=pool_liquidity_wallet_candidates_stream` 请求流式扫描
- **THEN** 代理 MUST 把上游响应的事件流逐块转发给浏览器
- **AND** 代理 MUST 保留 `text/event-stream` 响应类型
- **AND** 前端 MUST 能在扫描未结束前收到并展示至少一个 `candidate` 事件
