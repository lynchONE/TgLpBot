# Change: 提升开仓撤仓执行速度并前置开仓池子预检测

## Why
- 当前 BSC 链路的实际瓶颈不只在链上确认，还包括开仓/撤仓完成后的固定等待、保守的 RPC 同步轮询，以及同步写交易记录/交易历史带来的尾部耗时。
- 当前开仓界面只有在金额、区间和滑点都填写完成后才会触发预览，请求过晚，用户打开界面时看不到池子基础风险结果，也会放大后续等待感。
- 用户希望把非关键记录改为异步补写，并在打开开仓界面时就先做池子检测，这属于跨后端执行链路和前端交互时机的行为调整，需要先通过提案明确范围。

## What Changes
- 新增开仓预检测阶段，在开仓界面打开后立即触发与池子和钱包相关的基础检测，不再依赖金额和滑点先填写完整。
- 为开仓流程补充可复用的 prepare/precheck 上下文，让后续金额/滑点预览和最终执行尽量复用已获取的池子元数据与风控结果，减少重复 RPC。
- 将交易记录、交易历史、非关键通知与补账逻辑移出开仓/撤仓关键路径，改为后台异步补写和重试，不阻塞链上成功后的接口返回。
- 将开仓和撤仓中的固定 `sleep` 与长等待改为“短轮询快路径 + 保守回退”模式，针对 BSC 默认采用更激进的同步窗口。
- 将撤仓失败后的重试节奏改为梯度退避，按 `500ms -> 1s -> 2s -> 3s -> 5s -> 10s -> 30s` 递增，避免一开始等待过久，也避免高频重试把 RPC 压满。

## Impact
- Affected specs:
  - `position-execution-performance`
  - `open-position-precheck`
- Affected code:
  - Backend:
    - `backend/service/web_server/open_position.go`
    - `backend/service/liquidity/liquidity_enter.go`
    - `backend/service/liquidity/liquidity_exit.go`
    - `backend/service/liquidity/liquidity_dust.go`
    - `backend/service/trade/trade_record.go`
    - `backend/base/config/config.go`
  - WebApp:
    - `webapp/src/components/OpenPositionModal.jsx`
    - `webapp/src/App.jsx`
    - `webapp/src/api.js`
  - MiniApp:
    - `miniapp/src/App.jsx`
    - `miniapp/src/lib/api.js`
    - `miniapp/src/components/StepProgressModal.jsx`
- Backwards compatibility:
  - 旧的 `open_position_preview` 和 `open_position` 调用方式保持可用。
  - 新增的预检测能力优先供新版 WebApp 和 MiniApp 使用；旧客户端即使不接入 prepare，也必须继续按现有接口完成开仓。
