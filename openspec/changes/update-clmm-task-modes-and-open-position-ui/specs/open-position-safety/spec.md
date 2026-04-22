## RENAMED Requirements
- FROM: `### Requirement: 开仓前置兑换 MUST 支持预览与二次确认`
- TO: `### Requirement: 开仓前置兑换 MUST 支持预览与提交时确认`

## MODIFIED Requirements
### Requirement: 开仓前置兑换 MUST 支持预览与提交时确认
当本次开仓需要先把钱包资产兑换成 entry token 时，系统 MUST 在真实执行前提供无副作用的预览结果，并允许前端在同一张开仓界面内完成查看与最终提交确认，而无需额外的复选框确认。

页面在需要前置兑换时 MUST 至少展示：
- 推荐滑点
- 当前滑点
- 预计到账数量
- 兑换路径
- 本次前置兑换滑点输入

当用户基于当前预览点击最终“确认开仓”时，前端 SHALL 将其视为对当前前置兑换预览的确认，并向后端提交 `confirm_entry_swap=true`。

#### Scenario: 需要前置兑换时展示紧凑摘要
- **GIVEN** 某次开仓预览需要先执行前置兑换
- **WHEN** WebApp 或 MiniApp 渲染前置兑换区域
- **THEN** 页面 SHALL 以紧凑摘要方式展示兑换路径、预计到账和滑点信息
- **AND** 页面 SHALL NOT 再要求用户勾选单独的“确认本次前置兑换”复选框

#### Scenario: 用户点击最终确认开仓即视为确认前置兑换
- **GIVEN** 当前开仓预览需要前置兑换
- **AND** 前端已拿到与当前表单一致的前置兑换预览
- **WHEN** 用户点击最终“确认开仓”
- **THEN** 前端 SHALL 在本次提交中携带 `confirm_entry_swap=true`
- **AND** 后端 SHALL 允许继续执行真实的前置兑换与后续开仓流程

### Requirement: 需要前置兑换时未确认不得执行真实开仓
当系统判断本次开仓需要前置 entry swap 时，执行接口 MUST 在收到显式确认前拒绝真实执行；该显式确认可以来自最终开仓提交，而不要求额外的独立复选框。

#### Scenario: 缺少提交确认时拒绝真实执行
- **GIVEN** 本次开仓需要先做 entry swap
- **WHEN** 客户端直接调用真实执行接口但未携带 `confirm_entry_swap=true`
- **THEN** 系统 MUST 拒绝真实执行
- **AND** MUST NOT 发起真实兑换
- **AND** MUST NOT 继续后续开仓
- **AND** MUST 返回结构化的确认提示
