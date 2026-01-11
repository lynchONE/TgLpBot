# 管理员页面重构与在线用户功能

## 变更日期
2026-01-11

## 变更概述

重构 miniapp 的管理员页面，将原来集中的功能按类型分类到多个子页面中，并新增查看所有活跃任务和在线用户的功能。

## 变更详情

### 1. 后端 API 新增

#### 新增文件
- `backend/service/web_server/admin_online.go`：处理在线用户和活跃任务的 API 端点

#### 修改文件
- `backend/service/realtime/admin_realtime.go`：
  - 新增 `AdminOnlineUser` 结构体（包含 Auto 和手动任务数量）
  - 新增 `AdminActiveTask` 结构体（活跃任务信息）
  - 新增 `ListAllOnlineUsers()` 方法：获取所有有活跃任务的用户（包括 Auto 和手动）
  - 新增 `ListAllActiveTasks()` 方法：获取所有活跃任务

- `backend/service/web_server/server.go`：
  - 新增路由 `/api/admin/online_users`
  - 新增路由 `/api/admin/active_tasks`

### 2. 前端组件新增

#### 新增文件
- `miniapp/src/components/AdminPage.jsx`：管理员页面容器组件
  - 包含 4 个子页面 Tab：在线用户、活跃任务、系统配置、用户详情
  - 管理所有子页面的状态和数据加载

- `miniapp/src/components/AdminOnlineUsers.jsx`：在线用户列表组件
  - 显示所有有活跃任务的用户
  - 区分 Auto 任务数和手动任务数
  - 显示用户是否开启了 Auto 功能

- `miniapp/src/components/AdminActiveTasks.jsx`：活跃任务列表组件
  - 显示所有正在运行的任务
  - 标记任务类型（自动/手动）
  - 显示任务状态、池子信息、用户信息

#### 修改文件
- `miniapp/src/App.jsx`：
  - 引入新的 `AdminPage` 组件
  - 将原来的内联管理员页面代码替换为 `AdminPage` 组件

- `miniapp/src/lib/api.js`：
  - 新增 `fetchAdminOnlineUsers()` 函数
  - 新增 `fetchAdminActiveTasks()` 函数

- `miniapp/api/admin.js`：
  - 新增 `online_users` 端点支持
  - 新增 `active_tasks` 端点支持

### 3. 功能说明

#### 在线用户页面
- 显示所有有活跃任务的用户（不仅仅是开启 Auto 的用户）
- 每个用户显示：
  - 用户名称和 Telegram ID
  - Auto 任务数量
  - 手动任务数量
  - 是否开启了 Auto 功能（绿色标签）
  - 最后更新时间
- 点击用户可跳转到用户详情页面

#### 活跃任务页面
- 显示所有正在运行的任务列表
- 顶部显示统计摘要（总任务数、自动任务数、手动任务数）
- 每个任务显示：
  - 交易对名称和费率
  - 任务类型标签（自动/手动）
  - 任务状态标签（运行中/等待中等）
  - 暂停状态标签（如已暂停）
  - 用户信息
  - 仓位金额
  - 最后检查时间
- 点击任务可跳转到对应用户的详情页面

#### 系统配置页面
- 保留原有的 `SystemConfigCard` 组件
- 功能不变

#### 用户详情页面
- 查看选定用户的详细信息
- 显示用户余额信息
- 显示 Auto 统计（如果开启了 Auto）
- 显示用户仓位列表
- 提供"关闭 Auto"按钮

## 技术说明

### 数据结构

```go
// 在线用户（包含 Auto 和手动任务）
type AdminOnlineUser struct {
    UserID        uint      `json:"user_id"`
    TelegramID    int64     `json:"telegram_id"`
    Username      string    `json:"username"`
    FirstName     string    `json:"first_name"`
    LastName      string    `json:"last_name"`
    AutoTasks     int       `json:"auto_tasks"`
    ManualTasks   int       `json:"manual_tasks"`
    TotalTasks    int       `json:"total_tasks"`
    IsAutoEnabled bool      `json:"is_auto_enabled"`
    UpdatedAt     time.Time `json:"updated_at"`
}

// 活跃任务
type AdminActiveTask struct {
    TaskID        uint      `json:"task_id"`
    UserID        uint      `json:"user_id"`
    TelegramID    int64     `json:"telegram_id"`
    Username      string    `json:"username"`
    FirstName     string    `json:"first_name"`
    LastName      string    `json:"last_name"`
    PoolID        string    `json:"pool_id"`
    PoolVersion   string    `json:"pool_version"`
    Token0Symbol  string    `json:"token0_symbol"`
    Token1Symbol  string    `json:"token1_symbol"`
    Fee           int       `json:"fee"`
    IsAuto        bool      `json:"is_auto"`
    Status        string    `json:"status"`
    Paused        bool      `json:"paused"`
    AmountUSDT    float64   `json:"amount_usdt"`
    CreatedAt     time.Time `json:"created_at"`
    LastCheckTime time.Time `json:"last_check_time"`
}
```

### API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/admin/online_users` | POST | 获取所有有活跃任务的用户 |
| `/api/admin/active_tasks` | POST | 获取所有活跃任务 |

## 验证结果

- ✅ 后端编译成功
- ✅ 前端构建成功
