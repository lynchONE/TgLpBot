## 1. Implementation
- [x] 1.1 在 liquidity 退出参数中增加可选 `ExitPercent`，统一校验范围为 `(0, 100]`，未传时按 100%。
- [x] 1.2 V3 退出时按链上当前 liquidity 计算本次 `decreaseLiquidity` 数量；100% 沿用全撤，部分撤仓后不将任务标记为已停止。
- [x] 1.3 V4 退出时按链上当前 liquidity 计算本次 `DECREASE_LIQUIDITY` 数量；100% 沿用全撤，部分撤仓后不将任务标记为已停止。
- [x] 1.4 部分撤仓成功后刷新任务 `current_liquidity` 为链上剩余值，并清理与本次撤仓相关的临时错误状态；全撤仍沿用现有状态流转。
- [x] 1.5 Web/MiniApp 停止任务 API 接收可选 `exit_percent` / `exitPercent`，默认 100%，并在前端提供百分比输入或快捷比例。
- [x] 1.6 Telegram 文字版任务卡增加“部分撤仓”入口，支持用户输入百分比并提交后台执行。
- [x] 1.7 针对百分比校验、liquidity 计算、API 默认 100% 兼容性补充单元测试或集成测试。

## 2. Validation
- [x] 2.1 执行 `gofmt`。
- [x] 2.2 执行 `cd backend; go test ./...`。
- [x] 2.3 执行 `cd miniapp; npm run build` 和 `cd webapp; npm run build`。
- [x] 2.4 做针对性 diff 检查，确认默认停止任务仍为 100% 全撤，且所有调用方参数匹配。
