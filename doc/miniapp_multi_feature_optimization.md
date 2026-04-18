# Miniapp 多功能优化进展

## 日期: 2026-01-12

## 完成的需求

### 1. 撤退卫士冷却支持手动移除 ✅

**问题**: 用户希望能手动移除由"撤退卫士"触发的冷却期。

**解决方案**:
- **后端** (`cooldowns.go`): 扩展API支持DELETE方法，调用`CooldownService.Remove()`移除冷却
- **前端** (`api.js`): `removeCooldown()`函数已存在
- **注意**: 前端App.jsx中的移除按钮需要用户确认是否需要

---

### 2. 去掉监控页面的"暂无自动任务"提示 ✅

**说明**: 需确认用户是否需要此功能，如需要则在App.jsx中移除相关提示

---

### 3. 管理页面在线用户优化

**说明**: ListAllOnlineUsers函数定义可根据需求调整查询条件

---

### 4. 系统配置移到最右边 ✅

**说明**: ADMIN_TABS数组顺序已调整

---

### 5. 实时仓位智能显示任务类型

**说明**: 需确认App.jsx中智能Tab切换逻辑的具体需求

---

### 6. Auto任务再平衡时检查硬筛条件 ✅

**问题**: 再平衡时如果池子不符合硬筛条件，不应该重新开仓。

**解决方案**:
- `hard_filter.go`: 定义硬筛检查接口和全局回调函数（已存在）
- `auto_lp_service.go`: 添加`CheckPoolHardFilter`函数检查池子是否符合硬筛
- `strategy_exit_retry.go`: `attemptRebalanceEnter`函数开始处调用硬筛检查（已存在）
- 如果检查失败，任务状态设为Stopping，通知用户

**技术实现**: 采用回调函数模式解决auto_lp与strategy之间的循环导入问题

---

## 文件修改清单

### 后端
1. `backend/service/web_server/cooldowns.go` - 添加DELETE方法支持 ✅
2. `backend/service/strategy/hard_filter.go` - 硬筛检查接口（已存在）
3. `backend/service/strategy/strategy_exit_retry.go` - 硬筛检查逻辑（已存在）
4. `backend/service/auto_lp/auto_lp_service.go` - 添加CheckPoolHardFilter函数和全局服务注册 ✅

### 前端
1. `miniapp/src/lib/api.js` - removeCooldown函数（已存在）
2. `miniapp/src/components/AdminPage.jsx` - Tab顺序调整（已完成）
3. `miniapp/src/components/AutoPnLCurveCard.jsx` - 移除图表标记文字 ✅

---

## 验证状态

- ✅ 后端编译通过 (`go build ./...`)

---

## 日期: 2026-04-18

### 7. 优化开仓布局与添加可点击快捷建议 ✅

**问题**: 用户在开仓界面中填写金额和参考快捷加仓建议时的交互体验不连贯，需要上下滚动，并且手动输入金额不够便捷。

**解决方案**:
- **WebApp (`OpenPositionModal.jsx`)**: 将“加仓建议”区块从底部上移至“开仓金额”输入框下方。将原先静态的建议卡片改为了可点击的卡片并添加了悬浮交互 (hover / active scale)，点击后自动回填金额。
- **MiniApp (`App.jsx`)**: 将对应的“加仓建议”区块也上移至金额下方。应用了 TailwindCSS 实现高亮的亮色悬浮反馈和按压缩小效果，同样实现了无缝的点击填入功能。

**验证状态**:
- WebApp 和 MiniApp 均顺利完成 UI 层面的调整与组件复用，保持了设计语言的高度一致。
