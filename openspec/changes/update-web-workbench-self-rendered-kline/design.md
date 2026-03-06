## 背景
`webapp/src/App.jsx` 当前通过外部 `iframe` 渲染 K 线面板，并把 GMGN 作为外跳入口。这种方式能快速上线，但对于桌面版 Workbench 会带来三个产品问题：

1. 图表体验受第三方控制，而不是由 Workbench 自己掌控。
2. Workbench 无法在图表上叠加一方钱包活动标记。
3. 一旦上游站点调整嵌入策略、风控策略或布局，K 线模块会变得脆弱。

同时，仓库里已经具备部分原生图表基础：
- `webapp/src/components/KlineChart.jsx` 已经用 `lightweight-charts` 渲染蜡烛图和成交量。
- 项目已经有 OKX API 凭证配置，并在 `backend/service/exchange/okx_dex.go` 中封装了 OKX 鉴权请求。
- 聪明钱数据已存在于 ClickHouse 的 `smart_lp_events` 中，监控钱包标签存储在 MySQL。

当前 `backend/service/web_server/pool_ohlcv.go` 仍通过 GeckoTerminal 拉取 K 线数据，但你明确要求 WebApp 端 K 线优先使用 OKX 数据，而且项目现有 OKX 接入条件更成熟。因此本次设计以 OKX Market API 为主数据源。

本次变更的目标，就是把这些现有能力真正组装成 Web Workbench 的主 K 线方案。

## 目标 / 非目标

### 目标
- 在 `webapp/` 内完整渲染选中池子的 K 线。
- K 线数据优先使用 OKX `GET /api/v6/dex/market/candles`。
- 保持现有跨模块联动：在 Hot Pools 或 Smart Money 中选中池子后，K 线面板同步更新。
- 在 K 线上展示可点击的监控钱包 / 聪明钱活动标记。
- 在高密度标记场景下，通过聚合而不是简单堆叠来保证可读性。
- 保留 GMGN 外跳按钮，满足用户查看 GMGN 特有 KOL、活动或交易者页面的需求。

### 非目标
- 不尝试绕过 GMGN 的 `iframe` 限制、代理 GMGN 页面或镜像其私有逻辑。
- 不追求 1:1 复刻 GMGN 的私有 KOL 评分或活动分类体系。
- 本次不新增 swap 交易流采集；第一版仅叠加项目已有的 SmartLP 池子活动。
- 第一版不引入新的实时流式后端，轮询即可满足需求。
- 不再继续把 GeckoTerminal 作为 WebApp 主 K 线来源；若未来保留，也只应作为应急 fallback，而不是主方案。

## 总体设计

### 1. Web Workbench K 线面板
保留 `gmgn_kline` 这个 widget key，但面板本身改成一方图表模块：

- 头部：
  - 当前交易对 / 池子标题
  - 当前展示代币（默认推导，可切换）
  - 周期切换器（`1m`、`5m`、`15m`、`1h`）
  - 刷新按钮
  - 聪明钱覆盖层开关
  - GMGN 外跳按钮
- 主体：
  - 蜡烛图序列
  - 成交量柱状图
  - 用于聚合标记的 DOM 覆盖层
- 侧边抽屉 / 弹层：
  - 当前标记聚合的详细活动列表

### 2. 数据来源
- K 线：新增 `GET /api/token_candles`，由后端代理 OKX `GET /api/v6/dex/market/candles`
- 历史回补（可选增强）：OKX `GET /api/v6/dex/market/historical-candles`
- 标记事件：新增 `GET /api/smart_money_pool_markers`
- 标签补充：优先复用 MySQL 中的监控钱包标签，必要时叠加 ClickHouse 中的扫描 / watchlist 元信息

### 3. 刷新策略
- OKX K 线轮询：当 K 线组件可见且已选中池子时，每 `20-30s` 刷新一次
- 标记轮询：当覆盖层开启、组件可见且允许聪明钱数据时，每 `10-15s` 刷新一次
- 所有过期请求通过 `AbortController` 取消

这样能保持实现简单，也避免第一版就引入 WebSocket 依赖。

## 前端设计

### 状态模型
在 `webapp/src/App.jsx` 中新增或派生以下状态：
- `klineTokenAddress`
- `klineTokenSide`
- `klineInterval`
- `klineCandles`
- `klineLoading`
- `klineError`
- `klineOverlayEnabled`
- `klineMarkers`
- `klineMarkersLoading`
- `klineMarkersError`
- `selectedMarkerCluster`

