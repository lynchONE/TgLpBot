## ADDED Requirements
### Requirement: 热门池子应返回展示代币元数据
系统 SHALL 在热门池子接口返回中补充展示代币的地址、符号、名称和图标链接，以便前端直接渲染池子主题代币头像。

#### Scenario: 命中已有代币元数据
- **WHEN** 热门池子接口返回的池子能够解析出展示代币地址且系统缓存或数据库中已有该代币元数据
- **THEN** 响应中应包含该池子的 `display_token_address`
- **AND** 响应中应尽量包含 `display_token_symbol`、`display_token_name` 与 `display_token_logo_url`

#### Scenario: 代币元数据缺失时批量回源
- **WHEN** 热门池子接口中存在若干展示代币地址在缓存和数据库中都不存在
- **THEN** 系统应按链批量调用 OKX `token/basic-info` 接口补齐缺失元数据
- **AND** 回源成功后应将结果写入持久化存储与缓存

### Requirement: 代币元数据应避免重复回源查询
系统 SHALL 对代币元数据建立持久化与缓存层，避免热门池子接口每次请求都重复访问外部 OKX 元数据接口。

#### Scenario: Redis 命中直接返回
- **WHEN** 指定链和代币地址的元数据在 Redis 中存在且未过期
- **THEN** 系统应直接使用 Redis 中的结果
- **AND** 不应继续访问 MySQL 或 OKX

#### Scenario: OKX 未返回元数据时写入负缓存
- **WHEN** 某个代币地址经过 OKX 查询后仍没有可用元数据
- **THEN** 系统应写入短期负缓存或持久化缺失状态
- **AND** 在负缓存有效期内不应重复请求同一代币地址

### Requirement: 热门池子前端应优先展示主题代币图标
系统 SHALL 在 WebApp 热门池子列表中优先展示主题代币图标，并将交易所图标缩小后与协议版本组合展示。

#### Scenario: 存在代币图标
- **WHEN** 热门池子数据包含 `display_token_logo_url`
- **THEN** 前端池子头像位置应展示该代币图标
- **AND** 协议版本标签旁应继续展示缩小后的 DEX 图标

#### Scenario: 缺少代币图标时回退
- **WHEN** 热门池子数据缺少 `display_token_logo_url` 或图标加载失败
- **THEN** 前端应回退到交易所图标或首字母占位
- **AND** 不应影响池子列表的其他信息展示
