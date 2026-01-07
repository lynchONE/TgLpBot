# 再平衡重复执行Bug修复进展

## 修改日期
2026-01-07

## 问题描述
当一个仓位触发再平衡时，会不停地再平衡创建新的仓位，任务ID还都是一样的。根本原因是状态检查和任务提交之间存在竞态条件，导致同一个任务被重复提交到交易线程执行。

## 修复方案

### 1. 添加内存级别锁 (strategy_service.go)
在 `StrategyService` 中添加了 `inflightTasks` map 和 `inflightTasksMu` mutex，用于跟踪正在执行链上交易的任务ID。

### 2. 修改 processExitRetry (strategy_exit_retry.go)
- 提交任务前检查内存锁，如果任务已在执行中则跳过
- 先更新DB设置 `exit_next_retry_at` 为5分钟后
- 如果DB更新失败，不提交任务
- 任务完成后清理内存锁
- 如果钱包繁忙(TryRunUser返回false)，清理锁并恢复DB状态允许下次重试

### 3. 修改 processRebalanceRetry (strategy_exit_retry.go)
- 同样的双重锁机制
- 防止重新开仓操作重复提交

## 锁超时机制
- 内存锁超过10分钟自动清理（异常情况保护）
- DB锁设置5分钟超时

## 编译验证
✅ go build ./... 通过