以下条件变化时需要触发图表刷新：
- 选中的池子变化
- 链变化
- 展示代币变化
- 选择的周期变化
- 覆盖层开关变化
- 手动刷新

### 展示代币选择规则
由于 OKX candles 接口的核心参数是 `tokenContractAddress`，而当前 Workbench 的主选择对象是 pool，因此前端需要先把 pool 映射成“要展示的代币”：

1. 如果池子是一边稳定币、一边非稳定币，则默认展示非稳定币。
2. 如果池子是双非稳定币，则在 K 线面板头部提供 `token0 / token1` 切换按钮。
3. 如果无法识别稳定币角色，则优先使用 `token0`，并允许用户手动切换。

这样可以保持“从池子进入 K 线”的交互习惯，同时满足 OKX token 维度接口的输入要求。

### KlineChart 组件接口
扩展 `webapp/src/components/KlineChart.jsx`，让其接收：
- `candles`
- `markers`
- `intervalSec`
- `loading`
- `error`
- `onMarkerClick`

组件本身只负责：
- 创建图表与处理尺寸变化
- 绘制蜡烛图和成交量
- 将 candle 行转换为 `lightweight-charts` 所需的数据点
- 把标记聚合投影到屏幕坐标

组件 **不直接拉取数据**，数据获取仍由 `App.jsx` 负责。

### Marker 渲染策略
`lightweight-charts` 自带 marker 适合简单箭头或图标，但不适合头像式、多钱包聚合标记。因此设计上使用双层结构：

1. **Series layer**
   - 仅负责蜡烛图和成交量
2. **Absolute overlay layer**
   - 以绝对定位 DOM 节点方式渲染在图表画布之上
   - 在以下场景中更新：
     - candles 变化
     - 可视时间范围变化
     - 图表尺寸变化
     - marker 数据变化

### Marker 摆放规则
- `add` 活动锚定在 candle 高点上方
- `remove` 活动锚定在 candle 低点下方
- 同一根 candle 上的多个事件会聚合成一个 cluster
- cluster 上展示数量，例如 `3`
- 如果钱包存在自定义标签，cluster 预览优先展示标签缩写
- 若该 candle 只有一条事件，则显示单钱包徽标而不是数量气泡

### Marker 详情抽屉
点击 marker 后，打开一个详情抽屉或面板，展示该聚合内的事件列表：
- 钱包标签 / 缩短地址
- 动作（`add` / `remove`）
- 事件时间
- 估算 USD 金额
- 价格区间（`price_lower`、`price_upper`），若可用
- 交易哈希跳转

该抽屉仅针对当前池子和当前选中的 marker cluster。

需要注意：K 线是 token 维度，而 marker 是 pool 维度。第一版允许这种“token 主图 + pool overlay”的混合展示，因为用户当前就是从特定池子上下文进入 K 线面板，聪明钱活动也应以该池子为准。

## 后端设计

### 新接口
新增：

`GET /api/token_candles`

`GET /api/smart_money_pool_markers`

### 鉴权规则
- `token_candles`：
  - 要求有效 `initData`
  - 要求通过正常 web / miniapp 访问校验
- `smart_money_pool_markers`：
  - 要求有效 `initData`
  - 要求通过正常 web / miniapp 访问校验
- 覆盖层数据要求具备聪明钱权限

如果调用方没有聪明钱权限，则接口返回 `403`。前端应优雅降级：保留 K 线，仅禁用覆盖层。

### 请求参数
- `token_candles`
  - `initData`
  - `chain`
  - `token_address`
  - `bar`
  - `limit`
  - `before`
  - `after`
- `initData`
- `chain`
- `pool_version`
- `pool_id`
- `bucket_sec`
- `window_hours`
- `limit`

建议默认值：
- `token_candles`
  - `bar=1m`
  - `limit=240`
- `smart_money_pool_markers`
  - `bucket_sec=300`
  - `window_hours=12`
  - `limit=300`

建议上限：
- `token_candles`
  - `limit <= 299`（遵循 OKX candles 单次返回上限）
- `smart_money_pool_markers`
  - `bucket_sec >= 60`
  - `window_hours <= 48`
  - `limit <= 500`

### 返回结构
```json
{
  "chain": "bsc",
  "token_address": "0x...",
  "bar": "1m",
  "limit": 240,
  "updated_at": "2026-03-06T00:00:00Z",
  "candles": [
    {
      "t": 1772755200,
      "o": 0.0012,
      "h": 0.0013,
      "l": 0.0011,
      "c": 0.00125,
      "v": 123456.78,
      "v_usd": 45678.9,
      "confirm": true
    }
  ],
  "source": "okx"
}
```

