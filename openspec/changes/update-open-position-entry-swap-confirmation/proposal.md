# Change: 开仓前置兑换增加推荐滑点预览与确认

## Why
- 当前 `open_position` 在未显式传入 `slippage_tolerance` 时，会直接回落到用户全局滑点；对于 `USDC/WBNB` 这类池子，实际前置入场路径通常是 `USDT -> USDC`，继续使用偏高的全局滑点会放大稳定币兑换被夹的风险。
- 当前页面在真实兑换发生前，无法先看到这笔 entry swap 的推荐滑点和预计到账数量，用户也无法针对这笔兑换单独确认；这不符合“先看报价、再确认兑换、最后继续开仓”的资金安全预期。
- 当前后端执行链路会在一次开仓请求内直接进入“前置兑换 + 后续开仓”，缺少对“需要前置兑换”的显式确认门槛。

## What Changes
- 明确开仓总流程顺序：
  - 第一步必须先执行统一开仓风险校验，包括池子价格与 OKX 价格偏差、链上/美元流动性等所有既有安全门槛。
  - 若任一风险校验不通过，系统必须直接拦截，不得继续后续 entry swap 预览、确认或真实开仓。
  - 只有在风险校验全部通过后，系统才允许继续判断是否需要前置 entry swap。
- 为开仓流程新增 entry swap 预览能力：
  - 后端提供开仓预览接口；该接口必须先复用统一开仓风险校验，再判断本次请求是否需要前置 entry swap。
  - 若需要前置 entry swap，接口返回兑换方向、预计输入数量、预计到账数量、推荐滑点和当前生效滑点。
- 调整开仓执行语义：
  - 执行接口必须再次先走统一开仓风险校验；风险未通过时，直接返回结构化风险信息。
  - 若本次开仓需要前置 entry swap，但请求未明确确认，则后端不得执行真实兑换或后续开仓。
  - 用户确认后，后端必须先完成前置 entry swap，兑换成功后才继续后续私有 Zap / 开仓流程。
- 调整滑点来源规则：
  - 稳定币到稳定币的 entry swap 推荐滑点必须由后端单独给出，不得默认直接沿用全局滑点。
  - 页面允许用户按本次兑换修改滑点；修改后必须重新刷新预估到账数量，再允许确认执行。
- MiniApp 与 WebApp 的开仓页面都要补齐同一套交互：
  - 先展示推荐滑点与预计到账数量。
  - 用户可修改本次兑换滑点。
  - 只有在用户明确确认后，才允许真实执行 entry swap 和后续开仓。

## Impact
- Affected specs:
  - `open-position-safety`
- Affected code:
  - Backend:
    - `backend/service/web_server/open_position.go`
    - `backend/service/web_server/compat_routes.go`
    - `backend/service/web_server/server.go`
    - `backend/service/liquidity/liquidity_enter.go`
    - `backend/service/liquidity/okx_swap.go`
  - MiniApp:
    - `miniapp/src/App.jsx`
    - `miniapp/src/lib/api.js`
    - `miniapp/src/components/StepProgressModal.jsx`
  - WebApp:
    - `webapp/src/api.js`
    - `webapp/src/App.jsx`
    - `webapp/src/components/OpenPositionModal.jsx`
- Backwards compatibility:
  - 原有 `open_position` 成功响应结构保持兼容。
  - `open_position` 在检测到“需要前置 entry swap 但未确认”时，将返回新的结构化确认提示，而不是直接执行。
