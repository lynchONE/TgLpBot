# Auto 模式连续跌破任务状态修复

**日期**: 2026-01-10

## 问题描述

连续下跌两次撤出仓位后，任务状态是 `Waiting`，仓位已经撤出，但任务应该是 `Stopped` 状态。

## 修复内容

修改了 `finishCooldownAfterExit` 函数（`backend/service/strategy/strategy_exit_retry.go`）：

对于 auto 模式任务（`IsAuto=true`），连续跌破后直接结束任务状态为 `Stopped`，不再进入冷却等待重新开仓。

```diff
 func (s *StrategyService) finishCooldownAfterExit(...) {
     if task == nil {
         return
     }

+    // 对于 auto 模式任务，连续跌破后直接结束任务
+    if task.IsAuto {
+        reason := strings.TrimSpace(title)
+        if reason == "" {
+            reason = "连续跌破区间，已撤出"
+        }
+        s.finishStopAfterExit(task, now, reason, txHashes)
+        return
+    }

     // 非 auto 模式任务继续原有冷却逻辑...
 }
```

## 逻辑说明

| 任务类型 | 连续跌破2次后行为 |
|---------|------------------|
| 非 auto 任务 | 进入冷却 → 冷却结束后重新开仓（原有行为） |
| **auto 任务** | **直接结束任务 `Stopped`**（新行为） |

## 验证结果

- ✅ 后端编译通过
