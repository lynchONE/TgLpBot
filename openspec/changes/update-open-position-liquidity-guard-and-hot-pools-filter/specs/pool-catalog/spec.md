## ADDED Requirements

### Requirement: 热门池子响应必须过滤超低流动性池子
系统 MUST 在热门池子接口响应中排除当前流动性美元值 `< 100` 的池子，这些池子不得返回给前端展示。

#### Scenario: 默认热门池子榜单过滤低流动性池子
- **WHEN** 客户端请求默认热门池子列表
- **AND** 某些池子的当前流动性美元值 `< 100`
- **THEN** 这些池子 MUST NOT 出现在响应 `data` 中

#### Scenario: 带筛选条件的热门池子请求仍然过滤低流动性池子
- **WHEN** 客户端请求带 `token_address`、`dex` 或 `include_pools` 条件的热门池子列表
- **AND** 某个命中的池子当前流动性美元值 `< 100`
- **THEN** 该池子 MUST NOT 出现在响应 `data` 中
- **AND** 服务端 MUST 统一执行过滤，而不是依赖前端自行隐藏
