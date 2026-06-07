## 1. Proposal
- [x] 1.1 创建 MiniApp 模块边界重构提案

## 2. 第一阶段：低风险拆分
- [x] 2.1 拆出 API base 与 Telegram initData 开发模式判断
- [x] 2.2 拆出模块访问与顶部导航配置
- [x] 2.3 拆出热门池筛选、指标解析和排序辅助逻辑
- [x] 2.4 拆出开仓区间、tick、DCA 和展示格式化纯函数

## 3. 第二阶段：开仓向导拆分
- [x] 3.1 拆出开仓 draft 状态与重置逻辑
- [ ] 3.2 拆出开仓 prepare / preview / submit 流程 hook
- [ ] 3.3 拆出开仓 Sheet 与步骤内容组件

## 4. 后续阶段
- [x] 4.1 拆出 `SmartMoneyPage.jsx` 的 shared 纯工具子集
- [ ] 4.2 按业务域拆分 `lib/api.js` 并保留兼容导出

## 5. Validation
- [x] 5.1 `cd miniapp && npm run build`
- [x] 5.2 针对性检查 diff，确认 import、调用签名和用户可见行为未改变
