## 1. Backend
- [x] 1.1 在 smart money repository 中新增池子收益火焰图聚合查询，按活跃仓位 `opened_at -> now` 计算实时平均手续费速率。
- [x] 1.2 返回绝对手续费、选定窗口折算手续费和归一化速率。
- [x] 1.3 新增 `/api/sm/pool_fee_heatmap`（或兼容 `endpoint=pool_fee_heatmap`）接口，校验 `window`、`sort`、分页/limit 和过滤参数。
- [x] 1.4 保持 `/api/sm/pools` 当前响应和排序不变。
- [x] 1.5 增加后端测试覆盖手续费排序、速率排序、数据质量状态和非法参数。

## 2. Frontend
- [x] 2.1 MiniApp：聪明钱池子视图新增 `活跃池子` / `收益火焰图` 二级 Tab，当前池子列表移入 `活跃池子`。
- [x] 2.2 MiniApp：实现收益火焰图列表/热力卡片，支持 `手续费` / `速率` 切换和 `30s/1m/5m/1h` 窗口切换。
- [x] 2.3 MiniApp：火焰图卡片复用现有快捷跟单入口，传入池子基础信息。
- [x] 2.4 WebApp：按 MiniApp 同口径增加二级 Tab 和收益火焰图。
- [x] 2.5 WebApp/MiniApp：样本不足、加载失败、空列表状态必须可见且不误导为 0 收益。

## 3. Verification
- [x] 3.1 执行 `cd backend; go test ./service/smart_money ./service/web_server`。
- [x] 3.2 执行 `cd miniapp; npm run build`。
- [x] 3.3 执行 `cd webapp; npm run build`。
- [x] 3.4 做针对性 diff 检查，确认旧池子列表内容未丢失、快捷跟单入口仍可用、排序口径与 API 字段一致。
