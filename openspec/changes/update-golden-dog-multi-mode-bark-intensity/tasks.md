## 1. Proposal
- [x] 1.1 审阅并确认 `update-golden-dog-multi-mode-bark-intensity` 提案范围与默认假设

## 2. Backend
- [x] 2.1 扩展 `smart_money_golden_dog_configs` 模型与迁移，支持双模式配置、每种模式的 Bark 强度、聪明钱聚集金额阈值与金额阶梯强度配置
- [x] 2.2 扩展金狗通知配置读取/保存接口，返回双模式配置与 Bark 能力摘要
- [x] 2.3 新增金狗通知测试接口，支持按当前草稿强度发送 Bark 测试消息
- [x] 2.4 扩展 Bark helper，支持三档通知强度参数映射
- [x] 2.5 重构 GoldenDog worker，同时评估聪明钱聚集模式和池子参数模式；聪明钱聚集模式必须统计交易对窗口内 LP USD 合计金额
- [x] 2.6 为池子参数模式增加快照新鲜度判断、阈值筛选和按模式冷却去重
- [x] 2.7 为聪明钱聚集模式增加金额阈值过滤、缺失金额兜底、金额阶梯强度选择和通知正文金额展示
- [x] 2.8 补充后端测试，覆盖配置解析、金额阈值评估、阶梯强度选择、Bark 强度参数与测试接口

## 3. WebApp
- [x] 3.1 重构 `GoldenDogPanel`，改为更紧凑的双模式告警中心布局
- [x] 3.2 为两种模式增加强度选择器、摘要标签和测试按钮，并为聪明钱聚集模式增加金额阈值输入与金额阶梯强度编辑
- [x] 3.3 接入新的配置/测试 API，并处理 Bark 就绪状态、金额阈值校验与错误反馈

## 4. MiniApp
- [x] 4.1 重构 `GoldenDogPage`，改为更紧凑的双模式告警中心布局
- [x] 4.2 为两种模式增加强度选择器、摘要标签和测试按钮，并为聪明钱聚集模式增加金额阈值输入与金额阶梯强度编辑
- [x] 4.3 接入新的配置/测试 API，并处理 Bark 就绪状态、金额阈值校验与错误反馈

## 5. Verification
- [x] 5.1 执行 `cd backend && go test ./service/smart_money_golden_dog ./service/web_server`
- [ ] 5.2 执行 `webapp` 构建验证
- [ ] 5.3 执行 `miniapp` 构建验证
- [ ] 5.4 手工验证 WebApp / MiniApp 两端的保存、测试、真实触发与 UI 紧凑布局
