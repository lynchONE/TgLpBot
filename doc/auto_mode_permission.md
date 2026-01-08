# Auto模式权限控制功能 - 2026-01-08

## 变更概述
实现了管理员对Auto模式权限的完整控制：
1. 生成授权码时可选择是否赋予Auto权限
2. 管理员可修改用户的Auto权限和到期时间  
3. 没有Auto权限的用户无法使用自动任务功能

## 主要修改

### 模型层新增字段
| 模型 | 字段 | 说明 |
|------|------|------|
| `AuthCode` | `AutoModeEnabled` | 授权码是否赋予Auto权限 |
| `UserAccess` | `AutoModeEnabled` | 用户是否有Auto权限 |

### 管理界面更新

**授权码生成**
- 快速生成按钮改为：30天/90天/永久 × 有Auto/无Auto
- 自定义参数格式：`有效天数 人数 钱包 任务 [auto]`

**用户管理**
- 用户详情显示 Auto 模式权限状态
- 用户编辑支持修改到期时间和 Auto 权限
- 编辑参数格式：`钱包 任务 [到期天数] [auto]`

### 权限检查入口
- `/auto` 命令入口检查 Auto 权限
- 开启 AutoLP 时再次检查 Auto 权限
- 无权限用户会看到明确的提示信息

## 修改文件列表
- `base/models/auth_code.go`
- `base/models/user_access.go`
- `service/user/access.go`
- `service/bot/admin_handlers.go`
- `service/bot/auto_handlers.go`
- `service/bot/auto_config_callbacks.go`

## 验证状态
✅ 编译通过
✅ 数据库迁移（GORM自动添加新字段）

## 注意事项
- 现有用户和授权码的 `auto_mode_enabled` 默认为 `false`
- 需要管理员手动为需要的用户开通 Auto 权限
