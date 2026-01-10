# 硬筛阈值动态配置功能 - 进展文档

**日期**: 2026-01-10

## 功能概述

为 auto 模式增加交易笔数 (tx) 阈值配置，并支持管理员在 miniapp 上动态配置硬筛阈值。

## 实现进展

### ✅ 已完成

1. **后端 - 系统配置模型** (`backend/base/models/system_config.go`)
   - 创建 `SystemConfig` 模型存储 6 个硬筛阈值
   - 创建 `HardFilterConfig` 结构用于传递配置

2. **后端 - 系统配置服务** (`backend/service/user/system_config.go`)
   - `GetOrCreate()` - 获取或创建系统配置（单例）
   - `Update()` - 更新配置
   - `GetHardFilterConfig()` - 获取硬筛配置，优先数据库值，回退到环境变量

3. **后端 - 管理员 API** (`backend/service/web_server/admin_system_config.go`)
   - `GET/POST /api/admin/system_config` - 获取/更新系统配置
   - 返回当前配置和环境变量默认值

4. **后端 - 硬筛逻辑修改** (`backend/service/auto_lp/auto_lp_service.go`)
   - 使用 `SystemConfigService.GetHardFilterConfig()` 获取动态配置
   - 添加 5 分钟交易笔数 (`TransactionCount`) 阈值检查
   - 更新日志输出显示 tx 过滤统计

5. **前端 - SystemConfigCard 组件** (`miniapp/src/components/SystemConfigCard.jsx`)
   - 可展开/收起的配置卡片
   - 显示 6 个硬筛阈值输入框
   - 显示环境变量默认值供参考
   - 保存按钮提交配置更新

6. **前端 - API 函数** (`miniapp/src/lib/api.js`)
   - `fetchSystemConfig()` - 获取系统配置
   - `updateSystemConfig()` - 更新系统配置

7. **前端 - 代理端点** (`miniapp/api/admin.js`)
   - 添加 `system_config` 到有效端点列表

## 硬筛阈值说明

| 参数 | 字段名 | 说明 |
|------|--------|------|
| TVL 阈值 | `autolp_min_pool_value_usd` | 池子最小 TVL (USD) |
| 费率阈值 | `autolp_min_fee_percentage` | 池子最小费率 (%) |
| 5m 费用率 | `autolp_min_fee_rate_5m` | 5分钟费用率 (5m手续费/TVL, %) |
| 5m 手续费 | `autolp_min_total_fees_5m` | 5分钟最小手续费 (USD) |
| 5m 成交量 | `autolp_min_total_volume_5m` | 5分钟最小成交量 (USD) |
| 5m 交易笔数 | `autolp_min_tx_5m` | 5分钟最小交易笔数 |

> 配置值为 0 时，使用环境变量的默认值作为回退。

## 验证结果

- ✅ 后端编译通过 (`go build ./...`)
- ✅ Miniapp 构建成功 (`npm run build`)
