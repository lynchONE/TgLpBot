# 变更：扩展金狗通知为双模式监控并增加 Bark 强度控制

## Why
当前 Smart Money 的金狗通知只有一种触发方式：按交易对统计窗口内的聪明钱 LP 聚集度。这个模式可以抓到“多钱包同时上车”的信号，但抓不到另一类同样重要的机会：池子参数本身已经非常突出，比如手续费、交易笔数、TVL、成交量或费率已经满足关注条件。

现有 WebApp / MiniApp 的金狗通知页面也比较松散，信息密度不够，缺少“通知强度”与“测试发送”入口；用户很难在配置页里快速确认当前 Bark 会怎么响、会不会真的推到手机上。

同时，现有聚集模式只看钱包/仓位数量，缺少金额门槛。多个小额 LP 行为可能达到钱包数阈值但实际资金力度不足，容易产生低价值推送；用户需要按聪明钱 LP 金额配置阈值，并在金额越大时自动提高推送强度。

## What Changes
- 将 Smart Money 的金狗通知扩展为一个“双模式告警中心”，同时支持：
  - 聪明钱聚集模式：保留现有“按交易对聚集聪明钱 LP 活跃度”的模式
  - 池子参数模式：基于 PoolM 已落库到 MySQL `pools` 表的池子快照字段做阈值筛选
- WebApp 与 MiniApp 的金狗通知 UI 都改成更紧凑的两段式卡片布局，减少空白区域，并把摘要、开关、强度、测试按钮收拢到同一屏。
- 为两种模式分别增加 Bark 通知强度配置，支持：
  - 响铃
  - 持续响铃
  - 静音模式下响铃
- 为聪明钱聚集模式增加金额阈值配置：
  - 统计窗口内同交易对的有效 LP 金额合计达到阈值后才推送
  - 金额阈值为空或为 0 时保持仅按钱包/仓位数量触发的旧行为
  - 通知正文展示命中的合计金额，避免用户只看到钱包数量
- 支持按金额阶梯自动选择 Bark 强度：
  - 用户可以继续使用固定强度
  - 也可以配置金额阶梯，例如达到低档金额普通响铃、达到中档金额持续响铃、达到高档金额静音强提醒
  - 当同一信号命中多个阶梯时，使用金额最高的一档
- 新增测试通知能力，允许用户在保存前或保存后主动发送一条 Bark 测试消息，验证当前 Bark 配置与强度是否生效。
- 后端扩展金狗通知配置模型、配置 API 和扫描逻辑：
  - 保留原有模式
  - 新增 PoolM 池子参数模式
  - 在扫描与测试发送时统一应用 Bark 强度参数

## Impact
- Affected specs:
  - `smart-money-alerting`
- Affected code:
  - `backend/base/models/smart_money_golden_dog.go`
  - `backend/base/notify/bark.go`
  - `backend/service/smart_money_golden_dog/*`
  - `backend/service/web_server/smart_money_golden_dog_config.go`
  - `webapp/src/smartMoneyApi.js`
  - `miniapp/src/lib/smartMoneyApi.js`
  - `webapp/src/components/SmartMoneyDashboard.jsx`
  - `miniapp/src/components/SmartMoneyPage.jsx`
- Data / migration:
  - 需要扩展 `smart_money_golden_dog_configs`
  - 需要为聪明钱聚集模式新增金额阈值与金额阶梯强度配置字段
  - 继续复用 `smart_money_golden_dog_alert_states`，但去重键需要支持按模式区分
