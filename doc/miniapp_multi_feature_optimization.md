# Miniapp 多功能优化进展

## 日期: 2026-01-12

## 完成的需求

### 1. 撤退卫士冷却支持手动移除 ✅

**问题**: 用户希望能手动移除由"撤退卫士"触发的冷却期。

**解决方案**:
- **后端** (`cooldowns.go`): 扩展API支持DELETE方法，调用`CooldownService.Remove()`移除冷却
- **前端** (`api.js`): 新增`removeCooldown()`函数
- **前端** (`App.jsx`): 添加`handleRemoveCooldown`处理函数，在冷却列表每项添加"移除"按钮

---

### 2. 去掉监控页面的"暂无自动任务"提示 ✅

**问题**: 用户不希望在监控页面显示"暂无自动任务"的提示。

**解决方案**:
- 移除 `App.jsx` 中 `showEmptyAutoTasks` 相关的JSX渲染块

---

### 3. 管理页面在线用户优化 ✅

**问题**: 
- 在线用户应定义为开启了Auto的用户
- 用户详情只显示该用户的仓位

**解决方案**:
- **后端** (`admin_realtime.go`): 修改`ListAllOnlineUsers`查询，从`auto_lp_user_configs`表出发，只返回`enabled=1`的用户

---

### 4. 系统配置移到最右边 ✅

**问题**: 用户希望将"系统配置"Tab移到管理页面最右边。

**解决方案**:
- 修改 `AdminPage.jsx` 中的 `ADMIN_TABS` 数组顺序

---

### 5. 实时仓位智能显示任务类型 ✅

**问题**: 用户希望实时仓位页面能根据当前任务类型智能选择Tab。

**解决方案**:
- 添加 `hasAutoTasks` 和 `hasManualTasks` useMemo 计算属性
- 添加 useEffect 监听仓位变化，首次加载时智能设置Tab:
  - 两种都有 → 全部
  - 只有Auto → Auto
  - 只有手动 → 手动

---

### 6. Auto任务再平衡时检查硬筛条件 ✅

**问题**: 再平衡时如果池子不符合硬筛条件，不应该重新开仓。

**解决方案**:
- 创建 `hard_filter.go` 定义硬筛检查接口和全局回调函数
- 在 `auto_lp_service.go` 添加 `CheckPoolHardFilter` 函数检查池子是否符合硬筛
- 在 `strategy_exit_retry.go` 的 `attemptRebalanceEnter` 函数开始处调用硬筛检查
- 如果检查失败，任务状态设为Stopping，通知用户

**技术难点**: 解决auto_lp与strategy之间的循环导入问题，采用回调函数模式。

---

## 文件修改清单

### 后端
1. `backend/service/web_server/cooldowns.go` - 添加DELETE方法支持
2. `backend/service/realtime/admin_realtime.go` - 修改在线用户查询逻辑
3. `backend/service/strategy/hard_filter.go` - 新文件，定义硬筛检查接口
4. `backend/service/strategy/strategy_exit_retry.go` - 添加再平衡硬筛检查
5. `backend/service/auto_lp/auto_lp_service.go` - 添加CheckPoolHardFilter函数和回调注册

### 前端
1. `miniapp/src/lib/api.js` - 添加removeCooldown函数
2. `miniapp/src/App.jsx` - 多处修改：
   - 添加removeCooldown导入
   - 添加handleRemoveCooldown函数
   - 添加冷却列表移除按钮
   - 移除空任务提示
   - 添加智能Tab切换逻辑
3. `miniapp/src/components/AdminPage.jsx` - 调整Tab顺序

---

## 验证状态

- ✅ 后端编译通过 (`go build ./...`)
- ✅ 前端编译通过 (`npm run build`)
