## 1. Backend Data Model
- [x] 1.1 新增 `PoolDataSource` 模型，包含来源类型、base URL、请求模板、链/窗口/协议/DEX、current/enabled 与健康状态字段。
- [x] 1.2 将 `PoolDataSource` 加入 MySQL AutoMigrate，并保证同一链/窗口只有一个当前来源。
- [x] 1.3 增加数据源 store/manager，支持 list/add/update/switch/enable/disable/delete/check。

## 2. Pool Sync Runtime
- [x] 2.1 将 `pool_sync.Service` 从固定 `PoolMClient` 改为通过数据源 manager 选择当前来源。
- [x] 2.2 实现 `poolm_top_fees` 适配器，保持现有 PoolM 请求和响应解析兼容。
- [x] 2.3 实现 `market_pools` 适配器，支持 `/api/market/pools` 参数模板和 camelCase 响应归一化。
- [x] 2.4 兼容 v4 `poolAddress=null` 时用 `poolId` 作为池子主标识，并保留 `poolManager` 上下文。
- [x] 2.5 同步结果写入 `pools` 时记录实际数据源信息和原始 payload。
- [x] 2.6 当前来源失败时记录健康状态；按配置决定是否尝试 enabled 备用来源。

## 3. Admin API
- [x] 3.1 新增 `POST /api/admin/pool_data_sources` 管理接口。
- [x] 3.2 扩展 `/api/admin?endpoint=pool_data_sources` 兼容路由。
- [x] 3.3 扩展 MiniApp Vercel admin proxy allowlist。
- [x] 3.4 接口统一校验 Telegram WebApp `initData` 与管理员权限。

## 4. Admin UI
- [x] 4.1 MiniApp 管理员页新增“池子源”页签。
- [x] 4.2 实现来源列表、当前来源展示、健康状态、最后错误展示。
- [x] 4.3 实现新增、切换、启用/禁用、删除、连通性检查操作。
- [x] 4.4 如 webapp 管理页仍在使用，同步补齐池子源管理入口。

## 5. Tests
- [x] 5.1 增加数据源 manager 单元测试，覆盖 current 切换、禁用来源不可切换、空 DB 回退 env。
- [x] 5.2 增加 PoolM 与 market/pools 响应归一化测试，覆盖 snake_case、camelCase、v4 poolId fallback。
- [x] 5.3 增加 admin handler 请求校验测试，覆盖非法 action、非管理员、缺 initData。
- [x] 5.4 运行 `cd backend; go test ./...`。
