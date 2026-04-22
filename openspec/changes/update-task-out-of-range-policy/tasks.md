## 1. Implementation
- [x] 1.1 Backend：为任务增加可持久化的“区间已激活/首次进入区间时间”状态，覆盖新开仓与再平衡重开场景。
- [x] 1.2 Backend：重写越界处理逻辑，统一为“再平衡开启 => 任意方向缓冲后再平衡；再平衡关闭 => 任意方向缓冲后撤仓终止；暂停任务不自动处理”。
- [x] 1.3 Backend：单边池在首次进入区间前不启动越界倒计时；首次进入区间后切换为正常越界监控。
- [x] 1.4 Backend：保留 `stop_loss_enabled` / `stop_loss_delay_seconds` 兼容性，但 CLMM 越界逻辑不再依赖它们做执行分支。
- [x] 1.5 WebApp：开仓页移除“止损”作为越界执行模式入口，改为单一再平衡开关与新的越界说明文案。
- [x] 1.6 MiniApp：开仓页同步 WebApp 的越界执行心智与单边池说明。
- [x] 1.7 Tests：补充/更新后端单测，覆盖双方向越界、手动模式撤仓、暂停任务、单边池首次入区间后再处理等场景。
- [x] 1.8 Verification：运行 `cd backend; go test ./...`、`cd webapp; npm run build`、`cd miniapp; npm run build`。
