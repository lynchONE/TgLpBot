# 热门池子持仓信息功能 - 进展记录

**日期**: 2026-01-07

## 功能概述

在热门池子列表中集成用户仓位信息，实现：
1. 用户持有仓位的池子显示"持仓 $xxx"标签
2. 有仓位的池子自动置顶显示
3. 不在热门 Top 20 中的仓位池子也能显示

## 已完成的修改

### 后端

**hot_pools.go**
- 添加 `include_pools` 查询参数（逗号分隔的池子地址列表）
- 查询热门池子后，检查并额外查询 `include_pools` 中不在结果中的池子
- 合并额外池子数据（包括24小时统计）

### 前端

**api.js**
- `fetchHotPools` 函数添加 `includePools` 参数支持

**App.jsx**
- 新增 `positionsPoolMap`：从用户仓位构建 `pool_id -> position_usd` 映射
- 新增 `positionsPoolAddresses`：用户仓位的池子地址列表
- 调用 `fetchHotPools` 时传入仓位池子地址，确保仓位池子数据被查询
- `hotPoolsVisibleRows` 逻辑增强：
  - 为每个池子添加 `userPositionUsd` 字段
  - 有仓位的池子跳过筛选条件（始终显示）
  - 排序：有仓位的置顶，按仓位金额降序

**HotPoolCard.jsx**
- 新增 `PositionBadge` 组件：显示紫色持仓标签
- 在 `DexBadge` 旁边显示持仓信息

## 下一步

用户可自行验证功能：
1. 确保有正在运行的仓位任务
2. 打开 miniapp 热门池子页面
3. 验证有仓位的池子显示紫色"💰 持仓 $xxx"标签
4. 验证有仓位的池子排在列表最前面