`smart_money_pool_markers` 示例：

```json
{
  "chain": "bsc",
  "pool_version": "v3",
  "pool_id": "0x...",
  "bucket_sec": 300,
  "window_sec": 43200,
  "updated_at": "2026-03-06T00:00:00Z",
  "events": [
    {
      "event_id": "0xhash:12",
      "t": 1772755200,
      "bucket_t": 1772755200,
      "wallet_address": "0x...",
      "wallet_label": "Whale 01",
      "wallet_source": "user_managed",
      "action": "add",
      "tx_hash": "0x...",
      "tick_lower": -12345,
      "tick_upper": -12000,
      "price_lower": 0.00123,
      "price_upper": 0.00141,
      "anchor_price": 0.00132,
      "amount0": 123.45,
      "amount1": 456.78,
      "estimated_usd": 987.65
    }
  ],
  "warnings": []
}
```

### 查询策略
`token_candles`：
- 由后端代理 OKX `GET /api/v6/dex/market/candles`
- 使用现有 OKX API key / secret / passphrase 配置
- `chainIndex` 直接复用当前链配置中的 EVM `ChainID`
- `tokenContractAddress` 使用小写地址
- 最新 K 线请求走 `/candles`
- 若后续需要更深历史，可接 `/historical-candles`

`smart_money_pool_markers`：
- 从 `smart_lp_events` 读取池子维度的钱包活动数据
- 动作范围：`add`、`remove`
- 池子过滤：精确匹配 `pool_version + pool_id`
- 时间范围：`now() - window_hours`
- 排序：按最新事件优先
- 返回条数：严格限流

marker 接口后端还需要补充以下信息：
- 若 MySQL 中存在监控钱包标签，则返回 `wallet_label`
- 通过现有池子 / 代币定价逻辑估算 USD 金额
- 若能从 ticks 推导，则返回价格区间或中间锚点价格

### 事件归一化
为了简化前端逻辑：
- `token_candles` 后端负责把 OKX 返回的毫秒级 `ts` 归一化成秒级时间戳，并把字符串价格 / 成交量转为数值。
- `token_candles` 后端可保留 `confirm` 字段，用于前端识别未完结 K 线。
- 后端直接返回与请求周期对齐后的 `bucket_t`
- 后端计算 best-effort 的 `anchor_price`
- 后端使用稳定的 `event_id = tx_hash:log_index` 或等价唯一键

前端仍负责 cluster 聚合，以便根据图表当前可视范围实时调整展示，而无需额外发起服务端请求。

## 性能与缓存

### 后端
- 使用请求级超时（`3-5s`）
- 对 OKX 与 marker 返回条数都进行强限制
- marker 接口避免做高开销手续费模拟
- 尽量复用聪明钱池子详情接口已有的定价辅助逻辑
- 尽量复用现有 `backend/service/exchange/okx_dex.go` 的签名逻辑；若需要，可提取一个通用的 OKX 请求帮助方法，避免重复实现鉴权。

第一版不引入 Redis 或持久缓存，优先保持实现简单。

### 前端
- 保留一个小型内存缓存，key 由以下维度组成：
  - `chain`
  - `token_address`
  - `interval`
- 刷新期间复用最近一次成功结果，避免图表闪烁
- 必要时对周期切换和池子切换做轻量防抖

## 异常与降级

### OHLCV 不可用
- 图表面板展示错误态或空态
- 其他 Workbench 模块不受影响

### OKX K 线不可用
- 若 OKX 返回错误或限流，K 线面板展示非阻塞错误态
- 第一版不自动切回 GeckoTerminal，避免再次把 Gecko 作为主链路依赖

### Marker API 不可用
- K 线继续正常渲染
- 覆盖层开关禁用或提示非阻塞性告警

### 无聪明钱权限
- marker 接口返回 `403`
- 前端隐藏覆盖层，但保留 GMGN 外跳入口

### 当前窗口无事件
- 不展示任何 marker
- 图表本身继续正常工作

## 实施阶段

### 阶段 1
- 在 `webapp/src/App.jsx` 中接入 `token_candles`
- 用 `KlineChart` 替换 `iframe`
- 实现池子到展示代币的推导逻辑
- 保留 GMGN 外跳按钮

### 阶段 2
- 新增 `GET /api/token_candles`
- 新增 `GET /api/smart_money_pool_markers`
- 实现 marker 聚合覆盖层与事件详情抽屉

### 阶段 3
- 打磨周期切换、加载态与无权限降级体验
- 运行 `webapp` 构建并做 Workbench 手工验证
