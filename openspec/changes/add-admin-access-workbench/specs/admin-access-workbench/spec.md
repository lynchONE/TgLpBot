## ADDED Requirements
### Requirement: Web 管理端用户授权管理
系统 SHALL 允许管理员在 `webapp` 与 `miniapp` 管理页查看、搜索、创建或更新用户授权，并能关闭或恢复用户授权。

#### Scenario: 管理员查看用户授权列表
- **WHEN** 管理员打开管理页的用户授权入口
- **THEN** 系统返回分页用户授权列表，包含用户身份、授权状态、MiniApp 权限、有效期、钱包额度、活跃任务额度、备注和更新时间

#### Scenario: 管理员修改用户到期时间
- **WHEN** 管理员提交某用户新的 `active_to`
- **THEN** 系统更新该用户授权到期时间，并保持未提交字段不变

#### Scenario: 管理员关闭用户授权
- **WHEN** 管理员对某用户执行停用授权
- **THEN** 系统写入 `revoked_at` 与 `revoked_by_user_id`，后续普通接口 MUST 拒绝该用户访问

#### Scenario: 管理员恢复用户授权
- **WHEN** 管理员对已停用用户执行恢复授权
- **THEN** 系统清空停用字段，并按该用户有效期与 MiniApp 权限继续进行访问校验

### Requirement: Web 管理端授权码管理
系统 SHALL 允许管理员在 `webapp` 与 `miniapp` 管理页生成、查看、更新、启用和停用授权码。

#### Scenario: 管理员生成授权码
- **WHEN** 管理员填写有效期、最大兑换次数、钱包额度、活跃任务额度、MiniApp 权限和备注并提交
- **THEN** 系统生成唯一授权码，保存授权码配置，并返回明文授权码用于复制分发

#### Scenario: 管理员停用授权码
- **WHEN** 管理员停用某个授权码
- **THEN** 系统写入 `disabled_at`，后续用户兑换该授权码 MUST 失败

#### Scenario: 管理员调整授权码额度
- **WHEN** 管理员修改授权码的有效期、兑换次数或额度
- **THEN** 系统只更新管理员提交的字段，并在后续兑换时使用新配置

### Requirement: Web 管理端公告发布
系统 SHALL 允许管理员在 `webapp` 与 `miniapp` 管理页发布公告，并保存公告记录。

#### Scenario: 管理员发布公告
- **WHEN** 管理员提交标题和公告正文
- **THEN** 系统保存公告记录，向 Telegram 用户广播公告，并返回成功发送数与失败发送数

#### Scenario: 公告正文为空
- **WHEN** 管理员提交空白公告正文
- **THEN** 系统拒绝请求并返回参数错误，且不得创建公告记录

#### Scenario: 单个用户发送失败
- **WHEN** 公告广播过程中某个用户发送失败
- **THEN** 系统继续尝试发送给其他用户，并在响应中计入失败数量

### Requirement: 管理员 API 鉴权一致性
系统 SHALL 对所有新增管理 API 统一执行 Telegram WebApp initData 验证和管理员身份校验。

#### Scenario: 非管理员访问新增管理 API
- **WHEN** 非管理员用户调用用户授权、授权码或公告管理 API
- **THEN** 系统返回 forbidden，且不得执行任何写操作

#### Scenario: initData 无效
- **WHEN** 请求缺少 initData 或 initData 校验失败
- **THEN** 系统返回认证错误，且不得执行任何写操作
